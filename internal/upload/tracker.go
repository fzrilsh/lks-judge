// Package upload implements chunked, resumable uploads. Chunk presence on the
// filesystem is the only receipt: a chunk PUT never writes to the database, and
// assembly is a single sequential copy followed by one INSERT.
package upload

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// MaxChunkSize caps a single chunk body at 2 MiB.
const MaxChunkSize = 2 << 20

// MaxUploadSize caps a whole assembled upload at 2 GiB, bounding chunk count.
const MaxUploadSize = 2 << 30

// MaxChunks caps the declared chunk count so a manifest cannot exhaust inodes.
// At the 2 MiB minimum effective chunk size this comfortably covers MaxUploadSize.
const MaxChunks = 4096

// ErrChunkTooLarge is returned when a chunk body exceeds MaxChunkSize.
var ErrChunkTooLarge = errors.New("chunk exceeds max size")

// SafeName reduces an untrusted filename to a single path element.
// Returns an error for names that resolve to nothing usable.
func SafeName(name string) (string, error) {
	base := filepath.Base(strings.ReplaceAll(name, `\`, "/"))
	if base == "" || base == "." || base == ".." || base == string(filepath.Separator) {
		return "", fmt.Errorf("unsafe filename %q", name)
	}
	return base, nil
}

// TmpDir returns the chunk staging directory for an upload.
// uploadID is reduced to a single path element, so a traversal attempt cannot
// escape data/uploads_tmp.
func TmpDir(dataDir, uploadID string) string {
	id, err := SafeName(uploadID)
	if err != nil {
		id = "_invalid"
	}
	return filepath.Join(dataDir, "uploads_tmp", id)
}

func chunkPath(dataDir, uploadID string, n int) string {
	return filepath.Join(TmpDir(dataDir, uploadID), "chunk-"+strconv.Itoa(n))
}

// WriteChunk stages chunk n. Writing the same chunk twice is idempotent, so a
// client may retry a chunk it is unsure about.
func WriteChunk(dataDir, uploadID string, n int, r io.Reader) error {
	if n < 0 {
		return fmt.Errorf("chunk index %d out of range", n)
	}
	dir := TmpDir(dataDir, uploadID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir chunk dir: %w", err)
	}

	// Unique temp name per write: two concurrent PUTs of the same chunk index
	// must not share a staging path, or one truncates the other mid-copy and
	// the survivor is silently corrupt.
	f, err := os.CreateTemp(dir, "chunk-"+strconv.Itoa(n)+"-*.part")
	if err != nil {
		return fmt.Errorf("create chunk: %w", err)
	}
	tmp := f.Name()

	// One byte over the cap is enough to prove the body is too large.
	written, err := io.Copy(f, io.LimitReader(r, MaxChunkSize+1))
	cerr := f.Close()
	if err == nil && cerr != nil {
		err = cerr
	}
	if err == nil && written > MaxChunkSize {
		err = ErrChunkTooLarge
	}
	if err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("write chunk %d: %w", n, err)
	}

	if err := os.Rename(tmp, chunkPath(dataDir, uploadID, n)); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("commit chunk %d: %w", n, err)
	}
	return nil
}

// ReceivedChunks lists the chunk indices already staged, ascending by index
// only insofar as the directory listing is sorted; callers treat it as a set.
func ReceivedChunks(dataDir, uploadID string) ([]int, error) {
	entries, err := os.ReadDir(TmpDir(dataDir, uploadID))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read chunk dir: %w", err)
	}

	var out []int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		idx, ok := strings.CutPrefix(e.Name(), "chunk-")
		if !ok {
			continue // .part leftovers and anything else
		}
		n, err := strconv.Atoi(idx)
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	return out, nil
}

// MissingChunks returns the indices in [0,totalChunks) not yet staged.
func MissingChunks(dataDir, uploadID string, totalChunks int) ([]int, error) {
	got, err := ReceivedChunks(dataDir, uploadID)
	if err != nil {
		return nil, err
	}
	have := make(map[int]bool, len(got))
	for _, n := range got {
		have[n] = true
	}
	missing := []int{}
	for n := range totalChunks {
		if !have[n] {
			missing = append(missing, n)
		}
	}
	return missing, nil
}

// Assemble concatenates chunks 0..totalChunks-1 into dst and drops the staging
// directory. It writes to dst+".part" first so a crash never leaves a truncated
// file at the final path. wantSize is the declared total; assembly fails if the
// concatenated bytes do not match, so a client cannot short or overrun the
// manifest.
func Assemble(dataDir, uploadID string, totalChunks int, wantSize int64, dst string) error {
	missing, err := MissingChunks(dataDir, uploadID, totalChunks)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return fmt.Errorf("assemble: %d chunks missing (first %d)", len(missing), missing[0])
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir dest: %w", err)
	}
	tmp := dst + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create dest: %w", err)
	}

	var total int64
	for n := range totalChunks {
		in, err := os.Open(chunkPath(dataDir, uploadID, n))
		if err != nil {
			_ = out.Close()
			_ = os.Remove(tmp)
			return fmt.Errorf("open chunk %d: %w", n, err)
		}
		nc, cerr := io.Copy(out, in)
		_ = in.Close()
		if cerr != nil {
			_ = out.Close()
			_ = os.Remove(tmp)
			return fmt.Errorf("copy chunk %d: %w", n, cerr)
		}
		total += nc
	}
	if total != wantSize {
		_ = out.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("assemble: size mismatch, got %d want %d", total, wantSize)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close dest: %w", err)
	}

	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("commit dest: %w", err)
	}
	return os.RemoveAll(TmpDir(dataDir, uploadID))
}
