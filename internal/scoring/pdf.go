package scoring

import (
	"bytes"
	"fmt"

	"github.com/fzrilsh/lks-judge/internal/model"
	"github.com/go-pdf/fpdf"
)

// PDF renders the scaled results as a one-table A4 document matching the old
// design: a header with two optional logos and a centered title block, then a
// four-column table (Name, Member, Result, Award) with a dark-blue header row
// and zebra striping. leftLogo/rightLogo are image bytes (PNG or JPEG); an
// empty slice skips that logo. It writes text, not markup, so participant names
// need no escaping.
func PDF(comp *model.Competition, entries []Entry, leftLogo, rightLogo []byte) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 12, 15)
	pdf.AddPage()

	const pageW = 210.0
	if len(leftLogo) > 0 {
		regImage(pdf, "left", leftLogo)
		// LKS logo, anchored to the left margin. Height only, width auto.
		pdf.ImageOptions("left", 15, 12, 0, 22, false, fpdf.ImageOptions{ReadDpi: true}, 0, "")
	}
	if len(rightLogo) > 0 {
		regImage(pdf, "right", rightLogo)
		// WorldSkills logo, flush to the right margin. Its aspect ratio is wider
		// than tall, so derive the drawn width from the registered dimensions to
		// place the left edge; otherwise a fixed offset leaves it too far in.
		const rightH = 18.0
		info := pdf.GetImageInfo("right")
		w := rightH * info.Width() / info.Height()
		pdf.ImageOptions("right", pageW-15-w, 12, 0, rightH, false, fpdf.ImageOptions{ReadDpi: true}, 0, "")
	}

	pdf.SetY(14)
	pdf.SetFont("Helvetica", "", 18)
	pdf.CellFormat(0, 8, "Web Technologies", "", 1, "C", false, 0, "")
	pdf.SetFont("Helvetica", "", 12)
	pdf.CellFormat(0, 6, "WorldSkills Scale Results", "", 1, "C", false, 0, "")
	pdf.SetFont("Helvetica", "", 13)
	pdf.CellFormat(0, 7, comp.Name, "", 1, "C", false, 0, "")
	pdf.Ln(10)

	// Table header. Widths sum to the usable A4 width (210 minus two 15mm margins).
	widths := []float64{62, 48, 30, 40}
	heads := []string{"Name", "Member", "Result", "Award"}
	aligns := []string{"L", "L", "C", "C"}
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetFillColor(11, 61, 96)
	pdf.SetTextColor(255, 255, 255)
	for i, h := range heads {
		pdf.CellFormat(widths[i], 8, h, "1", 0, aligns[i], true, 0, "")
	}
	pdf.Ln(-1)

	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(0, 0, 0)
	for row, e := range entries {
		fill := row%2 == 1 // zebra: shade every other row
		pdf.SetFillColor(249, 249, 249)
		cells := []string{e.Name, e.School, fmt.Sprintf("%d", e.WSI), e.Award}
		for i, c := range cells {
			pdf.CellFormat(widths[i], 7, c, "1", 0, aligns[i], fill, 0, "")
		}
		pdf.Ln(-1)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("render pdf: %w", err)
	}
	return buf.Bytes(), nil
}

// regImage registers image bytes under a name. A custom reader cannot sniff
// the format (unlike the file-path variant), so the type must be set from the
// magic bytes: PNG starts 0x89 P N G, JPEG starts 0xFF 0xD8.
func regImage(pdf *fpdf.Fpdf, name string, data []byte) {
	imgType := "png"
	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xD8 {
		imgType = "jpg"
	}
	pdf.RegisterImageOptionsReader(name, fpdf.ImageOptions{ReadDpi: true, ImageType: imgType}, bytes.NewReader(data))
}
