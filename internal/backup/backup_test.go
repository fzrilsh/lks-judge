package backup

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/fzrilsh/lks-judge/internal/model"
	"github.com/fzrilsh/lks-judge/internal/store"
)

func TestRunOnceWritesSnapshot(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.UpsertCompetition(&model.Competition{
		Name: "Test", Level: "Nasional", AllowedIPs: `["127.0.0.1"]`,
		StartDate: "2026-01-01", EndDate: "2026-01-02", Status: "waiting",
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// RunOnce does not create the backups dir; main.go does. Mirror that here.
	if err := os.MkdirAll(filepath.Join(dir, "backups"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := RunOnce(dir, s.Writer); err != nil {
		t.Fatalf("run once: %v", err)
	}

	snaps, err := filepath.Glob(filepath.Join(dir, "backups", "lks-*.sqlite"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("want 1 snapshot, got %d", len(snaps))
	}

	db, err := sql.Open("sqlite", snaps[0])
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer func() { _ = db.Close() }()
	var name string
	if err := db.QueryRow(`SELECT name FROM competitions LIMIT 1`).Scan(&name); err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if name != "Test" {
		t.Fatalf("snapshot competition = %q", name)
	}
}

func TestPruneBackupsKeepsTwelve(t *testing.T) {
	dir := t.TempDir()
	// 15 fake snapshots with sortable timestamps.
	names := []string{}
	for i := range 15 {
		n := filepath.Join(dir, "lks-20260101-"+pad(i)+".sqlite")
		if err := os.WriteFile(n, []byte("x"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		names = append(names, n)
	}
	if err := pruneBackups(dir); err != nil {
		t.Fatalf("prune: %v", err)
	}
	left, _ := filepath.Glob(filepath.Join(dir, "lks-*.sqlite"))
	if len(left) != maxBackups {
		t.Fatalf("want %d left, got %d", maxBackups, len(left))
	}
	// the 12 lexicographically largest survive: indices 3..14
	for _, n := range names[:3] {
		if _, err := os.Stat(n); !os.IsNotExist(err) {
			t.Fatalf("oldest %s should be pruned", filepath.Base(n))
		}
	}
	for _, n := range names[3:] {
		if _, err := os.Stat(n); err != nil {
			t.Fatalf("newest %s should survive: %v", filepath.Base(n), err)
		}
	}
}

func pad(i int) string {
	return string(rune('0'+i/10)) + string(rune('0'+i%10)) + "0000"
}
