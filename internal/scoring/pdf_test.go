package scoring

import (
	"bytes"
	"testing"

	"github.com/fzrilsh/lks-judge/internal/model"
)

func TestPDFProducesPDF(t *testing.T) {
	pc := 1
	entries := []Entry{
		{Rank: 1, Name: "Foreno", School: "SMK 1", PCNumber: &pc, WSI: 854, Award: AwardGold},
		{Rank: 2, Name: "NoSeat", WSI: 700, Award: ""},
	}
	comp := &model.Competition{Name: "LKS Nasional"}
	out, err := PDF(comp, entries, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF-")) {
		t.Fatalf("output is not a PDF, first bytes: %q", out[:min(8, len(out))])
	}
	if len(out) < 512 {
		t.Errorf("PDF suspiciously small: %d bytes", len(out))
	}
}
