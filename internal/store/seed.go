package store

import (
	"fmt"
	"log"

	"golang.org/x/crypto/bcrypt"
)

// SeedDevData creates a default competition + participant for development.
// Idempotent: skips if competition already exists.
func (s *Store) SeedDevData() error {
	var count int
	err := s.Reader.QueryRow(`SELECT COUNT(*) FROM competitions`).Scan(&count)
	if err != nil {
		return fmt.Errorf("check competitions: %w", err)
	}

	if count > 0 {
		log.Println("dev seed: competitions exist, skipping")
		return nil
	}

	log.Println("dev seed: creating default competition + participant")

	// bcrypt hash for password "123456" with cost=8
	hash, err := bcrypt.GenerateFromPassword([]byte("123456"), 8)
	if err != nil {
		return fmt.Errorf("bcrypt hash: %w", err)
	}

	tx, err := s.Writer.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// insert competition
	_, err = tx.Exec(`
		INSERT INTO competitions (id, name, level, start_date, end_date, status, allowed_ips)
		VALUES (1, 'Dev Competition', 'Provinsi', date('now'), date('now', '+1 day'), 'waiting', '["127.0.0.1","::1"]')
	`)
	if err != nil {
		return fmt.Errorf("insert competition: %w", err)
	}

	// insert participant
	_, err = tx.Exec(`
		INSERT INTO participants (id, competition_id, name, school, pc_number, password)
		VALUES (1, 1, 'Dev Participant', 'Dev School', 1, ?)
	`, string(hash))
	if err != nil {
		return fmt.Errorf("insert participant: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	log.Println("dev seed: created competition_id=1, participant pc_number=1 (password: 123456)")
	return nil
}
