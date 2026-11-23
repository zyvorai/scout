// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package diff

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/zyvorai/scout/internal/model"
)

func as(id, name, status string, score int, rules ...string) model.Assessment {
	a := model.Assessment{VMID: id, VMName: name, Status: status, Score: score, Findings: []model.Finding{}}
	for _, r := range rules {
		sev := model.SeverityWarning
		if strings.HasPrefix(r, "!") {
			r, sev = r[1:], model.SeverityBlocker
		}
		a.Findings = append(a.Findings, model.Finding{RuleID: r, Severity: sev})
	}
	return a
}

func TestCompare(t *testing.T) {
	oldA := []model.Assessment{
		as("1", "same", "ready", 100),
		as("2", "worse", "review", 90, "tpm"),
		as("3", "better", "blocked", 60, "!rdm-disk"),
		as("4", "gone", "ready", 100),
		as("5", "renumbered", "ready", 100),
		as("6", "scoreonly", "review", 90, "tpm"),
	}
	newA := []model.Assessment{
		as("1", "same", "ready", 100),
		as("2", "worse", "blocked", 60, "tpm", "!encrypted-vm"),
		as("3", "better", "review", 90, "tpm"),
		as("new", "fresh", "blocked", 70, "!rdm-disk"),
		as("500", "Renumbered", "review", 92, "gpu"),
		as("6", "scoreonly", "review", 80, "tpm", "gpu"),
	}
	r := Compare(oldA, newA)
	if r.Old.VMs != 6 || r.New.Blocked != 2 || r.Improved != 1 || r.Regressed != 2 {
		t.Fatalf("%+v", r)
	}
	if len(r.Added) != 1 || r.Added[0] != "fresh" || len(r.Removed) != 1 || r.Removed[0] != "gone" {
		t.Fatalf("added %v removed %v", r.Added, r.Removed)
	}
	if r.Changed[0].VMName != "worse" || r.Changed[0].Verdict != "regressed" || len(r.Changed[0].RulesStarted) != 1 || r.Changed[0].RulesStarted[0] != "encrypted-vm" {
		t.Fatalf("%+v", r.Changed[0])
	}
	var better, renum, score *VMChange
	for i := range r.Changed {
		switch r.Changed[i].VMName {
		case "better":
			better = &r.Changed[i]
		case "Renumbered":
			renum = &r.Changed[i]
		case "scoreonly":
			score = &r.Changed[i]
		}
		if r.Changed[i].VMName == "same" {
			t.Fatal("unchanged VM listed")
		}
	}
	if better == nil || better.Verdict != "improved" || len(better.RulesStopped) != 1 || better.RulesStopped[0] != "rdm-disk" {
		t.Fatalf("%+v", better)
	}
	if renum == nil || renum.MatchedBy != "name" || renum.NewStatus != "review" {
		t.Fatalf("%+v", renum)
	}
	if score == nil || score.Verdict != "unchanged" || score.ScoreDelta != -10 {
		t.Fatalf("%+v", score)
	}
	byRule := map[string]RuleChange{}
	for _, c := range r.Rules {
		byRule[c.RuleID] = c
	}
	if c := byRule["encrypted-vm"]; c.OldCount != 0 || c.NewCount != 1 || len(c.Started) != 1 || c.Severity != "blocker" {
		t.Fatalf("%+v", c)
	}
	if c := byRule["rdm-disk"]; c.OldCount != 1 || c.NewCount != 1 || len(c.Started) != 0 || len(c.Stopped) != 1 {
		t.Fatalf("count is flat but it stopped on one VM (a new VM carries it now): %+v", c)
	}
	if _, ok := byRule["tpm"]; ok && byRule["tpm"].OldCount == byRule["tpm"].NewCount && len(byRule["tpm"].Started) == 0 {
		t.Fatal("unchanged rule listed")
	}
	if r.ScoreDelta != r.New.AverageScore-r.Old.AverageScore {
		t.Fatal("delta")
	}
}

func TestCompareIdenticalAndEmpty(t *testing.T) {
	a := []model.Assessment{as("1", "a", "ready", 100, "tpm")}
	r := Compare(a, a)
	if len(r.Changed)+len(r.Rules)+len(r.Added)+len(r.Removed) != 0 || r.ScoreDelta != 0 {
		t.Fatalf("%+v", r)
	}
	r = Compare(nil, nil)
	if r.Old.VMs != 0 || r.Changed == nil || r.Added == nil {
		t.Fatalf("%+v", r)
	}
	r = Compare(nil, a)
	if r.AddedCount != 1 {
		t.Fatalf("%+v", r)
	}
}

func TestCompareDuplicateNamesAreNotGuessed(t *testing.T) {
	oldA := []model.Assessment{as("1", "dup", "ready", 100), as("2", "dup", "ready", 100)}
	newA := []model.Assessment{as("9", "dup", "blocked", 0, "!x"), as("8", "dup", "ready", 100)}
	r := Compare(oldA, newA)
	if len(r.Changed) != 0 || r.AddedCount != 2 || r.RemovedCount != 2 {
		t.Fatalf("%+v", r)
	}
}

func TestCompareCapsLists(t *testing.T) {
	var o, n []model.Assessment
	for i := 0; i < MaxListed*3; i++ {
		id := fmt.Sprint(i)
		o = append(o, as(id, "vm"+id, "ready", 100))
		n = append(n, as(id, "vm"+id, "review", 90, "tpm"))
	}
	r := Compare(o, n)
	c := r.Rules[0]
	if c.NewCount != MaxListed*3 || len(c.Started) != MaxListed || c.StartedMore != MaxListed*2 {
		t.Fatalf("%d %d %d", c.NewCount, len(c.Started), c.StartedMore)
	}
	if r.Regressed != MaxListed*3 {
		t.Fatal(r.Regressed)
	}
}

func TestEvaluateGate(t *testing.T) {
	oldA := []model.Assessment{as("1", "a", "ready", 100), as("2", "b", "ready", 100)}
	newA := []model.Assessment{as("1", "a", "blocked", 60, "!rdm-disk"), as("2", "b", "ready", 100), as("3", "c", "blocked", 70, "!rdm-disk")}
	r := Compare(oldA, newA)
	tests := []struct {
		spec   string
		failed bool
		err    bool
	}{
		{"", false, false},
		{"regression", true, false},
		{"new-blocker", true, false},
		{"score-drop=5", true, false},
		{"score-drop=50", false, false},
		{"regression, new-blocker", true, false},
		{"score-drop=abc", false, true},
		{"score-drop=-1", false, true},
		{"score-drop=101", false, true},
		{"banana", false, true},
	}
	for _, tt := range tests {
		g, err := Evaluate(r, tt.spec, newA)
		if (err != nil) != tt.err || g.Failed != tt.failed {
			t.Errorf("%q: failed=%v err=%v reasons=%v", tt.spec, g.Failed, err, g.Reasons)
		}
	}
	same := Compare(oldA, oldA)
	if g, _ := Evaluate(same, "regression,new-blocker,score-drop=0", oldA); g.Failed {
		t.Fatalf("%+v", g)
	}
}

func TestLoad(t *testing.T) {
	out, _ := json.Marshal(map[string]any{"summary": map[string]int{}, "assessments": []model.Assessment{as("1", "a", "ready", 100)}, "estate": map[string]any{"x": 1}})
	got, err := Load(strings.NewReader(string(out)))
	if err != nil || len(got) != 1 {
		t.Fatalf("%v %v", got, err)
	}
	got, err = Load(strings.NewReader(`{"vms":[{"id":"1","name":"a","os":"RHEL 9","firmware":"uefi","toolsStatus":"running"}]}`))
	if err != nil || len(got) != 1 || got[0].Status != "ready" {
		t.Fatalf("%v %v", got, err)
	}
	bad := map[string]string{
		"empty":          ``,
		"neither":        `{"x":1}`,
		"array":          `[]`,
		"bad status":     `{"assessments":[{"vmId":"1","status":"great"}]}`,
		"no id":          `{"assessments":[{"status":"ready"}]}`,
		"bad inventory":  `{"vms":[{"id":"","name":"a"}]}`,
		"unknown inv":    `{"vms":[{"id":"1","name":"a","zzz":1}]}`,
		"oversize":       `{"assessments":[]}` + strings.Repeat(" ", MaxBytes),
		"truncated json": `{"assessments":[`,
	}
	for name, in := range bad {
		if _, err := Load(strings.NewReader(in)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
