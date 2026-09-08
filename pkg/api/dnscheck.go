package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/miekg/dns"

	"github.com/joohoi/acme-dns/pkg/acmedns"
)

// defaultCheckResolvers are the public recursive resolvers used to validate a
// customer CNAME when the config does not name any. Querying recursive
// resolvers (rather than our own authoritative server) is deliberate: it is the
// same path a CA takes, so it catches a broken delegation as well.
var defaultCheckResolvers = []string{"1.1.1.1:53", "8.8.8.8:53", "9.9.9.9:53"}

const (
	dnsQueryTimeout     = 5 * time.Second
	defaultTXTTimeout   = 30 * time.Second
	txtPollInterval     = 3 * time.Second
	checkStepStatusOK   = "ok"
	checkStepStatusFail = "failed"
	checkStepStatusSkip = "skipped"
)

// DNSCheckRequest is the payload of a management UI verification run.
type DNSCheckRequest struct {
	// Domain is the customer domain a certificate is wanted for, eg. example.com
	Domain string `json:"domain"`
	// Subdomain is the acme-dns registration the CNAME should point at
	Subdomain string `json:"subdomain"`
	// SkipTXT limits the run to the CNAME lookup and skips writing a test record
	SkipTXT bool `json:"skip_txt"`
}

// DNSCheckStep is one stage of the verification, rendered as its own row in the UI.
type DNSCheckStep struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

// DNSCheckResponse is the result of a verification run.
type DNSCheckResponse struct {
	Valid       bool           `json:"valid"`
	HasCNAME    bool           `json:"has_cname"`
	CNAMETarget string         `json:"cname_target"`
	Expected    string         `json:"expected"`
	Challenge   string         `json:"challenge"`
	Resolvers   []string       `json:"resolvers"`
	Steps       []DNSCheckStep `json:"steps"`
	Message     string         `json:"message"`
	Error       string         `json:"error,omitempty"`
}

func (a *AcmednsAPI) checkResolvers() []string {
	if len(a.Config.DNSCheck.Resolvers) == 0 {
		return defaultCheckResolvers
	}
	resolvers := make([]string, 0, len(a.Config.DNSCheck.Resolvers))
	for _, r := range a.Config.DNSCheck.Resolvers {
		if !strings.Contains(r, ":") {
			r += ":53"
		}
		resolvers = append(resolvers, r)
	}
	return resolvers
}

func (a *AcmednsAPI) txtTimeout() time.Duration {
	if a.Config.DNSCheck.TXTTimeoutSeconds > 0 {
		return time.Duration(a.Config.DNSCheck.TXTTimeoutSeconds) * time.Second
	}
	return defaultTXTTimeout
}

// webDNSCheck verifies that a customer domain is wired up correctly: the
// _acme-challenge CNAME has to point at the registration, and a freshly written
// TXT value has to become visible through public resolvers.
func (a *AcmednsAPI) webDNSCheck(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var req DNSCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, DNSCheckResponse{
			Error:   "invalid_request",
			Message: "Die Anfrage konnte nicht gelesen werden",
		})
		return
	}

	domain := strings.TrimSpace(strings.ToLower(req.Domain))
	domain = strings.TrimPrefix(domain, "*.")
	domain = strings.TrimSuffix(domain, ".")
	if domain == "" {
		writeJSON(w, http.StatusBadRequest, DNSCheckResponse{
			Error:   "missing_domain",
			Message: "Bitte die Domain angeben, für die das Zertifikat gelten soll",
		})
		return
	}
	if !validSubdomain(req.Subdomain) {
		writeJSON(w, http.StatusBadRequest, DNSCheckResponse{
			Error:   "bad_subdomain",
			Message: "Ungültige acme-dns-Subdomain",
		})
		return
	}
	if _, err := a.DB.GetBySubdomain(req.Subdomain); err != nil {
		writeJSON(w, http.StatusNotFound, DNSCheckResponse{
			Error:   "not_found",
			Message: "Diese acme-dns-Registrierung existiert nicht",
		})
		return
	}

	// The end-to-end test polls public resolvers for up to txtTimeout, which is
	// longer than the server's global WriteTimeout. Push this one response's
	// deadline out so the connection is not closed mid-check.
	if err := http.NewResponseController(w).SetWriteDeadline(
		time.Now().Add(a.txtTimeout() + 30*time.Second)); err != nil {
		a.Logger.Debugw("Could not extend the write deadline for the DNS check",
			"error", err.Error())
	}

	result := a.runDNSCheck(domain, req.Subdomain, req.SkipTXT)
	writeJSON(w, http.StatusOK, result)
}

// runDNSCheck performs the actual verification and is kept free of HTTP details
// so it can be exercised directly from tests.
func (a *AcmednsAPI) runDNSCheck(domain, subdomain string, skipTXT bool) DNSCheckResponse {
	challenge := "_acme-challenge." + domain + "."
	expected := strings.ToLower(subdomain + "." + a.Config.General.Domain + ".")
	resolvers := a.checkResolvers()

	resp := DNSCheckResponse{
		Challenge: challenge,
		Expected:  expected,
		Resolvers: resolvers,
		Steps:     []DNSCheckStep{},
	}

	a.Logger.Debugw("Running DNS check",
		"domain", domain,
		"challenge", challenge,
		"expected", expected)

	target, err := lookupCNAME(resolvers, challenge)
	switch {
	case err != nil:
		resp.Steps = append(resp.Steps, DNSCheckStep{
			ID:     "cname",
			Status: checkStepStatusFail,
			Title:  "CNAME-Abfrage",
			Detail: "Die DNS-Abfrage für " + challenge + " ist fehlgeschlagen: " + err.Error(),
		})
		resp.Message = "Die DNS-Abfrage ist fehlgeschlagen. Bitte prüfen, ob " + domain + " funktionierende Nameserver hat."
		resp.Error = "dns_error"
		resp.Steps = append(resp.Steps, skippedTXTStep())
		return resp
	case target == "":
		resp.Steps = append(resp.Steps, DNSCheckStep{
			ID:     "cname",
			Status: checkStepStatusFail,
			Title:  "CNAME-Abfrage",
			Detail: "Unter " + challenge + " existiert kein CNAME-Eintrag",
		})
		resp.Message = "Es gibt noch keinen CNAME-Eintrag. Bitte " + challenge + " als CNAME auf " + expected + " anlegen."
		resp.Steps = append(resp.Steps, skippedTXTStep())
		return resp
	}

	resp.HasCNAME = true
	resp.CNAMETarget = target

	if !strings.EqualFold(target, expected) {
		resp.Steps = append(resp.Steps, DNSCheckStep{
			ID:     "cname",
			Status: checkStepStatusFail,
			Title:  "CNAME-Abfrage",
			Detail: challenge + " zeigt auf " + target + " statt auf " + expected,
		})
		resp.Message = "Der CNAME zeigt auf das falsche Ziel. Erwartet: " + expected + " — gefunden: " + target
		resp.Steps = append(resp.Steps, skippedTXTStep())
		return resp
	}

	resp.Steps = append(resp.Steps, DNSCheckStep{
		ID:     "cname",
		Status: checkStepStatusOK,
		Title:  "CNAME-Abfrage",
		Detail: challenge + " zeigt korrekt auf " + target,
	})

	if skipTXT {
		resp.Valid = true
		resp.Steps = append(resp.Steps, skippedTXTStep())
		resp.Message = "Der CNAME stimmt. Der TXT-Test von Ende zu Ende wurde übersprungen."
		return resp
	}

	// End-to-end test: write a throwaway value into the registration and see
	// whether it becomes visible through public resolvers on the customer name.
	token := acmedns.GeneratePassword(43)
	if err := a.DB.Update(acmedns.ACMETxtPost{Subdomain: subdomain, Value: token}); err != nil {
		a.Logger.Errorw("Error writing test TXT value",
			"error", err.Error())
		resp.Steps = append(resp.Steps, DNSCheckStep{
			ID:     "txt",
			Status: checkStepStatusFail,
			Title:  "TXT-Test von Ende zu Ende",
			Detail: "Der Testwert konnte nicht gespeichert werden: " + err.Error(),
		})
		resp.Message = "Der CNAME stimmt, aber der Testeintrag konnte nicht geschrieben werden."
		resp.Error = "db_error"
		return resp
	}

	found, waited := pollTXT(resolvers, challenge, token, a.txtTimeout())
	if !found {
		resp.Steps = append(resp.Steps, DNSCheckStep{
			ID:     "txt",
			Status: checkStepStatusFail,
			Title:  "TXT-Test von Ende zu Ende",
			Detail: fmt.Sprintf("Der Testwert war nach %s unter %s noch nicht sichtbar", waited.Round(time.Second), challenge),
		})
		resp.Message = "Der CNAME stimmt, aber der TXT-Wert war nicht auflösbar. Das liegt meist an DNS-Caching oder an einer Delegierung, die diesen Server nicht erreicht."
		return resp
	}

	resp.Valid = true
	resp.Steps = append(resp.Steps, DNSCheckStep{
		ID:     "txt",
		Status: checkStepStatusOK,
		Title:  "TXT-Test von Ende zu Ende",
		Detail: fmt.Sprintf("Ein hier geschriebener Testwert war nach %s unter %s auflösbar", waited.Round(time.Second), challenge),
	})
	resp.Message = "Alles in Ordnung. Eine Zertifizierungsstelle kann " + domain + " über diese Registrierung validieren."
	return resp
}

func skippedTXTStep() DNSCheckStep {
	return DNSCheckStep{
		ID:     "txt",
		Status: checkStepStatusSkip,
		Title:  "TXT-Test von Ende zu Ende",
		Detail: "Übersprungen, weil der CNAME noch nicht steht",
	}
}

// lookupCNAME asks the given resolvers for the CNAME at name and returns the
// first target found. It reads the CNAME record straight out of the answer
// section instead of resolving the chain, so a CNAME to a name without an
// address record is still reported correctly.
func lookupCNAME(resolvers []string, name string) (string, error) {
	var lastErr error
	for _, resolver := range resolvers {
		msg, err := exchange(resolver, name, dns.TypeCNAME)
		if err != nil {
			lastErr = err
			continue
		}
		for _, rr := range msg.Answer {
			if cname, ok := rr.(*dns.CNAME); ok {
				return strings.ToLower(cname.Target), nil
			}
		}
		// A definitive answer without a CNAME record means there is none.
		if msg.Rcode == dns.RcodeSuccess || msg.Rcode == dns.RcodeNameError {
			return "", nil
		}
		lastErr = fmt.Errorf("resolver %s answered with %s", resolver, dns.RcodeToString[msg.Rcode])
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", nil
}

// lookupTXT collects the TXT strings visible at name across all resolvers.
func lookupTXT(resolvers []string, name string) []string {
	var values []string
	for _, resolver := range resolvers {
		msg, err := exchange(resolver, name, dns.TypeTXT)
		if err != nil {
			continue
		}
		for _, rr := range msg.Answer {
			if txt, ok := rr.(*dns.TXT); ok {
				values = append(values, txt.Txt...)
			}
		}
	}
	return values
}

// pollTXT waits for want to appear at name and reports how long that took.
func pollTXT(resolvers []string, name, want string, timeout time.Duration) (bool, time.Duration) {
	start := time.Now()
	deadline := start.Add(timeout)
	for {
		for _, value := range lookupTXT(resolvers, name) {
			if value == want {
				return true, time.Since(start)
			}
		}
		if time.Now().Add(txtPollInterval).After(deadline) {
			return false, time.Since(start)
		}
		time.Sleep(txtPollInterval)
	}
}

// exchange sends a single recursive query, retrying over TCP if the UDP answer
// came back truncated.
func exchange(resolver, name string, qtype uint16) (*dns.Msg, error) {
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(name), qtype)
	msg.RecursionDesired = true
	msg.SetEdns0(4096, false)

	client := &dns.Client{Timeout: dnsQueryTimeout}
	reply, _, err := client.Exchange(msg, resolver)
	if err != nil {
		return nil, err
	}
	if reply.Truncated {
		client.Net = "tcp"
		reply, _, err = client.Exchange(msg, resolver)
		if err != nil {
			return nil, err
		}
	}
	return reply, nil
}
