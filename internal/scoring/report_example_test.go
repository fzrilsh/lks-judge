package scoring

import "testing"

// Canonical worked example from "Perhitungan Skor CIS WorldSkills.md" section 5.2:
// raw 32,41,69,73,85 -> median 69, MAD 16 -> WSI 653,665,700,705,720.
func TestReportWorkedExample(t *testing.T) {
	totals := []float64{32, 41, 69, 73, 85}
	es := make([]Entry, len(totals))
	for i, v := range totals {
		es[i] = Entry{ParticipantID: int64(i + 1), TotalRaw: v}
	}
	got := Rank(es)
	byRaw := map[float64]int{}
	for _, e := range got {
		byRaw[e.TotalRaw] = e.WSI
	}
	want := map[float64]int{32: 653, 41: 665, 69: 700, 73: 705, 85: 720}
	for raw, w := range want {
		if byRaw[raw] != w {
			t.Errorf("raw %.0f -> WSI %d, want %d", raw, byRaw[raw], w)
		}
	}
}
