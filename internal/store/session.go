package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/fzrilsh/lks-judge/internal/model"
)

// ErrSessionNotFound means the token has no matching sessions row. Expected for
// stale/absent cookies, so callers may skip logging it.
var ErrSessionNotFound = errors.New("session not found")

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
	participant, err := s.GetParticipantByID(participantID)
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
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query session: %w", err)
	}

	if time.Now().After(expiresAt) {
		return nil, fmt.Errorf("session expired")
	}

	// load participant
	participant, err := s.GetParticipantByID(ownerID)
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

// invalidateParticipant evicts every cached session belonging to a participant,
// forcing the next ValidateSession to reload from DB. Called after any write
// that changes a participant (edit, seat shuffle, delete) so the cache cannot
// serve stale pc_number/school/password.
func invalidateParticipant(ids ...int64) {
	want := make(map[int64]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	sessionCache.Range(func(k, v any) bool {
		if p, ok := v.(*model.Participant); ok && want[p.ID] {
			sessionCache.Delete(k)
		}
		return true
	})
}

// generateToken creates a 32-byte random hex string.
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
