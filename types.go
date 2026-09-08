package main

import (
	"database/sql"
	"sync"

	"github.com/google/uuid"
)

// Config is global configuration struct
var Config DNSConfig

// DB is used to access the database functions in acme-dns
var DB database

// DNSConfig holds the config structure
type DNSConfig struct {
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

// DNS check config for the "verify my CNAME" feature of the management UI
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

type acmedb struct {
	Mutex sync.Mutex
	DB    *sql.DB
}

type database interface {
	Init(string, string) error
	Register(cidrslice) (ACMETxt, error)
	RegisterWithName(cidrslice, string) (ACMETxt, error)
	GetByUsername(uuid.UUID) (ACMETxt, error)
	GetTXTForDomain(string) ([]string, error)
	Update(ACMETxtPost) error
	GetBackend() *sql.DB
	SetBackend(*sql.DB)
	Close()
	GetAllDomains() ([]ACMETxt, error)
	UpdateDomainName(string, string) error
	GetBySubdomain(string) (ACMETxt, error)
	DeleteDomain(string) error
	RotatePassword(string) (string, error)
}
