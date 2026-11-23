// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package inventory

import (
	"fmt"
	"net"
	"strings"

	"github.com/zyvorai/scout/internal/model"
)

// SchemaV1 is the marker of the documented generic inventory format.
const SchemaV1 = "scout-inventory/1"

// Limits applied to imported inventories.
const (
	MaxVMs        = 200000
	MaxDisksPerVM = 1000
	MaxNICsPerVM  = 256
	MaxConns      = 1000000
	MaxString     = 1024
)

// Problem is one finding from Lint. VM is the zero-based index of the VM it
// concerns, or -1 for inventory-level problems.
type Problem struct {
	VM       int    `json:"vm"`
	Path     string `json:"path"`
	Severity string `json:"severity"` // "error" or "warning"
	Message  string `json:"message"`
}

// Lint checks an inventory more strictly than Validate and reports every
// problem instead of stopping at the first. Errors make an import fail;
// warnings do not.
func Lint(inv model.Inventory) []Problem {
	var out []Problem
	add := func(vm int, sev, path, format string, args ...any) {
		out = append(out, Problem{VM: vm, Path: path, Severity: sev, Message: fmt.Sprintf(format, args...)})
	}
	if inv.Schema != "" && inv.Schema != SchemaV1 {
		add(-1, "error", "schema", "unsupported schema %q (want %q)", inv.Schema, SchemaV1)
	}
	if len(inv.VMs) == 0 {
		add(-1, "error", "vms", "inventory contains no VMs")
	}
	if len(inv.VMs) > MaxVMs {
		add(-1, "error", "vms", "inventory has %d VMs; the limit is %d", len(inv.VMs), MaxVMs)
		return out
	}
	if len(inv.Connections) > MaxConns {
		add(-1, "error", "connections", "inventory has %d connections; the limit is %d", len(inv.Connections), MaxConns)
		return out
	}
	long := func(vm int, path, v string) {
		if len(v) > MaxString {
			add(vm, "error", path, "value is %d bytes; the limit is %d", len(v), MaxString)
		}
	}
	ids := map[string]int{}
	for i, vm := range inv.VMs {
		p := fmt.Sprintf("vms[%d]", i)
		switch {
		case strings.TrimSpace(vm.ID) == "":
			add(i, "error", p+".id", "id is empty")
		default:
			if first, dup := ids[vm.ID]; dup {
				add(i, "error", p+".id", "duplicate id %q (first used by vms[%d])", vm.ID, first)
			} else {
				ids[vm.ID] = i
			}
		}
		if strings.TrimSpace(vm.Name) == "" {
			add(i, "error", p+".name", "name is empty")
		}
		long(i, p+".id", vm.ID)
		long(i, p+".name", vm.Name)
		long(i, p+".os", vm.OS)
		long(i, p+".uuid", vm.UUID)
		if vm.CPUs < 0 {
			add(i, "error", p+".cpus", "cpus is negative (%d)", vm.CPUs)
		}
		if vm.MemoryMiB < 0 {
			add(i, "error", p+".memoryMiB", "memoryMiB is negative (%d)", vm.MemoryMiB)
		}
		if vm.Snapshots < 0 {
			add(i, "error", p+".snapshots", "snapshots is negative (%d)", vm.Snapshots)
		}
		if vm.HWVersion < 0 {
			add(i, "error", p+".hwVersion", "hwVersion is negative (%d)", vm.HWVersion)
		}
		if len(vm.Disks) > MaxDisksPerVM {
			add(i, "error", p+".disks", "%d disks; the limit is %d", len(vm.Disks), MaxDisksPerVM)
		}
		if len(vm.NICs) > MaxNICsPerVM {
			add(i, "error", p+".nics", "%d NICs; the limit is %d", len(vm.NICs), MaxNICsPerVM)
		}
		for di, d := range vm.Disks {
			if di >= MaxDisksPerVM {
				break
			}
			dp := fmt.Sprintf("%s.disks[%d]", p, di)
			if d.SizeGiB < 0 {
				add(i, "error", dp+".sizeGiB", "sizeGiB is negative (%d)", d.SizeGiB)
			}
			long(i, dp+".backing", d.Backing)
			long(i, dp+".datastore", d.Datastore)
		}
		for ni, n := range vm.NICs {
			if ni >= MaxNICsPerVM {
				break
			}
			np := fmt.Sprintf("%s.nics[%d]", p, ni)
			if n.MAC != "" {
				switch c := model.CanonicalMAC(n.MAC); {
				case c == "":
					add(i, "error", np+".mac", "%q is not a MAC address (six hex octets)", clip(n.MAC))
				case model.IsPlaceholderMAC(c):
					add(i, "warning", np+".mac", "placeholder MAC %s is ignored by duplicate detection", c)
				}
			}
			if n.IP != "" && net.ParseIP(strings.TrimSpace(n.IP)) == nil {
				add(i, "error", np+".ip", "%q is not an IP address", clip(n.IP))
			}
			if n.VLAN < 0 || n.VLAN > 4094 {
				add(i, "error", np+".vlan", "vlan %d is outside 0-4094", n.VLAN)
			}
			long(i, np+".network", n.Network)
		}
	}
	for ci, c := range inv.Connections {
		p := fmt.Sprintf("connections[%d]", ci)
		if _, ok := ids[c.From]; !ok {
			add(-1, "error", p+".from", "%q is not a VM id", clip(c.From))
		}
		if _, ok := ids[c.To]; !ok {
			add(-1, "error", p+".to", "%q is not a VM id", clip(c.To))
		}
		if c.Port < 0 || c.Port > 65535 {
			add(-1, "error", p+".port", "port %d is outside 0-65535", c.Port)
		}
		if len(out) > 5000 {
			add(-1, "error", "connections", "too many problems; stopped checking")
			break
		}
	}
	return out
}

func clip(s string) string {
	if len(s) > 40 {
		return s[:40] + "..."
	}
	return s
}

// Errors returns only the error-severity problems.
func Errors(ps []Problem) []Problem {
	var out []Problem
	for _, p := range ps {
		if p.Severity == "error" {
			out = append(out, p)
		}
	}
	return out
}
