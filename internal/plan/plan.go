// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

// Package plan turns assessed waves into a downtime and effort schedule under
// operator constraints. It is arithmetic over the inventory, not a measurement.
package plan

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/zyvorai/scout/internal/model"
)

const (
	DefaultMaxConcurrent  = 4
	DefaultWindowMinutes  = 240
	DefaultEfficiency     = 0.7
	DefaultCutoverMinutes = 15
	DefaultDeltaPercent   = 5.0
)

// Constraints are the operator inputs. Zero values select defaults, except
// LinkMbps where zero means unknown and copy time is not estimated.
type Constraints struct {
	MaxConcurrent   int        `json:"maxConcurrent,omitempty"`   // VMs cut over in parallel
	WindowMinutes   int        `json:"windowMinutes,omitempty"`   // length of one maintenance window
	LinkMbps        float64    `json:"linkMbps,omitempty"`        // usable migration link
	Efficiency      float64    `json:"efficiency,omitempty"`      // fraction of the link actually achieved, (0,1]
	CutoverMinutes  int        `json:"cutoverMinutes,omitempty"`  // fixed per-VM stop/start/verify overhead
	DeltaPercent    float64    `json:"deltaPercent,omitempty"`    // final warm sync as a percent of data
	ExclusiveGroups [][]string `json:"exclusiveGroups,omitempty"` // VM ids or names that must not cut over together
}

type Batch struct {
	Index           int      `json:"index"`
	VMs             []string `json:"vms"`
	DataGiB         int64    `json:"dataGiB"`
	DowntimeMinutes int      `json:"downtimeMinutes"`
}

type Wave struct {
	Wave            int     `json:"wave"`
	VMs             int     `json:"vms"`
	WarmVMs         int     `json:"warmVMs"`
	ColdVMs         int     `json:"coldVMs"`
	DataGiB         int64   `json:"dataGiB"`
	TransferMinutes int     `json:"transferMinutes"`
	DowntimeMinutes int     `json:"downtimeMinutes"`
	Windows         int     `json:"windows"`
	Batches         []Batch `json:"batches"`
}

type Totals struct {
	VMs             int   `json:"vms"`
	Skipped         int   `json:"skippedBlocked"`
	DataGiB         int64 `json:"dataGiB"`
	TransferMinutes int   `json:"transferMinutes"`
	DowntimeMinutes int   `json:"downtimeMinutes"`
	Windows         int   `json:"windows"`
}

type Plan struct {
	Basis       string      `json:"basis"`
	Constraints Constraints `json:"constraints"`
	Waves       []Wave      `json:"waves"`
	Totals      Totals      `json:"totals"`
	Warnings    []string    `json:"warnings,omitempty"`
}

// Normalize applies defaults and reports invalid values.
func (c Constraints) Normalize() (Constraints, error) {
	if c.MaxConcurrent < 0 || c.WindowMinutes < 0 || c.CutoverMinutes < 0 || c.LinkMbps < 0 || c.Efficiency < 0 || c.DeltaPercent < 0 {
		return c, fmt.Errorf("constraints must not be negative")
	}
	if c.Efficiency > 1 || c.DeltaPercent > 100 {
		return c, fmt.Errorf("efficiency must be at most 1 and deltaPercent at most 100")
	}
	if c.MaxConcurrent == 0 {
		c.MaxConcurrent = DefaultMaxConcurrent
	}
	if c.WindowMinutes == 0 {
		c.WindowMinutes = DefaultWindowMinutes
	}
	if c.Efficiency == 0 {
		c.Efficiency = DefaultEfficiency
	}
	if c.DeltaPercent == 0 {
		c.DeltaPercent = DefaultDeltaPercent
	}
	return c, nil
}

// FromInventory seeds constraints from the customer's assumptions.json values.
func FromInventory(inv model.Inventory) Constraints {
	var c Constraints
	if a := inv.Assumptions; a != nil {
		if a.LinkMbps != nil {
			c.LinkMbps = *a.LinkMbps
		}
		if a.CutoverMinutes != nil {
			c.CutoverMinutes = *a.CutoverMinutes
		}
	}
	if c.CutoverMinutes == 0 {
		c.CutoverMinutes = DefaultCutoverMinutes
	}
	return c
}

type item struct {
	name   string
	gib    int64
	warm   bool
	groups map[int]bool
}

// Build schedules every non-blocked VM. Warm migration requires CBT to be
// reported enabled; unknown CBT is planned as cold, the conservative choice.
func Build(inv model.Inventory, assessments []model.Assessment, c Constraints) (Plan, error) {
	c, err := c.Normalize()
	if err != nil {
		return Plan{}, err
	}
	p := Plan{Basis: model.BasisEstimated, Constraints: c, Waves: []Wave{}}
	if c.LinkMbps <= 0 {
		p.Basis = model.BasisNotMeasured
		p.Warnings = append(p.Warnings, "linkMbps not provided: copy time is not estimated and downtime counts only the fixed cutover overhead")
	}
	vms := map[string]model.VM{}
	groupOf := map[string][]int{}
	for _, v := range inv.VMs {
		vms[v.ID] = v
	}
	for gi, g := range c.ExclusiveGroups {
		for _, ref := range g {
			key := strings.ToLower(strings.TrimSpace(ref))
			found := false
			for _, v := range inv.VMs {
				if strings.ToLower(v.ID) == key || strings.ToLower(v.Name) == key {
					groupOf[v.ID] = append(groupOf[v.ID], gi)
					found = true
				}
			}
			if !found {
				p.Warnings = append(p.Warnings, fmt.Sprintf("exclusive group %d references unknown VM %q", gi+1, ref))
			}
		}
	}
	byWave := map[int][]item{}
	for _, a := range assessments {
		if a.Status == "blocked" || a.Wave == 0 {
			p.Totals.Skipped++
			continue
		}
		v := vms[a.VMID]
		it := item{name: v.Name, warm: v.CBTEnabled != nil && *v.CBTEnabled, groups: map[int]bool{}}
		for _, d := range v.Disks {
			it.gib += d.SizeGiB
		}
		for _, g := range groupOf[v.ID] {
			it.groups[g] = true
		}
		byWave[a.Wave] = append(byWave[a.Wave], it)
	}
	waveIDs := make([]int, 0, len(byWave))
	for w := range byWave {
		waveIDs = append(waveIDs, w)
	}
	sort.Ints(waveIDs)
	for _, w := range waveIDs {
		wp := buildWave(w, byWave[w], c, &p)
		p.Waves = append(p.Waves, wp)
		p.Totals.VMs += wp.VMs
		p.Totals.DataGiB += wp.DataGiB
		p.Totals.TransferMinutes += wp.TransferMinutes
		p.Totals.DowntimeMinutes += wp.DowntimeMinutes
		p.Totals.Windows += wp.Windows
	}
	return p, nil
}

func buildWave(n int, items []item, c Constraints, p *Plan) Wave {
	return costWave(n, packBatches(items, c), c, p)
}

// packBatches groups a wave's items into concurrent batches. Largest first so
// big copies share batches with small ones deterministically.
func packBatches(items []item, c Constraints) [][]item {
	sort.SliceStable(items, func(i, j int) bool { return items[i].gib > items[j].gib })
	type batch struct {
		items  []item
		groups map[int]bool
	}
	var batches []*batch
	for _, it := range items {
		placed := false
		for _, b := range batches {
			if len(b.items) >= c.MaxConcurrent || conflicts(b.groups, it.groups) {
				continue
			}
			b.items = append(b.items, it)
			for g := range it.groups {
				b.groups[g] = true
			}
			placed = true
			break
		}
		if !placed {
			b := &batch{items: []item{it}, groups: map[int]bool{}}
			for g := range it.groups {
				b.groups[g] = true
			}
			batches = append(batches, b)
		}
	}
	out := make([][]item, len(batches))
	for i, b := range batches {
		out[i] = b.items
	}
	return out
}

// costWave computes downtime, windows and warnings for already-formed batches.
func costWave(n int, batches [][]item, c Constraints, p *Plan) Wave {
	w := Wave{Wave: n, Batches: []Batch{}}
	var durations []int
	for i, items := range batches {
		bp := Batch{Index: i + 1}
		share := float64(len(items)) // concurrent copies split the link
		for _, it := range items {
			w.VMs++
			bp.VMs = append(bp.VMs, it.name)
			bp.DataGiB += it.gib
			w.DataGiB += it.gib
			copyGiB := float64(it.gib)
			if it.warm {
				w.WarmVMs++
				copyGiB *= c.DeltaPercent / 100
			} else {
				w.ColdVMs++
			}
			d := c.CutoverMinutes + copyMinutes(copyGiB, c.LinkMbps/share, c.Efficiency)
			if d > bp.DowntimeMinutes {
				bp.DowntimeMinutes = d
			}
		}
		if bp.DowntimeMinutes > c.WindowMinutes {
			p.Warnings = append(p.Warnings, fmt.Sprintf("wave %d batch %d needs %d minutes of downtime, longer than the %d-minute window; it spans several windows or should be made warm", n, i+1, bp.DowntimeMinutes, c.WindowMinutes))
		}
		w.DowntimeMinutes += bp.DowntimeMinutes
		durations = append(durations, bp.DowntimeMinutes)
		w.Batches = append(w.Batches, bp)
	}
	w.TransferMinutes = copyMinutes(float64(w.DataGiB), c.LinkMbps, c.Efficiency)
	w.Windows = windows(durations, c.WindowMinutes)
	return w
}

func conflicts(a, b map[int]bool) bool {
	for g := range b {
		if a[g] {
			return true
		}
	}
	return false
}

func copyMinutes(gib, mbps, eff float64) int {
	if mbps <= 0 || gib <= 0 {
		return 0
	}
	seconds := gib * 1024 * 1024 * 1024 * 8 / (mbps * eff * 1e6)
	return int(math.Ceil(seconds / 60))
}

// windows packs batches, in order, into maintenance windows. A batch longer
// than one window occupies ceil(d/window) windows by itself.
func windows(durations []int, window int) int {
	n, used := 0, 0
	for _, d := range durations {
		if d > window {
			if used > 0 {
				n++
				used = 0
			}
			n += (d + window - 1) / window
			continue
		}
		if used == 0 || used+d > window {
			if used > 0 {
				n++
			}
			used = d
		} else {
			used += d
		}
	}
	if used > 0 || (n == 0 && len(durations) > 0) {
		n++
	}
	return n
}
