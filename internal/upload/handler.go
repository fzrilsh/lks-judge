package upload

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/fzrilsh/lks-judge/internal/model"
	"github.com/fzrilsh/lks-judge/internal/realtime"
	"github.com/fzrilsh/lks-judge/internal/store"
)

// Uploader is the authenticated identity attached by the RequireUploader middleware.
// Role is "participant" or "jury"; ID is 0 for jury (matching the DB sentinel).
type Uploader struct {
	ID   int64
	Role string
}

type ctxKey struct{}

// WithUploader returns a context carrying the uploader identity.
func WithUploader(ctx context.Context, u Uploader) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

// UploaderFrom extracts the identity set by WithUploader. ok is false when absent.
func UploaderFrom(ctx context.Context) (Uploader, bool) {
	u, ok := ctx.Value(ctxKey{}).(Uploader)
	return u, ok
}

// newID returns a 16-byte random hex string, matching the session token pattern.
func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

type initRequest struct {
	Filename    string `json:"filename"`
	TotalChunks int    `json:"total_chunks"`
	TotalSize   int64  `json:"total_size"`
	UploadType  string `json:"upload_type"`
	ModuleID    *int64 `json:"module_id"`
}

// HandleInitPOST validates the manifest, inserts an upload_sessions row with a
// 2-hour expiry, and returns {"upload_id": ...}.
func HandleInitPOST(st *store.Store, _ string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := UploaderFrom(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
			return
		}
		comp := st.CompetitionCache.Load()
		if comp == nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "no competition configured"})
			return
		}

		var req initRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		name, err := SafeName(req.Filename)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid filename"})
			return
		}
		if req.TotalChunks < 1 || req.TotalSize < 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "total_chunks and total_size must be positive"})
			return
		}
		if req.TotalSize > MaxUploadSize {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "total_size exceeds limit"})
			return
		}
		// Bound chunk count so a manifest cannot exhaust inodes: at least enough
		// chunks to carry the bytes at the 2 MiB cap, and no more than MaxChunks.
		// The exact assembled size is verified again at Assemble.
		if minChunks := int((req.TotalSize + MaxChunkSize - 1) / MaxChunkSize); req.TotalChunks < minChunks || req.TotalChunks > MaxChunks {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "total_chunks out of range for total_size"})
			return
		}
		if req.UploadType != "file" && req.UploadType != "submission" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "upload_type must be file or submission"})
			return
		}
		// Jury-managed files are jury-only; participants may only submit.
		if req.UploadType == "file" && u.Role != "jury" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "only jury may upload files"})
			return
		}

		id, err := newID()
		if err != nil {
			log.Printf("upload init: id: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		sess := &model.UploadSession{
			ID: id, UploaderID: u.ID, UploaderRole: u.Role, CompetitionID: comp.ID,
			ModuleID: req.ModuleID, Filename: name, TotalChunks: req.TotalChunks,
			TotalSize: req.TotalSize, UploadType: req.UploadType,
			ExpiresAt: time.Now().UTC().Add(2 * time.Hour),
		}
		if err := st.CreateUploadSession(sess); err != nil {
			log.Printf("upload init: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"upload_id": id})
	}
}

// session loads the manifest and enforces ownership + expiry. It writes the
// error response itself and returns ok=false when the caller should stop.
func session(w http.ResponseWriter, r *http.Request, st *store.Store) (*model.UploadSession, bool) {
	u, ok := UploaderFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		return nil, false
	}
	sess, err := st.GetUploadSession(r.PathValue("id"))
	if errors.Is(err, store.ErrUploadSessionNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "upload not found"})
		return nil, false
	}
	if err != nil {
		log.Printf("upload session lookup: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return nil, false
	}
	if sess.UploaderRole != u.Role || sess.UploaderID != u.ID {
		// Same 404 as a missing session: don't confirm the ID to a non-owner.
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "upload not found"})
		return nil, false
	}
	if time.Now().UTC().After(sess.ExpiresAt) {
		writeJSON(w, http.StatusGone, map[string]string{"error": "upload expired"})
		return nil, false
	}
	return sess, true
}

// HandleChunkPUT stages one chunk. No DB write: chunk presence lives on disk.
func HandleChunkPUT(st *store.Store, dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess, ok := session(w, r, st)
		if !ok {
			return
		}
		n, err := strconv.Atoi(r.PathValue("n"))
		if err != nil || n < 0 || n >= sess.TotalChunks {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad chunk index"})
			return
		}
		defer func() { _ = r.Body.Close() }()
		if err := WriteChunk(dataDir, sess.ID, n, r.Body); err != nil {
			if errors.Is(err, ErrChunkTooLarge) {
				writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "chunk too large"})
				return
			}
			log.Printf("write chunk: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

// HandleStatusGET reports which chunks have landed, so a client can resume.
func HandleStatusGET(st *store.Store, dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess, ok := session(w, r, st)
		if !ok {
			return
		}
		got, err := ReceivedChunks(dataDir, sess.ID)
		if err != nil {
			log.Printf("received chunks: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"received_chunks": got,
			"total_chunks":    sess.TotalChunks,
		})
	}
}

// HandleCompletePOST assembles the file and records it. Submissions are stubbed
// 501 until Phase 10 wires store.CreateSubmission.
func HandleCompletePOST(st *store.Store, dataDir string, hub *realtime.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess, ok := session(w, r, st)
		if !ok {
			return
		}
		missing, err := MissingChunks(dataDir, sess.ID, sess.TotalChunks)
		if err != nil {
			log.Printf("missing chunks: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		if len(missing) > 0 {
			writeJSON(w, http.StatusConflict, map[string]any{"missing": missing})
			return
		}

		if sess.UploadType == "submission" {
			// ponytail: submission record needs store.CreateSubmission (Phase 10).
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "submissions not implemented yet"})
			return
		}

		id, err := newID()
		if err != nil {
			log.Printf("complete: id: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		dst := filepath.Join(dataDir, "files", strconv.FormatInt(sess.CompetitionID, 10), id+"-"+sess.Filename)
		if err := Assemble(dataDir, sess.ID, sess.TotalChunks, sess.TotalSize, dst); err != nil {
			log.Printf("assemble: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}

		f := &model.File{ID: id, CompetitionID: sess.CompetitionID, Name: sess.Filename, Path: dst}
		if err := st.CreateFile(f); err != nil {
			log.Printf("create file: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		if err := st.DeleteUploadSession(sess.ID); err != nil {
			log.Printf("delete upload session: %v", err)
		}

		hub.Broadcast(realtime.EvFileListUpdated, map[string]any{
			"id": f.ID, "name": f.Name, "path": f.Path, "is_public": f.IsPublic,
		})
		writeJSON(w, http.StatusOK, map[string]string{"file_id": f.ID})
	}
}
