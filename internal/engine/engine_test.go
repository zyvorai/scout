package engine

import (
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
