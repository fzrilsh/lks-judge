package store

import (
	"fmt"
	"time"
)

// UpsertScore inserts or replaces a raw score for participant+module.
// wsi_score is set to NULL here — Phase 11 will compute it after all scores are known.
func (s *Store) UpsertScore(participantID, moduleID int64, score *int) error {
	now := time.Now().UTC()
	_, err := s.Writer.Exec(`
		INSERT INTO scores(participant_id, module_id, score, wsi_score, created_at, updated_at)
		VALUES (?, ?, ?, NULL, ?, ?)
		ON CONFLICT(participant_id, module_id) DO UPDATE SET
		    score=excluded.score, updated_at=excluded.updated_at`,
		participantID, moduleID, score, now, now,
	)
	if err != nil {
		return fmt.Errorf("upsert score: %w", err)
	}
	return nil
}
