// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package plan

import (
	"strings"
	"testing"

	"github.com/zyvorai/scout/internal/model"
)

func bp(b bool) *bool { return &b }

func vm(id string, gib int64, cbt *bool) model.VM {
	return model.VM{ID: id, Name: id, CBTEnabled: cbt, Disks: []model.Disk{{ID: "d", SizeGiB: gib}}}
}

func asm(vms ...model.VM) (model.Inventory, []model.Assessment) {
	inv := model.Inventory{VMs: vms}
	var a []model.Assessment
	for _, v := range vms {
		a = append(a, model.Assessment{VMID: v.ID, VMName: v.Name, Status: "ready", Wave: 1})
	}
	return inv, a
}

func TestCopyMinutes(t *testing.T) {
	// 100 GiB over 1000 Mbps at 100% = 858.99 s = 14.3 min -> 15.
	if got := copyMinutes(100, 1000, 1); got != 15 {
		t.Fatalf("got %d", got)
	}
	if copyMinutes(100, 0, 1) != 0 || copyMinutes(0, 1000, 1) != 0 {
		t.Fatal("unknown link or no data must give 0")
	}
}

func TestWarmVersusColdDowntime(t *testing.T) {
	inv, a := asm(vm("cold", 100, nil))
	c := Constraints{LinkMbps: 1000, Efficiency: 1, CutoverMinutes: 10, DeltaPercent: 5}
	cold, err := Build(inv, a, c)
	if err != nil {
		t.Fatal(err)
	}
	inv2, a2 := asm(vm("warm", 100, bp(true)))
	warm, _ := Build(inv2, a2, c)
	if cold.Waves[0].DowntimeMinutes != 10+15 {
		t.Fatalf("cold downtime %d", cold.Waves[0].DowntimeMinutes)
	}
	// 5 GiB delta = 43 s -> 1 min.
	if warm.Waves[0].DowntimeMinutes != 11 || warm.Waves[0].WarmVMs != 1 {
		t.Fatalf("warm %+v", warm.Waves[0])
	}
}

func TestMaxConcurrentAndExclusiveGroups(t *testing.T) {
	inv, a := asm(vm("a", 10, nil), vm("b", 10, nil), vm("c", 10, nil), vm("d", 10, nil), vm("e", 10, nil))
	p, err := Build(inv, a, Constraints{MaxConcurrent: 2, LinkMbps: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(p.Waves[0].Batches); got != 3 {
		t.Fatalf("batches=%d want 3 (2+2+1)", got)
	}
	p, _ = Build(inv, a, Constraints{MaxConcurrent: 5, LinkMbps: 1000, ExclusiveGroups: [][]string{{"a", "B"}, {"c", "d", "e"}}})
	if got := len(p.Waves[0].Batches); got != 3 {
		t.Fatalf("exclusive batches=%d want 3 (c,d,e cannot share)", got)
	}
	for _, b := range p.Waves[0].Batches {
		seen := map[string]bool{}
		for _, n := range b.VMs {
			seen[n] = true
		}
		if (seen["c"] && seen["d"]) || (seen["c"] && seen["e"]) || (seen["d"] && seen["e"]) || (seen["a"] && seen["b"]) {
			t.Fatalf("exclusive VMs share batch %+v", b)
		}
	}
}

func TestBlockedSkippedAndUnknownLink(t *testing.T) {
	inv, a := asm(vm("a", 10, nil), vm("b", 10, nil))
	a[1].Status, a[1].Wave = "blocked", 0
	p, err := Build(inv, a, Constraints{ExclusiveGroups: [][]string{{"a", "ghost"}}})
	if err != nil {
		t.Fatal(err)
	}
	if p.Totals.Skipped != 1 || p.Totals.VMs != 1 {
		t.Fatalf("totals %+v", p.Totals)
	}
	if p.Basis != model.BasisNotMeasured || p.Waves[0].TransferMinutes != 0 {
		t.Fatalf("basis %s wave %+v", p.Basis, p.Waves[0])
	}
	joined := strings.Join(p.Warnings, "|")
	if !strings.Contains(joined, "linkMbps") || !strings.Contains(joined, "ghost") {
		t.Fatalf("warnings %v", p.Warnings)
	}
}

func TestWindows(t *testing.T) {
	cases := []struct {
		d    []int
		w    int
		want int
	}{
		{nil, 100, 0},
		{[]int{30, 30, 30}, 100, 1},
		{[]int{60, 60}, 100, 2},
		{[]int{250}, 100, 3},
		{[]int{50, 250, 50}, 100, 5},
		{[]int{0, 0}, 100, 1},
	}
	for _, tc := range cases {
		if got := windows(tc.d, tc.w); got != tc.want {
			t.Errorf("windows(%v,%d)=%d want %d", tc.d, tc.w, got, tc.want)
		}
	}
}

func TestNormalizeRejectsBadValues(t *testing.T) {
	for _, c := range []Constraints{{MaxConcurrent: -1}, {Efficiency: 1.5}, {DeltaPercent: 101}, {LinkMbps: -5}} {
		if _, err := c.Normalize(); err == nil {
			t.Errorf("%+v should be rejected", c)
		}
	}
	c, err := Constraints{}.Normalize()
	if err != nil || c.MaxConcurrent != DefaultMaxConcurrent || c.WindowMinutes != DefaultWindowMinutes || c.Efficiency != DefaultEfficiency {
		t.Fatalf("defaults %+v %v", c, err)
	}
}

func TestOversizedBatchWarns(t *testing.T) {
	inv, a := asm(vm("big", 10000, nil))
	p, _ := Build(inv, a, Constraints{LinkMbps: 1000, WindowMinutes: 60})
	if len(p.Warnings) == 0 || p.Waves[0].Windows < 2 {
		t.Fatalf("%+v", p)
	}
}
