package mapping

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"net/netip"
	"net/url"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const (
	insertRetries           = 20
	cleanupDebounceInterval = 1 * time.Second
)

var (
	initQueries = []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA synchronous=NORMAL`,
		`CREATE TABLE IF NOT EXISTS mapping (
  client_key TEXT NOT NULL,
  domain_name TEXT NOT NULL,
  mapped_addr TEXT NOT NULL,
  expire INTEGER,
  PRIMARY KEY (client_key, domain_name),
  UNIQUE (client_key, mapped_addr)
 ) STRICT`,
		`CREATE INDEX IF NOT EXISTS mapping_expire_idx ON mapping (expire ASC) WHERE expire IS NOT NULL`,
	}

	ErrTooManyAttempts = errors.New("too many failed attempts")
)

type AddrPool interface {
	GetRandom() netip.Addr
}

type SQLiteMapping struct {
	db          *sql.DB
	addrPool    AddrPool
	lastCleanup time.Time
	cleanupMux  sync.RWMutex
}

func New(dbPath string, addrPool AddrPool) (*SQLiteMapping, error) {
	dbURL := url.URL{
		Scheme:   "file",
		Path:     filepath.Join(dbPath, "mapping.db"),
		OmitHost: true,
	}
	db, err := sql.Open("sqlite", dbURL.String())
	if err != nil {
		return nil, fmt.Errorf("can't open database: %w", err)
	}

	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("DB ping failed: %w", err)
	}

	for _, query := range initQueries {
		if _, err = db.Exec(query); err != nil {
			return nil, fmt.Errorf("setup command (%q) error: %w", query, err)
		}
	}

	return &SQLiteMapping{
		db:       db,
		addrPool: addrPool,
	}, nil
}

func (m *SQLiteMapping) EnsureMapping(clientKey, domainName string, ttl time.Duration) (netip.Addr, error) {
	m.cleanup()

	for i := 0; i < insertRetries; i++ {
		addrCandidate := m.addrPool.GetRandom()
		expire := time.Now().Unix() + int64(math.Round(ttl.Seconds()))
		row := m.db.QueryRow(
			`INSERT INTO mapping (client_key, domain_name, mapped_addr, expire)
			VALUES (?, ?, ?, ?)
			ON CONFLICT (client_key, domain_name) DO UPDATE SET expire = ?
			ON CONFLICT (client_key, mapped_addr) DO NOTHING RETURNING mapped_addr`,
			clientKey, domainName, addrCandidate.String(), expire, expire,
		)
		var ipStr string
		if err := row.Scan(&ipStr); err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return netip.Addr{}, fmt.Errorf("upsert query error: %w", err)
		}
		res, err := netip.ParseAddr(ipStr)
		if err != nil {
			return netip.Addr{}, fmt.Errorf("can't parse IP address %q from DB: %w", ipStr, err)
		}

		return res, nil
	}
	return netip.Addr{}, ErrTooManyAttempts
}

func (m *SQLiteMapping) cleanup() {
	m.cleanupMux.RLock()
	lastCleanup := m.lastCleanup
	m.cleanupMux.RUnlock()

	if time.Now().Sub(lastCleanup) > cleanupDebounceInterval {
		m.cleanupMux.Lock()
		defer m.cleanupMux.Unlock()
		if err := m.purgeExpired(); err != nil {
			log.Printf("DB cleanup failed: %v", err)
		}
		m.lastCleanup = time.Now()
	}
}

func (m *SQLiteMapping) purgeExpired() error {
	_, err := m.db.Exec("DELETE FROM mapping WHERE expire < ?", time.Now().Unix())
	return err
}

func (m *SQLiteMapping) Close() error {
	return m.db.Close()
}

func (m *SQLiteMapping) ReverseLookup(clientKey string, addr netip.Addr) (domainName string, ok bool, err error) {
	row := m.db.QueryRow("SELECT domain_name FROM mapping WHERE client_key = ? AND mapped_addr = ? LIMIT 1",
		clientKey, addr.String())
	var res string
	if err := row.Scan(&res); err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, fmt.Errorf("rev lookup query returned error: %w", err)
	}

	return res, true, nil
}
