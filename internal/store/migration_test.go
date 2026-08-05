package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// TestMigrate002AddsPlainPasswordToOldDB fences the upgrade path: a DB whose
// participants table predates the plain_password column must gain it on Open,
// not fail every participantCols query with "no such column".
func TestMigrate002AddsPlainPasswordToOldDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "lks.sqlite")

	// Seed an old-schema participants table (no plain_password), then close.
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	if _, err := raw.Exec(`CREATE TABLE participants (
		id INTEGER PRIMARY KEY, competition_id INTEGER, name TEXT, school TEXT,
		pc_number INTEGER, password TEXT, ip_address TEXT,
		created_at DATETIME, updated_at DATETIME)`); err != nil {
		t.Fatalf("seed old table: %v", err)
	}
	_ = raw.Close()

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("open (migrate): %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	var n int
	if err := s.Reader.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('participants') WHERE name = 'plain_password'`,
	).Scan(&n); err != nil {
		t.Fatalf("pragma: %v", err)
	}
	if n != 1 {
		t.Fatalf("plain_password column not added, count = %d", n)
	}
}
