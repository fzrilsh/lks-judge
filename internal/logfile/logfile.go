// Package logfile provides a daily-rotating io.Writer for the process log.
// Each calendar day (local time) writes to {dir}/YYYY-MM-DD.log, appended.
// No size cap or compression: a LAN competition runs for a day, not weeks.
package logfile

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Rotator writes to a per-day log file, reopening when the date changes.
// Safe for concurrent use: log.Logger already serializes, but Write locks too
// so a manual writer cannot race the rotation.
type Rotator struct {
	dir string

	mu   sync.Mutex
	day  string // current file's date, "2006-01-02"
	file *os.File
}

// New returns a Rotator writing under dir. The directory must already exist.
func New(dir string) *Rotator { return &Rotator{dir: dir} }

// Write implements io.Writer, rotating to today's file first.
func (r *Rotator) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	day := time.Now().Format("2006-01-02")
	if day != r.day || r.file == nil {
		if r.file != nil {
			_ = r.file.Close()
		}
		f, err := os.OpenFile(filepath.Join(r.dir, day+".log"),
			os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return 0, err
		}
		r.file, r.day = f, day
	}
	return r.file.Write(p)
}

// Close closes the current file, if any.
func (r *Rotator) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return nil
	}
	err := r.file.Close()
	r.file = nil
	return err
}
