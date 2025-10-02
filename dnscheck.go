package main

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"github.com/julienschmidt/httprouter"
	log "github.com/sirupsen/logrus"
)

// DNSCheckRequest represents the request for DNS validation
type DNSCheckRequest struct {
	Domain     string `json:"domain"`
	Subdomain  string `json:"subdomain"`
	FullDomain string `json:"fulldomain"`
}

// DNSCheckResponse represents the response of DNS validation
type DNSCheckResponse struct {
	Valid       bool     `json:"valid"`
	HasCNAME    bool     `json:"has_cname"`
	CNAMETarget string   `json:"cname_target"`
	Expected    string   `json:"expected"`
	Error       string   `json:"error,omitempty"`
	Message     string   `json:"message"`
	Records     []string `json:"records,omitempty"`
}

// webDNSCheck handles DNS validation requests
func webDNSCheck(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var req DNSCheckRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(DNSCheckResponse{
			Valid:   false,
			Error:   "Invalid request format",
			Message: "Please provide a valid domain and subdomain",
		})
		return
	}

	// Validate input
	if req.Domain == "" || req.FullDomain == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(DNSCheckResponse{
			Valid:   false,
			Error:   "Missing required fields",
			Message: "Domain and full ACME-DNS domain are required",
		})
		return
	}

	// Build the _acme-challenge subdomain
	challengeDomain := "_acme-challenge." + req.Domain
	
	log.WithFields(log.Fields{
		"domain":          req.Domain,
		"challenge":       challengeDomain,
		"expected_target": req.FullDomain,
	}).Debug("Checking DNS CNAME record")

	// Perform DNS lookup
	response := checkCNAME(challengeDomain, req.FullDomain)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// checkCNAME performs the actual DNS lookup and validation
func checkCNAME(challengeDomain, expectedTarget string) DNSCheckResponse {
	// Normalize the expected target (ensure it ends with a dot)
	if !strings.HasSuffix(expectedTarget, ".") {
		expectedTarget = expectedTarget + "."
	}

	// Look up CNAME records
	cname, err := net.LookupCNAME(challengeDomain)
	if err != nil {
		// Check if it's a NXDOMAIN or other DNS error
		dnsErr, ok := err.(*net.DNSError)
		if ok && dnsErr.IsNotFound {
			return DNSCheckResponse{
				Valid:    false,
				HasCNAME: false,
				Expected: expectedTarget,
				Message:  "No CNAME record found for " + challengeDomain,
				Error:    "NXDOMAIN",
			}
		}
		
		return DNSCheckResponse{
			Valid:    false,
			HasCNAME: false,
			Expected: expectedTarget,
			Message:  "DNS lookup failed: " + err.Error(),
			Error:    "DNS_ERROR",
		}
	}

	// Normalize the CNAME result (ensure it ends with a dot)
	if !strings.HasSuffix(cname, ".") {
		cname = cname + "."
	}

	// Check if CNAME matches expected target
	isValid := strings.EqualFold(cname, expectedTarget)

	var message string
	if isValid {
		message = "DNS configuration is correct! CNAME points to " + cname
	} else if cname == challengeDomain+"." {
		// No CNAME record exists (returned the same domain)
		return DNSCheckResponse{
			Valid:    false,
			HasCNAME: false,
			Expected: expectedTarget,
			Message:  "No CNAME record found. Please create a CNAME record pointing to " + expectedTarget,
		}
	} else {
		message = "CNAME points to wrong target. Found: " + cname + ", Expected: " + expectedTarget
	}

	// Also try to get all records for more info
	records, _ := net.LookupHost(challengeDomain)

	return DNSCheckResponse{
		Valid:       isValid,
		HasCNAME:    cname != challengeDomain+".",
		CNAMETarget: cname,
		Expected:    expectedTarget,
		Message:     message,
		Records:     records,
	}
}