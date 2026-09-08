package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/julienschmidt/httprouter"
	"golang.org/x/crypto/bcrypt"
)

// setupAdminRouter wires the management endpoints against the shared test
// database and returns a router plus a teardown that restores the global Config.
func setupAdminRouter(t *testing.T) (http.Handler, func()) {
	t.Helper()
	previousConfig := Config
	previousCipher := CredentialCipher
	previousSessions := AdminSessions

	Config.General.Domain = "auth.example.org"
	Config.Database.Engine = "sqlite3"
	Config.Auth.AdminUser = "admin"
	// bcrypt hash of "correct horse battery staple"
	hash, err := hashForTest("correct horse battery staple")
	if err != nil {
		t.Fatalf("could not hash test password: %v", err)
	}
	Config.Auth.AdminPasswordHash = hash
	Config.Auth.SessionTTLHours = 1

	cipher, err := newCredentialCipher("unit-test-key")
	if err != nil {
		t.Fatalf("could not build cipher: %v", err)
	}
	CredentialCipher = cipher
	AdminSessions = newSessionStore()

	api := httprouter.New()
	api.POST("/api/admin/login", webAdminLogin)
	api.GET("/api/admin/server", AdminAuth(webAdminServerInfo))
	api.GET("/api/admin/domains", AdminAuth(webAdminListDomains))
	api.POST("/api/admin/domains", AdminAuth(webAdminCreateDomain))
	api.POST("/api/admin/domains/:subdomain/name", AdminAuth(webAdminRenameDomain))
	api.POST("/api/admin/domains/:subdomain/rotate", AdminAuth(webAdminRotateCredentials))
	api.DELETE("/api/admin/domains/:subdomain", AdminAuth(webAdminDeleteDomain))

	return api, func() {
		Config = previousConfig
		CredentialCipher = previousCipher
		AdminSessions = previousSessions
	}
}

func loginForTest(t *testing.T, router http.Handler) string {
	t.Helper()
	body := strings.NewReader(`{"username": "admin", "password": "correct horse battery staple"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/login", body)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login failed with status %d: %s", rec.Code, rec.Body.String())
	}
	var response loginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("could not decode login response: %v", err)
	}
	return response.Token
}

func doAuthed(t *testing.T, router http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestCredentialCipherRoundTrip(t *testing.T) {
	cipher, err := newCredentialCipher("a passphrase")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	secret := generatePassword(40)
	encrypted, err := cipher.Encrypt(secret)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	if encrypted == secret || encrypted == "" {
		t.Fatal("ciphertext must differ from plaintext and not be empty")
	}

	decrypted, err := cipher.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}
	if decrypted != secret {
		t.Fatalf("round trip mismatch: got %q, want %q", decrypted, secret)
	}

	// A different key must not be able to read it.
	other, _ := newCredentialCipher("another passphrase")
	if _, err := other.Decrypt(encrypted); err == nil {
		t.Fatal("decrypting with the wrong key should fail")
	}
}

func TestCredentialCipherEmptyValues(t *testing.T) {
	if _, err := newCredentialCipher(""); err == nil {
		t.Fatal("an empty key should be rejected")
	}

	cipher, _ := newCredentialCipher("key")
	got, err := cipher.Decrypt("")
	if err != nil || got != "" {
		t.Fatalf("empty ciphertext should decode to empty string without error, got %q / %v", got, err)
	}

	// The nil-safe helpers are what the database layer calls.
	previous := CredentialCipher
	CredentialCipher = nil
	defer func() { CredentialCipher = previous }()
	if encryptCredential("secret") != "" {
		t.Fatal("encryptCredential should return empty when no cipher is configured")
	}
	if decryptCredential("anything") != "" {
		t.Fatal("decryptCredential should return empty when no cipher is configured")
	}
}

func TestSessionStoreLifecycle(t *testing.T) {
	previousConfig := Config
	defer func() { Config = previousConfig }()
	Config.Auth.SessionTTLHours = 1

	store := newSessionStore()
	token, expires, err := store.Issue()
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}
	if !store.Valid(token) {
		t.Fatal("a freshly issued token should be valid")
	}
	if expires.Before(time.Now()) {
		t.Fatal("expiry should be in the future")
	}
	if store.Valid("") || store.Valid("not-a-token") {
		t.Fatal("unknown tokens must not validate")
	}

	store.Revoke(token)
	if store.Valid(token) {
		t.Fatal("a revoked token must not validate")
	}
}

func TestSessionStoreExpiry(t *testing.T) {
	store := newSessionStore()
	token, _, err := store.Issue()
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}
	// Backdate the token to simulate an expired session.
	store.mu.Lock()
	store.tokens[token] = time.Now().Add(-time.Minute)
	store.mu.Unlock()

	if store.Valid(token) {
		t.Fatal("an expired token must not validate")
	}
}

func TestSessionStoreLockout(t *testing.T) {
	store := newSessionStore()
	if store.LockedOut("10.0.0.1") {
		t.Fatal("an unseen IP must not be locked out")
	}
	for i := 0; i < maxLoginFailures; i++ {
		store.RecordFailure("10.0.0.1")
	}
	if !store.LockedOut("10.0.0.1") {
		t.Fatal("the IP should be locked out after the failure limit")
	}
	if store.LockedOut("10.0.0.2") {
		t.Fatal("the lockout must not leak to other IPs")
	}
	store.ClearFailures("10.0.0.1")
	if store.LockedOut("10.0.0.1") {
		t.Fatal("clearing failures should lift the lockout")
	}
}

func TestAdminEndpointsRejectMissingToken(t *testing.T) {
	router, teardown := setupAdminRouter(t)
	defer teardown()

	for _, path := range []string{"/api/admin/domains", "/api/admin/server"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s without a token returned %d, want 401", path, rec.Code)
		}
	}

	// A syntactically valid but unknown token must be rejected too.
	rec := doAuthed(t, router, http.MethodGet, "/api/admin/domains", "made-up-token", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unknown token returned %d, want 401", rec.Code)
	}
}

func TestAdminLoginRejectsWrongPassword(t *testing.T) {
	router, teardown := setupAdminRouter(t)
	defer teardown()

	req := httptest.NewRequest(http.MethodPost, "/api/admin/login",
		strings.NewReader(`{"username": "admin", "password": "wrong"}`))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password returned %d, want 401", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "token") {
		t.Fatalf("a failed login must not hand out a token: %s", rec.Body.String())
	}
}

func TestAdminDomainLifecycle(t *testing.T) {
	router, teardown := setupAdminRouter(t)
	defer teardown()
	token := loginForTest(t, router)

	// Create
	rec := doAuthed(t, router, http.MethodPost, "/api/admin/domains", token,
		`{"domain_name": "lifecycle.example.com"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create returned %d: %s", rec.Code, rec.Body.String())
	}
	var created AdminDomain
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("could not decode created domain: %v", err)
	}
	if created.Password == "" || !created.CredentialsAvailable {
		t.Fatal("a newly created registration must expose its password")
	}
	if created.Fulldomain != created.Subdomain+".auth.example.org" {
		t.Fatalf("unexpected fulldomain %q", created.Fulldomain)
	}

	// List returns the decrypted password again
	rec = doAuthed(t, router, http.MethodGet, "/api/admin/domains", token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list returned %d", rec.Code)
	}
	var listed []AdminDomain
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("could not decode list: %v", err)
	}
	found := false
	for _, d := range listed {
		if d.Subdomain != created.Subdomain {
			continue
		}
		found = true
		if d.Password != created.Password {
			t.Fatalf("stored password did not round trip: got %q, want %q", d.Password, created.Password)
		}
		if strings.HasPrefix(d.Password, "$2a$") {
			t.Fatal("the bcrypt hash must never be handed to the UI")
		}
	}
	if !found {
		t.Fatal("the created registration is missing from the list")
	}

	// Rename
	rec = doAuthed(t, router, http.MethodPost, "/api/admin/domains/"+created.Subdomain+"/name",
		token, `{"domain_name": "renamed.example.com"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("rename returned %d: %s", rec.Code, rec.Body.String())
	}
	var renamed AdminDomain
	_ = json.Unmarshal(rec.Body.Bytes(), &renamed)
	if renamed.DomainName != "renamed.example.com" {
		t.Fatalf("rename did not stick, got %q", renamed.DomainName)
	}

	// Rotate hands out a new password and invalidates the old one
	rec = doAuthed(t, router, http.MethodPost, "/api/admin/domains/"+created.Subdomain+"/rotate", token, "{}")
	if rec.Code != http.StatusOK {
		t.Fatalf("rotate returned %d: %s", rec.Code, rec.Body.String())
	}
	var rotated AdminDomain
	_ = json.Unmarshal(rec.Body.Bytes(), &rotated)
	if rotated.Password == "" || rotated.Password == created.Password {
		t.Fatal("rotate must return a new password")
	}
	if rotated.Subdomain != created.Subdomain {
		t.Fatal("rotate must keep the subdomain so the CNAME stays valid")
	}
	stored, err := DB.GetByUsername(mustParseUUID(t, created.Username))
	if err != nil {
		t.Fatalf("could not read back the record: %v", err)
	}
	if correctPassword(created.Password, stored.Password) {
		t.Fatal("the old password must stop working after a rotation")
	}
	if !correctPassword(rotated.Password, stored.Password) {
		t.Fatal("the rotated password must authenticate")
	}

	// Delete
	rec = doAuthed(t, router, http.MethodDelete, "/api/admin/domains/"+created.Subdomain, token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete returned %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := DB.GetBySubdomain(created.Subdomain); err != errNoSuchDomain {
		t.Fatalf("the record should be gone, got err %v", err)
	}

	// Deleting twice is a 404 rather than a silent success
	rec = doAuthed(t, router, http.MethodDelete, "/api/admin/domains/"+created.Subdomain, token, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("second delete returned %d, want 404", rec.Code)
	}
}

func TestAdminRejectsInvalidSubdomain(t *testing.T) {
	router, teardown := setupAdminRouter(t)
	defer teardown()
	token := loginForTest(t, router)

	rec := doAuthed(t, router, http.MethodDelete, "/api/admin/domains/not..valid", token, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid subdomain returned %d, want 400", rec.Code)
	}
}

func TestSPAFileServer(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<html>spa</html>"), 0o600); err != nil {
		t.Fatalf("could not write index.html: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.js"), []byte("console.log(1)"), 0o600); err != nil {
		t.Fatalf("could not write main.js: %v", err)
	}
	secret := filepath.Join(filepath.Dir(root), "outside.txt")
	if err := os.WriteFile(secret, []byte("do not serve me"), 0o600); err != nil {
		t.Fatalf("could not write the outside file: %v", err)
	}
	defer os.Remove(secret)

	handler := spaFileServer(root)

	cases := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{"existing asset", "/main.js", http.StatusOK, "console.log(1)"},
		{"index", "/", http.StatusOK, "<html>spa</html>"},
		{"deep route falls back to the SPA", "/domains/detail", http.StatusOK, "<html>spa</html>"},
		{"missing asset is a real 404", "/missing.js", http.StatusNotFound, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rec.Code != tc.wantStatus {
				t.Fatalf("%s returned %d, want %d", tc.path, rec.Code, tc.wantStatus)
			}
			if tc.wantBody != "" && !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Fatalf("%s returned %q, want it to contain %q", tc.path, rec.Body.String(), tc.wantBody)
			}
		})
	}

	t.Run("path traversal is refused", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.URL.Path = "/../outside.txt"
		handler.ServeHTTP(rec, req)
		if strings.Contains(rec.Body.String(), "do not serve me") {
			t.Fatal("the file server must not escape its root")
		}
	})

	t.Run("the API namespace answers with JSON instead of the SPA", func(t *testing.T) {
		previous := Config
		defer func() { Config = previous }()

		Config.Auth.AdminUser = ""
		Config.Auth.AdminPasswordHash = ""
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/admin/login", nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("/api/admin/login returned %d, want 404", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "management_api_disabled") {
			t.Fatalf("expected a JSON error explaining the disabled API, got %q", rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "<html>") {
			t.Fatal("API paths must never fall back to index.html")
		}
	})

	t.Run("non GET methods are refused", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/main.js", nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("POST returned %d, want 404", rec.Code)
		}
	})
}

func TestCheckResolversDefaultsAndPortSuffix(t *testing.T) {
	previous := Config
	defer func() { Config = previous }()

	Config.DNSCheck.Resolvers = nil
	if got := checkResolvers(); len(got) != len(defaultCheckResolvers) {
		t.Fatalf("expected the built in defaults, got %v", got)
	}

	Config.DNSCheck.Resolvers = []string{"192.0.2.1", "192.0.2.2:5353"}
	got := checkResolvers()
	if got[0] != "192.0.2.1:53" {
		t.Fatalf("a resolver without a port should default to :53, got %q", got[0])
	}
	if got[1] != "192.0.2.2:5353" {
		t.Fatalf("an explicit port must be kept, got %q", got[1])
	}
}

// hashForTest wraps bcrypt with a low cost so the suite stays fast.
func hashForTest(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	return string(hash), err
}

func mustParseUUID(t *testing.T, value string) uuid.UUID {
	t.Helper()
	parsed, err := uuid.Parse(value)
	if err != nil {
		t.Fatalf("could not parse uuid %q: %v", value, err)
	}
	return parsed
}
