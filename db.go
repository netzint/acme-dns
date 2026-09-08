package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
)

// DBVersion shows the database version this code uses. This is used for update checks.
var DBVersion = 3

// errNoSuchDomain is returned when a subdomain is not present in the records table
var errNoSuchDomain = errors.New("no such domain")

var acmeTable = `
	CREATE TABLE IF NOT EXISTS acmedns(
		Name TEXT,
		Value TEXT
	);`

var userTable = `
	CREATE TABLE IF NOT EXISTS records(
        Username TEXT UNIQUE NOT NULL PRIMARY KEY,
        Password TEXT UNIQUE NOT NULL,
        Subdomain TEXT UNIQUE NOT NULL,
		AllowFrom TEXT,
		DomainName TEXT DEFAULT '',
		CreatedAt INT DEFAULT 0,
		UpdatedAt INT DEFAULT 0,
		EncPassword TEXT DEFAULT ''
    );`

var txtTable = `
    CREATE TABLE IF NOT EXISTS txt(
		Subdomain TEXT NOT NULL,
		Value   TEXT NOT NULL DEFAULT '',
		LastUpdate INT
	);`

var txtTablePG = `
    CREATE TABLE IF NOT EXISTS txt(
		rowid SERIAL,
		Subdomain TEXT NOT NULL,
		Value   TEXT NOT NULL DEFAULT '',
		LastUpdate INT
	);`

// getSQLiteStmt replaces all PostgreSQL prepared statement placeholders (eg. $1, $2) with SQLite variant "?"
func getSQLiteStmt(s string) string {
	re, _ := regexp.Compile(`\$[0-9]`)
	return re.ReplaceAllString(s, "?")
}

func (d *acmedb) Init(engine string, connection string) error {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	db, err := sql.Open(engine, connection)
	if err != nil {
		return err
	}
	d.DB = db
	// Check version first to try to catch old versions without version string
	var versionString string
	_ = d.DB.QueryRow("SELECT Value FROM acmedns WHERE Name='db_version'").Scan(&versionString)
	if versionString == "" {
		versionString = "0"
	}
	_, _ = d.DB.Exec(acmeTable)
	_, _ = d.DB.Exec(userTable)
	if Config.Database.Engine == "sqlite3" {
		_, _ = d.DB.Exec(txtTable)
	} else {
		_, _ = d.DB.Exec(txtTablePG)
	}
	// If everything is fine, handle db upgrade tasks
	if err == nil {
		err = d.checkDBUpgrades(versionString)
	}
	if err == nil {
		if versionString == "0" {
			// No errors so we should now be in version 1
			insversion := fmt.Sprintf("INSERT INTO acmedns (Name, Value) values('db_version', '%d')", DBVersion)
			_, err = db.Exec(insversion)
		}
	}
	return err
}

func (d *acmedb) checkDBUpgrades(versionString string) error {
	var err error
	version, err := strconv.Atoi(versionString)
	if err != nil {
		return err
	}
	if version != DBVersion {
		return d.handleDBUpgrades(version)
	}
	return nil

}

func (d *acmedb) handleDBUpgrades(version int) error {
	if version == 0 {
		err := d.handleDBUpgradeTo1()
		if err != nil {
			return err
		}
		version = 1
	}
	if version == 1 {
		err := d.handleDBUpgradeTo2()
		if err != nil {
			return err
		}
		version = 2
	}
	if version == 2 {
		return d.handleDBUpgradeTo3()
	}
	return nil
}

// handleDBUpgradeTo3 adds the column holding the encrypted, recoverable copy of
// the generated API password. Records created before this upgrade keep an empty
// value and have to be rotated before the UI can show their credentials again.
func (d *acmedb) handleDBUpgradeTo3() error {
	log.Info("Upgrading database to version 3: Adding EncPassword column")

	tx, err := d.DB.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
			return
		}
		_ = tx.Commit()
	}()

	if Config.Database.Engine == "sqlite3" {
		var count int
		err = tx.QueryRow("SELECT COUNT(*) FROM pragma_table_info('records') WHERE name='EncPassword'").Scan(&count)
		if err != nil || count == 0 {
			_, err = tx.Exec("ALTER TABLE records ADD COLUMN EncPassword TEXT DEFAULT ''")
			if err != nil && !strings.Contains(err.Error(), "duplicate column") {
				log.WithFields(log.Fields{"error": err.Error()}).Error("Error adding EncPassword column")
				return err
			}
			err = nil
		}
	} else {
		_, err = tx.Exec("ALTER TABLE records ADD COLUMN IF NOT EXISTS EncPassword TEXT DEFAULT ''")
		if err != nil {
			log.WithFields(log.Fields{"error": err.Error()}).Error("Error adding EncPassword column")
			return err
		}
	}

	_, err = tx.Exec("UPDATE acmedns SET Value='3' WHERE Name='db_version'")
	if err != nil {
		log.WithFields(log.Fields{"error": err.Error()}).Error("Error updating database version")
		return err
	}

	log.Info("Database upgraded to version 3 successfully")
	return nil
}

func (d *acmedb) handleDBUpgradeTo1() error {
	var err error
	var subdomains []string
	rows, err := d.DB.Query("SELECT Subdomain FROM records")
	if err != nil {
		log.WithFields(log.Fields{"error": err.Error()}).Error("Error in DB upgrade")
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var subdomain string
		err = rows.Scan(&subdomain)
		if err != nil {
			log.WithFields(log.Fields{"error": err.Error()}).Error("Error in DB upgrade while reading values")
			return err
		}
		subdomains = append(subdomains, subdomain)
	}
	err = rows.Err()
	if err != nil {
		log.WithFields(log.Fields{"error": err.Error()}).Error("Error in DB upgrade while inserting values")
		return err
	}
	tx, err := d.DB.Begin()
	// Rollback if errored, commit if not
	defer func() {
		if err != nil {
			_ = tx.Rollback()
			return
		}
		_ = tx.Commit()
	}()
	_, _ = tx.Exec("DELETE FROM txt")
	for _, subdomain := range subdomains {
		if subdomain != "" {
			// Insert two rows for each subdomain to txt table
			err = d.NewTXTValuesInTransaction(tx, subdomain)
			if err != nil {
				log.WithFields(log.Fields{"error": err.Error()}).Error("Error in DB upgrade while inserting values")
				return err
			}
		}
	}
	// SQLite doesn't support dropping columns
	if Config.Database.Engine != "sqlite3" {
		_, _ = tx.Exec("ALTER TABLE records DROP COLUMN IF EXISTS Value")
		_, _ = tx.Exec("ALTER TABLE records DROP COLUMN IF EXISTS LastActive")
	}
	_, err = tx.Exec("UPDATE acmedns SET Value='1' WHERE Name='db_version'")
	return err
}

func (d *acmedb) handleDBUpgradeTo2() error {
	log.Info("Upgrading database to version 2: Adding DomainName, CreatedAt, UpdatedAt columns")

	tx, err := d.DB.Begin()
	if err != nil {
		return err
	}

	// Rollback if errored, commit if not
	defer func() {
		if err != nil {
			_ = tx.Rollback()
			return
		}
		_ = tx.Commit()
	}()

	// Add new columns if they don't exist
	// For SQLite, we need to check if columns exist first
	if Config.Database.Engine == "sqlite3" {
		// SQLite doesn't support ALTER TABLE ADD COLUMN IF NOT EXISTS
		// We need to check the table schema first
		var count int
		err = tx.QueryRow("SELECT COUNT(*) FROM pragma_table_info('records') WHERE name='DomainName'").Scan(&count)
		if err != nil || count == 0 {
			_, err = tx.Exec("ALTER TABLE records ADD COLUMN DomainName TEXT DEFAULT ''")
			if err != nil && !strings.Contains(err.Error(), "duplicate column") {
				log.WithFields(log.Fields{"error": err.Error()}).Error("Error adding DomainName column")
				return err
			}
		}

		err = tx.QueryRow("SELECT COUNT(*) FROM pragma_table_info('records') WHERE name='CreatedAt'").Scan(&count)
		if err != nil || count == 0 {
			_, err = tx.Exec("ALTER TABLE records ADD COLUMN CreatedAt INT DEFAULT 0")
			if err != nil && !strings.Contains(err.Error(), "duplicate column") {
				log.WithFields(log.Fields{"error": err.Error()}).Error("Error adding CreatedAt column")
				return err
			}
		}

		err = tx.QueryRow("SELECT COUNT(*) FROM pragma_table_info('records') WHERE name='UpdatedAt'").Scan(&count)
		if err != nil || count == 0 {
			_, err = tx.Exec("ALTER TABLE records ADD COLUMN UpdatedAt INT DEFAULT 0")
			if err != nil && !strings.Contains(err.Error(), "duplicate column") {
				log.WithFields(log.Fields{"error": err.Error()}).Error("Error adding UpdatedAt column")
				return err
			}
		}
	} else {
		// PostgreSQL supports ADD COLUMN IF NOT EXISTS
		_, err = tx.Exec("ALTER TABLE records ADD COLUMN IF NOT EXISTS DomainName TEXT DEFAULT ''")
		if err != nil {
			log.WithFields(log.Fields{"error": err.Error()}).Error("Error adding DomainName column")
			return err
		}
		_, err = tx.Exec("ALTER TABLE records ADD COLUMN IF NOT EXISTS CreatedAt INT DEFAULT 0")
		if err != nil {
			log.WithFields(log.Fields{"error": err.Error()}).Error("Error adding CreatedAt column")
			return err
		}
		_, err = tx.Exec("ALTER TABLE records ADD COLUMN IF NOT EXISTS UpdatedAt INT DEFAULT 0")
		if err != nil {
			log.WithFields(log.Fields{"error": err.Error()}).Error("Error adding UpdatedAt column")
			return err
		}
	}

	// Update version
	_, err = tx.Exec("UPDATE acmedns SET Value='2' WHERE Name='db_version'")
	if err != nil {
		log.WithFields(log.Fields{"error": err.Error()}).Error("Error updating database version")
		return err
	}

	log.Info("Database upgraded to version 2 successfully")
	return nil
}

// Create two rows for subdomain to the txt table
func (d *acmedb) NewTXTValuesInTransaction(tx *sql.Tx, subdomain string) error {
	var err error
	instr := fmt.Sprintf("INSERT INTO txt (Subdomain, LastUpdate) values('%s', 0)", subdomain)
	_, _ = tx.Exec(instr)
	_, _ = tx.Exec(instr)
	return err
}

func (d *acmedb) Register(afrom cidrslice) (ACMETxt, error) {
	return d.RegisterWithName(afrom, "")
}

func (d *acmedb) RegisterWithName(afrom cidrslice, domainName string) (ACMETxt, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	var err error
	tx, err := d.DB.Begin()
	// Rollback if errored, commit if not
	defer func() {
		if err != nil {
			_ = tx.Rollback()
			return
		}
		_ = tx.Commit()
	}()
	a := newACMETxt()
	a.AllowFrom = cidrslice(afrom.ValidEntries())
	a.DomainName = domainName
	a.CreatedAt = time.Now().Unix()
	a.UpdatedAt = time.Now().Unix()
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(a.Password), 10)
	regSQL := `
    INSERT INTO records(
        Username,
        Password,
        Subdomain,
		AllowFrom,
		DomainName,
		CreatedAt,
		UpdatedAt,
		EncPassword)
        values($1, $2, $3, $4, $5, $6, $7, $8)`
	if Config.Database.Engine == "sqlite3" {
		regSQL = getSQLiteStmt(regSQL)
	}
	sm, err := tx.Prepare(regSQL)
	if err != nil {
		log.WithFields(log.Fields{"error": err.Error()}).Error("Database error in prepare")
		return a, errors.New("SQL error")
	}
	defer sm.Close()
	_, err = sm.Exec(a.Username.String(), passwordHash, a.Subdomain, a.AllowFrom.JSON(), a.DomainName, a.CreatedAt, a.UpdatedAt, encryptCredential(a.Password))
	if err == nil {
		err = d.NewTXTValuesInTransaction(tx, a.Subdomain)
	}
	return a, err
}

// UpdateDomainName updates the domain name for a given subdomain
func (d *acmedb) UpdateDomainName(subdomain string, domainName string) error {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()

	query := `UPDATE records SET DomainName = $1, UpdatedAt = $2 WHERE Subdomain = $3`
	if Config.Database.Engine == "sqlite3" {
		query = getSQLiteStmt(query)
	}

	_, err := d.DB.Exec(query, domainName, time.Now().Unix(), subdomain)
	if err != nil {
		return err
	}

	return nil
}

// GetAllDomains returns every registration for the management UI. The returned
// Password is the decrypted plaintext when credential storage is enabled and the
// record was created after it was turned on; otherwise it is empty and the
// caller has to offer a credential rotation instead.
func (d *acmedb) GetAllDomains() ([]ACMETxt, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	var results []ACMETxt
	getSQL := `
	SELECT r.Username, r.Subdomain, r.AllowFrom,
	       COALESCE(r.DomainName, ''),
	       COALESCE(r.CreatedAt, 0),
	       COALESCE(r.UpdatedAt, 0),
	       COALESCE(r.EncPassword, ''),
	       COALESCE((SELECT MAX(t.LastUpdate) FROM txt t WHERE t.Subdomain = r.Subdomain), 0)
	FROM records r
	`
	rows, err := d.DB.Query(getSQL)
	if err != nil {
		return results, err
	}
	defer rows.Close()
	for rows.Next() {
		txt := ACMETxt{}
		afrom := ""
		encPassword := ""
		err = rows.Scan(&txt.Username, &txt.Subdomain, &afrom,
			&txt.DomainName, &txt.CreatedAt, &txt.UpdatedAt, &encPassword, &txt.LastActive)
		if err != nil {
			log.WithFields(log.Fields{"error": err.Error()}).Error("Database error in GetAllDomains")
			return results, err
		}
		_ = txt.AllowFrom.Unmarshal(afrom)
		// Never expose the bcrypt hash; hand out the recoverable copy instead
		txt.Password = decryptCredential(encPassword)
		txt.Fulldomain = txt.Subdomain + "." + Config.General.Domain
		results = append(results, txt)
	}
	return results, rows.Err()
}

// GetBySubdomain looks up a single registration by its subdomain. Like
// GetAllDomains it returns the decrypted password rather than the bcrypt hash.
func (d *acmedb) GetBySubdomain(subdomain string) (ACMETxt, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	getSQL := `
	SELECT r.Username, r.Subdomain, r.AllowFrom,
	       COALESCE(r.DomainName, ''),
	       COALESCE(r.CreatedAt, 0),
	       COALESCE(r.UpdatedAt, 0),
	       COALESCE(r.EncPassword, ''),
	       COALESCE((SELECT MAX(t.LastUpdate) FROM txt t WHERE t.Subdomain = r.Subdomain), 0)
	FROM records r
	WHERE r.Subdomain=$1 LIMIT 1
	`
	if Config.Database.Engine == "sqlite3" {
		getSQL = getSQLiteStmt(getSQL)
	}
	txt := ACMETxt{}
	afrom := ""
	encPassword := ""
	err := d.DB.QueryRow(getSQL, subdomain).Scan(&txt.Username, &txt.Subdomain, &afrom,
		&txt.DomainName, &txt.CreatedAt, &txt.UpdatedAt, &encPassword, &txt.LastActive)
	if err != nil {
		if err == sql.ErrNoRows {
			return ACMETxt{}, errNoSuchDomain
		}
		return ACMETxt{}, err
	}
	_ = txt.AllowFrom.Unmarshal(afrom)
	txt.Password = decryptCredential(encPassword)
	txt.Fulldomain = txt.Subdomain + "." + Config.General.Domain
	return txt, nil
}

// DeleteDomain removes a registration and its TXT rows for good. The CNAME
// pointing at it stops resolving afterwards, so the UI confirms this first.
func (d *acmedb) DeleteDomain(subdomain string) error {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	tx, err := d.DB.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
			return
		}
		_ = tx.Commit()
	}()

	delRecords := `DELETE FROM records WHERE Subdomain=$1`
	delTxt := `DELETE FROM txt WHERE Subdomain=$1`
	if Config.Database.Engine == "sqlite3" {
		delRecords = getSQLiteStmt(delRecords)
		delTxt = getSQLiteStmt(delTxt)
	}
	var res sql.Result
	res, err = tx.Exec(delRecords, subdomain)
	if err != nil {
		return err
	}
	affected, aerr := res.RowsAffected()
	if aerr == nil && affected == 0 {
		err = errNoSuchDomain
		return err
	}
	_, err = tx.Exec(delTxt, subdomain)
	return err
}

// RotatePassword issues a fresh API password for an existing subdomain and
// returns the new plaintext. The subdomain and therefore the customer CNAME
// stays valid; only the client credentials have to be updated.
func (d *acmedb) RotatePassword(subdomain string) (string, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()

	password := generatePassword(40)
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		return "", err
	}
	updSQL := `UPDATE records SET Password=$1, EncPassword=$2, UpdatedAt=$3 WHERE Subdomain=$4`
	if Config.Database.Engine == "sqlite3" {
		updSQL = getSQLiteStmt(updSQL)
	}
	res, err := d.DB.Exec(updSQL, string(passwordHash), encryptCredential(password), time.Now().Unix(), subdomain)
	if err != nil {
		return "", err
	}
	affected, err := res.RowsAffected()
	if err == nil && affected == 0 {
		return "", errNoSuchDomain
	}
	return password, nil
}

func (d *acmedb) GetByUsername(u uuid.UUID) (ACMETxt, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	var results []ACMETxt
	getSQL := `
	SELECT Username, Password, Subdomain, AllowFrom
	FROM records
	WHERE Username=$1 LIMIT 1
	`
	if Config.Database.Engine == "sqlite3" {
		getSQL = getSQLiteStmt(getSQL)
	}

	sm, err := d.DB.Prepare(getSQL)
	if err != nil {
		return ACMETxt{}, err
	}
	defer sm.Close()
	rows, err := sm.Query(u.String())
	if err != nil {
		return ACMETxt{}, err
	}
	defer rows.Close()

	// It will only be one row though
	for rows.Next() {
		txt, err := getModelFromRow(rows)
		if err != nil {
			return ACMETxt{}, err
		}
		results = append(results, txt)
	}
	if len(results) > 0 {
		return results[0], nil
	}
	return ACMETxt{}, errors.New("no user")
}

func (d *acmedb) GetTXTForDomain(domain string) ([]string, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	domain = sanitizeString(domain)
	var txts []string
	getSQL := `
	SELECT Value FROM txt WHERE Subdomain=$1 LIMIT 2
	`
	if Config.Database.Engine == "sqlite3" {
		getSQL = getSQLiteStmt(getSQL)
	}

	sm, err := d.DB.Prepare(getSQL)
	if err != nil {
		return txts, err
	}
	defer sm.Close()
	rows, err := sm.Query(domain)
	if err != nil {
		return txts, err
	}
	defer rows.Close()

	for rows.Next() {
		var rtxt string
		err = rows.Scan(&rtxt)
		if err != nil {
			return txts, err
		}
		txts = append(txts, rtxt)
	}
	return txts, nil
}

func (d *acmedb) Update(a ACMETxtPost) error {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	var err error
	// Data in a is already sanitized
	timenow := time.Now().Unix()

	updSQL := `
	UPDATE txt SET Value=$1, LastUpdate=$2
	WHERE rowid=(
		SELECT rowid FROM txt WHERE Subdomain=$3 ORDER BY LastUpdate LIMIT 1)
	`
	if Config.Database.Engine == "sqlite3" {
		updSQL = getSQLiteStmt(updSQL)
	}

	sm, err := d.DB.Prepare(updSQL)
	if err != nil {
		return err
	}
	defer sm.Close()
	_, err = sm.Exec(a.Value, timenow, a.Subdomain)
	if err != nil {
		return err
	}
	return nil
}

func getModelFromRow(r *sql.Rows) (ACMETxt, error) {
	txt := ACMETxt{}
	afrom := ""
	err := r.Scan(
		&txt.Username,
		&txt.Password,
		&txt.Subdomain,
		&afrom)
	if err != nil {
		log.WithFields(log.Fields{"error": err.Error()}).Error("Row scan error")
	}

	cslice := cidrslice{}
	err = json.Unmarshal([]byte(afrom), &cslice)
	if err != nil {
		log.WithFields(log.Fields{"error": err.Error()}).Error("JSON unmarshall error")
	}
	txt.AllowFrom = cslice
	return txt, err
}

func (d *acmedb) Close() {
	d.DB.Close()
}

func (d *acmedb) GetBackend() *sql.DB {
	return d.DB
}

func (d *acmedb) SetBackend(backend *sql.DB) {
	d.DB = backend
}
