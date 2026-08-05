package upload

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/fzrilsh/lks-judge/internal/model"
	"github.com/fzrilsh/lks-judge/internal/store"
)

func newTestStore(t *testing.T) (*store.Store, int64, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.UpsertCompetition(&model.Competition{
		Name: "Test", Level: "Nasional", AllowedIPs: `["127.0.0.1"]`,
		StartDate: "2026-01-01", EndDate: "2026-01-02", Status: "waiting",
	}); err != nil {
		t.Fatalf("upsert competition: %v", err)
	}
	return s, s.CompetitionCache.Load().ID, dir
}

// juryReq wraps a request with a jury uploader identity, as the middleware would.
func juryReq(r *http.Request) *http.Request {
	return r.WithContext(WithUploader(context.Background(), Uploader{ID: 0, Role: "jury"}))
}

func nowPlus2h() time.Time  { return time.Now().UTC().Add(2 * time.Hour) }
func nowMinus1m() time.Time { return time.Now().UTC().Add(-time.Minute) }

func TestUploadEndToEndFile(t *testing.T) {
	st, _, dir := newTestStore(t)

	// init
	body, _ := json.Marshal(initRequest{
		Filename: "brief.pdf", TotalChunks: 3, TotalSize: 30, UploadType: "file",
	})
	rec := httptest.NewRecorder()
	HandleInitPOST(st, dir)(rec, juryReq(httptest.NewRequest(http.MethodPost, "/upload/init", bytes.NewReader(body))))
	if rec.Code != http.StatusOK {
		t.Fatalf("init: want 200, got %d: %s", rec.Code, rec.Body)
	}
	var initResp struct {
		UploadID string `json:"upload_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &initResp); err != nil {
		t.Fatalf("decode init: %v", err)
	}
	uid := initResp.UploadID

	// chunks 0,1,2
	want := []byte("AAAAAAAAAABBBBBBBBBBCCCCCCCCCC")
	for n, part := range [][]byte{[]byte("AAAAAAAAAA"), []byte("BBBBBBBBBB"), []byte("CCCCCCCCCC")} {
		rec := httptest.NewRecorder()
		req := juryReq(httptest.NewRequest(http.MethodPut, "/x", bytes.NewReader(part)))
		req.SetPathValue("id", uid)
		req.SetPathValue("n", strconv.Itoa(n))
		HandleChunkPUT(st, dir)(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("chunk %d: want 200, got %d: %s", n, rec.Code, rec.Body)
		}
	}

	// status
	rec = httptest.NewRecorder()
	req := juryReq(httptest.NewRequest(http.MethodGet, "/x", nil))
	req.SetPathValue("id", uid)
	HandleStatusGET(st, dir)(rec, req)
	var stat struct {
		Received []int `json:"received_chunks"`
		Total    int   `json:"total_chunks"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &stat)
	if len(stat.Received) != 3 || stat.Total != 3 {
		t.Fatalf("status: got %+v", stat)
	}

	// complete
	rec = httptest.NewRecorder()
	req = juryReq(httptest.NewRequest(http.MethodPost, "/x", nil))
	req.SetPathValue("id", uid)
	HandleCompletePOST(st, dir, nil)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("complete: want 200, got %d: %s", rec.Code, rec.Body)
	}
	var comp struct {
		FileID string `json:"file_id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &comp)

	f, err := st.GetFileByID(comp.FileID)
	if err != nil {
		t.Fatalf("get file: %v", err)
	}
	got, err := os.ReadFile(f.Path)
	if err != nil {
		t.Fatalf("read assembled: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("assembled bytes differ:\n got %q\nwant %q", got, want)
	}
	if _, err := os.Stat(filepath.Dir(f.Path)); err != nil {
		t.Fatalf("dest dir missing: %v", err)
	}
	// upload session gone after complete
	if _, err := st.GetUploadSession(uid); err == nil {
		t.Fatal("upload session survived complete")
	}
}

func TestCompleteSubmissionStub501(t *testing.T) {
	st, _, dir := newTestStore(t)

	sess := &model.UploadSession{
		ID: "sub1", UploaderID: 7, UploaderRole: "participant",
		CompetitionID: st.CompetitionCache.Load().ID, Filename: "answer.zip",
		TotalChunks: 1, TotalSize: 1, UploadType: "submission",
		ExpiresAt: nowPlus2h(),
	}
	if err := st.CreateUploadSession(sess); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := WriteChunk(dir, "sub1", 0, bytes.NewReader([]byte("x"))); err != nil {
		t.Fatalf("write chunk: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", nil).WithContext(
		WithUploader(context.Background(), Uploader{ID: 7, Role: "participant"}))
	req.SetPathValue("id", "sub1")
	HandleCompletePOST(st, dir, nil)(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("submission complete: want 501, got %d: %s", rec.Code, rec.Body)
	}
}

func TestChunkExpiredSession(t *testing.T) {
	st, compID, dir := newTestStore(t)
	sess := &model.UploadSession{
		ID: "old1", UploaderID: 0, UploaderRole: "jury", CompetitionID: compID,
		Filename: "f", TotalChunks: 1, TotalSize: 1, UploadType: "file",
		ExpiresAt: nowMinus1m(),
	}
	if err := st.CreateUploadSession(sess); err != nil {
		t.Fatalf("create session: %v", err)
	}

	rec := httptest.NewRecorder()
	req := juryReq(httptest.NewRequest(http.MethodPut, "/x", bytes.NewReader([]byte("x"))))
	req.SetPathValue("id", "old1")
	req.SetPathValue("n", "0")
	HandleChunkPUT(st, dir)(rec, req)
	if rec.Code != http.StatusGone {
		t.Fatalf("expired chunk: want 410, got %d: %s", rec.Code, rec.Body)
	}
}
