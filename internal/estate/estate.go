// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

// Package estate turns inventory evidence into the assessment sections that
// are not per-VM compatibility rules: runway, DR, architecture, and TCO.
package estate

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/zyvorai/scout/internal/model"
)

const (
	defaultPricePerCore = 129.0
	defaultMinimum      = 15000.0
	defaultTermYears    = 3
	defaultIncludedVMs  = 100
	defaultCutoverMin   = 15
	drTestStaleAfter    = 90 * 24 * time.Hour
)

// Estate is the customer-specific assessment beyond per-VM compatibility.
type Estate struct {
	SnapshotWaste SnapshotWaste `json:"snapshotWaste"`
	Runway        []Runway      `json:"runway"`
	Architecture  string        `json:"architecture"`
	DR            DRResult      `json:"dr"`
	TCO           TCO           `json:"tco"`
}

type SnapshotWaste struct {
	Aged         int    `json:"aged"`
	Orphan       int    `json:"orphan"`
	WastedBytes  int64  `json:"wastedBytes"`
	SizeMeasured bool   `json:"sizeMeasured"`
	Basis        string `json:"basis"`
	Detail       string `json:"detail"`
}

type Runway struct {
	Source      string `json:"source"`
	FreeBytes   int64  `json:"freeBytes"`
	Capacity    int64  `json:"capacityBytes,omitempty"`
	FreePercent string `json:"freePercent,omitempty"`
	Days        string `json:"days"`
	Basis       string `json:"basis"`
}

type DRResult struct {
	Score  string `json:"score"`
	Basis  string `json:"basis"`
	Detail string `json:"detail"`
}

type TCOLine struct {
	Label  string `json:"label"`
	Amount string `json:"amount"`
	Basis  string `json:"basis"`
}

type TCO struct {
	Complete bool      `json:"complete"`
	Basis    string    `json:"basis"`
	Lines    []TCOLine `json:"lines"`
	Notes    []string  `json:"notes,omitempty"`
	Customer string    `json:"customerThreeYear,omitempty"`
	Zyvor    string    `json:"zyvorThreeYear,omitempty"`
	Delta    string    `json:"difference,omitempty"`
}

// Annotate fills estimated cutover minutes and returns the estate sections.
// Cutover minutes are an estimate from disk bytes and the customer link rate.
func Annotate(inv model.Inventory, assessments []model.Assessment) ([]model.Assessment, Estate) {
	out := append([]model.Assessment(nil), assessments...)
	byID := map[string]model.VM{}
	for _, vm := range inv.VMs {
		byID[vm.ID] = vm
	}
	link := 0.0
	window := defaultCutoverMin
	if inv.Assumptions != nil {
		if inv.Assumptions.LinkMbps != nil {
			link = *inv.Assumptions.LinkMbps
		}
		if inv.Assumptions.CutoverMinutes != nil && *inv.Assumptions.CutoverMinutes >= 0 {
			window = *inv.Assumptions.CutoverMinutes
		}
	}
	for i := range out {
		vm := byID[out[i].VMID]
		var bytes int64
		for _, d := range vm.Disks {
			bytes += d.SizeGiB * 1024 * 1024 * 1024
		}
		if link <= 0 {
			out[i].CutoverBasis = model.BasisNotMeasured
			out[i].EstimatedCutoverMinutes = 0
			continue
		}
		seconds := float64(bytes) * 8 / (link * 1e6)
		minutes := int(math.Ceil(seconds/60)) + window
		out[i].EstimatedCutoverMinutes = minutes
		out[i].CutoverBasis = model.BasisEstimated
	}
	return out, Estate{
		SnapshotWaste: snapshotWaste(inv),
		Runway:        runway(inv),
		Architecture:  architecture(inv),
		DR:            drScore(inv),
		TCO:           tco(inv),
	}
}

func snapshotWaste(inv model.Inventory) SnapshotWaste {
	var w SnapshotWaste
	var sized int
	consume := func(s model.Snapshot) {
		if s.Aged {
			w.Aged++
		}
		if s.Orphan {
			w.Orphan++
		}
		if s.Aged || s.Orphan {
			if s.SizeBytes > 0 {
				w.WastedBytes += s.SizeBytes
				sized++
			}
		}
	}
	for _, vm := range inv.VMs {
		for _, s := range vm.SnapshotDetails {
			consume(s)
		}
	}
	for _, s := range inv.OrphanSnapshots {
		consume(s)
	}
	switch {
	case w.Aged == 0 && w.Orphan == 0:
		w.Basis = model.BasisMeasured
		w.Detail = "No aged or orphaned snapshots were present in the export."
	case sized == 0:
		w.Basis = model.BasisNotMeasured
		w.Detail = fmt.Sprintf("%d aged and %d orphan snapshots. The sheet had no size, so wasted bytes were not measured.", w.Aged, w.Orphan)
	default:
		w.SizeMeasured = true
		w.Basis = model.BasisMeasured
		w.Detail = fmt.Sprintf("%d aged and %d orphan snapshots. Wasted bytes measured: %d.", w.Aged, w.Orphan, w.WastedBytes)
	}
	return w
}

func runway(inv model.Inventory) []Runway {
	if inv.Storage == nil {
		return []Runway{{
			Source: "storage",
			Days:   "runway unknown",
			Basis:  model.BasisNotMeasured,
		}}
	}
	growth := inv.Storage.GrowthBytesPerDay
	var rows []Runway
	add := func(source string, free, capacity int64) {
		if free == 0 && capacity == 0 {
			return
		}
		row := Runway{Source: source, FreeBytes: free, Capacity: capacity, Basis: model.BasisMeasured, Days: "runway unknown"}
		if capacity > 0 {
			row.FreePercent = fmt.Sprintf("%.1f%%", 100*float64(free)/float64(capacity))
		}
		if growth > 0 && free > 0 {
			days := float64(free) / float64(growth)
			row.Days = fmt.Sprintf("%.0f days (estimated from supplied growth)", days)
			row.Basis = model.BasisEstimated
		}
		rows = append(rows, row)
	}
	var dsFree, dsCap int64
	for _, ds := range inv.Storage.Datastores {
		dsFree += ds.FreeBytes
		dsCap += ds.Capacity
	}
	add("vmware-datastores", dsFree, dsCap)
	var cephFree, cephCap int64
	for _, p := range inv.Storage.CephPools {
		cephCap += p.MaxBytes
		if p.MaxBytes > p.UsedBytes {
			cephFree += p.MaxBytes - p.UsedBytes
		}
	}
	add("ceph", cephFree, cephCap)
	var nfsFree, nfsCap int64
	for _, e := range inv.Storage.NFSExports {
		nfsCap += e.SizeBytes
		if e.SizeBytes > e.UsedBytes {
			nfsFree += e.SizeBytes - e.UsedBytes
		}
	}
	add("nfs", nfsFree, nfsCap)
	var zfsFree, zfsCap int64
	for _, d := range inv.Storage.ZFSDatasets {
		zfsFree += d.AvailBytes
		zfsCap += d.UsedBytes + d.AvailBytes
	}
	add("zfs", zfsFree, zfsCap)
	if len(rows) == 0 {
		return []Runway{{Source: "storage", Days: "runway unknown", Basis: model.BasisNotMeasured}}
	}
	return rows
}

func architecture(inv model.Inventory) string {
	var parts []string
	if inv.Storage != nil {
		if len(inv.Storage.CephPools) > 0 {
			names := make([]string, 0, len(inv.Storage.CephPools))
			for _, p := range inv.Storage.CephPools {
				names = append(names, p.Name)
			}
			parts = append(parts, fmt.Sprintf("Ceph pools (%s) map to the Atlas Ceph driver.", strings.Join(names, ", ")))
		}
		if len(inv.Storage.NFSExports) > 0 {
			parts = append(parts, fmt.Sprintf("%d NFS exports map to the Atlas NFS driver.", len(inv.Storage.NFSExports)))
		}
		if len(inv.Storage.ZFSDatasets) > 0 {
			parts = append(parts, fmt.Sprintf("%d ZFS datasets map to the Atlas ZFS driver.", len(inv.Storage.ZFSDatasets)))
		}
		if inv.Storage.Kubernetes != nil && len(inv.Storage.Kubernetes.StorageClasses) > 0 {
			parts = append(parts, fmt.Sprintf("Kubernetes StorageClasses already present: %s.", strings.Join(inv.Storage.Kubernetes.StorageClasses, ", ")))
		}
	}
	if len(parts) == 0 {
		parts = append(parts, "No Ceph, NFS, or ZFS inventory was provided. A target architecture cannot be recommended from this export.")
	}
	parts = append(parts, "This is a recommendation, not a provisioned cluster.")
	return strings.Join(parts, " ")
}

func drScore(inv model.Inventory) DRResult {
	if inv.DR == nil {
		return DRResult{Score: "unknown", Basis: model.BasisNotMeasured, Detail: "No dr.yaml was provided, so DR readiness was not scored."}
	}
	now := inv.GeneratedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	score := 100
	var notes []string
	dr := inv.DR
	if dr.LastSuccessfulBackup == nil {
		score -= 40
		notes = append(notes, "no successful backup time was recorded")
	} else if now.Sub(*dr.LastSuccessfulBackup) > 7*24*time.Hour {
		score -= 15
		notes = append(notes, "last successful backup is older than 7 days")
	}
	if dr.LastTestRestore == nil {
		score -= 30
		notes = append(notes, "no test restore was recorded")
	} else if now.Sub(*dr.LastTestRestore) > drTestStaleAfter {
		score -= 25
		notes = append(notes, "last test restore is older than 90 days")
	}
	if dr.OffsiteCopy == nil || !*dr.OffsiteCopy {
		score -= 15
		notes = append(notes, "offsite copy is not declared")
	}
	if dr.TargetRPOMinutes == nil {
		score -= 10
		notes = append(notes, "target RPO was not set")
	}
	if score < 0 {
		score = 0
	}
	detail := "DR declaration scored from dr.yaml."
	if len(notes) > 0 {
		detail = detail + " " + strings.Join(notes, "; ") + "."
	}
	return DRResult{Score: fmt.Sprintf("%d", score), Basis: model.BasisMeasured, Detail: detail}
}

func tco(inv model.Inventory) TCO {
	out := TCO{Basis: model.BasisEstimated}
	if inv.Assumptions == nil {
		out.Notes = []string{"TCO was not calculated because assumptions.json was not provided."}
		out.Basis = model.BasisNotMeasured
		return out
	}
	a := inv.Assumptions
	price := defaultPricePerCore
	if a.PricePerCoreYear != nil {
		price = *a.PricePerCoreYear
	}
	minimum := defaultMinimum
	if a.MinimumAnnual != nil {
		minimum = *a.MinimumAnnual
	}
	term := defaultTermYears
	if a.TermYears != nil {
		term = *a.TermYears
	}
	includedVMs := defaultIncludedVMs
	if a.IncludedMigrationVMs != nil {
		includedVMs = *a.IncludedMigrationVMs
	}
	var missing []string
	if a.Cores == nil {
		missing = append(missing, "cores")
	}
	if a.VMwareAnnual == nil {
		missing = append(missing, "vmwareAnnual")
	}
	if a.StorageAnnual == nil {
		missing = append(missing, "storageAnnual")
	}
	if a.SupportAnnual == nil {
		missing = append(missing, "supportAnnual")
	}
	if len(missing) > 0 {
		out.Notes = []string{"TCO is incomplete. Missing customer figures: " + strings.Join(missing, ", ") + "."}
		out.Basis = model.BasisNotMeasured
		if inv.Storage != nil && inv.Storage.MeasuredHostCores > 0 {
			out.Notes = append(out.Notes, fmt.Sprintf("RVTools measured %d host cores. That count is not used as the contract core count until you set cores.", inv.Storage.MeasuredHostCores))
		}
		return out
	}
	cores := *a.Cores
	vmware := *a.VMwareAnnual
	storage := *a.StorageAnnual
	support := *a.SupportAnnual
	migration := 0.0
	if a.MigrationCost != nil {
		migration = *a.MigrationCost
	} else {
		out.Notes = append(out.Notes, "migrationCost was not provided and is treated as 0 on the customer side.")
	}
	zyvorAnnual := math.Max(float64(cores)*price, minimum)
	zyvorLicense := zyvorAnnual * float64(term)
	customer := (vmware+storage+support)*float64(term) + migration

	diskGiB := int64(0)
	for _, vm := range inv.VMs {
		for _, d := range vm.Disks {
			diskGiB += d.SizeGiB
		}
	}
	vms := len(inv.VMs)
	zyvorMigration := 0.0
	if a.IncludedDiskGiB == nil || *a.IncludedDiskGiB == 0 {
		out.Notes = append(out.Notes, "disk-size cap not provided; migration-services inclusion is incomplete. The Zyvor side does not add a migration figure.")
	} else if vms <= includedVMs && diskGiB <= *a.IncludedDiskGiB {
		out.Notes = append(out.Notes, fmt.Sprintf("Migration services in the three-year agreement cover the first %d VMs and %d GiB. This estate is inside that cap, so the customer migration figure is not added on the Zyvor side.", includedVMs, *a.IncludedDiskGiB))
	} else {
		vmFrac := 1.0
		if vms > 0 {
			vmFrac = float64(min(vms, includedVMs)) / float64(vms)
		}
		diskFrac := 1.0
		if diskGiB > 0 {
			diskFrac = float64(min64(diskGiB, *a.IncludedDiskGiB)) / float64(diskGiB)
		}
		covered := vmFrac * diskFrac
		if covered > 1 {
			covered = 1
		}
		zyvorMigration = migration * (1 - covered)
		out.Notes = append(out.Notes, "Migration services cover only the stated VM count and disk cap. The uncovered fraction of the customer migration figure is added on the Zyvor side.")
	}
	zyvorTotal := zyvorLicense + zyvorMigration
	delta := customer - zyvorTotal

	out.Complete = true
	out.Lines = []TCOLine{
		{"Customer cores", fmt.Sprintf("%d", cores), model.BasisMeasured},
		{"Zyvor price per core per year (assumption)", money(price), model.BasisEstimated},
		{"Zyvor minimum annual contract (assumption)", money(minimum), model.BasisEstimated},
		{"Zyvor annual license (max of cores x price and the minimum)", money(zyvorAnnual), model.BasisEstimated},
		{"Term years (assumption)", fmt.Sprintf("%d", term), model.BasisEstimated},
		{"Zyvor license for the term", money(zyvorLicense), model.BasisEstimated},
		{"Customer VMware renewal x term", money(vmware * float64(term)), model.BasisMeasured},
		{"Customer storage x term", money(storage * float64(term)), model.BasisMeasured},
		{"Customer support x term", money(support * float64(term)), model.BasisMeasured},
		{"Customer migration figure", money(migration), model.BasisMeasured},
		{"Zyvor migration services beyond the cap", money(zyvorMigration), model.BasisEstimated},
	}
	out.Customer = money(customer)
	out.Zyvor = money(zyvorTotal)
	out.Delta = money(delta)
	out.Notes = append(out.Notes, "The difference is arithmetic from the figures above. It is not a savings guarantee.")
	return out
}

func money(v float64) string {
	return fmt.Sprintf("%.2f", v)
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
