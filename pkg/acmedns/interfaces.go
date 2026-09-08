package acmedns

import (
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

// ErrNoSuchDomain is returned by AcmednsDB when a subdomain is not registered.
var ErrNoSuchDomain = errors.New("no such domain")

type AcmednsDB interface {
	Register(cidrslice Cidrslice) (ACMETxt, error)
	// RegisterWithName is Register plus the customer domain label used by the
	// management UI
	RegisterWithName(Cidrslice, string) (ACMETxt, error)
	GetByUsername(uuid.UUID) (ACMETxt, error)
	GetBySubdomain(string) (ACMETxt, error)
	GetAllDomains() ([]ACMETxt, error)
	GetTXTForDomain(string) ([]string, error)
	Update(ACMETxtPost) error
	// SetLastSource records where a successful /update came from
	SetLastSource(subdomain string, ip string) error
	UpdateDomainName(string, string) error
	DeleteDomain(string) error
	RotatePassword(string) (string, error)
	GetBackend() *sql.DB
	SetBackend(*sql.DB)
	Close()
}

type AcmednsNS interface {
	Start(errorChannel chan error)
	SetOwnAuthKey(key string)
	SetNotifyStartedFunc(func())
	ParseRecords()
}
