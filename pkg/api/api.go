package api

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/joohoi/acme-dns/pkg/acmedns"

	"github.com/caddyserver/certmagic"
	"github.com/julienschmidt/httprouter"
	"github.com/rs/cors"
	"go.uber.org/zap"
)

type AcmednsAPI struct {
	Config  *acmedns.AcmeDnsConfig
	DB      acmedns.AcmednsDB
	Logger  *zap.SugaredLogger
	errChan chan error
	// sessions holds the management UI logins. It is always non-nil so the
	// handlers do not have to guard against a missing store.
	sessions *sessionStore
	// ptr memoises reverse lookups of the recorded update sources.
	ptr *ptrCache
}

func Init(config *acmedns.AcmeDnsConfig, db acmedns.AcmednsDB, logger *zap.SugaredLogger, errChan chan error) AcmednsAPI {
	a := AcmednsAPI{
		Config:   config,
		DB:       db,
		Logger:   logger,
		errChan:  errChan,
		sessions: newSessionStore(time.Duration(config.Auth.SessionTTLHours) * time.Hour),
		ptr:      newPTRCache(),
	}
	return a
}

func (a *AcmednsAPI) buildHSTSHeader() string {
	if !a.Config.API.HSTSEnabled {
		return ""
	}

	maxAge := a.Config.API.HSTSMaxAge
	if maxAge <= 0 {
		maxAge = 31536000
	}

	header := fmt.Sprintf("max-age=%d", maxAge)

	if a.Config.API.HSTSIncludeSubDom {
		header += "; includeSubDomains"
	}

	if a.Config.API.HSTSPreload {
		header += "; preload"
	}

	return header
}

func (a *AcmednsAPI) hstsMiddleware(next httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		hstsHeader := a.buildHSTSHeader()
		if hstsHeader != "" {
			w.Header().Set("Strict-Transport-Security", hstsHeader)
		}
		next(w, r, ps)
	}
}

func (a *AcmednsAPI) Start(dnsservers []acmedns.AcmednsNS) {
	var err error
	//TODO: do we want to debug log the HTTP server?
	stderrorlog, err := zap.NewStdLogAt(a.Logger.Desugar(), zap.ErrorLevel)
	if err != nil {
		a.errChan <- err
		return
	}
	api := httprouter.New()
	c := cors.New(cors.Options{
		AllowedOrigins:     a.Config.API.CorsOrigins,
		AllowedMethods:     []string{"GET", "POST", "DELETE", "OPTIONS"},
		AllowedHeaders:     []string{"Accept", "Authorization", "Content-Type", "X-Api-User", "X-Api-Key", "X-Admin-Token"},
		OptionsPassthrough: false,
		Debug:              a.Config.General.Debug,
	})
	if a.Config.General.Debug {
		// Logwriter for saner log output
		c.Log = stderrorlog
	}
	if !a.Config.API.DisableRegistration {
		api.POST("/register", a.hstsMiddleware(a.webRegisterPost))
	}
	api.POST("/update", a.hstsMiddleware(a.Auth(a.webUpdatePost)))
	api.GET("/health", a.hstsMiddleware(a.healthCheck))

	// Management API. Every endpoint requires a session token from /api/admin/login.
	if a.adminEnabled() {
		a.Logger.Infow("Management API enabled",
			"user", a.Config.Auth.AdminUser)
		api.POST("/api/admin/login", a.hstsMiddleware(a.webAdminLogin))
		api.POST("/api/admin/logout", a.hstsMiddleware(a.AdminAuth(a.webAdminLogout)))
		api.GET("/api/admin/session", a.hstsMiddleware(a.AdminAuth(a.webAdminSession)))
		api.GET("/api/admin/server", a.hstsMiddleware(a.AdminAuth(a.webAdminServerInfo)))
		api.GET("/api/admin/domains", a.hstsMiddleware(a.AdminAuth(a.webAdminListDomains)))
		api.POST("/api/admin/domains", a.hstsMiddleware(a.AdminAuth(a.webAdminCreateDomain)))
		api.POST("/api/admin/domains/:subdomain/name", a.hstsMiddleware(a.AdminAuth(a.webAdminRenameDomain)))
		api.POST("/api/admin/domains/:subdomain/rotate", a.hstsMiddleware(a.AdminAuth(a.webAdminRotateCredentials)))
		api.DELETE("/api/admin/domains/:subdomain", a.hstsMiddleware(a.AdminAuth(a.webAdminDeleteDomain)))
		api.POST("/api/admin/dnscheck", a.hstsMiddleware(a.AdminAuth(a.webDNSCheck)))
		api.POST("/api/admin/match", a.hstsMiddleware(a.AdminAuth(a.webAdminMatchDomains)))
	} else {
		a.Logger.Info("Management API disabled (set auth.admin_user and auth.admin_password_hash to enable it)")
	}

	// Serve the management UI from the root path when it has been built into the
	// image. Unknown paths fall through to the SPA so Angular routing works on a
	// hard refresh.
	if a.uiAvailable() {
		a.Logger.Infow("Serving management UI",
			"path", a.Config.API.UIPath)
		api.NotFound = a.spaFileServer(a.Config.API.UIPath)
	} else {
		a.Logger.Infow("No management UI found, serving API only",
			"path", a.Config.API.UIPath)
	}

	host := a.Config.API.IP + ":" + a.Config.API.Port

	// TLS specific general settings
	cfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	switch a.Config.API.TLS {
	case acmedns.ApiTlsProviderLetsEncrypt, acmedns.ApiTlsProviderLetsEncryptStaging:
		magic := a.setupTLS(dnsservers)
		err = magic.ManageAsync(context.Background(), []string{a.Config.General.Domain})
		if err != nil {
			a.errChan <- err
			return
		}
		cfg.GetCertificate = magic.GetCertificate
		srv := &http.Server{
			Addr:         host,
			Handler:      c.Handler(api),
			TLSConfig:    cfg,
			ErrorLog:     stderrorlog,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  120 * time.Second,
		}
		a.Logger.Infow("Listening HTTPS",
			"host", host,
			"domain", a.Config.General.Domain)
		err = srv.ListenAndServeTLS("", "")
	case acmedns.ApiTlsProviderCert:
		srv := &http.Server{
			Addr:         host,
			Handler:      c.Handler(api),
			TLSConfig:    cfg,
			ErrorLog:     stderrorlog,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  120 * time.Second,
		}
		a.Logger.Infow("Listening HTTPS",
			"host", host,
			"domain", a.Config.General.Domain)
		err = srv.ListenAndServeTLS(a.Config.API.TLSCertFullchain, a.Config.API.TLSCertPrivkey)
	default:
		a.Logger.Infow("Listening HTTP",
			"host", host)
		err = http.ListenAndServe(host, c.Handler(api))
	}
	if err != nil {
		a.errChan <- err
	}
}

func (a *AcmednsAPI) setupTLS(dnsservers []acmedns.AcmednsNS) *certmagic.Config {
	provider := NewChallengeProvider(dnsservers)
	certmagic.Default.Logger = a.Logger.Desugar()
	storage := certmagic.FileStorage{Path: a.Config.API.ACMECacheDir}

	// Set up certmagic for getting certificate for acme-dns api
	certmagic.DefaultACME.DNS01Solver = &provider
	certmagic.DefaultACME.Agreed = true
	certmagic.DefaultACME.Logger = a.Logger.Desugar()
	if a.Config.API.TLS == acmedns.ApiTlsProviderLetsEncrypt {
		certmagic.DefaultACME.CA = certmagic.LetsEncryptProductionCA
	} else {
		certmagic.DefaultACME.CA = certmagic.LetsEncryptStagingCA
	}
	certmagic.DefaultACME.Email = a.Config.API.NotificationEmail

	certmagic.Default.Logger = a.Logger.Desugar()
	certmagic.Default.Storage = &storage
	certmagic.Default.DefaultServerName = a.Config.General.Domain

	magic := certmagic.NewDefault()
	return magic
}
