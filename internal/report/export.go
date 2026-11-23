// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package report

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"sort"
	"strconv"

	"github.com/zyvorai/scout/internal/model"
)

// SafeCell neutralises spreadsheet formula injection. VM names and OS strings
// come from the source environment, so a cell starting with = + - @ or a
// control character would execute when the CSV is opened in Excel. Plain
// numbers, including negative ones, are left alone.
func SafeCell(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '@', '\t', '\r':
		return "'" + s
	case '-':
		if _, err := strconv.ParseFloat(s, 64); err != nil {
			return "'" + s
		}
	}
	return s
}

// SafeRows applies SafeCell to every cell, returning a copy.
func SafeRows(rows [][]string) [][]string {
	out := make([][]string, len(rows))
	for i, r := range rows {
		out[i] = make([]string, len(r))
		for j, c := range r {
			out[i][j] = SafeCell(c)
		}
	}
	return out
}

// WriteFindingsCSV writes one row per finding in a spreadsheet-safe CSV.
func WriteFindingsCSV(w io.Writer, assessments []model.Assessment) error {
	rows := [][]string{{"vmId", "vm", "wave", "status", "score", "ruleId", "severity", "basis", "penalty", "title", "detail", "recommendation"}}
	for _, a := range assessments {
		for _, f := range a.Findings {
			rows = append(rows, []string{a.VMID, a.VMName, strconv.Itoa(a.Wave), a.Status, strconv.Itoa(a.Score), f.RuleID, string(f.Severity), f.Basis, strconv.Itoa(f.Penalty), f.Title, f.Detail, f.Recommendation})
		}
	}
	return csv.NewWriter(w).WriteAll(SafeRows(rows))
}

type sarifMsg struct {
	Text string `json:"text"`
}

// WriteSARIF writes findings as SARIF 2.1.0 so CI and code-scanning tools can
// ingest them. Each VM is a logical location; blocker maps to error, warning
// to warning and info to note.
func WriteSARIF(w io.Writer, version string, assessments []model.Assessment) error {
	type rule struct {
		ID               string   `json:"id"`
		ShortDescription sarifMsg `json:"shortDescription"`
		Help             sarifMsg `json:"help"`
	}
	rules := map[string]rule{}
	results := []map[string]any{}
	for _, a := range assessments {
		for _, f := range a.Findings {
			if _, ok := rules[f.RuleID]; !ok {
				rules[f.RuleID] = rule{ID: f.RuleID, ShortDescription: sarifMsg{f.Title}, Help: sarifMsg{f.Recommendation}}
			}
			level := "note"
			switch f.Severity {
			case model.SeverityBlocker:
				level = "error"
			case model.SeverityWarning:
				level = "warning"
			}
			results = append(results, map[string]any{
				"ruleId":  f.RuleID,
				"level":   level,
				"message": sarifMsg{f.Title + ": " + f.Detail},
				"locations": []map[string]any{{
					"logicalLocations": []map[string]string{{"name": a.VMName, "fullyQualifiedName": a.VMID, "kind": "virtualMachine"}},
				}},
				"properties": map[string]any{"basis": f.Basis, "penalty": f.Penalty, "vmStatus": a.Status, "wave": a.Wave},
			})
		}
	}
	ids := make([]string, 0, len(rules))
	for id := range rules {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	rs := make([]rule, 0, len(ids))
	for _, id := range ids {
		rs = append(rs, rules[id])
	}
	doc := map[string]any{
		"$schema": "https://json.schemastore.org/sarif-2.1.0.json",
		"version": "2.1.0",
		"runs": []map[string]any{{
			"tool": map[string]any{"driver": map[string]any{
				"name": "Zyvor Scout", "version": version, "informationUri": "https://github.com/zyvorai/scout", "rules": rs,
			}},
			"results": results,
		}},
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}
