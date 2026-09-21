// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"strings"
	"testing"

	"github.com/zyvorai/scout/internal/model"
)

func TestAssessStatuses(t *testing.T) {
	inv := model.Inventory{VMs: []model.VM{
		{ID: "a", Name: "clean", OS: "RHEL 9", Firmware: "uefi", ToolsStatus: "running"},
		{ID: "b", Name: "tpm", OS: "Windows Server 2022", Firmware: "uefi", ToolsStatus: "running", TPM: true},
		{ID: "c", Name: "rdm", OS: "Windows Server 2019", Firmware: "uefi", ToolsStatus: "running", RDM: true},
	}}
	a := Assess(inv, nil)
	if a[0].Status != "ready" || a[0].Score != 100 {
		t.Fatalf("clean=%+v", a[0])
	}
	if a[1].Status != "review" || a[1].Score >= 100 {
		t.Fatalf("tpm=%+v", a[1])
	}
	if a[2].Status != "blocked" {
		t.Fatalf("rdm=%+v", a[2])
	}
	if !hasRule(a[1].Findings, "windows-virtio") || !hasRule(a[2].Findings, "windows-virtio") {
		t.Fatalf("windows guests need a virtio review: %+v %+v", a[1].Findings, a[2].Findings)
	}
}

func TestWindowsVirtIOIsReviewNotAGuess(t *testing.T) {
	inv := model.Inventory{VMs: []model.VM{{ID: "w", Name: "dc", OS: "Windows Server 2022", Firmware: "uefi", ToolsStatus: "running"}}}
	a := Assess(inv, nil)
	if a[0].Status != "review" {
		t.Fatalf("status %+v", a[0])
	}
	f := findRule(a[0].Findings, "windows-virtio")
	if f == nil || f.Basis != model.BasisNotMeasured {
		t.Fatalf("finding %+v", f)
	}
	if !strings.Contains(f.Title, "VirtIO drivers not evidenced") || !strings.Contains(f.Title, "GuestKit") {
		t.Fatalf("title %q", f.Title)
	}
}

func hasRule(findings []model.Finding, id string) bool {
	return findRule(findings, id) != nil
}

func findRule(findings []model.Finding, id string) *model.Finding {
	for i := range findings {
		if findings[i].RuleID == id {
			return &findings[i]
		}
	}
	return nil
}

func TestSecureBootIsInformational(t *testing.T) {
	inv := model.Inventory{VMs: []model.VM{{ID: "a", Name: "secure", OS: "RHEL 9", Firmware: "uefi", ToolsStatus: "running", SecureBoot: true}}}
	a := Assess(inv, nil)
	if a[0].Status != "ready" || a[0].Score != 100 {
		t.Fatalf("secure boot should remain ready: %+v", a[0])
	}
	if len(a[0].Findings) != 1 || a[0].Findings[0].Severity != model.SeverityInfo {
		t.Fatalf("expected info finding: %+v", a[0].Findings)
	}
}
