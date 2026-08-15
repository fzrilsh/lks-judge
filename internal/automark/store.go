package automark

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Store is the automark config, encapsulated as a struct so the whole thing can
// be persisted as one JSON blob (auth + groups + grading). The jury edits it via
// the UI (JSON paste or visual builder); it is saved to {data}/automark.json.
type Store struct {
	Config Config `json:"config"`
}

// configPath returns the on-disk location under the data dir.
func configPath(dataDir string) string {
	return filepath.Join(dataDir, "automark.json")
}

// Load reads the saved config. A missing file returns a zero Config (not an
// error): first run has nothing yet, and the UI renders empty fields.
func Load(dataDir string) (*Store, error) {
	raw, err := os.ReadFile(configPath(dataDir))
	if errors.Is(err, os.ErrNotExist) {
		return &Store{}, nil
	}
	if err != nil {
		return nil, err
	}
	var s Store
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Save writes the config atomically (temp file + rename) so a crash mid-write
// never leaves a truncated automark.json.
func Save(dataDir string, s *Store) error {
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	path := configPath(dataDir)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ParseConfig validates a raw JSON config blob (the jury's pasted JSON) and
// returns it. Used by the web layer before saving, so a malformed paste is
// rejected with a clear error instead of persisting garbage.
func ParseConfig(raw []byte) (Config, error) {
	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		return Config{}, err
	}
	return c, nil
}
