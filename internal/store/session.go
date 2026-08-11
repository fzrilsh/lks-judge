package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
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

// DeleteExpiredSessions removes every session whose expires_at is before now and
// evicts each swept token from the cache. The eviction matters: ValidateSession
// serves cache hits without an expiry check (see above), so a DB-only delete
// would leave an expired token valid until the process restarts. Mirrors
// DeleteExpiredUploadSessions. Returns the swept tokens.
func (s *Store) DeleteExpiredSessions(now time.Time) ([]string, error) {
	rows, err := s.Reader.Query(`SELECT token FROM sessions WHERE expires_at < ?`, now)
	if err != nil {
		return nil, fmt.Errorf("select expired sessions: %w", err)
	}
	var tokens []string
	for rows.Next() {
		var tok string
		if err := rows.Scan(&tok); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan expired session: %w", err)
		}
		tokens = append(tokens, tok)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	if len(tokens) == 0 {
		return nil, nil
	}
	if _, err := s.Writer.Exec(`DELETE FROM sessions WHERE expires_at < ?`, now); err != nil {
		return nil, fmt.Errorf("delete expired sessions: %w", err)
	}
	for _, tok := range tokens {
		sessionCache.Delete(tok)
	}
	return tokens, nil
}

// ClearSessionCache evicts every cached token. Used by Reset after the sessions
// table is wiped. Ranges and deletes rather than reassigning the sync.Map, which
// would race with a concurrent Range.
func ClearSessionCache() {
	sessionCache.Range(func(k, _ any) bool {
		sessionCache.Delete(k)
		return true
	})
}

// StartSessionSweep deletes expired sessions every 30 minutes until done is
// closed, bounding memory across a multi-day run. Call as a goroutine from main.
func StartSessionSweep(s *Store, done <-chan struct{}) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			tokens, err := s.DeleteExpiredSessions(time.Now().UTC())
			if err != nil {
				log.Printf("session sweep: %v", err)
				continue
			}
			if len(tokens) > 0 {
				log.Printf("session sweep: removed %d expired session(s)", len(tokens))
			}
		case <-done:
			return
		}
	}
}

// generateToken creates a 32-byte random hex string.
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
