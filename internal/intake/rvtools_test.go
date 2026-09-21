// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package intake

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zyvorai/scout/internal/model"
)

func TestFromRVToolsMapsSheets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rvtools.xlsx")
	writeXLSX(t, path, map[string][][]string{
		"vInfo": {
			{"VM", "VM UUID", "Powerstate", "Template", "CPUs", "Memory MB", "OS according to the VMware Tools", "Firmware", "Datacenter"},
			{"web-01", "uuid-web", "poweredOn", "False", "4", "8192", "Windows Server 2022", "efi", "DC1"},
			{"db-01", "uuid-db", "poweredOn", "False", "8", "16384", "RHEL 9", "uefi", "DC1"},
			{"template-01", "uuid-t", "poweredOff", "True", "2", "4096", "RHEL 9", "bios", "DC1"},
		},
		"vDisk": {
			{"VM", "Disk", "Capacity MB", "Disk Mode", "Datastore"},
			{"web-01", "Hard disk 1", "40960", "persistent", "ds-prod"},
			{"db-01", "Hard disk 1", "102400", "persistent", "ds-prod"},
		},
		"vSnapshot": {
			{"VM", "Name", "Date / time UTC", "Size MB (total)"},
			{"web-01", "before-patch", "2026-01-01 00:00:00", "2048"},
			{"missing-vm", "leftover", "2026-01-02 00:00:00", "512"},
		},
		"vNetwork": {
			{"VM", "Network", "MAC Address", "IP Address"},
			{"web-01", "prod-app", "00:50:56:aa:bb:cc", "10.0.0.5"},
		},
		"vDatastore": {
			{"Name", "Capacity MB", "Free MB", "In Use MB"},
			{"ds-prod", "1048576", "524288", "524288"},
		},
		"vHost": {
			{"Host", "# Cores"},
			{"esx-01", "32"},
		},
		"vPartition": {
			{"VM", "Disk", "Capacity MB", "Free MB"},
			{"web-01", "Hard disk 1", "40960", "10240"},
		},
	})
	now := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	inv, err := FromRVTools(path, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.VMs) != 2 {
		t.Fatalf("templates should be skipped, got %d vms", len(inv.VMs))
	}
	if inv.VMs[0].OS != "Windows Server 2022" || inv.VMs[0].CPUs != 4 || inv.VMs[0].MemoryMiB != 8192 {
		t.Fatalf("web vm: %+v", inv.VMs[0])
	}
	if inv.VMs[0].Disks[0].SizeGiB != 40 {
		t.Fatalf("disk size: %+v", inv.VMs[0].Disks)
	}
	if len(inv.VMs[0].SnapshotDetails) != 1 || !inv.VMs[0].SnapshotDetails[0].Aged || inv.VMs[0].SnapshotDetails[0].SizeBytes == 0 {
		t.Fatalf("snapshot: %+v", inv.VMs[0].SnapshotDetails)
	}
	if len(inv.OrphanSnapshots) != 1 || !inv.OrphanSnapshots[0].Orphan {
		t.Fatalf("orphans: %+v", inv.OrphanSnapshots)
	}
	if inv.Storage == nil || inv.Storage.MeasuredHostCores != 32 || len(inv.Storage.Datastores) != 1 {
		t.Fatalf("storage: %+v", inv.Storage)
	}
	if len(inv.VMs[0].NICs) != 1 || inv.VMs[0].NICs[0].IP != "10.0.0.5" {
		t.Fatalf("nic: %+v", inv.VMs[0].NICs)
	}
	if len(inv.VMs[0].Partitions) != 1 {
		t.Fatalf("partition: %+v", inv.VMs[0].Partitions)
	}
}

func TestMergeDRAndAssumptions(t *testing.T) {
	dir := t.TempDir()
	drPath := filepath.Join(dir, "dr.yaml")
	os.WriteFile(drPath, []byte("lastSuccessfulBackup: 2026-09-20T00:00:00Z\ntargetRPOMinutes: 15\nlastTestRestore: 2026-01-01T00:00:00Z\noffsiteCopy: true\n"), 0o600)
	aPath := filepath.Join(dir, "assumptions.json")
	os.WriteFile(aPath, []byte(`{"cores":64,"vmwareAnnual":100000,"storageAnnual":20000,"supportAnnual":10000,"linkMbps":1000}`), 0o600)
	sPath := filepath.Join(dir, "storage.json")
	os.WriteFile(sPath, []byte(`{"ceph":{"pools":[{"name":"rbd","usedBytes":10,"maxBytes":100}]},"growthBytesPerDay":10}`), 0o600)
	inv := model.Inventory{VMs: []model.VM{{ID: "a", Name: "a"}}}
	if err := MergeDR(&inv, drPath); err != nil {
		t.Fatal(err)
	}
	if inv.DR == nil || inv.DR.TargetRPOMinutes == nil || *inv.DR.TargetRPOMinutes != 15 {
		t.Fatalf("dr: %+v", inv.DR)
	}
	if err := MergeAssumptions(&inv, aPath); err != nil {
		t.Fatal(err)
	}
	if inv.Assumptions == nil || inv.Assumptions.Cores == nil || *inv.Assumptions.Cores != 64 {
		t.Fatalf("assumptions: %+v", inv.Assumptions)
	}
	if err := MergeStorage(&inv, sPath); err != nil {
		t.Fatal(err)
	}
	if len(inv.Storage.CephPools) != 1 || inv.Storage.GrowthBytesPerDay != 10 {
		t.Fatalf("storage merge: %+v", inv.Storage)
	}
}

func TestDRRejectsNestedYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dr.yaml")
	os.WriteFile(path, []byte("jobs:\n  - name: veeam\n"), 0o600)
	inv := model.Inventory{VMs: []model.VM{{ID: "a", Name: "a"}}}
	if err := MergeDR(&inv, path); err == nil {
		t.Fatal("expected nested yaml to be rejected")
	}
}

func writeXLSX(t *testing.T, path string, sheets map[string][][]string) {
	t.Helper()
	var shared []string
	index := map[string]int{}
	intern := func(s string) int {
		if n, ok := index[s]; ok {
			return n
		}
		n := len(shared)
		shared = append(shared, s)
		index[s] = n
		return n
	}
	order := []string{"vInfo", "vDisk", "vSnapshot", "vHost", "vDatastore", "vPartition", "vNetwork"}
	var present []string
	for _, name := range order {
		if _, ok := sheets[name]; ok {
			present = append(present, name)
		}
	}
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	var wbSheets, rels stringsBuilder
	wbSheets.WriteString(`<?xml version="1.0" encoding="UTF-8"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>`)
	rels.WriteString(`<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for i, name := range present {
		id := i + 1
		wbSheets.WriteString(`<sheet name="` + name + `" sheetId="` + itoa(id) + `" r:id="rId` + itoa(id) + `"/>`)
		rels.WriteString(`<Relationship Id="rId` + itoa(id) + `" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet` + itoa(id) + `.xml"/>`)
		var sheet bytes.Buffer
		sheet.WriteString(`<?xml version="1.0" encoding="UTF-8"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
		for r, row := range sheets[name] {
			sheet.WriteString(`<row r="` + itoa(r+1) + `">`)
			for c, val := range row {
				ref := colName(c) + itoa(r+1)
				sheet.WriteString(`<c r="` + ref + `" t="s"><v>` + itoa(intern(val)) + `</v></c>`)
			}
			sheet.WriteString(`</row>`)
		}
		sheet.WriteString(`</sheetData></worksheet>`)
		w, err := zw.Create("xl/worksheets/sheet" + itoa(id) + ".xml")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(sheet.Bytes()); err != nil {
			t.Fatal(err)
		}
	}
	wbSheets.WriteString(`</sheets></workbook>`)
	rels.WriteString(`</Relationships>`)
	mustZip(t, zw, "xl/workbook.xml", wbSheets.String())
	mustZip(t, zw, "xl/_rels/workbook.xml.rels", rels.String())
	var sst bytes.Buffer
	sst.WriteString(`<?xml version="1.0" encoding="UTF-8"?><sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	for _, s := range shared {
		sst.WriteString(`<si><t>` + xmlEscape(s) + `</t></si>`)
	}
	sst.WriteString(`</sst>`)
	mustZip(t, zw, "xl/sharedStrings.xml", sst.String())
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

type stringsBuilder struct{ bytes.Buffer }

func mustZip(t *testing.T, zw *zip.Writer, name, body string) {
	t.Helper()
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
}

func colName(i int) string {
	i++
	var out []byte
	for i > 0 {
		i--
		out = append([]byte{byte('A' + i%26)}, out...)
		i /= 26
	}
	return string(out)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func xmlEscape(s string) string {
	s = bytesToString(bytes.ReplaceAll([]byte(s), []byte(`&`), []byte(`&amp;`)))
	s = bytesToString(bytes.ReplaceAll([]byte(s), []byte(`<`), []byte(`&lt;`)))
	return s
}

func bytesToString(b []byte) string { return string(b) }
