package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/fzrilsh/lks-judge/internal/model"
)

// sessionCache holds validated tokens → participant objects.
// Reduces DB reads on every request.
var sessionCache sync.Map

// CreateSession generates a 32-byte random token, stores it in DB with lifetime expiry,
// caches the participant object, and returns the token.
func (s *Store) CreateSession(participantID int64) (string, error) {
	token, err := generateToken()
	if err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}

	// expires_at = 9999-12-31 sentinel (lifetime session)
	expiresAt := time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)

	_, err = s.Writer.Exec(
		`INSERT INTO sessions (token, owner_id, expires_at) VALUES (?, ?, ?)`,
		token, participantID, expiresAt,
	)
	if err != nil {
		return "", fmt.Errorf("insert session: %w", err)
	}

	// load participant and cache
	participant, err := s.getParticipantByID(participantID)
	if err != nil {
		return "", fmt.Errorf("load participant: %w", err)
	}

	sessionCache.Store(token, participant)
	return token, nil
}

// ValidateSession checks cache first, falls back to DB + participant load.
// Returns participant or error if token invalid/expired.
func (s *Store) ValidateSession(token string) (*model.Participant, error) {
	// check cache
	if cached, ok := sessionCache.Load(token); ok {
		return cached.(*model.Participant), nil
	}

	// cache miss → DB lookup
	var ownerID int64
	var expiresAt time.Time
	err := s.Reader.QueryRow(
		`SELECT owner_id, expires_at FROM sessions WHERE token = ?`,
		token,
	).Scan(&ownerID, &expiresAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found")
	}
	if err != nil {
		return nil, fmt.Errorf("query session: %w", err)
	}

	if time.Now().After(expiresAt) {
		return nil, fmt.Errorf("session expired")
	}

	// load participant
	participant, err := s.getParticipantByID(ownerID)
	if err != nil {
		return nil, fmt.Errorf("load participant: %w", err)
	}

	// cache and return
	sessionCache.Store(token, participant)
	return participant, nil
}

// DeleteSession removes token from DB and evicts from cache.
func (s *Store) DeleteSession(token string) error {
	_, err := s.Writer.Exec(`DELETE FROM sessions WHERE token = ?`, token)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	sessionCache.Delete(token)
	return nil
}

// generateToken creates a 32-byte random hex string.
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// getParticipantByID loads participant from DB (internal helper).
func (s *Store) getParticipantByID(id int64) (*model.Participant, error) {
	var p model.Participant
	err := s.Reader.QueryRow(
		`SELECT id, competition_id, name, school, pc_number, password, ip_address, created_at, updated_at
		 FROM participants WHERE id = ?`,
		id,
	).Scan(
		&p.ID, &p.CompetitionID, &p.Name, &p.School,
		&p.PCNumber, &p.Password, &p.IPAddress,
		&p.CreatedAt, &p.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("participant not found")
	}
	if err != nil {
		return nil, fmt.Errorf("query participant: %w", err)
	}

	return &p, nil
}
