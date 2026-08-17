package scoring

import "testing"

func TestScaleScoreBounds(t *testing.T) {
	if got := ScaleScore(8.0, 8.0, 7.0); got != 700 { // center -> 700
		t.Errorf("center -> %d, want 700", got)
	}
	if got := ScaleScore(-1000, 8.0, 7.0); got != 0 { // clamp low
		t.Errorf("clamp low -> %d, want 0", got)
	}
	if got := ScaleScore(1e9, 8.0, 7.0); got != 1000 { // clamp high
		t.Errorf("clamp high -> %d, want 1000", got)
	}
	if got := ScaleScore(42, 8.0, 0); got != 700 { // degenerate spread=0
		t.Errorf("spread=0 -> %d, want 700", got)
	}
}

func TestMedianMAD(t *testing.T) {
	if got := Median([]float64{3, 1, 2}); got != 2 {
		t.Errorf("odd median -> %v, want 2", got)
	}
	if got := Median([]float64{1, 2, 3, 4}); got != 2.5 {
		t.Errorf("even median -> %v, want 2.5", got)
	}
	if got := Median([]float64{9}); got != 9 {
		t.Errorf("single median -> %v, want 9", got)
	}
	m := Median(fixtureTotals())
	if m != 8.0 {
		t.Fatalf("fixture median -> %v, want 8.0", m)
	}
	if d := MAD(fixtureTotals(), m); d != 7.0 {
		t.Errorf("fixture MAD -> %v, want 7.0", d)
	}
}

func fixtureTotals() []float64 {
	return []float64{
		61.42, 54.33, 39.58, 37.75, 35.50, 28.75, 28.42, 28.40, 19.00, 16.92,
		13.50, 13.05, 12.80, 12.25, 9.50, 8.75, 7.25, 6.00, 6.00, 4.25, 3.25,
		2.75, 2.00, 1.50, 1.40, 1.25, 0.75, 0.50, 0.50, 0.50, 0.25, 0.00,
	}
}

func TestRankFixture(t *testing.T) {
	totals := fixtureTotals()
	entries := make([]Entry, len(totals))
	for i, tot := range totals {
		pc := i + 1
		entries[i] = Entry{ParticipantID: int64(i + 1), Name: "P", PCNumber: &pc, TotalRaw: tot}
	}
	got := Rank(entries)

	wantWSI := []int{
		854, 834, 791, 786, 779, 760, 759, 759, 732, 726, 716, 715, 714, 712,
		704, 702, 698, 694, 694, 689, 686, 685, 683, 681, 681, 680, 679, 678,
		678, 678, 678, 677,
	}
	if len(got) != len(wantWSI) {
		t.Fatalf("got %d entries, want %d", len(got), len(wantWSI))
	}
	medallions := 0
	for i, e := range got {
		if e.Rank != i+1 {
			t.Errorf("row %d rank = %d, want %d", i, e.Rank, i+1)
		}
		if e.WSI != wantWSI[i] {
			t.Errorf("rank %d WSI = %d, want %d", i+1, e.WSI, wantWSI[i])
		}
		if e.Award == AwardMedallion {
			medallions++
		}
	}
	if got[0].Award != AwardGold || got[1].Award != AwardSilver || got[2].Award != AwardBronze {
		t.Errorf("top awards = %q/%q/%q", got[0].Award, got[1].Award, got[2].Award)
	}
	if got[15].Award != AwardMedallion || got[15].WSI != 702 {
		t.Errorf("rank16 = %q wsi %d, want Medallion 702", got[15].Award, got[15].WSI)
	}
	if got[16].Award != "" || got[16].WSI != 698 {
		t.Errorf("rank17 = %q wsi %d, want none 698", got[16].Award, got[16].WSI)
	}
	if medallions != 13 {
		t.Errorf("medallions = %d, want 13", medallions)
	}
}

// TestRankMADZeroFallback: majority of totals tie (MAD collapses to 0), so the
// std-dev fallback must still spread the scored participants off 700.
func TestRankMADZeroFallback(t *testing.T) {
	totals := []float64{950, 880, 820, 760, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	entries := make([]Entry, len(totals))
	for i, tot := range totals {
		entries[i] = Entry{ParticipantID: int64(i + 1), TotalRaw: tot}
	}
	got := Rank(entries)
	if got[0].WSI == 700 || got[0].WSI <= got[1].WSI {
		t.Fatalf("top WSI = %d (rank2 %d): fallback did not spread scores", got[0].WSI, got[1].WSI)
	}
}
