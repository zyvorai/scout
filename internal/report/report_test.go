// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"github.com/zyvorai/scout/internal/model"
)

func TestExecutivePDFAndWorkbook(t *testing.T) {
	cores := 32
	vmware, storage, support := 50000.0, 10000.0, 5000.0
	inv := model.Inventory{
		Source: "rvtools", Environment: "Plant A",
		VMs:         []model.VM{{ID: "a", Name: "app", OS: "RHEL 9", Firmware: "uefi", ToolsStatus: "running", Disks: []model.Disk{{ID: "d", SizeGiB: 20}}}},
		Assumptions: &model.Assumptions{Cores: &cores, VMwareAnnual: &vmware, StorageAnnual: &storage, SupportAnnual: &support},
	}
	data := Build(inv)
	var pdf bytes.Buffer
	if err := WritePDF(&pdf, data); err != nil {
		t.Fatal(err)
	}
	text := pdf.String()
	if !strings.HasPrefix(text, "%PDF-1.4") || !strings.Contains(text, "Target architecture") || strings.Contains(text, "70%") {
		t.Fatalf("pdf start %q contains70 %v", text[:20], strings.Contains(text, "70%"))
	}
	var book bytes.Buffer
	if err := WriteWorkbook(&book, data); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(book.Bytes()), int64(book.Len()))
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, f := range zr.File {
		found[f.Name] = true
	}
	for _, name := range []string{"vms.csv", "disks.csv", "findings.csv", "waves.csv", "tco.csv"} {
		if !found[name] {
			t.Fatalf("missing %s in %v", name, found)
		}
	}
}
