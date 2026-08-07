package scoring

import (
	"bytes"
	"fmt"

	"github.com/fzrilsh/lks-judge/internal/model"
	"github.com/go-pdf/fpdf"
)

// PDF renders the scaled results as a one-table A4 document: a header with two
// optional logos and a centered title, then Rank, No, Name, Member, Result and
// Award columns. leftLogo/rightLogo are image bytes (PNG or JPEG); an empty
// slice skips that logo. It writes text, not markup, so participant names need
// no escaping.
func PDF(comp *model.Competition, entries []Entry, leftLogo, rightLogo []byte) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 12, 15)
	pdf.AddPage()

	const pageW = 210.0
	if len(leftLogo) > 0 {
		regImage(pdf, "left", leftLogo)
		pdf.ImageOptions("left", 15, 10, 22, 0, false, fpdf.ImageOptions{ReadDpi: true}, 0, "")
	}
	if len(rightLogo) > 0 {
		regImage(pdf, "right", rightLogo)
		pdf.ImageOptions("right", pageW-15-22, 10, 22, 0, false, fpdf.ImageOptions{ReadDpi: true}, 0, "")
	}

	pdf.SetFont("Helvetica", "B", 13)
	pdf.SetY(12)
	pdf.CellFormat(0, 7, "Web Technologies", "", 1, "C", false, 0, "")
	pdf.SetFont("Helvetica", "", 11)
	pdf.CellFormat(0, 6, "WorldSkills Scale Results", "", 1, "C", false, 0, "")
	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(0, 6, comp.Name, "", 1, "C", false, 0, "")
	pdf.Ln(6)

	// Table header. Widths sum to the usable A4 width (210 minus two 15mm margins).
	widths := []float64{16, 14, 62, 52, 22, 14}
	heads := []string{"Rank", "No", "Name", "Member", "Result", "Award"}
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetFillColor(230, 230, 230)
	for i, h := range heads {
		pdf.CellFormat(widths[i], 7, h, "1", 0, "C", true, 0, "")
	}
	pdf.Ln(-1)

	pdf.SetFont("Helvetica", "", 9)
	for _, e := range entries {
		pc := ""
		if e.PCNumber != nil {
			pc = fmt.Sprintf("%02d", *e.PCNumber)
		}
		cells := []string{fmt.Sprintf("%d", e.Rank), pc, e.Name, e.School, fmt.Sprintf("%d", e.WSI), awardCode(e.Award)}
		aligns := []string{"C", "C", "L", "L", "C", "C"}
		for i, c := range cells {
			pdf.CellFormat(widths[i], 6, c, "1", 0, aligns[i], false, 0, "")
		}
		pdf.Ln(-1)
	}

	pdf.SetFont("Helvetica", "I", 7)
	pdf.Ln(2)
	pdf.CellFormat(0, 5, "G Gold  S Silver  B Bronze  M Medallion for Excellence", "", 1, "L", false, 0, "")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("render pdf: %w", err)
	}
	return buf.Bytes(), nil
}

// awardCode maps an award label to its single-letter table code, empty for none.
func awardCode(award string) string {
	switch award {
	case AwardGold:
		return "G"
	case AwardSilver:
		return "S"
	case AwardBronze:
		return "B"
	case AwardMedallion:
		return "M"
	}
	return ""
}

// regImage registers image bytes under a name, letting fpdf sniff the type.
func regImage(pdf *fpdf.Fpdf, name string, data []byte) {
	pdf.RegisterImageOptionsReader(name, fpdf.ImageOptions{ReadDpi: true}, bytes.NewReader(data))
}
