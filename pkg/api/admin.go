package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/julienschmidt/httprouter"

	"github.com/joohoi/acme-dns/pkg/acmedns"
)

// writeJSON serialises v as the response body with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
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
	// LastIP is where the last successful /update came from, empty until a
	// client renews once against this build.
	LastIP string `json:"last_ip"`
	// LastIPHost is the PTR of LastIP, when one exists.
	LastIPHost string `json:"last_ip_host"`
}

func toAdminDomain(a acmedns.ACMETxt) AdminDomain {
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
		LastIP:               a.LastIP,
	}
}

// webAdminServerInfo tells the UI what to render into the copy/paste snippets.
func (a *AcmednsAPI) webAdminServerInfo(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"acme_dns_domain":    a.Config.General.Domain,
		"api_base_url":       a.apiBaseURL(r),
		"credential_storage": a.Config.Auth.CredentialsKey != "",
		"registration_open":  !a.Config.API.DisableRegistration,
	})
}

// apiBaseURL reconstructs the externally visible base URL of this server so the
// generated client snippets point at the right host even behind a proxy.
func (a *AcmednsAPI) apiBaseURL(r *http.Request) string {
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
		host = a.Config.General.Domain
	}
	return scheme + "://" + host
}

// webAdminListDomains returns every registration including its credentials.
func (a *AcmednsAPI) webAdminListDomains(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	domains, err := a.DB.GetAllDomains()
	if err != nil {
		a.Logger.Errorw("Error fetching domains",
			"error", err.Error())
		writeJSONError(w, http.StatusInternalServerError, "db_error")
		return
	}
	response := make([]AdminDomain, 0, len(domains))
	addresses := make([]string, 0, len(domains))
	for _, d := range domains {
		response = append(response, toAdminDomain(d))
		addresses = append(addresses, d.LastIP)
	}

	// The reverse lookups are the point of recording the address at all, so do
	// them here rather than making the UI ask per row. They run concurrently and
	// are cached, so this costs one slow round on a cold cache.
	hosts := a.resolvePTRs(addresses)
	for i := range response {
		response[i].LastIPHost = hosts[response[i].LastIP]
	}

	writeJSON(w, http.StatusOK, response)
}

type createDomainRequest struct {
	DomainName string   `json:"domain_name"`
	AllowFrom  []string `json:"allowfrom"`
}

// webAdminCreateDomain registers a new subdomain on behalf of the UI.
func (a *AcmednsAPI) webAdminCreateDomain(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var req createDomainRequest
	// An empty body is allowed and means "register without a label".
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeJSONError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	allowFrom := acmedns.Cidrslice(req.AllowFrom)
	if len(req.AllowFrom) > 0 {
		if err := allowFrom.IsValid(); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_allowfrom_cidr")
			return
		}
	}

	created, err := a.DB.RegisterWithName(allowFrom, strings.TrimSpace(req.DomainName))
	if err != nil {
		a.Logger.Errorw("Error registering domain",
			"error", err.Error())
		writeJSONError(w, http.StatusInternalServerError, "db_error")
		return
	}
	a.Logger.Infow("Created new registration from management UI",
		"user", created.Username.String(),
		"name", created.DomainName)
	writeJSON(w, http.StatusCreated, toAdminDomain(created))
}

type renameRequest struct {
	DomainName string `json:"domain_name"`
}

// webAdminRenameDomain updates the human readable label of a registration.
func (a *AcmednsAPI) webAdminRenameDomain(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
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
	if err := a.DB.UpdateDomainName(subdomain, strings.TrimSpace(req.DomainName)); err != nil {
		if errors.Is(err, acmedns.ErrNoSuchDomain) {
			writeJSONError(w, http.StatusNotFound, "not_found")
			return
		}
		a.Logger.Errorw("Error updating domain name",
			"error", err.Error(),
			"subdomain", subdomain)
		writeJSONError(w, http.StatusInternalServerError, "db_error")
		return
	}
	updated, err := a.DB.GetBySubdomain(subdomain)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "db_error")
		return
	}
	writeJSON(w, http.StatusOK, toAdminDomain(updated))
}

// webAdminRotateCredentials issues a fresh password for an existing subdomain.
// The CNAME the customer created stays valid, only the client config changes.
func (a *AcmednsAPI) webAdminRotateCredentials(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	subdomain := p.ByName("subdomain")
	if !validSubdomain(subdomain) {
		writeJSONError(w, http.StatusBadRequest, "bad_subdomain")
		return
	}
	password, err := a.DB.RotatePassword(subdomain)
	if err != nil {
		if errors.Is(err, acmedns.ErrNoSuchDomain) {
			writeJSONError(w, http.StatusNotFound, "not_found")
			return
		}
		a.Logger.Errorw("Error rotating credentials",
			"error", err.Error(),
			"subdomain", subdomain)
		writeJSONError(w, http.StatusInternalServerError, "db_error")
		return
	}
	updated, err := a.DB.GetBySubdomain(subdomain)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "db_error")
		return
	}
	a.Logger.Infow("Rotated credentials from management UI",
		"subdomain", subdomain)
	// GetBySubdomain returns the stored copy, which is empty when credential
	// storage is off. Hand back the freshly generated password either way.
	result := toAdminDomain(updated)
	result.Password = password
	result.CredentialsAvailable = true
	writeJSON(w, http.StatusOK, result)
}

// webAdminDeleteDomain permanently removes a registration.
func (a *AcmednsAPI) webAdminDeleteDomain(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	subdomain := p.ByName("subdomain")
	if !validSubdomain(subdomain) {
		writeJSONError(w, http.StatusBadRequest, "bad_subdomain")
		return
	}
	if err := a.DB.DeleteDomain(subdomain); err != nil {
		if errors.Is(err, acmedns.ErrNoSuchDomain) {
			writeJSONError(w, http.StatusNotFound, "not_found")
			return
		}
		a.Logger.Errorw("Error deleting domain",
			"error", err.Error(),
			"subdomain", subdomain)
		writeJSONError(w, http.StatusInternalServerError, "db_error")
		return
	}
	a.Logger.Infow("Deleted registration from management UI",
		"subdomain", subdomain)
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}
