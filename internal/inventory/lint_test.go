// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package inventory

import (
	"strings"
	"testing"

	"github.com/zyvorai/scout/internal/model"
)

func TestDecodeRejectsTrailingDataAndBadSchema(t *testing.T) {
	good := `{"vms":[{"id":"1","name":"vm"}]}`
	if _, err := Decode(strings.NewReader(good)); err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(strings.NewReader(`{"schema":"scout-inventory/1","vms":[{"id":"1","name":"vm"}]}`)); err != nil {
		t.Fatal(err)
	}
	for name, in := range map[string]string{
		"trailing object": good + `{}`,
		"trailing text":   good + ` junk`,
		"other schema":    `{"schema":"scout-inventory/9","vms":[{"id":"1","name":"vm"}]}`,
	} {
		if _, err := Decode(strings.NewReader(in)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestLint(t *testing.T) {
	tests := []struct {
		name string
		inv  model.Inventory
		want string // substring of a problem; "" means no problems
		sev  string
	}{
		{"clean", model.Inventory{VMs: []model.VM{{ID: "a", Name: "a"}}}, "", ""},
		{"empty", model.Inventory{}, "no VMs", "error"},
		{"dup id", model.Inventory{VMs: []model.VM{{ID: "a", Name: "a"}, {ID: "a", Name: "b"}}}, "duplicate id", "error"},
		{"negative disk", model.Inventory{VMs: []model.VM{{ID: "a", Name: "a", Disks: []model.Disk{{SizeGiB: -1}}}}}, "sizeGiB", "error"},
		{"negative snapshots", model.Inventory{VMs: []model.VM{{ID: "a", Name: "a", Snapshots: -1}}}, "snapshots", "error"},
		{"bad ip", model.Inventory{VMs: []model.VM{{ID: "a", Name: "a", NICs: []model.NIC{{IP: "1.2.3"}}}}}, "not an IP", "error"},
		{"ipv6 ok", model.Inventory{VMs: []model.VM{{ID: "a", Name: "a", NICs: []model.NIC{{IP: "fe80::1"}}}}}, "", ""},
		{"placeholder mac", model.Inventory{VMs: []model.VM{{ID: "a", Name: "a", NICs: []model.NIC{{MAC: "ff:ff:ff:ff:ff:ff"}}}}}, "placeholder", "warning"},
		{"long name", model.Inventory{VMs: []model.VM{{ID: "a", Name: strings.Repeat("n", MaxString+1)}}}, "limit", "error"},
		{"bad port", model.Inventory{VMs: []model.VM{{ID: "a", Name: "a"}}, Connections: []model.Connection{{From: "a", To: "a", Port: 70000}}}, "port", "error"},
		{"bad schema", model.Inventory{Schema: "x", VMs: []model.VM{{ID: "a", Name: "a"}}}, "unsupported schema", "error"},
	}
	for _, tt := range tests {
		ps := Lint(tt.inv)
		if tt.want == "" {
			if len(ps) != 0 {
				t.Errorf("%s: %+v", tt.name, ps)
			}
			continue
		}
		found := false
		for _, p := range ps {
			found = found || (strings.Contains(p.Message, tt.want) || strings.Contains(p.Path, tt.want)) && p.Severity == tt.sev
		}
		if !found {
			t.Errorf("%s: want %q (%s) in %+v", tt.name, tt.want, tt.sev, ps)
		}
	}
	if len(Errors(Lint(model.Inventory{VMs: []model.VM{{ID: "a", Name: "a", NICs: []model.NIC{{MAC: "00:00:00:00:00:00"}}}}}))) != 0 {
		t.Error("warnings must not count as errors")
	}
}
