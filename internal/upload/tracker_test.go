package upload

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAssembleRoundtrip(t *testing.T) {
	dir := t.TempDir()

	// Five chunks of varying size, the last one short like a real tail chunk.
	var want bytes.Buffer
	for n := range 5 {
		chunk := bytes.Repeat([]byte{byte('a' + n)}, 1000+n*17)
		if n == 4 {
			chunk = chunk[:13]
		}
		want.Write(chunk)
		if err := WriteChunk(dir, "u1", n, bytes.NewReader(chunk)); err != nil {
			t.Fatalf("write chunk %d: %v", n, err)
		}
	}

	got, err := ReceivedChunks(dir, "u1")
	if err != nil {
		t.Fatalf("received chunks: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("want 5 chunks staged, got %v", got)
	}

	dst := filepath.Join(dir, "files", "1", "out.bin")
	if err := Assemble(dir, "u1", 5, dst); err != nil {
		t.Fatalf("assemble: %v", err)
	}

	assembled, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read assembled: %v", err)
	}
	if sha256.Sum256(assembled) != sha256.Sum256(want.Bytes()) {
		t.Fatalf("assembled bytes differ: got %d bytes, want %d", len(assembled), want.Len())
	}
	if _, err := os.Stat(TmpDir(dir, "u1")); !os.IsNotExist(err) {
		t.Fatalf("tmp dir survived assemble: %v", err)
	}
}

func TestAssembleMissingChunk(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []int{0, 2} {
		if err := WriteChunk(dir, "u1", n, bytes.NewReader([]byte("x"))); err != nil {
			t.Fatalf("write chunk %d: %v", n, err)
		}
	}

	missing, err := MissingChunks(dir, "u1", 3)
	if err != nil {
		t.Fatalf("missing chunks: %v", err)
	}
	if len(missing) != 1 || missing[0] != 1 {
		t.Fatalf("want [1] missing, got %v", missing)
	}

	dst := filepath.Join(dir, "out.bin")
	if err := Assemble(dir, "u1", 3, dst); err == nil {
		t.Fatal("want error assembling with a hole")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatal("failed assemble left a destination file behind")
	}
}

func TestWriteChunkRejectsOversize(t *testing.T) {
	dir := t.TempDir()
	body := bytes.Repeat([]byte("z"), MaxChunkSize+1)

	err := WriteChunk(dir, "u1", 0, bytes.NewReader(body))
	if !errors.Is(err, ErrChunkTooLarge) {
		t.Fatalf("want ErrChunkTooLarge, got %v", err)
	}
	got, err := ReceivedChunks(dir, "u1")
	if err != nil {
		t.Fatalf("received chunks: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("oversize chunk was staged anyway: %v", got)
	}
}

func TestSafeNameRejectsTraversal(t *testing.T) {
	for _, bad := range []string{"", ".", "..", "../../etc/passwd", "/", `..\..\win.ini`} {
		if _, err := SafeName(bad); err == nil {
			if n, _ := SafeName(bad); n == bad {
				t.Fatalf("SafeName(%q) passed through unchanged", bad)
			}
		}
	}
	for in, want := range map[string]string{
		"../../etc/passwd": "passwd",
		`..\..\win.ini`:    "win.ini",
		"dir/brief.pdf":    "brief.pdf",
		"brief.pdf":        "brief.pdf",
	} {
		got, err := SafeName(in)
		if err != nil {
			t.Fatalf("SafeName(%q): %v", in, err)
		}
		if got != want {
			t.Fatalf("SafeName(%q) = %q, want %q", in, got, want)
		}
	}
	for _, bad := range []string{"", ".", "..", "/"} {
		if _, err := SafeName(bad); err == nil {
			t.Fatalf("SafeName(%q) should error", bad)
		}
	}
}

// A traversing upload ID must not let chunks land outside data/uploads_tmp.
func TestTmpDirContainsTraversal(t *testing.T) {
	dir := t.TempDir()
	if err := WriteChunk(dir, "../escape", 0, bytes.NewReader([]byte("x"))); err != nil {
		t.Fatalf("write chunk: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "escape")); !os.IsNotExist(err) {
		t.Fatal("chunk escaped uploads_tmp")
	}
	if _, err := os.Stat(filepath.Join(dir, "uploads_tmp", "escape", "chunk-0")); err != nil {
		t.Fatalf("chunk not contained under uploads_tmp: %v", err)
	}
}
