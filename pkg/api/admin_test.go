package api

import (
	"encoding/json"
	"fmt"
	"io"
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

	"github.com/joohoi/acme-dns/pkg/acmedns"
	"github.com/joohoi/acme-dns/pkg/database"
)

const testAdminPassword = "correct horse battery staple"

// hashForTest wraps bcrypt with a low cost so the suite stays fast.
func hashForTest(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	return string(hash), err
}

// setupAdminAPI wires the management endpoints against a fresh in-memory
// database and returns the router alongside the API instance.
func setupAdminAPI(t *testing.T) (http.Handler, AcmednsAPI, acmedns.AcmednsDB) {
	t.Helper()

	config, logger := fakeConfigAndLogger()
	config.General.Domain = "auth.example.org"
	config.API.TLS = acmedns.ApiTlsProviderNone
	config.Auth.AdminUser = "admin"
	config.Auth.SessionTTLHours = 1
	config.Auth.CredentialsKey = "unit-test-key"

	hash, err := hashForTest(testAdminPassword)
	if err != nil {
		t.Fatalf("could not hash test password: %v", err)
	}
	config.Auth.AdminPasswordHash = hash

	db, err := database.Init(&config, logger)
	if err != nil {
		t.Fatalf("could not initialise the test database: %v", err)
	}
	t.Cleanup(db.Close)

	adnsapi := Init(&config, db, logger, make(chan error, 1))

	router := httprouter.New()
	router.POST("/api/admin/login", adnsapi.webAdminLogin)
	router.GET("/api/admin/server", adnsapi.AdminAuth(adnsapi.webAdminServerInfo))
	router.GET("/api/admin/domains", adnsapi.AdminAuth(adnsapi.webAdminListDomains))
	router.POST("/api/admin/domains", adnsapi.AdminAuth(adnsapi.webAdminCreateDomain))
	router.POST("/api/admin/domains/:subdomain/name", adnsapi.AdminAuth(adnsapi.webAdminRenameDomain))
	router.POST("/api/admin/domains/:subdomain/rotate", adnsapi.AdminAuth(adnsapi.webAdminRotateCredentials))
	router.DELETE("/api/admin/domains/:subdomain", adnsapi.AdminAuth(adnsapi.webAdminDeleteDomain))

	return router, adnsapi, db
}

func loginForTest(t *testing.T, router http.Handler) string {
	t.Helper()
	body := strings.NewReader(`{"username": "admin", "password": "` + testAdminPassword + `"}`)
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

func mustParseUUID(t *testing.T, value string) uuid.UUID {
	t.Helper()
	parsed, err := uuid.Parse(value)
	if err != nil {
		t.Fatalf("could not parse uuid %q: %v", value, err)
	}
	return parsed
}

func TestSessionStoreLifecycle(t *testing.T) {
	store := newSessionStore(time.Hour)
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
	store := newSessionStore(time.Hour)
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

func TestSessionStoreDefaultTTL(t *testing.T) {
	store := newSessionStore(0)
	if store.ttl != defaultSessionTTL {
		t.Fatalf("an unset TTL should fall back to the default, got %s", store.ttl)
	}
}

func TestSessionStoreLockout(t *testing.T) {
	store := newSessionStore(time.Hour)
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
	router, _, _ := setupAdminAPI(t)

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
	router, _, _ := setupAdminAPI(t)

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
	router, _, db := setupAdminAPI(t)
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
	stored, err := db.GetByUsername(mustParseUUID(t, created.Username))
	if err != nil {
		t.Fatalf("could not read back the record: %v", err)
	}
	if acmedns.CorrectPassword(created.Password, stored.Password) {
		t.Fatal("the old password must stop working after a rotation")
	}
	if !acmedns.CorrectPassword(rotated.Password, stored.Password) {
		t.Fatal("the rotated password must authenticate")
	}

	// Delete
	rec = doAuthed(t, router, http.MethodDelete, "/api/admin/domains/"+created.Subdomain, token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete returned %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := db.GetBySubdomain(created.Subdomain); err != acmedns.ErrNoSuchDomain {
		t.Fatalf("the record should be gone, got err %v", err)
	}

	// Deleting twice is a 404 rather than a silent success
	rec = doAuthed(t, router, http.MethodDelete, "/api/admin/domains/"+created.Subdomain, token, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("second delete returned %d, want 404", rec.Code)
	}
}

// Records created before credential storage was switched on have no recoverable
// password; the UI relies on the flag to offer a rotation instead.
func TestAdminReportsMissingCredentials(t *testing.T) {
	router, _, db := setupAdminAPI(t)
	token := loginForTest(t, router)

	created, err := db.RegisterWithName(acmedns.Cidrslice{}, "legacy.example.com")
	if err != nil {
		t.Fatalf("could not create a registration: %v", err)
	}
	// Simulate a pre-upgrade row by clearing the stored copy directly.
	if _, err := db.GetBackend().Exec("UPDATE records SET EncPassword='' WHERE Subdomain=?", created.Subdomain); err != nil {
		t.Fatalf("could not clear the stored credential: %v", err)
	}

	rec := doAuthed(t, router, http.MethodGet, "/api/admin/domains", token, "")
	var listed []AdminDomain
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("could not decode list: %v", err)
	}
	for _, d := range listed {
		if d.Subdomain != created.Subdomain {
			continue
		}
		if d.CredentialsAvailable || d.Password != "" {
			t.Fatalf("a record without a stored password must be reported as unavailable, got %+v", d)
		}
		return
	}
	t.Fatal("the registration is missing from the list")
}

func TestAdminRejectsInvalidSubdomain(t *testing.T) {
	router, _, _ := setupAdminAPI(t)
	token := loginForTest(t, router)

	rec := doAuthed(t, router, http.MethodDelete, "/api/admin/domains/not..valid", token, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid subdomain returned %d, want 400", rec.Code)
	}
}

func TestSPAFileServer(t *testing.T) {
	_, adnsapi, _ := setupAdminAPI(t)

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

	handler := adnsapi.spaFileServer(root)

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
		// A deployment that ships the UI but leaves the management API off must
		// still answer /api/* with JSON, or the UI parses index.html.
		adnsapi.Config.Auth.AdminUser = ""
		adnsapi.Config.Auth.AdminPasswordHash = ""

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
	_, adnsapi, _ := setupAdminAPI(t)

	adnsapi.Config.DNSCheck.Resolvers = nil
	if got := adnsapi.checkResolvers(); len(got) != len(defaultCheckResolvers) {
		t.Fatalf("expected the built in defaults, got %v", got)
	}

	adnsapi.Config.DNSCheck.Resolvers = []string{"192.0.2.1", "192.0.2.2:5353"}
	got := adnsapi.checkResolvers()
	if got[0] != "192.0.2.1:53" {
		t.Fatalf("a resolver without a port should default to :53, got %q", got[0])
	}
	if got[1] != "192.0.2.2:5353" {
		t.Fatalf("an explicit port must be kept, got %q", got[1])
	}
}

// The end-to-end DNS check polls public resolvers for up to txt_timeout_seconds,
// which is longer than the server's WriteTimeout in the TLS branch. webDNSCheck
// relies on http.ResponseController to push that one response's deadline out;
// without it the connection would be closed mid-check and the UI would show a
// network error instead of a verdict.
func TestResponseDeadlineCanOutliveWriteTimeout(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
			t.Errorf("could not extend the write deadline: %v", err)
		}
		time.Sleep(1500 * time.Millisecond)
		_, _ = w.Write([]byte("done"))
	})

	srv := httptest.NewUnstartedServer(handler)
	srv.Config.WriteTimeout = 500 * time.Millisecond
	srv.Start()
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL)
	if err != nil {
		t.Fatalf("request failed, the write deadline was not extended: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("could not read the response: %v", err)
	}
	if string(body) != "done" {
		t.Fatalf("got %q, want the full response written after the original deadline", body)
	}
}

func TestSetLastSourceIsVisibleToTheUI(t *testing.T) {
	router, _, db := setupAdminAPI(t)
	token := loginForTest(t, router)

	created, err := db.RegisterWithName(acmedns.Cidrslice{}, "source.example.com")
	if err != nil {
		t.Fatalf("could not create a registration: %v", err)
	}

	// Nothing recorded yet: the column exists but stays empty until a client writes.
	fresh, err := db.GetBySubdomain(created.Subdomain)
	if err != nil {
		t.Fatalf("could not read the registration: %v", err)
	}
	if fresh.LastIP != "" {
		t.Fatalf("a new registration must not carry a source address, got %q", fresh.LastIP)
	}

	if err := db.SetLastSource(created.Subdomain, "203.0.113.7"); err != nil {
		t.Fatalf("could not record the source: %v", err)
	}

	stored, err := db.GetBySubdomain(created.Subdomain)
	if err != nil {
		t.Fatalf("could not read the registration back: %v", err)
	}
	if stored.LastIP != "203.0.113.7" {
		t.Fatalf("source did not round trip, got %q", stored.LastIP)
	}

	rec := doAuthed(t, router, http.MethodGet, "/api/admin/domains", token, "")
	var listed []AdminDomain
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("could not decode the list: %v", err)
	}
	for _, d := range listed {
		if d.Subdomain == created.Subdomain {
			if d.LastIP != "203.0.113.7" {
				t.Fatalf("the list must expose the recorded source, got %q", d.LastIP)
			}
			return
		}
	}
	t.Fatal("the registration is missing from the list")
}

// An empty address must not overwrite a previously recorded one, otherwise the
// DNS check's own writes would erase the only owner hint we have.
func TestSetLastSourceIgnoresEmptyAddress(t *testing.T) {
	_, _, db := setupAdminAPI(t)

	created, err := db.RegisterWithName(acmedns.Cidrslice{}, "keep.example.com")
	if err != nil {
		t.Fatalf("could not create a registration: %v", err)
	}
	if err := db.SetLastSource(created.Subdomain, "198.51.100.4"); err != nil {
		t.Fatalf("could not record the source: %v", err)
	}
	if err := db.SetLastSource(created.Subdomain, ""); err != nil {
		t.Fatalf("an empty source should be a no-op, got %v", err)
	}

	stored, _ := db.GetBySubdomain(created.Subdomain)
	if stored.LastIP != "198.51.100.4" {
		t.Fatalf("the recorded source was lost, got %q", stored.LastIP)
	}
}

func TestPTRCacheHonoursExpiry(t *testing.T) {
	cache := newPTRCache()

	if _, ok := cache.get("192.0.2.1"); ok {
		t.Fatal("an unseen address must be a miss")
	}

	cache.put("192.0.2.1", "host.example.com")
	host, ok := cache.get("192.0.2.1")
	if !ok || host != "host.example.com" {
		t.Fatalf("expected the cached host, got %q / %v", host, ok)
	}

	// A negative result is cached too, so a failing lookup is not retried per request.
	cache.put("192.0.2.2", "")
	host, ok = cache.get("192.0.2.2")
	if !ok || host != "" {
		t.Fatalf("a negative result should be cached, got %q / %v", host, ok)
	}

	cache.mu.Lock()
	cache.entries["192.0.2.1"] = ptrEntry{host: "host.example.com", expires: time.Now().Add(-time.Minute)}
	cache.mu.Unlock()
	if _, ok := cache.get("192.0.2.1"); ok {
		t.Fatal("an expired entry must be a miss")
	}
}

func TestMatchRejectsBadInput(t *testing.T) {
	_, adnsapi, _ := setupAdminAPI(t)

	cases := []struct {
		name string
		body string
		want string
	}{
		{"empty list", `{"domains": []}`, "no_domains"},
		{"only separators", `{"domains": ["  , ; \n "]}`, "no_domains"},
		{"malformed json", `not json`, "invalid_request"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/admin/match", strings.NewReader(tc.body))
			adnsapi.webAdminMatchDomains(rec, req, nil)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), tc.want) {
				t.Fatalf("got %q, want it to mention %q", rec.Body.String(), tc.want)
			}
		})
	}

	t.Run("too many candidates", func(t *testing.T) {
		domains := make([]string, maxMatchCandidates+1)
		for i := range domains {
			domains[i] = fmt.Sprintf("host%d.example.com", i)
		}
		body, _ := json.Marshal(matchRequest{Domains: domains})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/admin/match", strings.NewReader(string(body)))
		adnsapi.webAdminMatchDomains(rec, req, nil)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "too_many_domains") {
			t.Fatalf("got %d %q, want 400 too_many_domains", rec.Code, rec.Body.String())
		}
	})
}
