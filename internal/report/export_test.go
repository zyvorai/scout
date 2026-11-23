// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package report

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"

	"github.com/zyvorai/scout/internal/model"
)

func TestSafeCell(t *testing.T) {
	cases := map[string]string{
		"":                   "",
		"plain":              "plain",
		"=cmd|' /C calc'!A0": "'=cmd|' /C calc'!A0",
		"+1+1":               "'+1+1",
		"@SUM(A1)":           "'@SUM(A1)",
		"-2+3":               "'-2+3",
		"-5":                 "-5",
		"-1.5":               "-1.5",
		"\tx":                "'\tx",
		"a=b":                "a=b",
	}
	for in, want := range cases {
		if got := SafeCell(in); got != want {
			t.Errorf("SafeCell(%q)=%q want %q", in, got, want)
		}
	}
}

func sampleAssessments() []model.Assessment {
	return []model.Assessment{{VMID: "1", VMName: "=evil", Status: "blocked", Score: 70, Wave: 0, Findings: []model.Finding{
		{RuleID: "rdm-disk", Severity: model.SeverityBlocker, Title: "RDM", Detail: "d, with comma", Recommendation: "r", Penalty: 30, Basis: "measured"},
		{RuleID: "tools", Severity: model.SeverityWarning, Title: "Tools", Detail: "d", Penalty: 5},
		{RuleID: "secure-boot", Severity: model.SeverityInfo, Title: "SB", Detail: "d"},
	}}}
}

func TestFindingsCSV(t *testing.T) {
	var b bytes.Buffer
	if err := WriteFindingsCSV(&b, sampleAssessments()); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(&b).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 || rows[1][1] != "'=evil" || rows[1][10] != "d, with comma" {
		t.Fatalf("rows %v", rows)
	}
}

func TestSARIF(t *testing.T) {
	var b bytes.Buffer
	if err := WriteSARIF(&b, "9.9", sampleAssessments()); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Version string `json:"version"`
		Runs    []struct {
			Tool struct {
				Driver struct {
					Version string                `json:"version"`
					Rules   []struct{ ID string } `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
			Results []struct {
				RuleID string `json:"ruleId"`
				Level  string `json:"level"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(b.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	r := doc.Runs[0]
	if doc.Version != "2.1.0" || r.Tool.Driver.Version != "9.9" || len(r.Tool.Driver.Rules) != 3 {
		t.Fatalf("doc %+v", doc)
	}
	var levels []string
	for _, x := range r.Results {
		levels = append(levels, x.Level)
	}
	if strings.Join(levels, ",") != "error,warning,note" {
		t.Fatalf("levels %v", levels)
	}
	// An empty assessment list must still produce an array, not null.
	b.Reset()
	_ = WriteSARIF(&b, "x", nil)
	if !strings.Contains(b.String(), `"results": []`) {
		t.Fatalf("empty results: %s", b.String())
	}
}
