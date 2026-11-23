// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package plan

import (
	"strings"
	"testing"

	"github.com/zyvorai/scout/internal/model"
)

func vmx(id, name string, gib int64) model.VM {
	return model.VM{ID: id, Name: name, Disks: []model.Disk{{ID: "d", SizeGiB: gib}}}
}

func fixture() (model.Inventory, []model.Assessment) {
	inv := model.Inventory{
		VMs:         []model.VM{vmx("app", "app-1", 100), vmx("db", "db-1", 100), vmx("db2", "db-2", 100), vmx("web", "web-1", 10), vmx("bad", "rdm-1", 10)},
		Connections: []model.Connection{{From: "app", To: "db", Protocol: "tcp", Port: 5432}},
	}
	as := []model.Assessment{
		{VMID: "app", VMName: "app-1", Status: "ready", Wave: 1}, {VMID: "db", VMName: "db-1", Status: "ready", Wave: 1},
		{VMID: "db2", VMName: "db-2", Status: "ready", Wave: 2}, {VMID: "web", VMName: "web-1", Status: "ready", Wave: 3},
		{VMID: "bad", VMName: "rdm-1", Status: "blocked"},
	}
	return inv, as
}

func codes(v Validation) map[string][]Violation {
	m := map[string][]Violation{}
	for _, x := range v.Violations {
		m[x.Code] = append(m[x.Code], x)
	}
	return m
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name  string
		plan  string
		want  []string // codes that must appear
		avoid []string // codes that must not
		ok    bool
	}{
		{"clean", `{"waves":[{"wave":1,"vms":["app-1","db-1"]},{"wave":2,"vms":["db-2"]},{"wave":3,"vms":["web-1"]}]}`, nil, []string{"dependency-order", "unplanned-vm"}, true},
		{"caller before dependency", `{"waves":[{"wave":1,"vms":["app-1"]},{"wave":2,"vms":["db-1","db-2","web-1"]}]}`, []string{"dependency-order"}, []string{"dependency-split"}, false},
		{"dependency first is only a split warning", `{"waves":[{"wave":1,"vms":["db-1","db-2","web-1"]},{"wave":2,"vms":["app-1"]}]}`, []string{"dependency-split"}, []string{"dependency-order"}, true},
		{"explicit dependsOn", `{"dependsOn":[{"vm":"web-1","on":"db-2"}],"waves":[{"wave":1,"vms":["web-1","app-1","db-1"]},{"wave":2,"vms":["db-2"]}]}`, []string{"dependency-order"}, nil, false},
		{"unknown and ambiguous refs", `{"waves":[{"wave":1,"vms":["nope","APP-1","app-1"]}]}`, []string{"unknown-vm", "duplicate-assignment"}, nil, false},
		{"blocked scheduled", `{"waves":[{"wave":1,"vms":["rdm-1","app-1","db-1","db-2","web-1"]}]}`, []string{"blocked-vm"}, []string{"unplanned-vm"}, false},
		{"unplanned", `{"waves":[{"wave":1,"vms":["app-1","db-1"]}]}`, []string{"unplanned-vm"}, nil, true},
		{"bad wave numbers", `{"waves":[{"wave":0,"vms":["app-1"]},{"wave":-3,"vms":["db-1"]},{"wave":2,"vms":["db-2"]},{"wave":2,"vms":["web-1"]}]}`, []string{"invalid-wave", "duplicate-wave"}, nil, false},
		{"exclusive group in explicit batch", `{"constraints":{"exclusiveGroups":[["db-1","db-2"]]},"waves":[{"wave":1,"vms":["db-1","db-2","app-1","web-1"],"batches":[["db-1","db-2"],["app-1","web-1"]]}]}`, []string{"exclusive-group"}, nil, false},
		{"exclusive group auto-batched is fine", `{"constraints":{"exclusiveGroups":[["db-1","db-2"]]},"waves":[{"wave":1,"vms":["db-1","db-2","app-1","web-1"]}]}`, nil, []string{"exclusive-group"}, true},
		{"concurrency", `{"constraints":{"maxConcurrent":2},"waves":[{"wave":1,"vms":["db-1","db-2","app-1"],"batches":[["db-1","db-2","app-1"]]}]}`, []string{"concurrency"}, nil, false},
		{"batch mismatch", `{"waves":[{"wave":1,"vms":["db-1","app-1"],"batches":[["db-1","web-1"]]}]}`, []string{"batch-mismatch"}, nil, false},
		{"window overrun", `{"constraints":{"linkMbps":1,"windowMinutes":30},"waves":[{"wave":1,"vms":["app-1","db-1"]}]}`, []string{"window-overrun"}, nil, true},
		{"negative constraints", `{"constraints":{"maxConcurrent":-1},"waves":[{"wave":1,"vms":["app-1","db-1"]}]}`, []string{"invalid-constraints"}, nil, false},
		{"empty plan", `{}`, []string{"unplanned-vm"}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv, as := fixture()
			e, err := ParseEdited(strings.NewReader(tt.plan))
			if err != nil {
				t.Fatal(err)
			}
			v := Validate(inv, as, e)
			got := codes(v)
			for _, c := range tt.want {
				if len(got[c]) == 0 {
					t.Errorf("missing %s in %+v", c, v.Violations)
				}
			}
			for _, c := range tt.avoid {
				if len(got[c]) != 0 {
					t.Errorf("unexpected %s: %+v", c, got[c])
				}
			}
			if v.OK != tt.ok {
				t.Errorf("ok=%v want %v: %+v", v.OK, tt.ok, v.Violations)
			}
		})
	}
}

func TestValidateAmbiguousNameAndViolationVMs(t *testing.T) {
	inv, as := fixture()
	inv.VMs = append(inv.VMs, vmx("app2", "APP-1", 5))
	as = append(as, model.Assessment{VMID: "app2", VMName: "APP-1", Status: "ready", Wave: 4})
	v := Validate(inv, as, Edited{Waves: []EditedWave{{Wave: 1, VMs: []string{"app-1"}}}})
	if len(codes(v)["ambiguous-vm"]) != 1 {
		t.Fatalf("%+v", v.Violations)
	}
	// ids always resolve
	v = Validate(inv, as, Edited{Waves: []EditedWave{{Wave: 1, VMs: []string{"app", "app2"}}}})
	if len(codes(v)["ambiguous-vm"]) != 0 {
		t.Fatalf("%+v", v.Violations)
	}
}

func TestValidateSharedDiskSplit(t *testing.T) {
	inv, as := fixture()
	inv.VMs[0].Disks[0].Backing, inv.VMs[2].Disks[0].Backing = "x.vmdk", "x.vmdk"
	e := Edited{Waves: []EditedWave{{Wave: 1, VMs: []string{"app-1"}}, {Wave: 2, VMs: []string{"db-2"}}}}
	v := Validate(inv, as, e)
	g := codes(v)["shared-disk-split"]
	if len(g) != 1 || len(g[0].VMs) != 2 {
		t.Fatalf("%+v", v.Violations)
	}
	e = Edited{Waves: []EditedWave{{Wave: 1, VMs: []string{"app-1", "db-2"}}}}
	if len(codes(Validate(inv, as, e))["shared-disk-split"]) != 0 {
		t.Fatal("together should pass")
	}
}

func TestParseEditedHostile(t *testing.T) {
	bad := map[string]string{
		"unknown field":  `{"waves":[],"extra":1}`,
		"trailing":       `{"waves":[]} {"x":1}`,
		"wrong schema":   `{"schema":"scout-plan/9","waves":[]}`,
		"not json":       `waves`,
		"wrong type":     `{"waves":[{"wave":"one","vms":[]}]}`,
		"oversize":       `{"waves":[],"constraints":{}}` + strings.Repeat(" ", MaxEditedBytes),
		"too many waves": `{"waves":[` + strings.Repeat(`{"wave":1,"vms":[]},`, maxEditedWaves) + `{"wave":2,"vms":[]}]}`,
	}
	for name, in := range bad {
		if _, err := ParseEdited(strings.NewReader(in)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := ParseEdited(strings.NewReader(`{"schema":"scout-plan/1","waves":[]}`)); err != nil {
		t.Fatal(err)
	}
}

func TestValidateLargePlanCapsViolationVMs(t *testing.T) {
	inv := model.Inventory{}
	var as []model.Assessment
	for i := 0; i < 500; i++ {
		id := string(rune('a'+i%26)) + strings.Repeat("x", i/26)
		inv.VMs = append(inv.VMs, vmx(id, id, 1))
		as = append(as, model.Assessment{VMID: id, VMName: id, Status: "ready", Wave: 1})
	}
	v := Validate(inv, as, Edited{})
	u := codes(v)["unplanned-vm"]
	if len(u) != 1 || len(u[0].VMs) != maxViolationVMs {
		t.Fatalf("%d", len(u[0].VMs))
	}
}

func TestDraftRoundTrips(t *testing.T) {
	inv, as := fixture()
	d := Draft(inv, as, Constraints{})
	v := Validate(inv, as, d)
	if !v.OK || len(codes(v)["unplanned-vm"]) != 0 {
		t.Fatalf("%+v", v.Violations)
	}
	if d.Schema != EditedSchema || len(d.Waves) != 3 {
		t.Fatalf("%+v", d)
	}
}

func TestWhatIf(t *testing.T) {
	inv, as := fixture()
	e := Edited{Waves: []EditedWave{{Wave: 1, VMs: []string{"app-1", "db-2"}}, {Wave: 2, VMs: []string{"db-1"}}, {Wave: 3, VMs: []string{"web-1"}}}}
	r, err := WhatIf(inv, as, e, []Move{{VM: "db-1", ToWave: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(codes(r.Before)["dependency-order"]) != 1 || len(codes(r.After)["dependency-order"]) != 0 {
		t.Fatalf("%+v", r)
	}
	if len(r.Resolved) != 1 || r.Resolved[0].Code != "dependency-order" || len(r.Introduced) != 0 {
		t.Fatalf("%+v %+v", r.Resolved, r.Introduced)
	}
	// moving into a new wave creates it, an emptied wave disappears
	r, _ = WhatIf(inv, as, e, []Move{{VM: "web-1", ToWave: 9}})
	if r.Edited.Waves[len(r.Edited.Waves)-1].Wave != 9 || len(r.Edited.Waves) != 3 {
		t.Fatalf("%+v", r.Edited.Waves)
	}
	// moving the caller away from its dependency introduces a violation
	r, _ = WhatIf(inv, as, Edited{Waves: []EditedWave{{Wave: 1, VMs: []string{"app-1", "db-1"}}}}, []Move{{VM: "app-1", ToWave: 0}, {VM: "ghost", ToWave: 2}, {VM: "app-1", ToWave: 0}})
	if len(r.Notes) < 2 {
		t.Fatalf("notes %v", r.Notes)
	}
	r, _ = WhatIf(inv, as, Edited{Waves: []EditedWave{{Wave: 1, VMs: []string{"app-1", "db-1"}}}}, []Move{{VM: "db-1", ToWave: 2}})
	if len(r.Introduced) == 0 || r.Introduced[0].Code != "dependency-split" && r.Introduced[0].Code != "dependency-order" {
		t.Fatalf("%+v", r.Introduced)
	}
	if _, err := WhatIf(inv, as, e, nil); err == nil {
		t.Fatal("empty moves accepted")
	}
	if _, err := WhatIf(inv, as, e, make([]Move, maxMoves+1)); err == nil {
		t.Fatal("too many moves accepted")
	}
}

func TestWhatIfDropsExplicitBatches(t *testing.T) {
	inv, as := fixture()
	e := Edited{Waves: []EditedWave{{Wave: 1, VMs: []string{"app-1", "db-1"}, Batches: [][]string{{"app-1"}, {"db-1"}}}, {Wave: 2, VMs: []string{"db-2"}}}}
	r, _ := WhatIf(inv, as, e, []Move{{VM: "db-2", ToWave: 1}})
	if len(r.Notes) == 0 || r.Edited.Waves[0].Batches != nil || len(r.Edited.Waves) != 1 {
		t.Fatalf("%+v", r)
	}
	if e.Waves[0].Batches == nil {
		t.Fatal("input mutated")
	}
}
