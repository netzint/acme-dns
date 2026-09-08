package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/glebarez/go-sqlite"
	_ "github.com/lib/pq"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/joohoi/acme-dns/pkg/acmedns"
)

type acmednsdb struct {
	DB     *sql.DB
	Mutex  sync.RWMutex
	Logger *zap.SugaredLogger
	Config *acmedns.AcmeDnsConfig
	// cipher encrypts the recoverable copies of the generated API passwords.
	// It is nil when auth.credentials_key is not configured, in which case only
	// bcrypt hashes are stored and passwords cannot be shown again.
	cipher *acmedns.CredentialCipher
}

// DBVersion shows the database version this code uses. This is used for update checks.
var DBVersion = 4

// dbDefaultTimeout bounds every database operation. Without it, a single stalled
// query (e.g. a Postgres connection silently dropped by a stateful firewall, or a
// locked SQLite file) blocks forever while holding the shared lock, freezing the
// DNS hot path and leaking a goroutine per query until the process is OOM-killed.
const dbDefaultTimeout = 20 * time.Second

// dbReadTimeout bounds the DNS hot-path read (GetTXTForDomain). It is deliberately
// much shorter than dbDefaultTimeout: a DNS resolver abandons the query within a
// few seconds, so a 20s ceiling only lets a stalled connection pin a worker long
// after the client has given up. Failing fast frees the connection and prevents
// the goroutine pileup that bounded-but-still-slow reads would otherwise cause.
const dbReadTimeout = 5 * time.Second

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
		EncPassword TEXT DEFAULT '',
		LastIP TEXT DEFAULT ''
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

// txtIndex backs the DNS hot-path lookup (SELECT ... FROM txt WHERE Subdomain=$1).
// Without it every DNS query is a full table scan whose latency grows with the
// number of registered accounts, eventually crossing the read timeout under load.
// CREATE INDEX IF NOT EXISTS is idempotent and valid for both SQLite and
// PostgreSQL, so existing deployments gain the index on the next startup with no
// explicit migration step.
var txtIndex = `
	CREATE INDEX IF NOT EXISTS idx_txt_subdomain ON txt (Subdomain);`

// getSQLiteStmt replaces all PostgreSQL prepared statement placeholders (eg. $1, $2) with SQLite variant "?"
func getSQLiteStmt(s string) string {
	re, _ := regexp.Compile(`\$[0-9]`)
	return re.ReplaceAllString(s, "?")
}

func Init(config *acmedns.AcmeDnsConfig, logger *zap.SugaredLogger) (acmedns.AcmednsDB, error) {
	var d = &acmednsdb{Config: config, Logger: logger}
	d.Mutex.Lock()
	defer d.Mutex.Unlock()

	if config.Database.Engine == "sqlite3" {
		logger.Warn("DB: sqlite3 engine has been replaced by the sqlite engine. Please update your config")
		config.Database.Engine = "sqlite"
	}

	if config.Auth.CredentialsKey != "" {
		cipher, cerr := acmedns.NewCredentialCipher(config.Auth.CredentialsKey)
		if cerr != nil {
			return d, cerr
		}
		d.cipher = cipher
		logger.Info("DB: credential storage enabled, generated passwords are recoverable through the management UI")
	}

	db, err := sql.Open(config.Database.Engine, config.Database.Connection)
	if err != nil {
		return d, err
	}
	d.DB = db
	// Bound the connection pool. SQLite (file or :memory:) is not safe for
	// concurrent writers, so pin it to a single connection — this serializes
	// access at the pool level, which is the job the global mutex used to do.
	// For other engines, recycle connections so a silently-dropped TCP
	// connection is never reused for a query that would then block forever.
	if config.Database.Engine == "sqlite" {
		d.DB.SetMaxOpenConns(1)
	} else {
		d.DB.SetMaxOpenConns(10)
		d.DB.SetMaxIdleConns(2)
		d.DB.SetConnMaxLifetime(5 * time.Minute)
		// Recycle idle connections quickly so a silently-dropped TCP connection is
		// retired within a minute instead of lingering until ConnMaxLifetime and
		// stalling the next query that borrows it.
		d.DB.SetConnMaxIdleTime(1 * time.Minute)
	}
	// Check version first to try to catch old versions without version string
	var versionString string
	_ = d.DB.QueryRow("SELECT Value FROM acmedns WHERE Name='db_version'").Scan(&versionString)
	if versionString == "" {
		versionString = "0"
	}
	_, _ = d.DB.Exec(acmeTable)
	_, _ = d.DB.Exec(userTable)
	if config.Database.Engine == "sqlite" {
		_, _ = d.DB.Exec(txtTable)
	} else {
		_, _ = d.DB.Exec(txtTablePG)
	}
	_, _ = d.DB.Exec(txtIndex)
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
	return d, err
}

func (d *acmednsdb) checkDBUpgrades(versionString string) error {
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

func (d *acmednsdb) handleDBUpgrades(version int) error {
	if version == 0 {
		if err := d.handleDBUpgradeTo1(); err != nil {
			return err
		}
		version = 1
	}
	if version == 1 {
		if err := d.handleDBUpgradeTo2(); err != nil {
			return err
		}
		version = 2
	}
	if version == 2 {
		if err := d.handleDBUpgradeTo3(); err != nil {
			return err
		}
		version = 3
	}
	if version == 3 {
		return d.handleDBUpgradeTo4()
	}
	return nil
}

// handleDBUpgradeTo4 adds the column recording where the last /update came from.
// Existing rows stay empty until their client renews the next certificate.
func (d *acmednsdb) handleDBUpgradeTo4() error {
	d.Logger.Info("Upgrading database to version 4: Adding LastIP column")
	columns := []struct {
		name string
		ddl  string
	}{
		{"LastIP", "ALTER TABLE records ADD COLUMN LastIP TEXT DEFAULT ''"},
	}
	if err := d.addColumns(columns, "4"); err != nil {
		return err
	}
	d.Logger.Info("Database upgraded to version 4 successfully")
	return nil
}

// handleDBUpgradeTo2 adds the columns the management UI needs to label a
// registration and show when it was created or last touched.
func (d *acmednsdb) handleDBUpgradeTo2() error {
	d.Logger.Info("Upgrading database to version 2: Adding DomainName, CreatedAt, UpdatedAt columns")
	columns := []struct {
		name string
		ddl  string
	}{
		{"DomainName", "ALTER TABLE records ADD COLUMN DomainName TEXT DEFAULT ''"},
		{"CreatedAt", "ALTER TABLE records ADD COLUMN CreatedAt INT DEFAULT 0"},
		{"UpdatedAt", "ALTER TABLE records ADD COLUMN UpdatedAt INT DEFAULT 0"},
	}
	if err := d.addColumns(columns, "2"); err != nil {
		return err
	}
	d.Logger.Info("Database upgraded to version 2 successfully")
	return nil
}

// handleDBUpgradeTo3 adds the column holding the encrypted, recoverable copy of
// the generated API password. Records created before this upgrade keep an empty
// value and have to be rotated before the UI can show their credentials again.
func (d *acmednsdb) handleDBUpgradeTo3() error {
	d.Logger.Info("Upgrading database to version 3: Adding EncPassword column")
	columns := []struct {
		name string
		ddl  string
	}{
		{"EncPassword", "ALTER TABLE records ADD COLUMN EncPassword TEXT DEFAULT ''"},
	}
	if err := d.addColumns(columns, "3"); err != nil {
		return err
	}
	d.Logger.Info("Database upgraded to version 3 successfully")
	return nil
}

// addColumns applies additive schema changes and records the new version. SQLite
// has no ADD COLUMN IF NOT EXISTS, so existing columns are detected first and a
// duplicate column error is tolerated as a no-op either way.
func (d *acmednsdb) addColumns(columns []struct {
	name string
	ddl  string
}, newVersion string) error {
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

	for _, column := range columns {
		if d.Config.Database.Engine == "sqlite" {
			var count int
			cerr := tx.QueryRow(getSQLiteStmt("SELECT COUNT(*) FROM pragma_table_info('records') WHERE name=$1"), column.name).Scan(&count)
			if cerr == nil && count > 0 {
				continue
			}
			if _, aerr := tx.Exec(column.ddl); aerr != nil && !strings.Contains(aerr.Error(), "duplicate column") {
				d.Logger.Errorw("Error adding column",
					"column", column.name,
					"error", aerr.Error())
				err = aerr
				return err
			}
			continue
		}
		if _, aerr := tx.Exec(strings.Replace(column.ddl, "ADD COLUMN", "ADD COLUMN IF NOT EXISTS", 1)); aerr != nil {
			d.Logger.Errorw("Error adding column",
				"column", column.name,
				"error", aerr.Error())
			err = aerr
			return err
		}
	}

	versionSQL := "UPDATE acmedns SET Value=$1 WHERE Name='db_version'"
	if d.Config.Database.Engine == "sqlite" {
		versionSQL = getSQLiteStmt(versionSQL)
	}
	_, err = tx.Exec(versionSQL, newVersion)
	if err != nil {
		d.Logger.Errorw("Error updating database version",
			"error", err.Error())
	}
	return err
}

func (d *acmednsdb) handleDBUpgradeTo1() error {
	var err error
	var subdomains []string
	rows, err := d.DB.Query("SELECT Subdomain FROM records")
	if err != nil {
		d.Logger.Errorw("Error in DB upgrade",
			"error", err.Error())
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var subdomain string
		err = rows.Scan(&subdomain)
		if err != nil {
			d.Logger.Errorw("Error in DB upgrade while reading values",
				"error", err.Error())
			return err
		}
		subdomains = append(subdomains, subdomain)
	}
	err = rows.Err()
	if err != nil {
		d.Logger.Errorw("Error in DB upgrade while inserting values",
			"error", err.Error())
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
				d.Logger.Errorw("Error in DB upgrade while inserting values",
					"error", err.Error())
				return err
			}
		}
	}
	// SQLite doesn't support dropping columns
	if d.Config.Database.Engine != "sqlite" {
		_, _ = tx.Exec("ALTER TABLE records DROP COLUMN IF EXISTS Value")
		_, _ = tx.Exec("ALTER TABLE records DROP COLUMN IF EXISTS LastActive")
	}
	_, err = tx.Exec("UPDATE acmedns SET Value='1' WHERE Name='db_version'")
	return err
}

// NewTXTValuesInTransaction creates two rows for subdomain to the txt table
func (d *acmednsdb) NewTXTValuesInTransaction(tx *sql.Tx, subdomain string) error {
	var err error
	instr := fmt.Sprintf("INSERT INTO txt (Subdomain, LastUpdate) values('%s', 0)", subdomain)
	_, _ = tx.Exec(instr)
	_, _ = tx.Exec(instr)
	return err
}

func (d *acmednsdb) Register(afrom acmedns.Cidrslice) (acmedns.ACMETxt, error) {
	return d.RegisterWithName(afrom, "")
}

// RegisterWithName creates a registration and labels it with the customer domain
// the management UI created it for. When credential storage is enabled it also
// keeps an encrypted copy of the generated password so the UI can show it again.
func (d *acmednsdb) RegisterWithName(afrom acmedns.Cidrslice, domainName string) (acmedns.ACMETxt, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), dbDefaultTimeout)
	defer cancel()
	var err error
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return acmedns.ACMETxt{}, err
	}
	// Rollback if errored, commit if not
	defer func() {
		if err != nil {
			_ = tx.Rollback()
			return
		}
		_ = tx.Commit()
	}()
	a := acmedns.NewACMETxt()
	a.AllowFrom = acmedns.Cidrslice(afrom.ValidEntries())
	a.DomainName = domainName
	a.CreatedAt = time.Now().Unix()
	a.UpdatedAt = a.CreatedAt
	a.Fulldomain = a.Subdomain + "." + d.Config.General.Domain
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(a.Password), 10)
	if err != nil {
		return a, err
	}
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
	if d.Config.Database.Engine == "sqlite" {
		regSQL = getSQLiteStmt(regSQL)
	}
	sm, err := tx.PrepareContext(ctx, regSQL)
	if err != nil {
		d.Logger.Errorw("Database error in prepare",
			"error", err.Error())
		return a, fmt.Errorf("failed to prepare registration statement: %w", err)
	}
	defer sm.Close()
	_, err = sm.ExecContext(ctx, a.Username.String(), passwordHash, a.Subdomain, a.AllowFrom.JSON(),
		a.DomainName, a.CreatedAt, a.UpdatedAt, d.cipher.EncryptOrEmpty(a.Password))
	if err == nil {
		err = d.NewTXTValuesInTransaction(tx, a.Subdomain)
	}
	return a, err
}

func (d *acmednsdb) GetByUsername(u uuid.UUID) (acmedns.ACMETxt, error) {
	d.Mutex.RLock()
	defer d.Mutex.RUnlock()
	ctx, cancel := context.WithTimeout(context.Background(), dbDefaultTimeout)
	defer cancel()
	var results []acmedns.ACMETxt
	getSQL := `
	SELECT Username, Password, Subdomain, AllowFrom
	FROM records
	WHERE Username=$1 LIMIT 1
	`
	if d.Config.Database.Engine == "sqlite" {
		getSQL = getSQLiteStmt(getSQL)
	}

	sm, err := d.DB.PrepareContext(ctx, getSQL)
	if err != nil {
		return acmedns.ACMETxt{}, err
	}
	defer sm.Close()
	rows, err := sm.QueryContext(ctx, u.String())
	if err != nil {
		return acmedns.ACMETxt{}, fmt.Errorf("failed to query user: %w", err)
	}
	defer rows.Close()

	// It will only be one row though
	for rows.Next() {
		txt, err := d.getModelFromRow(rows)
		if err != nil {
			return acmedns.ACMETxt{}, err
		}
		results = append(results, txt)
	}
	if len(results) > 0 {
		return results[0], nil
	}
	return acmedns.ACMETxt{}, fmt.Errorf("user not found: %s", u.String())
}

func (d *acmednsdb) GetTXTForDomain(domain string) ([]string, error) {
	d.Mutex.RLock()
	defer d.Mutex.RUnlock()
	ctx, cancel := context.WithTimeout(context.Background(), dbReadTimeout)
	defer cancel()
	domain = acmedns.SanitizeString(domain)
	var txts []string
	getSQL := `
	SELECT Value FROM txt WHERE Subdomain=$1 LIMIT 2
	`
	if d.Config.Database.Engine == "sqlite" {
		getSQL = getSQLiteStmt(getSQL)
	}

	sm, err := d.DB.PrepareContext(ctx, getSQL)
	if err != nil {
		return txts, err
	}
	defer sm.Close()
	rows, err := sm.QueryContext(ctx, domain)
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

func (d *acmednsdb) Update(a acmedns.ACMETxtPost) error {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), dbDefaultTimeout)
	defer cancel()
	var err error
	// Data in a is already sanitized
	timenow := time.Now().Unix()

	updSQL := `
	UPDATE txt SET Value=$1, LastUpdate=$2
	WHERE rowid=(
		SELECT rowid FROM txt WHERE Subdomain=$3 ORDER BY LastUpdate LIMIT 1)
	`
	if d.Config.Database.Engine == "sqlite" {
		updSQL = getSQLiteStmt(updSQL)
	}

	sm, err := d.DB.PrepareContext(ctx, updSQL)
	if err != nil {
		return err
	}
	defer sm.Close()
	_, err = sm.ExecContext(ctx, a.Value, timenow, a.Subdomain)
	if err != nil {
		return err
	}
	return nil
}

func (d *acmednsdb) getModelFromRow(r *sql.Rows) (acmedns.ACMETxt, error) {
	txt := acmedns.ACMETxt{}
	afrom := ""
	err := r.Scan(
		&txt.Username,
		&txt.Password,
		&txt.Subdomain,
		&afrom)
	if err != nil {
		d.Logger.Errorw("Row scan error",
			"error", err.Error())
	}

	cslice := acmedns.Cidrslice{}
	err = json.Unmarshal([]byte(afrom), &cslice)
	if err != nil {
		d.Logger.Errorw("JSON unmarshall error",
			"error", err.Error())
	}
	txt.AllowFrom = cslice
	return txt, err
}

func (d *acmednsdb) Close() {
	d.DB.Close()
}

func (d *acmednsdb) GetBackend() *sql.DB {
	return d.DB
}

func (d *acmednsdb) SetBackend(backend *sql.DB) {
	d.DB = backend
}

// selectDomainSQL is shared by GetAllDomains and GetBySubdomain. LastActive
// comes from the newest TXT write for the registration, which is what the UI
// shows as "in use since".
const selectDomainSQL = `
	SELECT r.Username, r.Subdomain, r.AllowFrom,
	       COALESCE(r.DomainName, ''),
	       COALESCE(r.CreatedAt, 0),
	       COALESCE(r.UpdatedAt, 0),
	       COALESCE(r.EncPassword, ''),
	       COALESCE((SELECT MAX(t.LastUpdate) FROM txt t WHERE t.Subdomain = r.Subdomain), 0),
	       COALESCE(r.LastIP, '')
	FROM records r
	`

// scanDomain reads one row of selectDomainSQL. The returned Password is the
// decrypted plaintext when credential storage is on and the record was created
// after it was enabled; otherwise it is empty.
func (d *acmednsdb) scanDomain(scan func(...interface{}) error) (acmedns.ACMETxt, error) {
	txt := acmedns.ACMETxt{}
	afrom := ""
	encPassword := ""
	err := scan(&txt.Username, &txt.Subdomain, &afrom,
		&txt.DomainName, &txt.CreatedAt, &txt.UpdatedAt, &encPassword, &txt.LastActive, &txt.LastIP)
	if err != nil {
		return acmedns.ACMETxt{}, err
	}
	cslice := acmedns.Cidrslice{}
	if uerr := json.Unmarshal([]byte(afrom), &cslice); uerr != nil {
		d.Logger.Errorw("JSON unmarshall error",
			"error", uerr.Error())
	}
	txt.AllowFrom = cslice
	// Never hand the bcrypt hash to a caller; give the recoverable copy instead.
	txt.Password = d.cipher.DecryptOrEmpty(encPassword)
	txt.Fulldomain = txt.Subdomain + "." + d.Config.General.Domain
	return txt, nil
}

// GetAllDomains returns every registration for the management UI.
func (d *acmednsdb) GetAllDomains() ([]acmedns.ACMETxt, error) {
	d.Mutex.RLock()
	defer d.Mutex.RUnlock()
	ctx, cancel := context.WithTimeout(context.Background(), dbDefaultTimeout)
	defer cancel()

	var results []acmedns.ACMETxt
	rows, err := d.DB.QueryContext(ctx, selectDomainSQL)
	if err != nil {
		return results, err
	}
	defer rows.Close()
	for rows.Next() {
		txt, serr := d.scanDomain(rows.Scan)
		if serr != nil {
			d.Logger.Errorw("Database error in GetAllDomains",
				"error", serr.Error())
			return results, serr
		}
		results = append(results, txt)
	}
	return results, rows.Err()
}

// GetBySubdomain looks up a single registration by its subdomain.
func (d *acmednsdb) GetBySubdomain(subdomain string) (acmedns.ACMETxt, error) {
	d.Mutex.RLock()
	defer d.Mutex.RUnlock()
	ctx, cancel := context.WithTimeout(context.Background(), dbDefaultTimeout)
	defer cancel()

	getSQL := selectDomainSQL + " WHERE r.Subdomain=$1 LIMIT 1"
	if d.Config.Database.Engine == "sqlite" {
		getSQL = getSQLiteStmt(getSQL)
	}
	txt, err := d.scanDomain(d.DB.QueryRowContext(ctx, getSQL, subdomain).Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return acmedns.ACMETxt{}, acmedns.ErrNoSuchDomain
		}
		return acmedns.ACMETxt{}, err
	}
	return txt, nil
}

// UpdateDomainName changes the human readable label of a registration.
func (d *acmednsdb) UpdateDomainName(subdomain string, domainName string) error {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), dbDefaultTimeout)
	defer cancel()

	updSQL := `UPDATE records SET DomainName=$1, UpdatedAt=$2 WHERE Subdomain=$3`
	if d.Config.Database.Engine == "sqlite" {
		updSQL = getSQLiteStmt(updSQL)
	}
	res, err := d.DB.ExecContext(ctx, updSQL, domainName, time.Now().Unix(), subdomain)
	if err != nil {
		return err
	}
	if affected, aerr := res.RowsAffected(); aerr == nil && affected == 0 {
		return acmedns.ErrNoSuchDomain
	}
	return nil
}

// DeleteDomain removes a registration and its TXT rows for good. The CNAME
// pointing at it stops resolving afterwards, so the UI confirms this first.
func (d *acmednsdb) DeleteDomain(subdomain string) error {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), dbDefaultTimeout)
	defer cancel()

	tx, err := d.DB.BeginTx(ctx, nil)
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
	if d.Config.Database.Engine == "sqlite" {
		delRecords = getSQLiteStmt(delRecords)
		delTxt = getSQLiteStmt(delTxt)
	}
	var res sql.Result
	res, err = tx.ExecContext(ctx, delRecords, subdomain)
	if err != nil {
		return err
	}
	if affected, aerr := res.RowsAffected(); aerr == nil && affected == 0 {
		err = acmedns.ErrNoSuchDomain
		return err
	}
	_, err = tx.ExecContext(ctx, delTxt, subdomain)
	return err
}

// RotatePassword issues a fresh API password for an existing subdomain and
// returns the new plaintext. The subdomain and therefore the customer CNAME
// stays valid; only the client credentials have to be updated.
func (d *acmednsdb) RotatePassword(subdomain string) (string, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), dbDefaultTimeout)
	defer cancel()

	password := acmedns.GeneratePassword(40)
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		return "", err
	}
	updSQL := `UPDATE records SET Password=$1, EncPassword=$2, UpdatedAt=$3 WHERE Subdomain=$4`
	if d.Config.Database.Engine == "sqlite" {
		updSQL = getSQLiteStmt(updSQL)
	}
	res, err := d.DB.ExecContext(ctx, updSQL, string(passwordHash),
		d.cipher.EncryptOrEmpty(password), time.Now().Unix(), subdomain)
	if err != nil {
		return "", err
	}
	if affected, aerr := res.RowsAffected(); aerr == nil && affected == 0 {
		return "", acmedns.ErrNoSuchDomain
	}
	return password, nil
}

// SetLastSource records the address a successful /update came from. It is a
// separate call rather than a parameter on Update so the upstream Update stays
// byte for byte mergeable, and so the DNS check's own writes never overwrite a
// real client's address.
func (d *acmednsdb) SetLastSource(subdomain string, ip string) error {
	if ip == "" {
		return nil
	}
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), dbDefaultTimeout)
	defer cancel()

	updSQL := `UPDATE records SET LastIP=$1 WHERE Subdomain=$2`
	if d.Config.Database.Engine == "sqlite" {
		updSQL = getSQLiteStmt(updSQL)
	}
	_, err := d.DB.ExecContext(ctx, updSQL, ip, subdomain)
	return err
}
