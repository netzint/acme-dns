package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/julienschmidt/httprouter"
	log "github.com/sirupsen/logrus"
)

// writeJSON serialises v as the response body with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.WithFields(log.Fields{"error": err.Error()}).Error("Error writing JSON response")
	}
}

// writeJSONError writes the {"error": "..."} shape used across the API.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(jsonError(message))
}

// AdminDomain is the representation of a registration handed to the management UI.
type AdminDomain struct {
	Username string `json:"username"`
	// Password is the recoverable plaintext. It is empty for records created
	// before credential storage was enabled; CredentialsAvailable says which.
	Password             string   `json:"password"`
	CredentialsAvailable bool     `json:"credentials_available"`
	Fulldomain           string   `json:"fulldomain"`
	Subdomain            string   `json:"subdomain"`
	Allowfrom            []string `json:"allowfrom"`
	DomainName           string   `json:"domain_name"`
	CreatedAt            int64    `json:"created_at"`
	UpdatedAt            int64    `json:"updated_at"`
	LastActive           int64    `json:"last_active"`
}

func toAdminDomain(a ACMETxt) AdminDomain {
	return AdminDomain{
		Username:             a.Username.String(),
		Password:             a.Password,
		CredentialsAvailable: a.Password != "",
		Fulldomain:           a.Fulldomain,
		Subdomain:            a.Subdomain,
		Allowfrom:            a.AllowFrom.ValidEntries(),
		DomainName:           a.DomainName,
		CreatedAt:            a.CreatedAt,
		UpdatedAt:            a.UpdatedAt,
		LastActive:           a.LastActive,
	}
}

// webAdminServerInfo tells the UI what to render into the copy/paste snippets.
func webAdminServerInfo(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"acme_dns_domain":    Config.General.Domain,
		"api_base_url":       apiBaseURL(r),
		"credential_storage": CredentialCipher != nil,
		"registration_open":  !Config.API.DisableRegistration,
	})
}

// apiBaseURL reconstructs the externally visible base URL of this server so the
// generated client snippets point at the right host even behind a proxy.
func apiBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = fwd
	}
	if host == "" {
		host = Config.General.Domain
	}
	return scheme + "://" + host
}

// webAdminListDomains returns every registration including its credentials.
func webAdminListDomains(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	domains, err := DB.GetAllDomains()
	if err != nil {
		log.WithFields(log.Fields{"error": err.Error()}).Error("Error fetching domains")
		writeJSONError(w, http.StatusInternalServerError, "db_error")
		return
	}
	response := make([]AdminDomain, 0, len(domains))
	for _, d := range domains {
		response = append(response, toAdminDomain(d))
	}
	writeJSON(w, http.StatusOK, response)
}

type createDomainRequest struct {
	DomainName string   `json:"domain_name"`
	AllowFrom  []string `json:"allowfrom"`
}

// webAdminCreateDomain registers a new subdomain on behalf of the UI.
func webAdminCreateDomain(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var req createDomainRequest
	// An empty body is allowed and means "register without a label".
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeJSONError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	allowFrom := cidrslice(req.AllowFrom)
	if len(req.AllowFrom) > 0 {
		if err := allowFrom.isValid(); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_allowfrom_cidr")
			return
		}
	}

	created, err := DB.RegisterWithName(allowFrom, strings.TrimSpace(req.DomainName))
	if err != nil {
		log.WithFields(log.Fields{"error": err.Error()}).Error("Error registering domain")
		writeJSONError(w, http.StatusInternalServerError, "db_error")
		return
	}
	created.Fulldomain = created.Subdomain + "." + Config.General.Domain
	log.WithFields(log.Fields{"user": created.Username.String(), "name": created.DomainName}).Info("Created new registration from management UI")
	writeJSON(w, http.StatusCreated, toAdminDomain(created))
}

type renameRequest struct {
	DomainName string `json:"domain_name"`
}

// webAdminRenameDomain updates the human readable label of a registration.
func webAdminRenameDomain(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	subdomain := p.ByName("subdomain")
	if !validSubdomain(subdomain) {
		writeJSONError(w, http.StatusBadRequest, "bad_subdomain")
		return
	}
	var req renameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if _, err := DB.GetBySubdomain(subdomain); err != nil {
		if err == errNoSuchDomain {
			writeJSONError(w, http.StatusNotFound, "not_found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "db_error")
		return
	}
	if err := DB.UpdateDomainName(subdomain, strings.TrimSpace(req.DomainName)); err != nil {
		log.WithFields(log.Fields{"error": err.Error(), "subdomain": subdomain}).Error("Error updating domain name")
		writeJSONError(w, http.StatusInternalServerError, "db_error")
		return
	}
	updated, err := DB.GetBySubdomain(subdomain)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "db_error")
		return
	}
	writeJSON(w, http.StatusOK, toAdminDomain(updated))
}

// webAdminRotateCredentials issues a fresh password for an existing subdomain.
// The CNAME the customer created stays valid, only the client config changes.
func webAdminRotateCredentials(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	subdomain := p.ByName("subdomain")
	if !validSubdomain(subdomain) {
		writeJSONError(w, http.StatusBadRequest, "bad_subdomain")
		return
	}
	password, err := DB.RotatePassword(subdomain)
	if err != nil {
		if err == errNoSuchDomain {
			writeJSONError(w, http.StatusNotFound, "not_found")
			return
		}
		log.WithFields(log.Fields{"error": err.Error(), "subdomain": subdomain}).Error("Error rotating credentials")
		writeJSONError(w, http.StatusInternalServerError, "db_error")
		return
	}
	updated, err := DB.GetBySubdomain(subdomain)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "db_error")
		return
	}
	log.WithFields(log.Fields{"subdomain": subdomain}).Info("Rotated credentials from management UI")
	// GetBySubdomain returns the stored copy, which is empty when credential
	// storage is off. Hand back the freshly generated password either way.
	result := toAdminDomain(updated)
	result.Password = password
	result.CredentialsAvailable = true
	writeJSON(w, http.StatusOK, result)
}

// webAdminDeleteDomain permanently removes a registration.
func webAdminDeleteDomain(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	subdomain := p.ByName("subdomain")
	if !validSubdomain(subdomain) {
		writeJSONError(w, http.StatusBadRequest, "bad_subdomain")
		return
	}
	if err := DB.DeleteDomain(subdomain); err != nil {
		if err == errNoSuchDomain {
			writeJSONError(w, http.StatusNotFound, "not_found")
			return
		}
		log.WithFields(log.Fields{"error": err.Error(), "subdomain": subdomain}).Error("Error deleting domain")
		writeJSONError(w, http.StatusInternalServerError, "db_error")
		return
	}
	log.WithFields(log.Fields{"subdomain": subdomain}).Info("Deleted registration from management UI")
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}
