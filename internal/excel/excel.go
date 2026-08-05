package excel

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"runtime"
	"strconv"
	"strings"

	"github.com/fzrilsh/lks-judge/internal/store"
	"github.com/xuri/excelize/v2"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/sync/errgroup"
)

// ImportedParticipant is returned per row after a successful import.
type ImportedParticipant struct {
	Name     string
	PCNumber *int
	Password string // plain-text, empty if participant already existed
}

// ImportParticipants reads an Excel file and upserts participants + scores.
// Header row: no_pc, ip_address, member, name, [module_cols...]
func ImportParticipants(st *store.Store, data []byte) ([]ImportedParticipant, error) {
	comp := st.CompetitionCache.Load()
	if comp == nil {
		return nil, fmt.Errorf("no competition configured")
	}

	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("open xlsx: %w", err)
	}
	defer func() { _ = f.Close() }()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("xlsx has no sheets")
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("get rows: %w", err)
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("xlsx has no data rows")
	}

	// parse header
	header := rows[0]
	fixed := map[string]int{"no_pc": -1, "ip_address": -1, "member": -1, "name": -1}
	var moduleCols []struct {
		idx  int
		name string
	}
	for i, h := range header {
		key := strings.ToLower(strings.TrimSpace(h))
		if _, ok := fixed[key]; ok {
			fixed[key] = i
		} else if key != "" {
			moduleCols = append(moduleCols, struct {
				idx  int
				name string
			}{i, strings.TrimSpace(h)})
		}
	}
	if fixed["name"] == -1 {
		return nil, fmt.Errorf("xlsx missing 'name' column")
	}

	// ensure module rows exist
	moduleIDs := make(map[string]int64, len(moduleCols))
	for _, mc := range moduleCols {
		id, err := st.UpsertModuleByName(comp.ID, mc.name)
		if err != nil {
			return nil, fmt.Errorf("upsert module %s: %w", mc.name, err)
		}
		moduleIDs[mc.name] = id
	}

	type rowData struct {
		name      string
		school    string
		pcNumber  *int
		ipAddress *string
		scores    map[string]*int // module name → raw score
	}

	dataRows := rows[1:]
	parsed := make([]rowData, 0, len(dataRows))
	for _, row := range dataRows {
		cell := func(idx int) string {
			if idx < 0 || idx >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[idx])
		}
		name := cell(fixed["name"])
		if name == "" {
			continue
		}
		rd := rowData{
			name:   name,
			school: cell(fixed["member"]),
			scores: map[string]*int{},
		}
		if ip := cell(fixed["ip_address"]); ip != "" {
			rd.ipAddress = &ip
		}
		if pcStr := cell(fixed["no_pc"]); pcStr != "" {
			if n, err := strconv.Atoi(pcStr); err == nil {
				rd.pcNumber = &n
			}
		}
		for _, mc := range moduleCols {
			if v := cell(mc.idx); v != "" {
				if n, err := strconv.Atoi(v); err == nil {
					rd.scores[mc.name] = &n
				}
			}
		}
		parsed = append(parsed, rd)
	}

	// generate passwords in parallel (bcrypt is CPU-bound)
	type hashResult struct {
		plain string
		hash  []byte
	}
	hashes := make([]hashResult, len(parsed))
	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(runtime.NumCPU())
	for i := range parsed {
		i := i
		g.Go(func() error {
			plain, err := RandomPassword()
			if err != nil {
				return err
			}
			h, err := bcrypt.GenerateFromPassword([]byte(plain), 8)
			if err != nil {
				return err
			}
			hashes[i] = hashResult{plain: plain, hash: h}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("bcrypt: %w", err)
	}

	results := make([]ImportedParticipant, 0, len(parsed))
	for i, rd := range parsed {
		id, plain, err := st.UpsertParticipantByName(
			comp.ID, rd.name, rd.school, rd.pcNumber, rd.ipAddress,
			string(hashes[i].hash), hashes[i].plain,
		)
		if err != nil {
			return nil, fmt.Errorf("upsert %s: %w", rd.name, err)
		}
		for modName, score := range rd.scores {
			modID := moduleIDs[modName]
			if err := st.UpsertScore(id, modID, score); err != nil {
				return nil, fmt.Errorf("upsert score %s/%s: %w", rd.name, modName, err)
			}
		}
		results = append(results, ImportedParticipant{
			Name:     rd.name,
			PCNumber: rd.pcNumber,
			Password: plain,
		})
	}
	return results, nil
}

// ExportParticipants writes participant list + scores to an xlsx and returns the bytes.
func ExportParticipants(st *store.Store) ([]byte, error) {
	comp := st.CompetitionCache.Load()
	if comp == nil {
		return nil, fmt.Errorf("no competition configured")
	}

	participants, err := st.ListParticipants(comp.ID)
	if err != nil {
		return nil, err
	}
	modules, err := st.ListModules(comp.ID)
	if err != nil {
		return nil, err
	}

	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	sheet := "Sheet1"

	// header
	header := []string{"NO PC", "IP_ADDRESS", "MEMBER", "NAME", "PASSWORD"}
	for _, m := range modules {
		header = append(header, m.Name)
	}
	for col, h := range header {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		if err := f.SetCellValue(sheet, cell, h); err != nil {
			return nil, fmt.Errorf("write header cell %s: %w", cell, err)
		}
	}

	for row, p := range participants {
		rowNum := row + 2
		pcStr := ""
		if p.PCNumber != nil {
			pcStr = fmt.Sprintf("%02d", *p.PCNumber)
		}
		ipStr := ""
		if p.IPAddress != nil {
			ipStr = *p.IPAddress
		}
		vals := []string{pcStr, ipStr, p.School, p.Name, p.PlainPassword}

		_ = modules // module scores not fetched here — ponytail: add score join when Phase 11 adds score queries
		for _, m := range modules {
			_ = m
			vals = append(vals, "")
		}
		for col, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(col+1, rowNum)
			if err := f.SetCellValue(sheet, cell, v); err != nil {
				return nil, fmt.Errorf("write cell %s: %w", cell, err)
			}
		}
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("write xlsx: %w", err)
	}
	return buf.Bytes(), nil
}

// RandomPassword generates an 8-character password from an unambiguous
// alphanumeric alphabet. Longer and larger keyspace than the old 6-digit numeric
// (~47 bits vs ~20), which was brute-forceable in minutes on a LAN.
func RandomPassword() (string, error) {
	// No 0/O/1/I/l to keep hand-typed passwords unambiguous.
	const alphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz"
	b := make([]byte, 8)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		b[i] = alphabet[n.Int64()]
	}
	return string(b), nil
}

// ponytail: ExportParticipants score cells are empty — add score JOIN in Phase 11 when store/submission.go has ListScoresByCompetition.
