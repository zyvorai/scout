// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package model

import (
	"reflect"
	"testing"
)

func TestSharedDiskGroups(t *testing.T) {
	d := func(ds, b string) []Disk { return []Disk{{Datastore: ds, Backing: b}} }
	tests := []struct {
		name   string
		vms    []VM
		shared bool
	}{
		{"none", []VM{{ID: "a", Disks: d("ds1", "[ds1] a.vmdk")}, {ID: "b", Disks: d("ds1", "[ds1] b.vmdk")}}, false},
		{"same path, case and space insensitive", []VM{{ID: "b", Disks: d("DS1", "[ds1] s.vmdk")}, {ID: "a", Disks: d("ds1", " [DS1] S.vmdk ")}}, true},
		{"empty backing never groups", []VM{{ID: "a", Disks: d("ds1", "")}, {ID: "b", Disks: d("ds1", "")}}, false},
		{"same path other datastore", []VM{{ID: "a", Disks: d("ds1", "x.vmdk")}, {ID: "b", Disks: d("ds2", "x.vmdk")}}, false},
		{"one VM twice is not a group", []VM{{ID: "a", Disks: append(d("ds1", "x"), d("ds1", "x")...)}}, false},
	}
	for _, tt := range tests {
		got := SharedDiskGroups(Inventory{VMs: tt.vms})
		if tt.shared {
			if len(got) != 1 || !reflect.DeepEqual(got[0].VMs, []string{"a", "b"}) {
				t.Errorf("%s: %+v", tt.name, got)
			}
		} else if len(got) != 0 {
			t.Errorf("%s: %+v", tt.name, got)
		}
	}
}
