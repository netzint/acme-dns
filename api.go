package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/julienschmidt/httprouter"
	log "github.com/sirupsen/logrus"
)

// RegResponse is a struct for registration response JSON
type RegResponse struct {
	Username   string   `json:"username"`
	Password   string   `json:"password"`
	Fulldomain string   `json:"fulldomain"`
	Subdomain  string   `json:"subdomain"`
	Allowfrom  []string `json:"allowfrom"`
}

func webRegisterPost(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var regStatus int
	var reg []byte
	var err error
	
	// Parse request body for domain_name
	type RegisterRequest struct {
		DomainName string `json:"domain_name"`
		AllowFrom  []string `json:"allowfrom"`
	}
	
	var reqData RegisterRequest
	bdata, _ := io.ReadAll(r.Body)
	if len(bdata) > 0 {
		_ = json.Unmarshal(bdata, &reqData)
	}
	
	// Convert AllowFrom to cidrslice
	var allowFrom cidrslice
	if len(reqData.AllowFrom) > 0 {
		allowFrom = cidrslice(reqData.AllowFrom)
		// Validate CIDR masks
		err = allowFrom.isValid()
		if err != nil {
			regStatus = http.StatusBadRequest
			reg = jsonError("invalid_allowfrom_cidr")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(regStatus)
			_, _ = w.Write(reg)
			return
		}
	}

	// Create new user with name
	nu, err := DB.RegisterWithName(allowFrom, reqData.DomainName)
	if err != nil {
		errstr := fmt.Sprintf("%v", err)
		reg = jsonError(errstr)
		regStatus = http.StatusInternalServerError
		log.WithFields(log.Fields{"error": err.Error()}).Debug("Error in registration")
	} else {
		log.WithFields(log.Fields{"user": nu.Username.String()}).Debug("Created new user")
		regStruct := RegResponse{nu.Username.String(), nu.Password, nu.Subdomain + "." + Config.General.Domain, nu.Subdomain, nu.AllowFrom.ValidEntries()}
		regStatus = http.StatusCreated
		reg, err = json.Marshal(regStruct)
		if err != nil {
			regStatus = http.StatusInternalServerError
			reg = jsonError("json_error")
			log.WithFields(log.Fields{"error": "json"}).Debug("Could not marshal JSON")
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(regStatus)
	_, _ = w.Write(reg)
}

func webUpdatePost(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var updStatus int
	var upd []byte
	// Get user
	a, ok := r.Context().Value(ACMETxtKey).(ACMETxt)
	if !ok {
		log.WithFields(log.Fields{"error": "context"}).Error("Context error")
	}
	// NOTE: An invalid subdomain should not happen - the auth handler should
	// reject POSTs with an invalid subdomain before this handler. Reject any
	// invalid subdomains anyway as a matter of caution.
	if !validSubdomain(a.Subdomain) {
		log.WithFields(log.Fields{"error": "subdomain", "subdomain": a.Subdomain, "txt": a.Value}).Debug("Bad update data")
		updStatus = http.StatusBadRequest
		upd = jsonError("bad_subdomain")
	} else if !validTXT(a.Value) {
		log.WithFields(log.Fields{"error": "txt", "subdomain": a.Subdomain, "txt": a.Value}).Debug("Bad update data")
		updStatus = http.StatusBadRequest
		upd = jsonError("bad_txt")
	} else if validSubdomain(a.Subdomain) && validTXT(a.Value) {
		err := DB.Update(a.ACMETxtPost)
		if err != nil {
			log.WithFields(log.Fields{"error": err.Error()}).Debug("Error while trying to update record")
			updStatus = http.StatusInternalServerError
			upd = jsonError("db_error")
		} else {
			log.WithFields(log.Fields{"subdomain": a.Subdomain, "txt": a.Value}).Debug("TXT updated")
			updStatus = http.StatusOK
			upd = []byte("{\"txt\": \"" + a.Value + "\"}")
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(updStatus)
	_, _ = w.Write(upd)
}

// Endpoint used to check the readiness and/or liveness (health) of the server.
func healthCheck(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	w.WriteHeader(http.StatusOK)
}

// DomainResponse is a struct for domain list response JSON
type DomainResponse struct {
	Username   string   `json:"username"`
	Fulldomain string   `json:"fulldomain"`
	Subdomain  string   `json:"subdomain"`
	Allowfrom  []string `json:"allowfrom"`
	DomainName string   `json:"domain_name"`
	CreatedAt  int64    `json:"created_at"`
	UpdatedAt  int64    `json:"updated_at"`
}

// webGetDomains returns all registered domains from the database
func webGetDomains(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	// Simple auth check - you might want to add proper authentication here
	apiKey := r.Header.Get("X-Api-Key")
	if apiKey == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write(jsonError("unauthorized"))
		return
	}

	domains, err := DB.GetAllDomains()
	if err != nil {
		log.WithFields(log.Fields{"error": err.Error()}).Error("Error fetching domains")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(jsonError("db_error"))
		return
	}

	var response []DomainResponse
	for _, domain := range domains {
		resp := DomainResponse{
			Username:   domain.Username.String(),
			Fulldomain: domain.Fulldomain,
			Subdomain:  domain.Subdomain,
			Allowfrom:  domain.AllowFrom.ValidEntries(),
			DomainName: domain.DomainName,
			CreatedAt:  domain.CreatedAt,
			UpdatedAt:  domain.UpdatedAt,
		}
		response = append(response, resp)
	}

	respJSON, err := json.Marshal(response)
	if err != nil {
		log.WithFields(log.Fields{"error": err.Error()}).Error("Error marshaling domains")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(jsonError("json_error"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(respJSON)
}
