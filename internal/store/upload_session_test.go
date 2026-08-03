package store

import (
	"errors"
	"testing"
	"time"

	"github.com/fzrilsh/lks-judge/internal/model"
)

func TestUploadSessionRoundtrip(t *testing.T) {
	s, compID := newTestStore(t)
	exp := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)

	u := &model.UploadSession{
		ID: "u1", UploaderID: 0, UploaderRole: "jury", CompetitionID: compID,
		Filename: "brief.pdf", TotalChunks: 5, TotalSize: 10 << 20,
		UploadType: "file", ExpiresAt: exp,
	}
	if err := s.CreateUploadSession(u); err != nil {
		t.Fatalf("create upload session: %v", err)
	}

	got, err := s.GetUploadSession("u1")
	if err != nil {
		t.Fatalf("get upload session: %v", err)
	}
	if got.TotalChunks != 5 || got.UploadType != "file" || got.ModuleID != nil {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	if !got.ExpiresAt.Equal(exp) {
		t.Fatalf("expires_at mismatch: got %v want %v", got.ExpiresAt, exp)
	}

	if err := s.DeleteUploadSession("u1"); err != nil {
		t.Fatalf("delete upload session: %v", err)
	}
	if _, err := s.GetUploadSession("u1"); !errors.Is(err, ErrUploadSessionNotFound) {
		t.Fatalf("want ErrUploadSessionNotFound, got %v", err)
	}
}

func TestDeleteExpiredUploadSessions(t *testing.T) {
	s, compID := newTestStore(t)
	now := time.Now().UTC()

	mk := func(id string, exp time.Time) {
		t.Helper()
		if err := s.CreateUploadSession(&model.UploadSession{
			ID: id, UploaderRole: "jury", CompetitionID: compID, Filename: "f",
			TotalChunks: 1, TotalSize: 1, UploadType: "file", ExpiresAt: exp,
		}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	mk("stale", now.Add(-time.Minute))
	mk("fresh", now.Add(time.Hour))

	ids, err := s.DeleteExpiredUploadSessions(now)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if len(ids) != 1 || ids[0] != "stale" {
		t.Fatalf("want [stale], got %v", ids)
	}
	if _, err := s.GetUploadSession("stale"); !errors.Is(err, ErrUploadSessionNotFound) {
		t.Fatalf("stale survived sweep: %v", err)
	}
	if _, err := s.GetUploadSession("fresh"); err != nil {
		t.Fatalf("fresh was swept: %v", err)
	}
}
