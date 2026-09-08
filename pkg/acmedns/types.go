package acmedns

import "github.com/google/uuid"

type Account struct {
	Username  string
	Password  string
	Subdomain string
}

// AcmeDnsConfig holds the config structure
type AcmeDnsConfig struct {
	General   general
	Database  dbsettings
	API       httpapi
	Auth      authconfig
	DNSCheck  dnscheckconfig `toml:"dnscheck"`
	Logconfig logconfig
}

// Config file general section
type general struct {
	Listen        string
	Proto         string `toml:"protocol"`
	Domain        string
	Nsname        string
	Nsadmin       string
	Debug         bool
	StaticRecords []string `toml:"records"`
}

type dbsettings struct {
	Engine     string
	Connection string
}

// API config
type httpapi struct {
	Domain              string `toml:"api_domain"`
	IP                  string
	DisableRegistration bool   `toml:"disable_registration"`
	AutocertPort        string `toml:"autocert_port"`
	Port                string `toml:"port"`
	TLS                 string
	TLSCertPrivkey      string `toml:"tls_cert_privkey"`
	TLSCertFullchain    string `toml:"tls_cert_fullchain"`
	ACMECacheDir        string `toml:"acme_cache_dir"`
	NotificationEmail   string `toml:"notification_email"`
	CorsOrigins         []string
	UseHeader           bool   `toml:"use_header"`
	HeaderName          string `toml:"header_name"`
	HSTSEnabled         bool   `toml:"hsts_enabled"`
	HSTSMaxAge          int    `toml:"hsts_max_age"`
	HSTSIncludeSubDom   bool   `toml:"hsts_include_subdomains"`
	HSTSPreload         bool   `toml:"hsts_preload"`
	// UIPath is the directory holding the built management UI. When it does not
	// exist, acme-dns serves the plain API only.
	UIPath string `toml:"ui_path"`
}

// Auth config for the management UI. Without an admin_password_hash the
// management endpoints stay disabled and only the plain acme-dns API is served.
type authconfig struct {
	AdminUser         string `toml:"admin_user"`
	AdminPasswordHash string `toml:"admin_password_hash"`
	SessionTTLHours   int    `toml:"session_ttl_hours"`
	// CredentialsKey enables storing a recoverable (AES-256-GCM encrypted) copy
	// of every generated API password so the UI can show it again later.
	CredentialsKey string `toml:"credentials_key"`
}

// DNSCheck config for the "verify my CNAME" feature of the management UI
type dnscheckconfig struct {
	// Resolvers queried when validating a customer CNAME, "ip:port" each
	Resolvers []string `toml:"resolvers"`
	// How long to wait for a written test TXT record to become visible
	TXTTimeoutSeconds int `toml:"txt_timeout_seconds"`
}

// Logging config
type logconfig struct {
	Level   string `toml:"loglevel"`
	Logtype string `toml:"logtype"`
	File    string `toml:"logfile"`
	Format  string `toml:"logformat"`
}

// ACMETxt is the default structure for the user controlled record
type ACMETxt struct {
	Username uuid.UUID
	Password string
	ACMETxtPost
	AllowFrom Cidrslice
	// Fulldomain is derived, not stored: Subdomain plus the server's own domain
	Fulldomain string `json:"fulldomain"`
	// DomainName is the customer domain this registration was created for
	DomainName string `json:"domain_name"`
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
	// LastActive is the newest LastUpdate across the registration's TXT rows
	LastActive int64 `json:"last_active"`
}

// ACMETxtPost holds the DNS part of the ACMETxt struct
type ACMETxtPost struct {
	Subdomain string `json:"subdomain"`
	Value     string `json:"txt"`
}
