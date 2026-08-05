package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/fzrilsh/lks-judge/internal/model"
)

// ErrSubmissionNotFound is returned when a submission ID doesn't exist.
var ErrSubmissionNotFound = errors.New("submission not found")

// UpsertSubmission inserts or replaces the submission for a participant+module.
// It honors UNIQUE(participant_id, module_id): a re-submit updates the row in place.
// oldPath is the previous file_path (empty when this is the first submission), so the
// caller can unlink the superseded file from disk.
func (s *Store) UpsertSubmission(sub *model.Submission) (oldPath string, err error) {
	err = s.Writer.QueryRow(
		`SELECT file_path FROM submissions WHERE participant_id = ? AND module_id = ?`,
		sub.ParticipantID, sub.ModuleID,
	).Scan(&oldPath)
	if err != nil && err != sql.ErrNoRows {
		return "", fmt.Errorf("lookup submission: %w", err)
	}

	now := time.Now().UTC()
	_, err = s.Writer.Exec(`
		INSERT INTO submissions(id, participant_id, module_id, name, file_path, submitted_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(participant_id, module_id) DO UPDATE SET
		    id=excluded.id, name=excluded.name, file_path=excluded.file_path,
		    submitted_at=excluded.submitted_at, updated_at=excluded.updated_at`,
		sub.ID, sub.ParticipantID, sub.ModuleID, sub.Name, sub.FilePath, sub.SubmittedAt, now, now,
	)
	if err != nil {
		return "", fmt.Errorf("upsert submission: %w", err)
	}
	sub.CreatedAt, sub.UpdatedAt = now, now
	return oldPath, nil
}

// GetSubmissionByID returns one submission. Returns ErrSubmissionNotFound when absent.
func (s *Store) GetSubmissionByID(id string) (*model.Submission, error) {
	sub, err := scanSubmission(s.Reader.QueryRow(
		`SELECT id, participant_id, module_id, name, file_path, submitted_at, created_at, updated_at
		 FROM submissions WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, ErrSubmissionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get submission: %w", err)
	}
	return sub, nil
}

// GetSubmissionForParticipant returns the submission for one participant+module, or
// ErrSubmissionNotFound. Used by the dashboard to show what was already submitted.
func (s *Store) GetSubmissionForParticipant(participantID, moduleID int64) (*model.Submission, error) {
	sub, err := scanSubmission(s.Reader.QueryRow(
		`SELECT id, participant_id, module_id, name, file_path, submitted_at, created_at, updated_at
		 FROM submissions WHERE participant_id = ? AND module_id = ?`, participantID, moduleID))
	if err == sql.ErrNoRows {
		return nil, ErrSubmissionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get submission for participant: %w", err)
	}
	return sub, nil
}

// ListSubmissions returns every submission in a competition, ordered by participant then module.
// Powers the jury matrix and the ZIP export.
func (s *Store) ListSubmissions(competitionID int64) ([]*model.Submission, error) {
	rows, err := s.Reader.Query(
		`SELECT sub.id, sub.participant_id, sub.module_id, sub.name, sub.file_path,
		        sub.submitted_at, sub.created_at, sub.updated_at
		 FROM submissions sub
		 JOIN participants p ON p.id = sub.participant_id
		 WHERE p.competition_id = ?
		 ORDER BY sub.participant_id, sub.module_id`, competitionID)
	if err != nil {
		return nil, fmt.Errorf("list submissions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*model.Submission
	for rows.Next() {
		sub, err := scanSubmission(rows)
		if err != nil {
			return nil, fmt.Errorf("scan submission: %w", err)
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

// scanSubmission maps one row into a model.Submission, coping with a NULL submitted_at.
func scanSubmission(row interface{ Scan(...any) error }) (*model.Submission, error) {
	var sub model.Submission
	var submittedAt sql.NullTime
	if err := row.Scan(&sub.ID, &sub.ParticipantID, &sub.ModuleID, &sub.Name,
		&sub.FilePath, &submittedAt, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
		return nil, err
	}
	if submittedAt.Valid {
		sub.SubmittedAt = &submittedAt.Time
	}
	return &sub, nil
}

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
