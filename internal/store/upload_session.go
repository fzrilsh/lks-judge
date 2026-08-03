package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/fzrilsh/lks-judge/internal/model"
)

// ErrUploadSessionNotFound is returned when an upload ID doesn't exist.
var ErrUploadSessionNotFound = errors.New("upload session not found")

// CreateUploadSession inserts the upload manifest. ID and ExpiresAt must already be set.
func (s *Store) CreateUploadSession(u *model.UploadSession) error {
	now := time.Now().UTC()
	_, err := s.Writer.Exec(
		`INSERT INTO upload_sessions(id, uploader_id, uploader_role, competition_id, module_id,
		     filename, total_chunks, total_size, upload_type, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.UploaderID, u.UploaderRole, u.CompetitionID, u.ModuleID,
		u.Filename, u.TotalChunks, u.TotalSize, u.UploadType, u.ExpiresAt, now,
	)
	if err != nil {
		return fmt.Errorf("create upload session: %w", err)
	}
	u.CreatedAt = now
	return nil
}

// GetUploadSession returns one manifest. Returns ErrUploadSessionNotFound when absent.
func (s *Store) GetUploadSession(id string) (*model.UploadSession, error) {
	var u model.UploadSession
	var moduleID sql.NullInt64
	err := s.Reader.QueryRow(
		`SELECT id, uploader_id, uploader_role, competition_id, module_id,
		        filename, total_chunks, total_size, upload_type, expires_at, created_at
		 FROM upload_sessions WHERE id = ?`, id,
	).Scan(&u.ID, &u.UploaderID, &u.UploaderRole, &u.CompetitionID, &moduleID,
		&u.Filename, &u.TotalChunks, &u.TotalSize, &u.UploadType, &u.ExpiresAt, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrUploadSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get upload session: %w", err)
	}
	if moduleID.Valid {
		u.ModuleID = &moduleID.Int64
	}
	return &u, nil
}

// DeleteUploadSession removes one manifest.
func (s *Store) DeleteUploadSession(id string) error {
	if _, err := s.Writer.Exec(`DELETE FROM upload_sessions WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete upload session: %w", err)
	}
	return nil
}

// DeleteExpiredUploadSessions removes manifests past their expiry and returns the deleted
// IDs so the caller can drop the matching tmp chunk directories.
func (s *Store) DeleteExpiredUploadSessions(now time.Time) ([]string, error) {
	rows, err := s.Reader.Query(`SELECT id FROM upload_sessions WHERE expires_at < ?`, now)
	if err != nil {
		return nil, fmt.Errorf("list expired upload sessions: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan expired upload session: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	if len(ids) == 0 {
		return nil, nil
	}
	if _, err := s.Writer.Exec(`DELETE FROM upload_sessions WHERE expires_at < ?`, now); err != nil {
		return nil, fmt.Errorf("delete expired upload sessions: %w", err)
	}
	return ids, nil
}
