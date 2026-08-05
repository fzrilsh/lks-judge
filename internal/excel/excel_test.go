package excel

import (
	"bytes"
	"testing"

	"github.com/fzrilsh/lks-judge/internal/model"
	"github.com/fzrilsh/lks-judge/internal/store"
	"github.com/xuri/excelize/v2"
)

func newTestStore(t *testing.T) (*store.Store, int64) {
	t.Helper()
	s, err := store.Open(t.TempDir())
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
	return s, s.CompetitionCache.Load().ID
}

// xlsx builds an in-memory workbook from rows (row 0 is the header).
func xlsx(t *testing.T, rows [][]string) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	for r, row := range rows {
		for c, v := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+1)
			if err := f.SetCellValue("Sheet1", cell, v); err != nil {
				t.Fatalf("set cell: %v", err)
			}
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("write xlsx: %v", err)
	}
	return buf.Bytes()
}

func TestImportParticipantsCreatesParticipantsAndModules(t *testing.T) {
	s, compID := newTestStore(t)
	data := xlsx(t, [][]string{
		{"no_pc", "ip_address", "member", "name", "MA"},
		{"1", "10.0.0.1", "SMK 1", "Ana", "80"},
		{"2", "", "SMK 2", "Budi", "90"},
		{"", "", "", "", ""}, // blank name row skipped
	})
	res, err := ImportParticipants(s, data)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("want 2 imported, got %d", len(res))
	}
	if res[0].Password == "" || res[1].Password == "" || res[0].Password == res[1].Password {
		t.Fatalf("passwords empty or identical: %q %q", res[0].Password, res[1].Password)
	}
	mods, err := s.ListModules(compID)
	if err != nil {
		t.Fatalf("list modules: %v", err)
	}
	if len(mods) != 1 || mods[0].Name != "MA" {
		t.Fatalf("want module MA, got %+v", mods)
	}
}

func TestImportParticipantsMissingNameColumn(t *testing.T) {
	s, compID := newTestStore(t)
	data := xlsx(t, [][]string{
		{"no_pc", "member", "MA"},
		{"1", "SMK 1", "80"},
	})
	if _, err := ImportParticipants(s, data); err == nil {
		t.Fatal("want error for missing name column")
	}
	list, _ := s.ListParticipants(compID)
	if len(list) != 0 {
		t.Fatalf("no participants should be written, got %d", len(list))
	}
}

func TestImportParticipantsSecondRunReturnsEmptyPassword(t *testing.T) {
	s, _ := newTestStore(t)
	data := xlsx(t, [][]string{
		{"no_pc", "member", "name"},
		{"1", "SMK 1", "Ana"},
		{"2", "SMK 2", "Budi"},
	})
	if _, err := ImportParticipants(s, data); err != nil {
		t.Fatalf("import 1: %v", err)
	}
	res, err := ImportParticipants(s, data)
	if err != nil {
		t.Fatalf("import 2: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("want 2 rows, got %d", len(res))
	}
	for _, r := range res {
		if r.Password != "" {
			t.Fatalf("second run should return empty password, got %q", r.Password)
		}
	}
}

func TestExportParticipantsHeaderAndRow(t *testing.T) {
	s, compID := newTestStore(t)
	if _, err := s.UpsertModuleByName(compID, "MA"); err != nil {
		t.Fatalf("module: %v", err)
	}
	pc := 1
	ip := "10.0.0.1"
	if _, _, err := s.UpsertParticipantByName(compID, "Ana", "SMK 1", &pc, &ip, "hash", "plainpw1"); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	data, err := ExportParticipants(s)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("open export: %v", err)
	}
	defer func() { _ = f.Close() }()
	rows, err := f.GetRows("Sheet1")
	if err != nil {
		t.Fatalf("rows: %v", err)
	}
	wantHeader := []string{"NO PC", "IP_ADDRESS", "MEMBER", "NAME", "PASSWORD", "MA"}
	if len(rows) < 2 {
		t.Fatalf("want header + 1 row, got %d", len(rows))
	}
	for i, h := range wantHeader {
		if rows[0][i] != h {
			t.Fatalf("header[%d] = %q, want %q", i, rows[0][i], h)
		}
	}
	row := rows[1]
	if row[0] != "01" || row[1] != "10.0.0.1" || row[2] != "SMK 1" || row[3] != "Ana" || row[4] != "plainpw1" {
		t.Fatalf("row = %+v", row)
	}
	// module score cell empty (ponytail marker: score join lands in Phase 11)
	if len(row) > 5 && row[5] != "" {
		t.Fatalf("module score cell should be empty, got %q", row[5])
	}
}

// TestExportThenImportPreservesSeats is the regression fence for Fix 1:
// export then re-import must keep pc_number and not mint NO PC / PASSWORD modules.
func TestExportThenImportPreservesSeats(t *testing.T) {
	s, compID := newTestStore(t)
	pc := 5
	ip := "10.0.0.2"
	if _, _, err := s.UpsertParticipantByName(compID, "Ana", "SMK 1", &pc, &ip, "hash", "plainpw1"); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	data, err := ExportParticipants(s)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if _, err := ImportParticipants(s, data); err != nil {
		t.Fatalf("reimport: %v", err)
	}

	p, err := s.GetParticipantByPCNumber(compID, 5)
	if err != nil {
		t.Fatalf("seat lost after roundtrip: %v", err)
	}
	if p.Name != "Ana" {
		t.Fatalf("wrong participant at seat 5: %q", p.Name)
	}
	mods, err := s.ListModules(compID)
	if err != nil {
		t.Fatalf("list modules: %v", err)
	}
	for _, m := range mods {
		if m.Name == "NO PC" || m.Name == "PASSWORD" || m.Name == "NO_PC" {
			t.Fatalf("phantom module minted: %q", m.Name)
		}
	}
}

func TestRandomPasswordDigitsAndLength(t *testing.T) {
	seen := map[string]bool{}
	for range 200 {
		pw, err := RandomPassword()
		if err != nil {
			t.Fatalf("gen: %v", err)
		}
		if len(pw) != 6 {
			t.Fatalf("len = %d, want 6", len(pw))
		}
		for _, c := range pw {
			if c < '0' || c > '9' {
				t.Fatalf("non-digit char %q in %q", c, pw)
			}
		}
		seen[pw] = true
	}
	if len(seen) < 2 {
		t.Fatal("200 draws all identical")
	}
}
