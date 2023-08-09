package mapping

import (
	"database/sql"
	"fmt"
	"net/netip"
	"net/url"
	"path/filepath"

	_ "modernc.org/sqlite"
)

var initQueries = []string{
	"PRAGMA journal_mode=WAL",
	"PRAGMA synchronous=NORMAL",
}

type AddrPool interface {
	GetRandom() netip.Addr
}

type SQLiteMapping struct {
	db       *sql.DB
	addrPool AddrPool
}

func New(dbPath string, addrPool AddrPool) (*SQLiteMapping, error) {
	dbURL := url.URL{
		Scheme:   "file",
		Path:     filepath.Join(dbPath, "mapping.db"),
		OmitHost: true,
		RawQuery: url.Values{
			"_pragma": []string{
				"journal_mode(WAL)",
				"synchronous(NORMAL)",
			},
		}.Encode(),
	}
	db, err := sql.Open("sqlite", dbURL.String())
	if err != nil {
		return nil, fmt.Errorf("can't open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("DB ping failed: %w", err)
	}

	return &SQLiteMapping{
		db:       db,
		addrPool: addrPool,
	}, nil
}

func (m *SQLiteMapping) Close() error {
	return m.db.Close()
}
