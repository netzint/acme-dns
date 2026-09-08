package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
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
	// defaultSessionTTL is used when session_ttl_hours is not set in the config
	defaultSessionTTL = 12 * time.Hour
	// maxLoginFailures is the number of failed logins per source IP before that
	// IP is locked out for loginLockout
	maxLoginFailures = 5
	loginLockout     = 15 * time.Minute
)

// sessionStore holds the tokens handed out by the management login. Sessions
// live in memory only, so restarting acme-dns logs every admin out again.
type sessionStore struct {
	mu       sync.Mutex
	tokens   map[string]time.Time
	failures map[string]*loginFailure
	ttl      time.Duration
}

type loginFailure struct {
	count int
	until time.Time
}

func newSessionStore(ttl time.Duration) *sessionStore {
	if ttl <= 0 {
		ttl = defaultSessionTTL
	}
	return &sessionStore{
		tokens:   make(map[string]time.Time),
		failures: make(map[string]*loginFailure),
		ttl:      ttl,
	}
}

// Issue creates a new session token and returns it together with its expiry.
func (s *sessionStore) Issue() (string, time.Time, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expires := time.Now().Add(s.ttl)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()
	s.tokens[token] = expires
	return token, expires, nil
}

// Valid reports whether the token exists and has not expired yet.
func (s *sessionStore) Valid(token string) bool {
	if token == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	expires, ok := s.tokens[token]
	if !ok {
		return false
	}
	if time.Now().After(expires) {
		delete(s.tokens, token)
		return false
	}
	return true
}

// Revoke drops a single token, used by the logout endpoint.
func (s *sessionStore) Revoke(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, token)
}

func (s *sessionStore) pruneLocked() {
	now := time.Now()
	for token, expires := range s.tokens {
		if now.After(expires) {
			delete(s.tokens, token)
		}
	}
	for ip, f := range s.failures {
		if now.After(f.until) && f.count >= maxLoginFailures {
			delete(s.failures, ip)
		}
	}
}

// LockedOut reports whether an IP has to wait before trying to log in again.
func (s *sessionStore) LockedOut(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.failures[ip]
	if !ok {
		return false
	}
	return f.count >= maxLoginFailures && time.Now().Before(f.until)
}

func (s *sessionStore) RecordFailure(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.failures[ip]
	if !ok {
		f = &loginFailure{}
		s.failures[ip] = f
	}
	f.count++
	f.until = time.Now().Add(loginLockout)
}

func (s *sessionStore) ClearFailures(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.failures, ip)
}

// adminEnabled reports whether the management API is configured. Without an
// admin user and password hash the management endpoints are not registered at
// all, which keeps the plain acme-dns deployment unchanged.
func (a *AcmednsAPI) adminEnabled() bool {
	return a.Config.Auth.AdminUser != "" && a.Config.Auth.AdminPasswordHash != ""
}

// AdminAuth wraps a handler so it only runs for requests carrying a valid
// session token in the Authorization header.
func (a *AcmednsAPI) AdminAuth(handler httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		if !a.sessions.Valid(tokenFromRequest(r)) {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		handler(w, r, p)
	}
}

func tokenFromRequest(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	return r.Header.Get("X-Admin-Token")
}

func (a *AcmednsAPI) requestIP(r *http.Request) string {
	if a.Config.API.UseHeader {
		ips := getIPListFromHeader(r.Header.Get(a.Config.API.HeaderName))
		if len(ips) > 0 {
			return ips[0]
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
	Username  string `json:"username"`
}

// webAdminLogin exchanges admin credentials for a session token.
func (a *AcmednsAPI) webAdminLogin(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	ip := a.requestIP(r)
	if a.sessions.LockedOut(ip) {
		a.Logger.Warnw("Admin login locked out",
			"ip", ip)
		writeJSONError(w, http.StatusTooManyRequests, "too_many_attempts")
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	userOK := subtle.ConstantTimeCompare([]byte(req.Username), []byte(a.Config.Auth.AdminUser)) == 1
	passOK := acmedns.CorrectPassword(req.Password, a.Config.Auth.AdminPasswordHash)
	if !userOK || !passOK {
		a.sessions.RecordFailure(ip)
		a.Logger.Warnw("Failed admin login",
			"ip", ip,
			"user", req.Username)
		writeJSONError(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}

	token, expires, err := a.sessions.Issue()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "token_error")
		return
	}
	a.sessions.ClearFailures(ip)
	a.Logger.Infow("Admin logged in",
		"ip", ip,
		"user", req.Username)
	writeJSON(w, http.StatusOK, loginResponse{
		Token:     token,
		ExpiresAt: expires.Unix(),
		Username:  req.Username,
	})
}

// webAdminLogout invalidates the caller's session token.
func (a *AcmednsAPI) webAdminLogout(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	a.sessions.Revoke(tokenFromRequest(r))
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// webAdminSession lets the UI check whether a stored token is still good.
func (a *AcmednsAPI) webAdminSession(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"valid":    true,
		"username": a.Config.Auth.AdminUser,
	})
}
