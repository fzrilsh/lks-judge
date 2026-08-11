package store

import (
	"database/sql"
	_ "embed"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/fzrilsh/lks-judge/internal/model"
	_ "modernc.org/sqlite"
)

//go:embed migrations/001_initial.sql
var migrationSQL string

//go:embed migrations/002_plain_password.sql
var migration002SQL string

// Store holds the dual connection pool.
// writer: serialized single connection (WAL writer).
// reader: read-only pool, up to 16 concurrent connections.
type Store struct {
	Writer           *sql.DB
	Reader           *sql.DB
	CompetitionCache atomic.Pointer[model.Competition]
	// allowedNets is the parsed jury allowlist, refreshed alongside
	// CompetitionCache. Every /jury/* and /upload/* request reads it, so
	// parsing the JSON + IPs once per write beats doing it per request.
	allowedNets atomic.Pointer[[]net.IPNet]
	// extraNets is a non-persisted jury allowlist from the --jury-ip flag. It is
	// checked alongside allowedNets and is untouched by competition writes or
	// Reset, so a fixed operator machine keeps access across a wipe.
	extraNets atomic.Pointer[[]net.IPNet]
}

// pragmaDSN returns DSN params for SQLite pragmas.
// modernc.org/sqlite supports _pragma= DSN params.
func pragmaDSN(base string, readOnly bool) string {
	mode := ""
	if readOnly {
		mode = "&mode=ro"
	}
	return fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)&_pragma=synchronous(NORMAL)&_pragma=cache_size(-32000)&_pragma=temp_store(MEMORY)&_pragma=mmap_size(268435456)%s",
		base, mode,
	)
}

// Open creates the data directory, opens writer + reader pools, and runs migrations.
func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("store: mkdir %s: %w", dataDir, err)
	}

	dbPath := filepath.Join(dataDir, "lks.sqlite")

	writer, err := sql.Open("sqlite", pragmaDSN(dbPath, false))
	if err != nil {
		return nil, fmt.Errorf("store: open writer: %w", err)
	}
	writer.SetMaxOpenConns(1)
	writer.SetMaxIdleConns(1)
	writer.SetConnMaxLifetime(0)

	if err := writer.Ping(); err != nil {
		return nil, fmt.Errorf("store: ping writer: %w", err)
	}

	if err := migrate(writer); err != nil {
		return nil, fmt.Errorf("store: migrate: %w", err)
	}

	reader, err := sql.Open("sqlite", pragmaDSN(dbPath, true))
	if err != nil {
		return nil, fmt.Errorf("store: open reader: %w", err)
	}
	reader.SetMaxOpenConns(16)
	reader.SetMaxIdleConns(16)
	reader.SetConnMaxLifetime(0)

	if err := reader.Ping(); err != nil {
		return nil, fmt.Errorf("store: ping reader: %w", err)
	}

	return &Store{Writer: writer, Reader: reader}, nil
}

// migrate runs the embedded SQL once, guarded by schema_migrations.
func migrate(db *sql.DB) error {
	if _, err := db.Exec(migrationSQL); err != nil {
		return err
	}
	// 002: ADD COLUMN plain_password on DBs created before 001 carried it.
	// SQLite ALTER TABLE has no IF NOT EXISTS, so gate on the column's absence.
	var has bool
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('participants') WHERE name = 'plain_password'`,
	).Scan(&has); err != nil {
		return err
	}
	if !has {
		if _, err := db.Exec(migration002SQL); err != nil {
			return err
		}
	}
	return nil
}

// Close closes both pools.
func (s *Store) Close() error {
	werr := s.Writer.Close()
	rerr := s.Reader.Close()
	if werr != nil {
		return werr
	}
	return rerr
}
