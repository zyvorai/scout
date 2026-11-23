// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package intake

import (
	"strings"
	"testing"
	"time"
)

func hasIssue(is []Issue, line int, sev, contains string) bool {
	for _, i := range is {
		if (line < 0 || i.Line == line) && i.Severity == sev && strings.Contains(i.String(), contains) {
			return true
		}
	}
	return false
}

func TestFromCSVValid(t *testing.T) {
	in := "\xef\xbb\xbfName,ID,OS,CPUs,memory_mib,cbt_enabled,tpm,disk_gib,disk_datastores,disk_backings,networks,macs,ips,x_owner\n" +
		"app-1,vm-1,RHEL 9,4,8192,yes,no,100;50,ds1;ds1,[ds1] a.vmdk;[ds1] b.vmdk,prod;mgmt,00:50:56:aa:aa:aa;00-50-56-aa-aa-ab,10.0.0.1;,alice\n" +
		"\n" +
		"db-1,,Ubuntu 24.04,8,16384,,true,500,,,prod,,,bob\n"
	inv, is := FromCSV(strings.NewReader(in), time.Unix(0, 0))
	if HasErrors(is) {
		t.Fatal(is)
	}
	if inv.Schema != "scout-inventory/1" || len(inv.VMs) != 2 {
		t.Fatalf("%+v", inv)
	}
	a, b := inv.VMs[0], inv.VMs[1]
	if a.CBTEnabled == nil || !*a.CBTEnabled || len(a.Disks) != 2 || a.Disks[1].Backing != "[ds1] b.vmdk" || len(a.NICs) != 2 || a.NICs[1].MAC != "00-50-56-aa-aa-ab" || a.NICs[1].IP != "" {
		t.Fatalf("%+v", a)
	}
	if b.ID != "db-1" || b.CBTEnabled != nil || !b.TPM || b.Disks[0].SizeGiB != 500 {
		t.Fatalf("%+v", b)
	}
}

func TestFromCSVErrorsAreLineNumbered(t *testing.T) {
	tests := []struct {
		name string
		in   string
		line int
		want string
	}{
		{"unknown column", "name,colour\na,red\n", 1, "unknown column"},
		{"missing name column", "id,os\na,b\n", 1, "required column"},
		{"duplicate column", "name,name\na,b\n", 1, "more than once"},
		{"empty file", "", 1, "empty"},
		{"header only", "name\n", 0, "no VM rows"},
		{"bad int", "name,cpus\na,1\nb,four\n", 3, "cpus"},
		{"negative int", "name,cpus\na,-2\n", 2, "cpus"},
		{"huge int", "name,memory_mib\na,99999999999999999999999\n", 2, "memory_mib"},
		{"bad bool", "name,tpm\na,maybe\n", 2, "not a boolean"},
		{"bad cbt", "name,cbt_enabled\na,2\n", 2, "not a boolean"},
		{"short row", "name,os,cpus\na,x\n", 2, "2 fields but the header has 3"},
		{"long row", "name\na,b\n", 2, "2 fields"},
		{"unterminated quote", "name,os\na,\"x\nb,y\n", 2, "quote"},
		{"empty name", "name,os\n,RHEL\n", 2, "name is empty"},
		{"duplicate id", "name\nx\ny\nx\n", 4, "duplicate id"},
		{"bad mac", "name,networks,macs\na,p,zz:zz\n", 2, "not a MAC"},
		{"bad ip", "name,networks,ips\na,p,300.1.1.1\n", 2, "not an IP"},
		{"list length mismatch", "name,disk_gib,disk_datastores\na,1;2,ds\n", 2, "same number of disks"},
		{"datastores without sizes", "name,disk_datastores\na,ds\n", 2, "disk_gib"},
		{"nic list mismatch", "name,networks,macs\na,p;q,00:50:56:aa:aa:aa\n", 2, "same number of NICs"},
		{"bad disk size", "name,disk_gib\na,big\n", 2, "not a size"},
		{"negative disk", "name,disk_gib\na,-5\n", 2, "not a size"},
		{"oversize field", "name\n" + strings.Repeat("a", MaxCSVField+1) + "\n", 2, "limit"},
		{"invalid utf8", "name\n\xff\xfe\n", 0, "UTF-8"},
		{"too many columns", strings.Repeat("x_a,", MaxCSVColumns) + "name\na\n", 1, "columns"},
		{"too many disks", "name,disk_gib\na," + strings.Repeat("1;", 1000) + "1\n", 2, "disks"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, is := FromCSV(strings.NewReader(tt.in), time.Time{})
			if !hasIssue(is, tt.line, "error", tt.want) {
				t.Fatalf("want line %d %q in %v", tt.line, tt.want, is)
			}
		})
	}
}

func TestFromCSVStopsReporting(t *testing.T) {
	var b strings.Builder
	b.WriteString("name,cpus\n")
	for i := 0; i < 1000; i++ {
		b.WriteString("a,bad\n")
	}
	_, is := FromCSV(strings.NewReader(b.String()), time.Time{})
	if len(is) > MaxIssues+1 || !hasIssue(is, 0, "error", "stopped reporting") {
		t.Fatalf("%d issues", len(is))
	}
}

func TestFromCSVSizeCap(t *testing.T) {
	_, is := FromCSV(strings.NewReader("name\n"+strings.Repeat("a\n", MaxGenericBytes/2+1)), time.Time{})
	if !hasIssue(is, 0, "error", "exceeds") {
		t.Fatalf("%v", is[:1])
	}
}

func TestFromCSVPlaceholderMACIsWarning(t *testing.T) {
	inv, is := FromCSV(strings.NewReader("name,networks,macs\na,p,00:00:00:00:00:00\n"), time.Time{})
	if HasErrors(is) || !hasIssue(is, 2, "warning", "placeholder") || len(inv.VMs) != 1 {
		t.Fatal(is)
	}
}

const goodJSON = `{
  "schema": "scout-inventory/1",
  "source": "x",
  "environment": "e",
  "vms": [
    {"id": "a", "name": "a", "cpus": 1},
    {"id": "b", "name": "b"}
  ]
}`

func TestFromScoutJSON(t *testing.T) {
	inv, is := FromScoutJSON(strings.NewReader(goodJSON))
	if HasErrors(is) || len(inv.VMs) != 2 {
		t.Fatal(is)
	}
	tests := []struct {
		name string
		in   string
		line int
		want string
	}{
		{"missing schema", strings.Replace(goodJSON, `"schema": "scout-inventory/1",`, "", 1), 1, "missing"},
		{"wrong schema", strings.Replace(goodJSON, "/1", "/2", 1), 0, "unsupported schema"},
		{"syntax", "{\n  \"schema\": \"scout-inventory/1\",\n  \"vms\": [,]\n}", 3, "invalid character"},
		{"type", "{\n\"schema\":\"scout-inventory/1\",\n\"vms\":[{\"id\":\"a\",\"name\":\"a\",\n\"cpus\":\"many\"}]}", 4, "cpus"},
		{"unknown field", "{\n\"schema\":\"scout-inventory/1\",\n\"vms\":[{\"id\":\"a\",\"name\":\"a\",\n\"mystery\":1}]}", 4, "mystery"},
		{"truncated", "{\n\"schema\":\"scout-inventory/1\",\n\"vms\":[", 3, "unexpected EOF"},
		{"trailing", goodJSON + "\n{}", 10, "unexpected data"},
		{"empty", "", 1, "EOF"},
		{"duplicate id", strings.Replace(goodJSON, `"id": "b"`, `"id": "a"`, 1), 7, "duplicate id"},
		{"empty name", strings.Replace(goodJSON, `"name": "b"`, `"name": " "`, 1), 7, "name is empty"},
		{"negative", strings.Replace(goodJSON, `"cpus": 1`, `"cpus": -1`, 1), 6, "negative"},
		{"bad mac", strings.Replace(goodJSON, `{"id": "b", "name": "b"}`, `{"id":"b","name":"b","nics":[{"id":"n","mac":"nope"}]}`, 1), 7, "not a MAC"},
		{"bad vlan", strings.Replace(goodJSON, `{"id": "b", "name": "b"}`, `{"id":"b","name":"b","nics":[{"id":"n","vlan":5000}]}`, 1), 7, "vlan"},
		{"connection to nowhere", strings.Replace(goodJSON, `"vms"`, `"connections":[{"from":"a","to":"zz","protocol":"tcp","port":1}],"vms"`, 1), 0, "not a VM id"},
		{"no vms", `{"schema":"scout-inventory/1","vms":[]}`, 0, "no VMs"},
		{"oversize", goodJSON + strings.Repeat(" ", MaxGenericBytes), 0, "exceeds"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, is := FromScoutJSON(strings.NewReader(tt.in))
			if !hasIssue(is, tt.line, "error", tt.want) {
				t.Fatalf("want line %d %q in %v", tt.line, tt.want, is)
			}
		})
	}
}
