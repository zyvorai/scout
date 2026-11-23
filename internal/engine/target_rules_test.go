// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package engine

import (
	"testing"

	"github.com/zyvorai/scout/internal/model"
)

func bp(b bool) *bool { return &b }

func TestTargetRules(t *testing.T) {
	cases := []struct {
		name string
		vm   model.VM
		rule string
		want model.Severity // empty means the rule must not fire
	}{
		{"solaris blocks", model.VM{OS: "Oracle Solaris 11"}, "guest-os-support", model.SeverityBlocker},
		{"macos blocks", model.VM{OS: "Apple macOS 12"}, "guest-os-support", model.SeverityBlocker},
		{"win2003 warns", model.VM{OS: "Microsoft Windows Server 2003"}, "guest-os-support", model.SeverityWarning},
		{"freebsd warns", model.VM{OS: "FreeBSD 13"}, "guest-os-support", model.SeverityWarning},
		{"other unknown", model.VM{OS: "Other (64-bit)"}, "guest-os-support", model.SeverityWarning},
		{"other linux ok", model.VM{OS: "Other 5.x Linux (64-bit)"}, "guest-os-support", ""},
		{"empty os silent", model.VM{OS: ""}, "guest-os-support", ""},
		{"rhel ok", model.VM{OS: "RHEL 9.4"}, "guest-os-support", ""},
		{"hw 4 old", model.VM{HWVersion: 4}, "hw-version", model.SeverityWarning},
		{"hw 7 ok", model.VM{HWVersion: 7}, "hw-version", ""},
		{"hw unknown ok", model.VM{}, "hw-version", ""},
		{"cbt off powered on", model.VM{CBTEnabled: bp(false), PowerState: "POWERED_ON"}, "cbt-disabled", model.SeverityInfo},
		{"cbt off powered off", model.VM{CBTEnabled: bp(false), PowerState: "poweredOff"}, "cbt-disabled", ""},
		{"cbt on", model.VM{CBTEnabled: bp(true)}, "cbt-disabled", ""},
		{"cbt unknown", model.VM{}, "cbt-disabled", ""},
		{"snapshots 23", model.VM{Snapshots: 23}, "snapshot-cbt-limit", ""},
		{"snapshots 24", model.VM{Snapshots: 24}, "snapshot-cbt-limit", model.SeverityWarning},
		{"snapshots 30", model.VM{Snapshots: 30}, "snapshot-cbt-limit", model.SeverityWarning},
		{"independent persistent", model.VM{Disks: []model.Disk{{Mode: "independent_persistent"}}}, "independent-disk", model.SeverityWarning},
		{"independent nonpersistent", model.VM{Disks: []model.Disk{{Mode: "independent_nonpersistent"}}}, "independent-disk", model.SeverityWarning},
		{"dependent disk", model.VM{Disks: []model.Disk{{Mode: "persistent"}}}, "independent-disk", ""},
		{"vmxnet2 nic", model.VM{NICs: []model.NIC{{Type: "Vmxnet2"}}}, "legacy-nic", model.SeverityInfo},
		{"vmxnet3 nic", model.VM{NICs: []model.NIC{{Type: "Vmxnet3"}}}, "legacy-nic", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.vm.ID, tc.vm.Name = "v", "v"
			a := Assess(model.Inventory{VMs: []model.VM{tc.vm}}, nil)
			f := findRule(a[0].Findings, tc.rule)
			if tc.want == "" {
				if f != nil {
					t.Fatalf("rule %s fired: %+v", tc.rule, f)
				}
				return
			}
			if f == nil || f.Severity != tc.want {
				t.Fatalf("rule %s = %+v, want severity %s", tc.rule, f, tc.want)
			}
		})
	}
}

func TestDefaultRulesCountAndUniqueIDs(t *testing.T) {
	rules := DefaultRules()
	if len(rules) != 20 {
		t.Fatalf("rule count = %d, README and docs say 20", len(rules))
	}
	seen := map[string]bool{}
	for _, r := range rules {
		if seen[r.ID()] {
			t.Fatalf("duplicate rule id %s", r.ID())
		}
		seen[r.ID()] = true
	}
}

func TestUnidentifiedOSIsNotMeasured(t *testing.T) {
	a := Assess(model.Inventory{VMs: []model.VM{{ID: "a", Name: "a", OS: "Other"}}}, nil)
	if f := findRule(a[0].Findings, "guest-os-support"); f == nil || f.Basis != model.BasisNotMeasured {
		t.Fatalf("finding %+v", f)
	}
}
