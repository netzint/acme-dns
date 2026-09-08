package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/julienschmidt/httprouter"

	"github.com/joohoi/acme-dns/pkg/acmedns"
)

const (
	// ptrTimeout bounds a single reverse lookup. These run in bulk for the
	// domain list, so a single unresponsive nameserver must not hold up the page.
	ptrTimeout = 2 * time.Second
	// ptrCacheTTL keeps reverse lookups out of the request path on repeat visits.
	ptrCacheTTL = time.Hour
	// ptrConcurrency caps the parallel reverse lookups per request.
	ptrConcurrency = 8
)

type ptrEntry struct {
	host    string
	expires time.Time
}

// ptrCache memoises reverse lookups. A miss and a negative result are cached
// alike, because a repeatedly failing lookup is the expensive case.
type ptrCache struct {
	mu      sync.Mutex
	entries map[string]ptrEntry
}

func newPTRCache() *ptrCache {
	return &ptrCache{entries: make(map[string]ptrEntry)}
}

func (c *ptrCache) get(ip string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[ip]
	if !ok || time.Now().After(entry.expires) {
		return "", false
	}
	return entry.host, true
}

func (c *ptrCache) put(ip, host string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[ip] = ptrEntry{host: host, expires: time.Now().Add(ptrCacheTTL)}
}

// resolvePTRs reverse-resolves every address concurrently and returns the map of
// address to hostname. Addresses without a PTR record are absent from the result.
func (a *AcmednsAPI) resolvePTRs(ips []string) map[string]string {
	result := make(map[string]string, len(ips))
	pending := make([]string, 0, len(ips))

	seen := make(map[string]bool, len(ips))
	for _, ip := range ips {
		if ip == "" || seen[ip] {
			continue
		}
		seen[ip] = true
		if host, ok := a.ptr.get(ip); ok {
			if host != "" {
				result[ip] = host
			}
			continue
		}
		pending = append(pending, ip)
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, ptrConcurrency)

	for _, ip := range pending {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()

			ctx, cancel := context.WithTimeout(context.Background(), ptrTimeout)
			defer cancel()
			names, err := net.DefaultResolver.LookupAddr(ctx, ip)

			host := ""
			if err == nil && len(names) > 0 {
				host = strings.TrimSuffix(names[0], ".")
			}
			a.ptr.put(ip, host)
			if host != "" {
				mu.Lock()
				result[ip] = host
				mu.Unlock()
			}
		}(ip)
	}
	wg.Wait()

	return result
}

type matchRequest struct {
	// Domains is the candidate list, one hostname per entry. Newline or comma
	// separated input is split by the caller.
	Domains []string `json:"domains"`
	// Apply writes the matched domain onto the registration's label.
	Apply bool `json:"apply"`
}

// MatchResult is one candidate's outcome.
type MatchResult struct {
	Domain string `json:"domain"`
	// Status is "matched", "foreign" (a CNAME pointing somewhere else),
	// "no_cname" or "error".
	Status string `json:"status"`
	// Target is the CNAME the candidate points at, when there is one.
	Target string `json:"target,omitempty"`
	// Subdomain is set for a match: the registration the candidate belongs to.
	Subdomain string `json:"subdomain,omitempty"`
	// CurrentName is the label the registration carries today.
	CurrentName string `json:"current_name,omitempty"`
	Applied     bool   `json:"applied"`
	Error       string `json:"error,omitempty"`
}

type matchResponse struct {
	Checked int           `json:"checked"`
	Matched int           `json:"matched"`
	Applied int           `json:"applied"`
	Results []MatchResult `json:"results"`
}

// maxMatchCandidates bounds one request, since each candidate costs a DNS query.
const maxMatchCandidates = 500

// webAdminMatchDomains resolves _acme-challenge.<candidate> for every supplied
// domain and reports which registration it belongs to. The CNAME is public DNS,
// so this identifies a registration's owner without the server having stored
// anything about them.
func (a *AcmednsAPI) webAdminMatchDomains(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var req matchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	candidates := make([]string, 0, len(req.Domains))
	seen := make(map[string]bool)
	for _, raw := range req.Domains {
		for _, part := range strings.FieldsFunc(raw, func(c rune) bool {
			return c == ',' || c == ';' || c == '\n' || c == '\r' || c == ' ' || c == '\t'
		}) {
			domain := strings.TrimSuffix(strings.ToLower(strings.TrimPrefix(strings.TrimSpace(part), "*.")), ".")
			if domain == "" || seen[domain] {
				continue
			}
			seen[domain] = true
			candidates = append(candidates, domain)
		}
	}

	if len(candidates) == 0 {
		writeJSONError(w, http.StatusBadRequest, "no_domains")
		return
	}
	if len(candidates) > maxMatchCandidates {
		writeJSONError(w, http.StatusBadRequest, "too_many_domains")
		return
	}

	registrations, err := a.DB.GetAllDomains()
	if err != nil {
		a.Logger.Errorw("Error fetching domains for the match run",
			"error", err.Error())
		writeJSONError(w, http.StatusInternalServerError, "db_error")
		return
	}
	bySubdomain := make(map[string]acmedns.ACMETxt, len(registrations))
	for _, reg := range registrations {
		bySubdomain[reg.Subdomain] = reg
	}

	// Each candidate costs a DNS round trip, so a long list outlives the
	// server's WriteTimeout without this.
	if derr := http.NewResponseController(w).SetWriteDeadline(
		time.Now().Add(time.Duration(len(candidates))*dnsQueryTimeout + 30*time.Second)); derr != nil {
		a.Logger.Debugw("Could not extend the write deadline for the match run",
			"error", derr.Error())
	}

	resolvers := a.checkResolvers()
	response := matchResponse{Checked: len(candidates), Results: make([]MatchResult, 0, len(candidates))}

	for _, domain := range candidates {
		result := MatchResult{Domain: domain}
		target, lerr := lookupCNAME(resolvers, "_acme-challenge."+domain+".")
		switch {
		case lerr != nil:
			result.Status = "error"
			result.Error = lerr.Error()
		case target == "":
			result.Status = "no_cname"
		default:
			result.Target = strings.TrimSuffix(target, ".")
			subdomain := strings.SplitN(result.Target, ".", 2)[0]
			reg, known := bySubdomain[subdomain]
			if !known {
				result.Status = "foreign"
				break
			}
			result.Status = "matched"
			result.Subdomain = subdomain
			result.CurrentName = reg.DomainName
			response.Matched++

			if req.Apply && reg.DomainName != domain {
				if uerr := a.DB.UpdateDomainName(subdomain, domain); uerr != nil {
					result.Error = uerr.Error()
					a.Logger.Errorw("Could not apply a matched domain name",
						"error", uerr.Error(),
						"subdomain", subdomain)
				} else {
					result.Applied = true
					response.Applied++
				}
			}
		}
		response.Results = append(response.Results, result)
	}

	a.Logger.Infow("Domain match run finished",
		"checked", response.Checked,
		"matched", response.Matched,
		"applied", response.Applied)
	writeJSON(w, http.StatusOK, response)
}
