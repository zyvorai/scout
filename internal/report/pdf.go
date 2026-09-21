// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

// WritePDF writes a short executive PDF with the standard Type1 Helvetica font.
// It does not launch a browser.
func WritePDF(w io.Writer, data Data) error {
	lines := executiveLines(data)
	payload, err := renderPDF(lines)
	if err != nil {
		return err
	}
	_, err = w.Write(payload)
	return err
}

func executiveLines(data Data) []string {
	var lines []string
	add := func(s string) {
		for _, part := range wrap(ascii(s), 90) {
			lines = append(lines, part)
		}
	}
	add("Zyvor infrastructure assessment")
	add(data.Inventory.Environment + "  " + data.Generated)
	add(fmt.Sprintf("%d VMs. %d ready, %d review, %d blocked. Average score %d.", data.Summary.TotalVMs, data.Summary.Ready, data.Summary.Review, data.Summary.Blocked, data.Summary.AverageScore))
	add("")
	add("Target architecture")
	add(data.Estate.Architecture)
	add("")
	add("Snapshots (" + data.Estate.SnapshotWaste.Basis + ")")
	add(data.Estate.SnapshotWaste.Detail)
	add("")
	add("Capacity runway")
	for _, row := range data.Estate.Runway {
		add(fmt.Sprintf("%s free %d bytes %s %s [%s]", row.Source, row.FreeBytes, row.FreePercent, row.Days, row.Basis))
	}
	add("")
	add("DR readiness (" + data.Estate.DR.Basis + ")")
	add("Score " + data.Estate.DR.Score + ". " + data.Estate.DR.Detail)
	add("")
	add("Estimated cutover (not a measured outage)")
	for _, a := range data.Assessments {
		add(fmt.Sprintf("%s wave %d %s %d min [%s]", a.VMName, a.Wave, a.Status, a.EstimatedCutoverMinutes, a.CutoverBasis))
	}
	add("")
	add("Three-year figures")
	for _, line := range data.Estate.TCO.Lines {
		add(line.Label + ": " + line.Amount + " [" + line.Basis + "]")
	}
	if data.Estate.TCO.Customer != "" {
		add("Customer three-year total: " + data.Estate.TCO.Customer)
		add("Zyvor three-year total: " + data.Estate.TCO.Zyvor)
		add("Arithmetic difference: " + data.Estate.TCO.Delta)
	}
	for _, note := range data.Estate.TCO.Notes {
		add(note)
	}
	add("")
	add("Produced locally. Source workloads were not modified. This file was not uploaded.")
	return lines
}

func ascii(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 32 && r < 127 {
			b.WriteRune(r)
		} else if r == '\n' || r == '\t' {
			b.WriteByte(' ')
		} else {
			b.WriteByte('?')
		}
	}
	return b.String()
}

func wrap(s string, width int) []string {
	if s == "" {
		return []string{""}
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	cur := words[0]
	for _, w := range words[1:] {
		if len(cur)+1+len(w) > width {
			lines = append(lines, cur)
			cur = w
			continue
		}
		cur += " " + w
	}
	return append(lines, cur)
}

func renderPDF(lines []string) ([]byte, error) {
	const perPage = 46
	if len(lines) == 0 {
		lines = []string{""}
	}
	var pages []string
	for i := 0; i < len(lines); i += perPage {
		end := i + perPage
		if end > len(lines) {
			end = len(lines)
		}
		pages = append(pages, pageStream(lines[i:end]))
	}
	var body bytes.Buffer
	offsets := []int{0}
	writeObj := func(n int, content string) {
		offsets = append(offsets, body.Len())
		fmt.Fprintf(&body, "%d 0 obj\n%s\nendobj\n", n, content)
	}
	// 1 catalog, 2 pages, 3 font, then page/content pairs.
	nPages := len(pages)
	kids := make([]string, nPages)
	for i := 0; i < nPages; i++ {
		kids[i] = fmt.Sprintf("%d 0 R", 4+i*2)
	}
	writeObj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	writeObj(2, fmt.Sprintf("<< /Type /Pages /Count %d /Kids [%s] >>", nPages, strings.Join(kids, " ")))
	writeObj(3, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	for i, stream := range pages {
		pageNum := 4 + i*2
		contentNum := pageNum + 1
		writeObj(pageNum, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents %d 0 R /Resources << /Font << /F1 3 0 R >> >> >>", contentNum))
		writeObj(contentNum, fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(stream), stream))
	}
	xrefAt := body.Len()
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	out.Write(body.Bytes())
	fmt.Fprintf(&out, "xref\n0 %d\n", len(offsets))
	fmt.Fprintf(&out, "0000000000 65535 f \n")
	// Offsets are relative to body, but the header "%PDF-1.4\n" is 9 bytes.
	const header = 9
	for _, off := range offsets[1:] {
		fmt.Fprintf(&out, "%010d 00000 n \n", off+header)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xrefAt+header)
	return out.Bytes(), nil
}

func pageStream(lines []string) string {
	var b strings.Builder
	b.WriteString("BT\n/F1 11 Tf\n54 750 Td\n")
	for i, line := range lines {
		if i > 0 {
			b.WriteString("0 -15 Td\n")
		}
		b.WriteString("(")
		b.WriteString(pdfEscape(line))
		b.WriteString(") Tj\n")
	}
	b.WriteString("ET\n")
	return b.String()
}

func pdfEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "(", `\(`)
	s = strings.ReplaceAll(s, ")", `\)`)
	return s
}
