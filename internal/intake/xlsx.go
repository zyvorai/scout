// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

// Package intake reads customer exports that already sit on disk.
// It does not dial vCenter, Ceph, or an API server, and it does not upload anything.
package intake

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Sheet is a header-keyed table. The first recognized header row is the schema.
type Sheet struct {
	Name string
	Rows []map[string]string
}

// Workbook is the set of sheets in an xlsx file, keyed by sheet name.
type Workbook struct {
	Sheets map[string]Sheet
}

// OpenXLSX reads an Office Open XML workbook with the standard library.
// Missing optional sheets are simply absent; the caller decides which names matter.
func OpenXLSX(path string) (Workbook, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return Workbook{}, fmt.Errorf("open xlsx: %w", err)
	}
	defer zr.Close()
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}
	var shared []string
	if f, ok := files["xl/sharedStrings.xml"]; ok {
		rc, err := f.Open()
		if err != nil {
			return Workbook{}, err
		}
		shared, err = readSharedStrings(rc)
		rc.Close()
		if err != nil {
			return Workbook{}, err
		}
	}
	names, err := sheetTargets(files)
	if err != nil {
		return Workbook{}, err
	}
	out := Workbook{Sheets: map[string]Sheet{}}
	for name, target := range names {
		f, ok := files[target]
		if !ok {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return Workbook{}, err
		}
		rows, err := readSheet(rc, shared)
		rc.Close()
		if err != nil {
			return Workbook{}, fmt.Errorf("sheet %s: %w", name, err)
		}
		out.Sheets[name] = Sheet{Name: name, Rows: rows}
	}
	return out, nil
}

func sheetTargets(files map[string]*zip.File) (map[string]string, error) {
	wb, ok := files["xl/workbook.xml"]
	if !ok {
		return nil, fmt.Errorf("xlsx is missing xl/workbook.xml")
	}
	rc, err := wb.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	type sheetRef struct {
		name string
		id   string
	}
	var refs []sheetRef
	dec := xml.NewDecoder(rc)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != "sheet" {
			continue
		}
		var name, id string
		for _, a := range start.Attr {
			switch a.Name.Local {
			case "name":
				name = a.Value
			case "id":
				id = a.Value
			}
		}
		if name != "" && id != "" {
			refs = append(refs, sheetRef{name: name, id: id})
		}
	}
	rels, ok := files["xl/_rels/workbook.xml.rels"]
	if !ok {
		return nil, fmt.Errorf("xlsx is missing workbook relationships")
	}
	rr, err := rels.Open()
	if err != nil {
		return nil, err
	}
	defer rr.Close()
	targets := map[string]string{}
	dec = xml.NewDecoder(rr)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != "Relationship" {
			continue
		}
		var id, target string
		for _, a := range start.Attr {
			switch a.Name.Local {
			case "Id":
				id = a.Value
			case "Target":
				target = a.Value
			}
		}
		if id != "" && target != "" {
			targets[id] = cleanTarget(target)
		}
	}
	out := map[string]string{}
	for _, ref := range refs {
		if target, ok := targets[ref.id]; ok {
			out[ref.name] = target
		}
	}
	return out, nil
}

func cleanTarget(target string) string {
	target = strings.TrimPrefix(target, "/")
	if !strings.HasPrefix(target, "xl/") {
		target = "xl/" + strings.TrimPrefix(target, "/")
	}
	return target
}

func readSharedStrings(r io.Reader) ([]string, error) {
	dec := xml.NewDecoder(r)
	var out []string
	var cur strings.Builder
	inSI := false
	inT := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "si":
				inSI = true
				cur.Reset()
			case "t":
				if inSI {
					inT++
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				if inT > 0 {
					inT--
				}
			case "si":
				out = append(out, cur.String())
				inSI = false
			}
		case xml.CharData:
			if inT > 0 {
				cur.Write(t)
			}
		}
	}
	return out, nil
}

func readSheet(r io.Reader, shared []string) ([]map[string]string, error) {
	dec := xml.NewDecoder(r)
	var table [][]string
	var row []string
	inRow := false
	col := -1
	cellType := ""
	var val strings.Builder
	inV := false
	inInline := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "row":
				inRow = true
				row = nil
			case "c":
				if !inRow {
					continue
				}
				cellType = ""
				col = len(row)
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "r":
						col = colIndex(a.Value)
					case "t":
						cellType = a.Value
					}
				}
				val.Reset()
			case "v":
				inV = true
				val.Reset()
			case "t":
				if cellType == "inlineStr" {
					inInline = true
					val.Reset()
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "v":
				inV = false
			case "t":
				inInline = false
			case "c":
				if !inRow {
					continue
				}
				text := strings.TrimSpace(val.String())
				if cellType == "s" {
					if n, err := strconv.Atoi(text); err == nil && n >= 0 && n < len(shared) {
						text = shared[n]
					}
				}
				for len(row) <= col {
					row = append(row, "")
				}
				if col >= 0 {
					row[col] = text
				}
			case "row":
				if inRow && !rowEmpty(row) {
					table = append(table, row)
				}
				inRow = false
			}
		case xml.CharData:
			if inV || inInline {
				val.Write(t)
			}
		}
	}
	return tableToMaps(table), nil
}

func rowEmpty(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

func tableToMaps(table [][]string) []map[string]string {
	if len(table) == 0 {
		return nil
	}
	headerAt := 0
	for i := 0; i < len(table) && i < 8; i++ {
		for _, c := range table[i] {
			n := normHeader(c)
			if n == "vm" || n == "name" || n == "host" {
				headerAt = i
				i = 8
				break
			}
		}
	}
	headers := make([]string, len(table[headerAt]))
	for i, h := range table[headerAt] {
		headers[i] = normHeader(h)
	}
	var rows []map[string]string
	for _, raw := range table[headerAt+1:] {
		m := map[string]string{}
		empty := true
		for i, h := range headers {
			if h == "" || i >= len(raw) {
				continue
			}
			v := strings.TrimSpace(raw[i])
			if v != "" {
				empty = false
			}
			m[h] = v
		}
		if !empty {
			rows = append(rows, m)
		}
	}
	return rows
}

func normHeader(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "_", " ")
	return strings.Join(strings.Fields(s), " ")
}

func colIndex(ref string) int {
	n := 0
	for _, c := range ref {
		switch {
		case c >= 'A' && c <= 'Z':
			n = n*26 + int(c-'A'+1)
		case c >= 'a' && c <= 'z':
			n = n*26 + int(c-'a'+1)
		default:
			return n - 1
		}
	}
	return n - 1
}
