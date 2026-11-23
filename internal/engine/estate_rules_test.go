// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package engine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/zyvorai/scout/internal/model"
)

func cleanVM(id, name string) model.VM {
	return model.VM{ID: id, Name: name, OS: "RHEL 9", Firmware: "uefi", ToolsStatus: "running"}
}

func byVM(a []model.Assessment, id string) model.Assessment {
	for _, x := range a {
		if x.VMID == id {
			return x
		}
	}
	return model.Assessment{}
}

func TestNormalizeMAC(t *testing.T) {
	tests := []struct{ in, want string }{
		{"00:50:56:AB:CD:EF", "00:50:56:ab:cd:ef"},
		{"00-50-56-ab-cd-ef", "00:50:56:ab:cd:ef"},
		{"0050.56ab.cdef", "00:50:56:ab:cd:ef"},
		{"005056abcdef", "00:50:56:ab:cd:ef"},
		{" 00:50:56:ab:cd:ef ", "00:50:56:ab:cd:ef"},
		{"", ""},
		{"00:00:00:00:00:00", ""},
		{"ff:ff:ff:ff:ff:ff", ""},
		{"00:50:56:ab:cd", ""},
		{"00:50:56:ab:cd:ef:01", ""},
		{"0:50:56:ab:cd:ef", ""},
		{"zz:50:56:ab:cd:ef", ""},
		{strings.Repeat("a", 5000), ""},
		{"::::::", ""},
	}
	for _, tt := range tests {
		if got := NormalizeMAC(tt.in); got != tt.want {
			t.Errorf("NormalizeMAC(%q)=%q want %q", tt.in[:min(len(tt.in), 20)], got, tt.want)
		}
	}
}

func TestEstateRules(t *testing.T) {
	mk := func(id, name, mac string) model.VM {
		v := cleanVM(id, name)
		v.NICs = []model.NIC{{ID: "n", Network: "prod", MAC: mac}}
		return v
	}
	tests := []struct {
		name    string
		inv     model.Inventory
		rule    string
		flagged []string
		clean   []string
	}{
		{"duplicate mac, mixed notation", model.Inventory{VMs: []model.VM{mk("a", "a", "00:50:56:AA:AA:AA"), mk("b", "b", "00-50-56-aa-aa-aa"), mk("c", "c", "00:50:56:aa:aa:cc")}}, "duplicate-mac", []string{"a", "b"}, []string{"c"}},
		{"placeholder macs are not duplicates", model.Inventory{VMs: []model.VM{mk("a", "a", "00:00:00:00:00:00"), mk("b", "b", "00:00:00:00:00:00"), mk("c", "c", "")}}, "duplicate-mac", nil, []string{"a", "b", "c"}},
		{"same mac twice on one VM is not a cross-VM duplicate", model.Inventory{VMs: []model.VM{func() model.VM {
			v := cleanVM("a", "a")
			v.NICs = []model.NIC{{MAC: "00:50:56:aa:aa:aa"}, {MAC: "00:50:56:aa:aa:aa"}}
			return v
		}()}}, "duplicate-mac", nil, []string{"a"}},
		{"duplicate uuid ignores case and blanks", model.Inventory{VMs: []model.VM{
			func() model.VM { v := cleanVM("a", "a"); v.UUID = "ABC-1"; return v }(),
			func() model.VM { v := cleanVM("b", "b"); v.UUID = "abc-1"; return v }(),
			func() model.VM { v := cleanVM("c", "c"); return v }(),
			func() model.VM { v := cleanVM("d", "d"); return v }(),
		}}, "duplicate-uuid", []string{"a", "b"}, []string{"c", "d"}},
		{"duplicate name is case-insensitive", model.Inventory{VMs: []model.VM{cleanVM("a", "Web-01"), cleanVM("b", "web-01 "), cleanVM("c", "web-02")}}, "duplicate-name", []string{"a", "b"}, []string{"c"}},
		{"missing datastore", model.Inventory{Storage: &model.StorageEstate{Datastores: []model.Datastore{{Name: "ds1"}}}, VMs: []model.VM{
			func() model.VM { v := cleanVM("a", "a"); v.Disks = []model.Disk{{ID: "d", Datastore: "DS1"}}; return v }(),
			func() model.VM {
				v := cleanVM("b", "b")
				v.Disks = []model.Disk{{ID: "d", Datastore: "gone"}, {ID: "e", Datastore: "gone"}}
				return v
			}(),
			func() model.VM { v := cleanVM("c", "c"); v.Disks = []model.Disk{{ID: "d"}}; return v }(),
		}}, "missing-datastore", []string{"b"}, []string{"a", "c"}},
		{"no datastore list means no check", model.Inventory{VMs: []model.VM{func() model.VM { v := cleanVM("a", "a"); v.Disks = []model.Disk{{Datastore: "x"}}; return v }()}}, "missing-datastore", nil, []string{"a"}},
		{"missing network", model.Inventory{Networks: []string{"prod"}, VMs: []model.VM{
			func() model.VM { v := cleanVM("a", "a"); v.NICs = []model.NIC{{Network: "Prod"}}; return v }(),
			func() model.VM { v := cleanVM("b", "b"); v.NICs = []model.NIC{{Network: "dmz"}}; return v }(),
		}}, "missing-network", []string{"b"}, []string{"a"}},
		{"no network list means no check", model.Inventory{VMs: []model.VM{func() model.VM { v := cleanVM("a", "a"); v.NICs = []model.NIC{{Network: "dmz"}}; return v }()}}, "missing-network", nil, []string{"a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := Evaluate(tt.inv)
			for _, id := range tt.flagged {
				if !hasRule(byVM(a, id).Findings, tt.rule) {
					t.Errorf("%s should be flagged by %s: %+v", id, tt.rule, byVM(a, id).Findings)
				}
			}
			for _, id := range tt.clean {
				if hasRule(byVM(a, id).Findings, tt.rule) {
					t.Errorf("%s should not be flagged by %s", id, tt.rule)
				}
			}
		})
	}
}

func TestSharedDiskSplitAcrossWaves(t *testing.T) {
	shared := func(id, name string) model.VM {
		v := cleanVM(id, name)
		v.Disks = []model.Disk{{ID: "d", Datastore: "ds1", Backing: "[ds1] rac/shared.vmdk"}}
		return v
	}
	blocker := shared("b", "node-b")
	blocker.RDM = true // blocked, so held out of every wave
	inv := model.Inventory{VMs: []model.VM{shared("a", "node-a"), blocker, shared("c", "node-c")}}
	a := Evaluate(inv)
	if byVM(a, "a").Wave != byVM(a, "c").Wave || byVM(a, "a").Wave == 0 {
		t.Fatalf("shared-disk members must land in one wave: a=%d c=%d", byVM(a, "a").Wave, byVM(a, "c").Wave)
	}
	for _, id := range []string{"a", "b", "c"} {
		f := findRule(byVM(a, id).Findings, "shared-disk-split")
		if f == nil {
			t.Fatalf("%s missing shared-disk-split: %+v", id, byVM(a, id).Findings)
		}
		if f.Severity == model.SeverityBlocker {
			t.Fatal("estate rules must not block")
		}
	}
	if byVM(a, "a").Status != "review" {
		t.Fatalf("a should drop to review: %+v", byVM(a, "a"))
	}
	if byVM(a, "b").Status != "blocked" {
		t.Fatalf("b stays blocked: %+v", byVM(a, "b"))
	}
	// Without a blocker the group lands in one wave and nothing fires.
	inv2 := model.Inventory{VMs: []model.VM{shared("a", "node-a"), shared("c", "node-c")}}
	for _, x := range Evaluate(inv2) {
		if hasRule(x.Findings, "shared-disk-split") {
			t.Fatalf("unsplit group flagged: %+v", x)
		}
	}
	// Disks with no backing path never form a group.
	inv3 := model.Inventory{VMs: []model.VM{cleanVM("a", "a"), cleanVM("b", "b")}}
	if g := model.SharedDiskGroups(inv3); len(g) != 0 {
		t.Fatalf("groups=%v", g)
	}
}

func TestEstateRulesScoreAndSort(t *testing.T) {
	a, b := cleanVM("a", "dup"), cleanVM("b", "dup")
	a.TPM = true
	inv := model.Inventory{VMs: []model.VM{a, b}}
	out := Evaluate(inv)
	x := byVM(out, "a")
	if x.Score != 100-12-5 {
		t.Fatalf("score=%d", x.Score)
	}
	if x.Findings[0].Severity != model.SeverityWarning || len(x.Findings) != 2 {
		t.Fatalf("%+v", x.Findings)
	}
	if y := byVM(out, "b"); y.Status != "review" || y.Score != 95 {
		t.Fatalf("%+v", y)
	}
}

type blockingRule struct{}

func (blockingRule) ID() string { return "x" }
func (blockingRule) Evaluate(c EstateContext) []VMFinding {
	return []VMFinding{
		{VMID: "a", Finding: model.Finding{Severity: model.SeverityBlocker, Title: "t", Penalty: 1}},
		{VMID: "ghost", Finding: model.Finding{Severity: model.SeverityWarning}},
	}
}

func TestApplyEstateRulesCustomAndHostile(t *testing.T) {
	inv := model.Inventory{VMs: []model.VM{cleanVM("a", "a")}}
	in := Assess(inv, nil)
	out := ApplyEstateRules(inv, in, []EstateRule{blockingRule{}})
	if out[0].Findings[0].Severity != model.SeverityWarning || out[0].Findings[0].RuleID != "x" || out[0].Findings[0].Basis != model.BasisMeasured {
		t.Fatalf("%+v", out[0].Findings)
	}
	if len(in[0].Findings) != 0 {
		t.Fatal("input assessments must not be mutated")
	}
	if got := ApplyEstateRules(model.Inventory{}, nil, nil); len(got) != 0 {
		t.Fatal(got)
	}
}

func TestEstateFindingsGroupingAndCaps(t *testing.T) {
	var vms []model.VM
	for i := 0; i < 50; i++ {
		v := cleanVM(fmt.Sprintf("id-%d", i), "same")
		v.NICs = []model.NIC{{MAC: "00:50:56:00:00:01"}}
		vms = append(vms, v)
	}
	a := Evaluate(model.Inventory{VMs: vms})
	groups := EstateFindings(a)
	if len(groups) != 2 || groups[0].RuleID != "duplicate-mac" || groups[0].Count != 50 {
		t.Fatalf("%+v", groups)
	}
	for _, g := range groups {
		for _, v := range g.VMs {
			if len(v.Detail) > 1024 || !strings.Contains(v.Detail, "and 44 more") && g.RuleID == "duplicate-mac" {
				t.Fatalf("detail not capped: %q", v.Detail)
			}
		}
	}
	if !IsEstateRule("duplicate-mac") || IsEstateRule("tpm") {
		t.Fatal("IsEstateRule")
	}
}
