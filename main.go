package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/joohoi/acme-dns/pkg/acmedns"
	"github.com/joohoi/acme-dns/pkg/api"
	"github.com/joohoi/acme-dns/pkg/database"
	"github.com/joohoi/acme-dns/pkg/nameserver"

	"go.uber.org/zap"
)

func main() {
	setUmask()
	configPtr := flag.String("c", "/etc/acme-dns/config.cfg", "config file location")
	hashPwPtr := flag.Bool("hashpw", false, "read a password from stdin and print its bcrypt hash for auth.admin_password_hash")
	flag.Parse()
	if *hashPwPtr {
		if err := printPasswordHash(); err != nil {
			fmt.Printf("Could not hash password: %s\n", err)
			os.Exit(1)
		}
		return
	}
	// Read global config
	var err error
	var logger *zap.Logger
	config, usedConfigFile, err := acmedns.ReadConfig(*configPtr, "./config.cfg")
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}
	logger, err = acmedns.SetupLogging(config)
	if err != nil {
		fmt.Printf("Could not set up logging: %s\n", err)
		os.Exit(1)
	}
	// Make sure to flush the zap logger buffer before exiting
	defer logger.Sync() //nolint:all
	sugar := logger.Sugar()

	sugar.Infow("Using config file",
		"file", usedConfigFile)
	sugar.Info("Starting up")
	db, err := database.Init(&config, sugar)
	// Error channel for servers
	errChan := make(chan error, 1)
	api := api.Init(&config, db, sugar, errChan)
	dnsservers := nameserver.InitAndStart(&config, db, sugar, errChan)
	go api.Start(dnsservers)
	if err != nil {
		sugar.Error(err)
	}
	for {
		err = <-errChan
		if err != nil {
			sugar.Fatal(err)
		}
	}
}

// printPasswordHash reads a password from stdin and prints the bcrypt hash to
// paste into auth.admin_password_hash. Backs the "-hashpw" flag.
func printPasswordHash() error {
	fmt.Fprint(os.Stderr, "Password: ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return err
	}
	password := strings.TrimRight(line, "\r\n")
	if password == "" {
		return errors.New("empty password")
	}
	hash, err := acmedns.HashPassword(password)
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr)
	fmt.Println(hash)
	return nil
}
