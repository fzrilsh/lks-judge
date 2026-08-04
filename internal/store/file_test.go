package store

import (
	"errors"
	"testing"

	"github.com/fzrilsh/lks-judge/internal/model"
)

func TestFileRoundtrip(t *testing.T) {
	s, compID := newTestStore(t)

	f := &model.File{ID: "f1", CompetitionID: compID, Name: "brief.pdf", Path: "/data/files/1/f1-brief.pdf"}
	if err := s.CreateFile(f); err != nil {
		t.Fatalf("create file: %v", err)
	}

	got, err := s.GetFileByID("f1")
	if err != nil {
		t.Fatalf("get file: %v", err)
	}
	if got.Name != "brief.pdf" || got.Path != f.Path || got.IsPublic {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}

	list, err := s.ListFiles(compID)
	if err != nil {
		t.Fatalf("list files: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("want 1 file, got %d", len(list))
	}
}

func TestToggleFilePublic(t *testing.T) {
	s, compID := newTestStore(t)
	if err := s.CreateFile(&model.File{ID: "f1", CompetitionID: compID, Name: "a", Path: "p"}); err != nil {
		t.Fatalf("create file: %v", err)
	}

	on, err := s.ToggleFilePublic("f1")
	if err != nil {
		t.Fatalf("toggle on: %v", err)
	}
	if !on.IsPublic {
		t.Fatal("want is_public true after first toggle")
	}

	off, err := s.ToggleFilePublic("f1")
	if err != nil {
		t.Fatalf("toggle off: %v", err)
	}
	if off.IsPublic {
		t.Fatal("want is_public false after second toggle")
	}

	if _, err := s.ToggleFilePublic("nope"); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("want ErrFileNotFound, got %v", err)
	}
}

func TestDeleteFileReturnsPath(t *testing.T) {
	s, compID := newTestStore(t)
	if err := s.CreateFile(&model.File{
		ID: "f1", CompetitionID: compID, Name: "a", Path: "/data/files/1/f1-a",
	}); err != nil {
		t.Fatalf("create file: %v", err)
	}

	path, err := s.DeleteFile("f1")
	if err != nil {
		t.Fatalf("delete file: %v", err)
	}
	if path != "/data/files/1/f1-a" {
		t.Fatalf("want path back, got %q", path)
	}
	if _, err := s.GetFileByID("f1"); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("want ErrFileNotFound after delete, got %v", err)
	}
	if _, err := s.DeleteFile("f1"); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("want ErrFileNotFound on second delete, got %v", err)
	}
}
