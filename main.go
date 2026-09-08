//go:build !test
// +build !test

package main

import (
	"context"
	"crypto/tls"
	"flag"
	stdlog "log"
	"net/http"
	"os"
	"strings"
	"syscall"

	"github.com/caddyserver/certmagic"
	legolog "github.com/go-acme/lego/v3/log"
	"github.com/julienschmidt/httprouter"
	"github.com/rs/cors"
	log "github.com/sirupsen/logrus"
)

func main() {
	// Created files are not world writable
	syscall.Umask(0077)
	configPtr := flag.String("c", "/etc/acme-dns/config.cfg", "config file location")
	hashPwPtr := flag.Bool("hashpw", false, "read a password from stdin and print its bcrypt hash for auth.admin_password_hash")
	flag.Parse()
	if *hashPwPtr {
		if err := printPasswordHash(); err != nil {
			log.Errorf("Could not hash password: %s", err)
			os.Exit(1)
		}
		return
	}
	// Read global config
	var err error
	if fileIsAccessible(*configPtr) {
		log.WithFields(log.Fields{"file": *configPtr}).Info("Using config file")
		Config, err = readConfig(*configPtr)
	} else if fileIsAccessible("./config.cfg") {
		log.WithFields(log.Fields{"file": "./config.cfg"}).Info("Using config file")
		Config, err = readConfig("./config.cfg")
	} else {
		log.Errorf("Configuration file not found.")
		os.Exit(1)
	}
	if err != nil {
		log.Errorf("Encountered an error while trying to read configuration file:  %s", err)
		os.Exit(1)
	}

	setupLogging(Config.Logconfig.Format, Config.Logconfig.Level)

	// Optional recoverable credential storage for the management UI
	if Config.Auth.CredentialsKey != "" {
		CredentialCipher, err = newCredentialCipher(Config.Auth.CredentialsKey)
		if err != nil {
			log.Errorf("Could not initialise credential encryption: %s", err)
			os.Exit(1)
		}
		log.Info("Credential storage enabled, generated passwords are recoverable through the management UI")
	}
	if adminEnabled() {
		log.WithFields(log.Fields{"user": Config.Auth.AdminUser}).Info("Management API enabled")
	} else {
		log.Info("Management API disabled (set auth.admin_user and auth.admin_password_hash to enable it)")
	}

	// Open database
	newDB := new(acmedb)
	err = newDB.Init(Config.Database.Engine, Config.Database.Connection)
	if err != nil {
		log.Errorf("Could not open database [%v]", err)
		os.Exit(1)
	} else {
		log.Info("Connected to database")
	}
	DB = newDB
	defer DB.Close()

	// Error channel for servers
	errChan := make(chan error, 1)

	// DNS server
	dnsservers := make([]*DNSServer, 0)
	if strings.HasPrefix(Config.General.Proto, "both") {
		// Handle the case where DNS server should be started for both udp and tcp
		udpProto := "udp"
		tcpProto := "tcp"
		if strings.HasSuffix(Config.General.Proto, "4") {
			udpProto += "4"
			tcpProto += "4"
		} else if strings.HasSuffix(Config.General.Proto, "6") {
			udpProto += "6"
			tcpProto += "6"
		}
		dnsServerUDP := NewDNSServer(DB, Config.General.Listen, udpProto, Config.General.Domain)
		dnsservers = append(dnsservers, dnsServerUDP)
		dnsServerUDP.ParseRecords(Config)
		dnsServerTCP := NewDNSServer(DB, Config.General.Listen, tcpProto, Config.General.Domain)
		dnsservers = append(dnsservers, dnsServerTCP)
		// No need to parse records from config again
		dnsServerTCP.Domains = dnsServerUDP.Domains
		dnsServerTCP.SOA = dnsServerUDP.SOA
		go dnsServerUDP.Start(errChan)
		go dnsServerTCP.Start(errChan)
	} else {
		dnsServer := NewDNSServer(DB, Config.General.Listen, Config.General.Proto, Config.General.Domain)
		dnsservers = append(dnsservers, dnsServer)
		dnsServer.ParseRecords(Config)
		go dnsServer.Start(errChan)
	}

	// HTTP API
	go startHTTPAPI(errChan, Config, dnsservers)

	// block waiting for error
	for {
		err = <-errChan
		if err != nil {
			log.Fatal(err)
		}
	}
}

func startHTTPAPI(errChan chan error, config DNSConfig, dnsservers []*DNSServer) {
	// Setup http logger
	logger := log.New()
	logwriter := logger.Writer()
	defer logwriter.Close()
	// Setup logging for different dependencies to log with logrus
	// Certmagic
	stdlog.SetOutput(logwriter)
	// Lego
	legolog.Logger = logger

	api := httprouter.New()
	c := cors.New(cors.Options{
		AllowedOrigins:     Config.API.CorsOrigins,
		AllowedMethods:     []string{"GET", "POST", "DELETE", "OPTIONS"},
		AllowedHeaders:     []string{"Accept", "Authorization", "Content-Type", "X-Api-User", "X-Api-Key", "X-Admin-Token"},
		OptionsPassthrough: false,
		Debug:              Config.General.Debug,
	})
	if Config.General.Debug {
		// Logwriter for saner log output
		c.Log = stdlog.New(logwriter, "", 0)
	}

	// Plain acme-dns API, unchanged and compatible with every acme-dns client
	if !Config.API.DisableRegistration {
		api.POST("/register", webRegisterPost)
	}
	api.POST("/update", Auth(webUpdatePost))
	api.GET("/health", healthCheck)

	// Management API. Every endpoint requires a session token from /api/admin/login.
	if adminEnabled() {
		api.POST("/api/admin/login", webAdminLogin)
		api.POST("/api/admin/logout", AdminAuth(webAdminLogout))
		api.GET("/api/admin/session", AdminAuth(webAdminSession))
		api.GET("/api/admin/server", AdminAuth(webAdminServerInfo))
		api.GET("/api/admin/domains", AdminAuth(webAdminListDomains))
		api.POST("/api/admin/domains", AdminAuth(webAdminCreateDomain))
		api.POST("/api/admin/domains/:subdomain/name", AdminAuth(webAdminRenameDomain))
		api.POST("/api/admin/domains/:subdomain/rotate", AdminAuth(webAdminRotateCredentials))
		api.DELETE("/api/admin/domains/:subdomain", AdminAuth(webAdminDeleteDomain))
		api.POST("/api/admin/dnscheck", AdminAuth(webDNSCheck))
	}

	// Serve the management UI from the root path when it has been built into the
	// image. Unknown paths fall through to the SPA so Angular routing works on
	// a hard refresh.
	var handler http.Handler = api
	if uiAvailable() {
		log.WithFields(log.Fields{"path": Config.API.UIPath}).Info("Serving management UI")
		api.NotFound = spaFileServer(Config.API.UIPath)
	} else {
		log.WithFields(log.Fields{"path": Config.API.UIPath}).Info("No management UI found, serving API only")
	}

	host := Config.API.IP + ":" + Config.API.Port

	// TLS specific general settings
	cfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	provider := NewChallengeProvider(dnsservers)
	storage := certmagic.FileStorage{Path: Config.API.ACMECacheDir}

	// Set up certmagic for getting certificate for acme-dns api
	certmagic.DefaultACME.DNS01Solver = &provider
	certmagic.DefaultACME.Agreed = true
	if Config.API.TLS == "letsencrypt" {
		certmagic.DefaultACME.CA = certmagic.LetsEncryptProductionCA
	} else {
		certmagic.DefaultACME.CA = certmagic.LetsEncryptStagingCA
	}
	certmagic.DefaultACME.Email = Config.API.NotificationEmail
	magicConf := certmagic.NewDefault()
	magicConf.Storage = &storage
	magicConf.DefaultServerName = Config.General.Domain

	magicCache := certmagic.NewCache(certmagic.CacheOptions{
		GetConfigForCert: func(cert certmagic.Certificate) (*certmagic.Config, error) {
			return magicConf, nil
		},
	})

	magic := certmagic.New(magicCache, *magicConf)
	var err error
	switch Config.API.TLS {
	case "letsencryptstaging":
		err = magic.ManageAsync(context.Background(), []string{Config.General.Domain})
		if err != nil {
			errChan <- err
			return
		}
		cfg.GetCertificate = magic.GetCertificate

		srv := &http.Server{
			Addr:      host,
			Handler:   c.Handler(handler),
			TLSConfig: cfg,
			ErrorLog:  stdlog.New(logwriter, "", 0),
		}
		log.WithFields(log.Fields{"host": host, "domain": Config.General.Domain}).Info("Listening HTTPS")
		err = srv.ListenAndServeTLS("", "")
	case "letsencrypt":
		err = magic.ManageAsync(context.Background(), []string{Config.General.Domain})
		if err != nil {
			errChan <- err
			return
		}
		cfg.GetCertificate = magic.GetCertificate
		srv := &http.Server{
			Addr:      host,
			Handler:   c.Handler(handler),
			TLSConfig: cfg,
			ErrorLog:  stdlog.New(logwriter, "", 0),
		}
		log.WithFields(log.Fields{"host": host, "domain": Config.General.Domain}).Info("Listening HTTPS")
		err = srv.ListenAndServeTLS("", "")
	case "cert":
		srv := &http.Server{
			Addr:      host,
			Handler:   c.Handler(handler),
			TLSConfig: cfg,
			ErrorLog:  stdlog.New(logwriter, "", 0),
		}
		log.WithFields(log.Fields{"host": host}).Info("Listening HTTPS")
		err = srv.ListenAndServeTLS(Config.API.TLSCertFullchain, Config.API.TLSCertPrivkey)
	default:
		log.WithFields(log.Fields{"host": host}).Info("Listening HTTP")
		err = http.ListenAndServe(host, c.Handler(handler))
	}
	if err != nil {
		errChan <- err
	}
}
