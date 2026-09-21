// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package estate

import (
	"strings"
	"testing"
	"time"

	"github.com/zyvorai/scout/internal/model"
)

func TestRunwayUnknownWithoutGrowth(t *testing.T) {
	inv := model.Inventory{Storage: &model.StorageEstate{Datastores: []model.Datastore{{Name: "ds", Capacity: 1000, FreeBytes: 250}}}}
	_, est := Annotate(inv, nil)
	if len(est.Runway) != 1 || est.Runway[0].Days != "runway unknown" || est.Runway[0].FreePercent != "25.0%" {
		t.Fatalf("%+v", est.Runway)
	}
	if est.Runway[0].Basis != model.BasisMeasured {
		t.Fatalf("basis %s", est.Runway[0].Basis)
	}
}

func TestRunwayUsesSuppliedGrowth(t *testing.T) {
	inv := model.Inventory{Storage: &model.StorageEstate{
		Datastores:        []model.Datastore{{Name: "ds", Capacity: 1000, FreeBytes: 500}},
		GrowthBytesPerDay: 10,
	}}
	_, est := Annotate(inv, nil)
	if est.Runway[0].Basis != model.BasisEstimated || !strings.Contains(est.Runway[0].Days, "50 days") {
		t.Fatalf("%+v", est.Runway)
	}
}

func TestDRScoreDropsWhenRestoreIsStale(t *testing.T) {
	backup := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	restore := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rpo := 15
	offsite := true
	inv := model.Inventory{
		GeneratedAt: time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC),
		DR: &model.DRPosture{
			LastSuccessfulBackup: &backup,
			LastTestRestore:      &restore,
			TargetRPOMinutes:     &rpo,
			OffsiteCopy:          &offsite,
		},
	}
	_, est := Annotate(inv, nil)
	if est.DR.Score != "75" || est.DR.Basis != model.BasisMeasured {
		t.Fatalf("%+v", est.DR)
	}
	if !strings.Contains(est.DR.Detail, "older than 90 days") {
		t.Fatalf("detail %s", est.DR.Detail)
	}
}

func TestDRUnknownWithoutFile(t *testing.T) {
	_, est := Annotate(model.Inventory{}, nil)
	if est.DR.Score != "unknown" || est.DR.Basis != model.BasisNotMeasured {
		t.Fatalf("%+v", est.DR)
	}
}

func TestTCOArithmeticUsesCustomerFigures(t *testing.T) {
	cores, included, window := 64, 100, 15
	vmware, storage, support, migration := 100000.0, 20000.0, 10000.0, 5000.0
	link := 1000.0
	capGiB := int64(100)
	inv := model.Inventory{
		VMs: []model.VM{{ID: "a", Name: "a", Disks: []model.Disk{{SizeGiB: 10}}}},
		Assumptions: &model.Assumptions{
			Cores:                &cores,
			VMwareAnnual:         &vmware,
			StorageAnnual:        &storage,
			SupportAnnual:        &support,
			MigrationCost:        &migration,
			LinkMbps:             &link,
			CutoverMinutes:       &window,
			IncludedMigrationVMs: &included,
			IncludedDiskGiB:      &capGiB,
		},
	}
	assessments, est := Annotate(inv, []model.Assessment{{VMID: "a", VMName: "a"}})
	if !est.TCO.Complete || est.TCO.Customer != "395000.00" || est.TCO.Zyvor != "45000.00" || est.TCO.Delta != "350000.00" {
		t.Fatalf("%+v notes=%v", est.TCO, est.TCO.Notes)
	}
	for _, line := range est.TCO.Lines {
		if strings.Contains(line.Label, "70%") || strings.Contains(line.Amount, "70%") {
			t.Fatalf("generic claim in %+v", line)
		}
	}
	for _, note := range est.TCO.Notes {
		if strings.Contains(note, "70%") || strings.Contains(strings.ToLower(note), "cheaper") {
			t.Fatalf("generic claim in %s", note)
		}
	}
	// 10 GiB at 1000 Mbps is 87.4s, ceil to 2 minutes, plus 15.
	if assessments[0].EstimatedCutoverMinutes != 17 || assessments[0].CutoverBasis != model.BasisEstimated {
		t.Fatalf("cutover %+v", assessments[0])
	}
}

func TestArchitectureNamesDrivers(t *testing.T) {
	inv := model.Inventory{Storage: &model.StorageEstate{
		CephPools:   []model.Pool{{Name: "rbd"}},
		NFSExports:  []model.Export{{Path: "/export"}},
		ZFSDatasets: []model.Dataset{{Name: "tank/vm"}},
	}}
	_, est := Annotate(inv, nil)
	if !strings.Contains(est.Architecture, "Atlas Ceph driver") || !strings.Contains(est.Architecture, "Atlas NFS driver") || !strings.Contains(est.Architecture, "Atlas ZFS driver") {
		t.Fatalf("%s", est.Architecture)
	}
	if !strings.Contains(est.Architecture, "not a provisioned cluster") {
		t.Fatalf("%s", est.Architecture)
	}
}
