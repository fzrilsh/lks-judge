package logfile

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRotatesOnDateChange(t *testing.T) {
	dir := t.TempDir()
	r := New(dir)
	defer func() { _ = r.Close() }()

	if _, err := r.Write([]byte("first\n")); err != nil {
		t.Fatalf("write 1: %v", err)
	}
	today := time.Now().Format("2006-01-02")

	// Force the rotator to believe it is still on an older day, so the next
	// Write reopens under today's name only if that name differs. Instead we
	// simulate the day boundary by stamping a stale day and a matching file.
	r.mu.Lock()
	stale := "2000-01-01"
	_ = r.file.Close()
	f, err := os.OpenFile(filepath.Join(dir, stale+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		r.mu.Unlock()
		t.Fatalf("open stale: %v", err)
	}
	r.file, r.day = f, stale
	r.mu.Unlock()

	if _, err := r.Write([]byte("second\n")); err != nil {
		t.Fatalf("write 2: %v", err)
	}

	for _, name := range []string{stale + ".log", today + ".log"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected %s: %v", name, err)
		}
	}
}
