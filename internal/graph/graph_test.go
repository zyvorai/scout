package graph

import (
	"github.com/zyvorai/scout/internal/model"
	"testing"
)

func TestAssignWavesKeepsDependenciesTogether(t *testing.T) {
	inv := model.Inventory{VMs: []model.VM{{ID: "a", Name: "a"}, {ID: "b", Name: "b"}, {ID: "c", Name: "c"}, {ID: "d", Name: "d"}}, Connections: []model.Connection{{From: "a", To: "b"}, {From: "c", To: "d"}}}
	a := []model.Assessment{{VMID: "a", Status: "ready"}, {VMID: "b", Status: "review"}, {VMID: "c", Status: "ready"}, {VMID: "d", Status: "blocked"}}
	a = AssignWaves(inv, a)
	by := map[string]int{}
	for _, x := range a {
		by[x.VMID] = x.Wave
	}
	if by["a"] == 0 || by["a"] != by["b"] {
		t.Fatalf("a/b should share wave: %#v", by)
	}
	if by["c"] == 0 || by["c"] == by["a"] {
		t.Fatalf("c should be separate wave: %#v", by)
	}
	if by["d"] != 0 {
		t.Fatalf("blocked VM must have wave 0: %#v", by)
	}
}
