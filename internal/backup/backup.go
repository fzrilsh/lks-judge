package backup

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const maxBackups = 12

// RunOnce executes VACUUM INTO with a timestamped filename, then prunes to keep last maxBackups.
func RunOnce(dataDir string, writer *sql.DB) error {
	dir := filepath.Join(dataDir, "backups")
	name := fmt.Sprintf("lks-%s.sqlite", time.Now().UTC().Format("20060102-150405"))
	dest := filepath.Join(dir, name)

	if _, err := writer.Exec(fmt.Sprintf("VACUUM INTO '%s'", dest)); err != nil {
		return fmt.Errorf("backup vacuum: %w", err)
	}
	log.Printf("backup: created %s", name)
	return pruneBackups(dir)
}

// Start runs RunOnce every 5 minutes until done is closed. Call as a goroutine from main.
func Start(dataDir string, writer *sql.DB, done <-chan struct{}) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := RunOnce(dataDir, writer); err != nil {
				log.Printf("backup tick: %v", err)
			}
		case <-done:
			return
		}
	}
}

func pruneBackups(dir string) error {
	entries, err := filepath.Glob(filepath.Join(dir, "lks-*.sqlite"))
	if err != nil {
		return err
	}
	sort.Strings(entries) // lexicographic = chronological given format "20060102-150405"
	for len(entries) > maxBackups {
		if err := os.Remove(entries[0]); err != nil {
			log.Printf("backup prune: %v", err)
		}
		entries = entries[1:]
	}
	return nil
}
