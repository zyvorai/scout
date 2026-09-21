// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"archive/zip"
	"encoding/csv"
	"fmt"
	"io"
)

// WriteWorkbook writes a zip of CSV tables. It is the technical workbook.
func WriteWorkbook(w io.Writer, data Data) error {
	zw := zip.NewWriter(w)
	files := []struct {
		name string
		rows [][]string
	}{
		{"vms.csv", vmRows(data)},
		{"disks.csv", diskRows(data)},
		{"findings.csv", findingRows(data)},
		{"waves.csv", waveRows(data)},
		{"snapshots.csv", snapshotRows(data)},
		{"tco.csv", tcoRows(data)},
		{"storage.csv", storageRows(data)},
		{"dr.csv", drRows(data)},
	}
	for _, file := range files {
		fw, err := zw.Create(file.name)
		if err != nil {
			return err
		}
		cw := csv.NewWriter(fw)
		if err := cw.WriteAll(file.rows); err != nil {
			return fmt.Errorf("%s: %w", file.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("workbook: %w", err)
	}
	return nil
}

func vmRows(data Data) [][]string {
	rows := [][]string{{"id", "name", "os", "cpus", "memoryMiB", "powerState", "status", "score", "wave", "estimatedCutoverMinutes", "cutoverBasis"}}
	byID := map[string]int{}
	for i, a := range data.Assessments {
		byID[a.VMID] = i
	}
	for _, vm := range data.Inventory.VMs {
		a := data.Assessments[byID[vm.ID]]
		rows = append(rows, []string{
			vm.ID, vm.Name, vm.OS, fmt.Sprint(vm.CPUs), fmt.Sprint(vm.MemoryMiB), vm.PowerState,
			a.Status, fmt.Sprint(a.Score), fmt.Sprint(a.Wave), fmt.Sprint(a.EstimatedCutoverMinutes), a.CutoverBasis,
		})
	}
	return rows
}

func diskRows(data Data) [][]string {
	rows := [][]string{{"vm", "disk", "sizeGiB", "format", "datastore", "rdm", "shared"}}
	for _, vm := range data.Inventory.VMs {
		for _, d := range vm.Disks {
			rows = append(rows, []string{vm.Name, d.ID, fmt.Sprint(d.SizeGiB), d.Format, d.Datastore, fmt.Sprint(d.RDM), fmt.Sprint(d.Shared)})
		}
	}
	return rows
}

func findingRows(data Data) [][]string {
	rows := [][]string{{"vm", "rule", "severity", "basis", "title", "detail"}}
	for _, a := range data.Assessments {
		for _, f := range a.Findings {
			rows = append(rows, []string{a.VMName, f.RuleID, string(f.Severity), f.Basis, f.Title, f.Detail})
		}
	}
	return rows
}

func waveRows(data Data) [][]string {
	rows := [][]string{{"wave", "vm", "status", "estimatedCutoverMinutes", "basis"}}
	for _, a := range data.Assessments {
		rows = append(rows, []string{fmt.Sprint(a.Wave), a.VMName, a.Status, fmt.Sprint(a.EstimatedCutoverMinutes), a.CutoverBasis})
	}
	return rows
}

func snapshotRows(data Data) [][]string {
	rows := [][]string{{"vm", "name", "createdAt", "sizeBytes", "aged", "orphan"}}
	for _, vm := range data.Inventory.VMs {
		for _, s := range vm.SnapshotDetails {
			rows = append(rows, []string{vm.Name, s.Name, s.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"), fmt.Sprint(s.SizeBytes), fmt.Sprint(s.Aged), fmt.Sprint(s.Orphan)})
		}
	}
	for _, s := range data.Inventory.OrphanSnapshots {
		rows = append(rows, []string{"", s.Name, s.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"), fmt.Sprint(s.SizeBytes), fmt.Sprint(s.Aged), fmt.Sprint(s.Orphan)})
	}
	return rows
}

func tcoRows(data Data) [][]string {
	rows := [][]string{{"label", "amount", "basis"}}
	for _, line := range data.Estate.TCO.Lines {
		rows = append(rows, []string{line.Label, line.Amount, line.Basis})
	}
	if data.Estate.TCO.Customer != "" {
		rows = append(rows,
			[]string{"Customer three-year total", data.Estate.TCO.Customer, "measured inputs"},
			[]string{"Zyvor three-year total", data.Estate.TCO.Zyvor, "estimated"},
			[]string{"Arithmetic difference", data.Estate.TCO.Delta, "estimated"},
		)
	}
	for _, note := range data.Estate.TCO.Notes {
		rows = append(rows, []string{"note", note, data.Estate.TCO.Basis})
	}
	return rows
}

func storageRows(data Data) [][]string {
	rows := [][]string{{"kind", "name", "bytes", "note"}}
	if data.Inventory.Storage == nil {
		return rows
	}
	st := data.Inventory.Storage
	for _, ds := range st.Datastores {
		rows = append(rows, []string{"datastore", ds.Name, fmt.Sprint(ds.FreeBytes), "free bytes"})
	}
	for _, p := range st.CephPools {
		rows = append(rows, []string{"ceph", p.Name, fmt.Sprint(p.MaxBytes - p.UsedBytes), "free bytes"})
	}
	for _, e := range st.NFSExports {
		rows = append(rows, []string{"nfs", e.Server + ":" + e.Path, fmt.Sprint(e.SizeBytes - e.UsedBytes), "free bytes"})
	}
	for _, d := range st.ZFSDatasets {
		rows = append(rows, []string{"zfs", d.Name, fmt.Sprint(d.AvailBytes), "available bytes"})
	}
	for _, row := range data.Estate.Runway {
		rows = append(rows, []string{"runway", row.Source, row.Days, row.Basis})
	}
	return rows
}

func drRows(data Data) [][]string {
	rows := [][]string{{"field", "value"}}
	rows = append(rows, []string{"score", data.Estate.DR.Score}, []string{"basis", data.Estate.DR.Basis}, []string{"detail", data.Estate.DR.Detail})
	if data.Inventory.DR != nil {
		dr := data.Inventory.DR
		if dr.LastSuccessfulBackup != nil {
			rows = append(rows, []string{"lastSuccessfulBackup", dr.LastSuccessfulBackup.UTC().Format("2006-01-02T15:04:05Z")})
		}
		if dr.LastTestRestore != nil {
			rows = append(rows, []string{"lastTestRestore", dr.LastTestRestore.UTC().Format("2006-01-02T15:04:05Z")})
		}
		if dr.TargetRPOMinutes != nil {
			rows = append(rows, []string{"targetRPOMinutes", fmt.Sprint(*dr.TargetRPOMinutes)})
		}
		if dr.OffsiteCopy != nil {
			rows = append(rows, []string{"offsiteCopy", fmt.Sprint(*dr.OffsiteCopy)})
		}
	}
	return rows
}
