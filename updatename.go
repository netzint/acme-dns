package main

import (
	"encoding/json"
	"net/http"

	"github.com/julienschmidt/httprouter"
	log "github.com/sirupsen/logrus"
)

// UpdateNameRequest represents the request to update a domain name
type UpdateNameRequest struct {
	FullDomain string `json:"fulldomain"`
	DomainName string `json:"domain_name"`
}

// webUpdateName handles domain name update requests
func webUpdateName(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var req UpdateNameRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(jsonError("invalid_request"))
		return
	}

	// Validate input
	if req.FullDomain == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(jsonError("missing_fulldomain"))
		return
	}

	// Extract subdomain from fulldomain (remove the base domain)
	subdomain := ""
	if len(req.FullDomain) > len(Config.General.Domain)+1 {
		subdomain = req.FullDomain[:len(req.FullDomain)-len(Config.General.Domain)-1]
	}

	if subdomain == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(jsonError("invalid_fulldomain"))
		return
	}

	// Update the domain name in database
	err = DB.UpdateDomainName(subdomain, req.DomainName)
	if err != nil {
		log.WithFields(log.Fields{
			"error":     err.Error(),
			"subdomain": subdomain,
		}).Error("Error updating domain name")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(jsonError("db_error"))
		return
	}

	log.WithFields(log.Fields{
		"subdomain":   subdomain,
		"domain_name": req.DomainName,
	}).Debug("Domain name updated")

	// Return success
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"fulldomain":  req.FullDomain,
		"domain_name": req.DomainName,
	})
}