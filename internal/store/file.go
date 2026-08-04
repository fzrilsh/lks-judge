package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/fzrilsh/lks-judge/internal/model"
)

// ErrFileNotFound is returned when a file ID doesn't exist.
var ErrFileNotFound = errors.New("file not found")

// CreateFile inserts a jury-uploaded file row. ID and Path must already be set.
func (s *Store) CreateFile(f *model.File) error {
	now := time.Now().UTC()
	_, err := s.Writer.Exec(
		`INSERT INTO files(id, competition_id, name, path, is_public, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		f.ID, f.CompetitionID, f.Name, f.Path, f.IsPublic, now, now,
	)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	f.CreatedAt, f.UpdatedAt = now, now
	return nil
}

// GetFileByID returns one file. Returns ErrFileNotFound when it does not exist.
func (s *Store) GetFileByID(id string) (*model.File, error) {
	var f model.File
	err := s.Reader.QueryRow(
		`SELECT id, competition_id, name, path, is_public, created_at, updated_at
		 FROM files WHERE id = ?`, id,
	).Scan(&f.ID, &f.CompetitionID, &f.Name, &f.Path, &f.IsPublic, &f.CreatedAt, &f.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrFileNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get file: %w", err)
	}
	return &f, nil
}

// ListFiles returns all files for a competition, newest first.
func (s *Store) ListFiles(competitionID int64) ([]*model.File, error) {
	rows, err := s.Reader.Query(
		`SELECT id, competition_id, name, path, is_public, created_at, updated_at
		 FROM files WHERE competition_id = ? ORDER BY created_at DESC`, competitionID)
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*model.File
	for rows.Next() {
		var f model.File
		if err := rows.Scan(&f.ID, &f.CompetitionID, &f.Name, &f.Path,
			&f.IsPublic, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan file: %w", err)
		}
		out = append(out, &f)
	}
	return out, rows.Err()
}

// ToggleFilePublic flips is_public and returns the updated row for broadcasting.
func (s *Store) ToggleFilePublic(id string) (*model.File, error) {
	res, err := s.Writer.Exec(
		`UPDATE files SET is_public = NOT is_public, updated_at = ? WHERE id = ?`,
		time.Now().UTC(), id,
	)
	if err != nil {
		return nil, fmt.Errorf("toggle file public: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return nil, ErrFileNotFound
	}
	return s.GetFileByID(id)
}

// DeleteFile removes the row and returns the on-disk path so the caller can unlink it.
func (s *Store) DeleteFile(id string) (string, error) {
	f, err := s.GetFileByID(id)
	if err != nil {
		return "", err
	}
	if _, err := s.Writer.Exec(`DELETE FROM files WHERE id = ?`, id); err != nil {
		return "", fmt.Errorf("delete file: %w", err)
	}
	return f.Path, nil
}
