// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package model

import (
	"sort"
	"strings"
)

// SharedDiskGroup is a set of VMs that reference one backing disk file.
type SharedDiskGroup struct {
	Key string   `json:"key"`
	VMs []string `json:"vms"` // VM ids, sorted
}

// SharedDiskGroups returns the groups of two or more VMs whose disks have the
// same datastore and backing path. Disks without a backing path never form a
// group: Scout does not guess sharing from sizes or names. The result is
// sorted by key so callers are deterministic.
func SharedDiskGroups(inv Inventory) []SharedDiskGroup {
	byKey := map[string]map[string]bool{}
	for _, vm := range inv.VMs {
		for _, d := range vm.Disks {
			b := strings.ToLower(strings.TrimSpace(d.Backing))
			if b == "" {
				continue
			}
			key := b
			if ds := strings.ToLower(strings.TrimSpace(d.Datastore)); ds != "" {
				key = ds + "|" + b
			}
			if byKey[key] == nil {
				byKey[key] = map[string]bool{}
			}
			byKey[key][vm.ID] = true
		}
	}
	keys := make([]string, 0, len(byKey))
	for k, vms := range byKey {
		if len(vms) > 1 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	out := make([]SharedDiskGroup, 0, len(keys))
	for _, k := range keys {
		g := SharedDiskGroup{Key: k}
		for id := range byKey[k] {
			g.VMs = append(g.VMs, id)
		}
		sort.Strings(g.VMs)
		out = append(out, g)
	}
	return out
}
