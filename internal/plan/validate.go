// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package plan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/zyvorai/scout/internal/model"
)

// Limits on user-supplied plan files.
const (
	EditedSchema    = "scout-plan/1"
	MaxEditedBytes  = 8 << 20
	maxEditedWaves  = 10000
	maxEditedRefs   = 200000
	maxViolationVMs = 100
	maxMoves        = 1000
	severityError   = "error"
	severityWarning = "warning"
)

// Edited is a wave plan a person has written or changed by hand.
//
// Waves list VM ids or names. Batches are optional: when a wave has none,
// Scout packs it with the planner (which respects concurrency and exclusive
// groups); when present they are checked as written. DependsOn adds explicit
// ordering constraints on top of the inventory's connections.
type Edited struct {
	Schema      string       `json:"schema,omitempty"`
	Constraints Constraints  `json:"constraints"`
	Waves       []EditedWave `json:"waves"`
	DependsOn   []Dependency `json:"dependsOn,omitempty"`
}

type EditedWave struct {
	Wave    int        `json:"wave"`
	VMs     []string   `json:"vms"`
	Batches [][]string `json:"batches,omitempty"`
}

// Dependency states that VM needs On: On must not be in a later wave.
type Dependency struct {
	VM string `json:"vm"`
	On string `json:"on"`
}

// Violation is one problem found in an edited plan.
type Violation struct {
	Code     string   `json:"code"`
	Severity string   `json:"severity"` // "error" or "warning"
	Wave     int      `json:"wave,omitempty"`
	VMs      []string `json:"vms,omitempty"`
	Message  string   `json:"message"`
}

// Validation is the result of checking an edited plan.
type Validation struct {
	OK         bool        `json:"ok"` // no errors (warnings allowed)
	Errors     int         `json:"errors"`
	Warnings   int         `json:"warnings"`
	Violations []Violation `json:"violations"`
	Plan       Plan        `json:"plan"`
}

// ParseEdited reads and strictly decodes an edited plan, rejecting unknown
// fields, trailing data, a wrong schema marker and oversize input.
func ParseEdited(r io.Reader) (Edited, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxEditedBytes+1))
	if err != nil {
		return Edited{}, err
	}
	if len(data) > MaxEditedBytes {
		return Edited{}, fmt.Errorf("plan file exceeds %d bytes", MaxEditedBytes)
	}
	var e Edited
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&e); err != nil {
		return Edited{}, fmt.Errorf("decode plan: %w", err)
	}
	if dec.More() {
		return Edited{}, fmt.Errorf("decode plan: trailing data after the JSON document")
	}
	if e.Schema != "" && e.Schema != EditedSchema {
		return Edited{}, fmt.Errorf("unsupported plan schema %q (want %q)", e.Schema, EditedSchema)
	}
	if len(e.Waves) > maxEditedWaves {
		return Edited{}, fmt.Errorf("plan has %d waves; the limit is %d", len(e.Waves), maxEditedWaves)
	}
	refs := len(e.DependsOn)
	for _, w := range e.Waves {
		refs += len(w.VMs)
		for _, b := range w.Batches {
			refs += len(b)
		}
	}
	if refs > maxEditedRefs {
		return Edited{}, fmt.Errorf("plan has %d VM references; the limit is %d", refs, maxEditedRefs)
	}
	return e, nil
}

// Draft turns the automatic assignment into an editable plan.
func Draft(inv model.Inventory, assessments []model.Assessment, c Constraints) Edited {
	e := Edited{Schema: EditedSchema, Constraints: c, Waves: []EditedWave{}}
	byWave := map[int][]string{}
	for _, a := range assessments {
		if a.Status != "blocked" && a.Wave > 0 {
			byWave[a.Wave] = append(byWave[a.Wave], a.VMName)
		}
	}
	ids := make([]int, 0, len(byWave))
	for w := range byWave {
		ids = append(ids, w)
	}
	sort.Ints(ids)
	for _, w := range ids {
		e.Waves = append(e.Waves, EditedWave{Wave: w, VMs: byWave[w]})
	}
	return e
}

type resolver struct {
	byID   map[string]model.VM
	byName map[string][]string
}

func newResolver(inv model.Inventory) resolver {
	r := resolver{byID: map[string]model.VM{}, byName: map[string][]string{}}
	for _, v := range inv.VMs {
		r.byID[v.ID] = v
		k := strings.ToLower(strings.TrimSpace(v.Name))
		r.byName[k] = append(r.byName[k], v.ID)
	}
	return r
}

// resolve maps a reference (id first, then case-insensitive name) to a VM id.
func (r resolver) resolve(ref string) (id string, code string) {
	if _, ok := r.byID[ref]; ok {
		return ref, ""
	}
	ids := r.byName[strings.ToLower(strings.TrimSpace(ref))]
	switch len(ids) {
	case 0:
		return "", "unknown-vm"
	case 1:
		return ids[0], ""
	}
	return "", "ambiguous-vm"
}

func capVMs(v []string) []string {
	if len(v) > maxViolationVMs {
		return v[:maxViolationVMs]
	}
	return v
}

// Validate checks an edited plan against the inventory, its assessments and
// the constraints, and reports every violation with the VMs involved. It also
// returns the plan that results (downtime and windows) so edits can be
// compared. Unreadable references never abort validation; they are reported.
func Validate(inv model.Inventory, assessments []model.Assessment, e Edited) Validation {
	var out Validation
	add := func(v Violation) {
		v.VMs = capVMs(v.VMs)
		out.Violations = append(out.Violations, v)
		if v.Severity == severityError {
			out.Errors++
		} else {
			out.Warnings++
		}
	}
	c := FromInventory(inv)
	if e.Constraints.LinkMbps != 0 {
		c.LinkMbps = e.Constraints.LinkMbps
	}
	if e.Constraints.CutoverMinutes != 0 {
		c.CutoverMinutes = e.Constraints.CutoverMinutes
	}
	c.MaxConcurrent, c.WindowMinutes = e.Constraints.MaxConcurrent, e.Constraints.WindowMinutes
	c.Efficiency, c.DeltaPercent = e.Constraints.Efficiency, e.Constraints.DeltaPercent
	c.ExclusiveGroups = e.Constraints.ExclusiveGroups
	nc, err := c.Normalize()
	if err != nil {
		add(Violation{Code: "invalid-constraints", Severity: severityError, Message: err.Error()})
		c.MaxConcurrent, c.WindowMinutes, c.Efficiency, c.DeltaPercent = 0, 0, 0, 0
		c.LinkMbps, c.CutoverMinutes = max0f(c.LinkMbps), max0(c.CutoverMinutes)
		nc, _ = c.Normalize()
	}
	c = nc
	out.Plan = Plan{Basis: model.BasisEstimated, Constraints: c, Waves: []Wave{}}
	if c.LinkMbps <= 0 {
		out.Plan.Basis = model.BasisNotMeasured
	}

	res := newResolver(inv)
	status := map[string]string{}
	for _, a := range assessments {
		status[a.VMID] = a.Status
	}
	groupOf := map[string][]int{}
	for gi, g := range c.ExclusiveGroups {
		for _, ref := range g {
			if id, code := res.resolve(ref); code == "" {
				groupOf[id] = append(groupOf[id], gi)
			} else {
				add(Violation{Code: code, Severity: severityWarning, Message: fmt.Sprintf("exclusive group %d references %s %q", gi+1, strings.TrimSuffix(code, "-vm")+" VM", ref)})
			}
		}
	}

	waveOf := map[string]int{}    // vm id -> wave
	members := map[int][]string{} // wave -> vm ids (deduplicated)
	seenWave := map[int]bool{}
	for _, w := range e.Waves {
		if w.Wave <= 0 {
			add(Violation{Code: "invalid-wave", Severity: severityError, Wave: w.Wave, VMs: capVMs(w.VMs), Message: fmt.Sprintf("wave number %d is not positive", w.Wave)})
			continue
		}
		if seenWave[w.Wave] {
			add(Violation{Code: "duplicate-wave", Severity: severityError, Wave: w.Wave, Message: fmt.Sprintf("wave %d is defined more than once; entries are merged", w.Wave)})
		}
		seenWave[w.Wave] = true
		for _, ref := range w.VMs {
			id, code := res.resolve(ref)
			if code != "" {
				msg := fmt.Sprintf("%q does not match any VM id or name", ref)
				if code == "ambiguous-vm" {
					msg = fmt.Sprintf("%q matches several VMs by name; use the id", ref)
				}
				add(Violation{Code: code, Severity: severityError, Wave: w.Wave, VMs: []string{ref}, Message: msg})
				continue
			}
			if prev, dup := waveOf[id]; dup {
				add(Violation{Code: "duplicate-assignment", Severity: severityError, Wave: w.Wave, VMs: []string{res.byID[id].Name}, Message: fmt.Sprintf("%s is assigned to wave %d and wave %d", res.byID[id].Name, prev, w.Wave)})
				continue
			}
			waveOf[id] = w.Wave
			members[w.Wave] = append(members[w.Wave], id)
			if status[id] == "blocked" {
				add(Violation{Code: "blocked-vm", Severity: severityError, Wave: w.Wave, VMs: []string{res.byID[id].Name}, Message: fmt.Sprintf("%s is blocked by readiness findings and cannot be scheduled", res.byID[id].Name)})
			}
		}
	}
	var unplanned []string
	for _, v := range inv.VMs {
		if _, ok := waveOf[v.ID]; !ok && status[v.ID] != "blocked" {
			unplanned = append(unplanned, v.Name)
		}
	}
	if len(unplanned) > 0 {
		add(Violation{Code: "unplanned-vm", Severity: severityWarning, VMs: unplanned, Message: fmt.Sprintf("%d non-blocked VM(s) are not in any wave", len(unplanned))})
	}

	mkItem := func(id string) item {
		v := res.byID[id]
		it := item{name: v.Name, warm: v.CBTEnabled != nil && *v.CBTEnabled, groups: map[int]bool{}}
		for _, d := range v.Disks {
			it.gib += d.SizeGiB
		}
		for _, g := range groupOf[id] {
			it.groups[g] = true
		}
		return it
	}
	explicit := map[int][][]string{}
	for _, w := range e.Waves {
		if w.Wave > 0 && len(w.Batches) > 0 {
			explicit[w.Wave] = append(explicit[w.Wave], w.Batches...)
		}
	}
	waveNums := make([]int, 0, len(members))
	for n := range members {
		waveNums = append(waveNums, n)
	}
	sort.Ints(waveNums)
	for _, n := range waveNums {
		ids := members[n]
		var batches [][]item
		if bs, ok := explicit[n]; ok {
			inWave := map[string]bool{}
			for _, id := range ids {
				inWave[id] = true
			}
			covered := map[string]bool{}
			for bi, b := range bs {
				var bi2 []item
				grp := map[int][]string{}
				var names []string
				for _, ref := range b {
					id, code := res.resolve(ref)
					if code != "" || !inWave[id] || covered[id] {
						add(Violation{Code: "batch-mismatch", Severity: severityError, Wave: n, VMs: []string{ref}, Message: fmt.Sprintf("batch %d of wave %d lists %q, which is unknown, not in this wave, or already in another batch", bi+1, n, ref)})
						continue
					}
					covered[id] = true
					it := mkItem(id)
					bi2 = append(bi2, it)
					names = append(names, it.name)
					for g := range it.groups {
						grp[g] = append(grp[g], it.name)
					}
				}
				if len(bi2) > c.MaxConcurrent {
					add(Violation{Code: "concurrency", Severity: severityError, Wave: n, VMs: names, Message: fmt.Sprintf("batch %d of wave %d cuts over %d VMs together; maxConcurrent is %d", bi+1, n, len(bi2), c.MaxConcurrent)})
				}
				var gis []int
				for g, ns := range grp {
					if len(ns) > 1 {
						gis = append(gis, g)
					}
				}
				sort.Ints(gis)
				for _, g := range gis {
					add(Violation{Code: "exclusive-group", Severity: severityError, Wave: n, VMs: grp[g], Message: fmt.Sprintf("batch %d of wave %d cuts over %s together, but exclusive group %d forbids it", bi+1, n, strings.Join(grp[g], ", "), g+1)})
				}
				if len(bi2) > 0 {
					batches = append(batches, bi2)
				}
			}
			var missing []string
			for _, id := range ids {
				if !covered[id] {
					missing = append(missing, res.byID[id].Name)
					batches = append(batches, []item{mkItem(id)})
				}
			}
			if len(missing) > 0 {
				add(Violation{Code: "batch-mismatch", Severity: severityError, Wave: n, VMs: missing, Message: fmt.Sprintf("wave %d has explicit batches but %d VM(s) are in none; each is costed alone", n, len(missing))})
			}
		} else {
			items := make([]item, 0, len(ids))
			for _, id := range ids {
				items = append(items, mkItem(id))
			}
			batches = packBatches(items, c)
		}
		wp := costWave(n, batches, c, &out.Plan)
		for _, b := range wp.Batches {
			if b.DowntimeMinutes > c.WindowMinutes {
				add(Violation{Code: "window-overrun", Severity: severityWarning, Wave: n, VMs: b.VMs, Message: fmt.Sprintf("wave %d batch %d needs %d minutes of downtime, longer than the %d-minute window (%s)", n, b.Index, b.DowntimeMinutes, c.WindowMinutes, strings.Join(capVMs(b.VMs), ", "))})
			}
		}
		out.Plan.Waves = append(out.Plan.Waves, wp)
		out.Plan.Totals.VMs += wp.VMs
		out.Plan.Totals.DataGiB += wp.DataGiB
		out.Plan.Totals.TransferMinutes += wp.TransferMinutes
		out.Plan.Totals.DowntimeMinutes += wp.DowntimeMinutes
		out.Plan.Totals.Windows += wp.Windows
	}
	out.Plan.Warnings = nil // window overruns are reported as violations instead
	for _, a := range assessments {
		if a.Status == "blocked" || a.Wave == 0 {
			out.Plan.Totals.Skipped++
		}
	}

	name := func(id string) string { return res.byID[id].Name }
	// Inventory dependencies: a VM that calls another must not move before it.
	for _, cn := range inv.Connections {
		wf, okf := waveOf[cn.From]
		wt, okt := waveOf[cn.To]
		if !okf || !okt || wf == wt {
			continue
		}
		if wf < wt {
			add(Violation{Code: "dependency-order", Severity: severityError, Wave: wf, VMs: []string{name(cn.From), name(cn.To)}, Message: fmt.Sprintf("%s (wave %d) calls %s:%d but moves before it (wave %d); migrate the dependency first or together", name(cn.From), wf, name(cn.To), cn.Port, wt)})
		} else {
			add(Violation{Code: "dependency-split", Severity: severityWarning, Wave: wt, VMs: []string{name(cn.To), name(cn.From)}, Message: fmt.Sprintf("%s (wave %d) and its caller %s (wave %d) are connected on port %d but cut over in different waves; expect cross-site traffic in between", name(cn.To), wt, name(cn.From), wf, cn.Port)})
		}
	}
	for _, d := range e.DependsOn {
		a, ca := res.resolve(d.VM)
		b, cb := res.resolve(d.On)
		if ca != "" || cb != "" {
			add(Violation{Code: "unknown-vm", Severity: severityError, VMs: []string{d.VM, d.On}, Message: fmt.Sprintf("dependsOn %q -> %q references a VM that is unknown or ambiguous", d.VM, d.On)})
			continue
		}
		wa, oka := waveOf[a]
		wb, okb := waveOf[b]
		if oka && okb && wa < wb {
			add(Violation{Code: "dependency-order", Severity: severityError, Wave: wa, VMs: []string{name(a), name(b)}, Message: fmt.Sprintf("%s (wave %d) depends on %s (wave %d) but moves before it", name(a), wa, name(b), wb)})
		}
	}
	for _, g := range model.SharedDiskGroups(inv) {
		waves := map[int]bool{}
		var desc []string
		for _, id := range g.VMs {
			w, ok := waveOf[id]
			if !ok {
				w = 0
			}
			waves[w] = true
			if w == 0 {
				desc = append(desc, name(id)+" (not scheduled)")
			} else {
				desc = append(desc, fmt.Sprintf("%s (wave %d)", name(id), w))
			}
		}
		if len(waves) > 1 {
			var vms []string
			for _, id := range g.VMs {
				vms = append(vms, name(id))
			}
			add(Violation{Code: "shared-disk-split", Severity: severityError, VMs: vms, Message: "VMs sharing a disk must cut over together: " + strings.Join(capVMs(desc), ", ")})
		}
	}
	sort.SliceStable(out.Violations, func(i, j int) bool {
		a, b := out.Violations[i], out.Violations[j]
		if a.Severity != b.Severity {
			return a.Severity == severityError
		}
		return false
	})
	if out.Violations == nil {
		out.Violations = []Violation{}
	}
	out.OK = out.Errors == 0
	return out
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

func max0f(f float64) float64 {
	if f < 0 {
		return 0
	}
	return f
}

// Move is one what-if change: put VM into ToWave (a new number creates the wave).
type Move struct {
	VM     string `json:"vm"`
	ToWave int    `json:"toWave"`
}

// WhatIfResult compares a plan before and after moving VMs between waves.
type WhatIfResult struct {
	Moves         []Move      `json:"moves"`
	Before        Validation  `json:"before"`
	After         Validation  `json:"after"`
	Resolved      []Violation `json:"resolved"`
	Introduced    []Violation `json:"introduced"`
	DowntimeDelta int         `json:"downtimeMinutesDelta"`
	WindowsDelta  int         `json:"windowsDelta"`
	Edited        Edited      `json:"edited"`
	Notes         []string    `json:"notes,omitempty"`
}

// ApplyMoves returns a copy of e with the VMs moved. Explicit batches of the
// touched waves are dropped (the moved VM has no batch in its new wave), and a
// note says so. Unknown VMs are reported in notes and otherwise ignored.
func ApplyMoves(inv model.Inventory, e Edited, moves []Move) (Edited, []string) {
	res := newResolver(inv)
	var notes []string
	out := e
	out.Waves = make([]EditedWave, len(e.Waves))
	for i, w := range e.Waves {
		out.Waves[i] = EditedWave{Wave: w.Wave, VMs: append([]string(nil), w.VMs...), Batches: w.Batches}
	}
	for _, m := range moves {
		id, code := res.resolve(m.VM)
		if code != "" {
			notes = append(notes, fmt.Sprintf("move skipped: %q is %s", m.VM, strings.Replace(code, "-vm", "", 1)))
			continue
		}
		if m.ToWave <= 0 {
			notes = append(notes, fmt.Sprintf("move skipped: wave %d is not positive", m.ToWave))
			continue
		}
		for i := range out.Waves {
			var keep []string
			for _, ref := range out.Waves[i].VMs {
				if rid, c := res.resolve(ref); c == "" && rid == id {
					if out.Waves[i].Batches != nil {
						out.Waves[i].Batches = nil
						notes = append(notes, fmt.Sprintf("explicit batches of wave %d were dropped and will be re-packed", out.Waves[i].Wave))
					}
					continue
				}
				keep = append(keep, ref)
			}
			out.Waves[i].VMs = keep
		}
		placed := false
		for i := range out.Waves {
			if out.Waves[i].Wave == m.ToWave {
				out.Waves[i].VMs = append(out.Waves[i].VMs, res.byID[id].Name)
				if out.Waves[i].Batches != nil {
					out.Waves[i].Batches = nil
					notes = append(notes, fmt.Sprintf("explicit batches of wave %d were dropped and will be re-packed", m.ToWave))
				}
				placed = true
				break
			}
		}
		if !placed {
			out.Waves = append(out.Waves, EditedWave{Wave: m.ToWave, VMs: []string{res.byID[id].Name}})
		}
	}
	kept := out.Waves[:0]
	for _, w := range out.Waves {
		if len(w.VMs) > 0 {
			kept = append(kept, w)
		}
	}
	out.Waves = kept
	sort.SliceStable(out.Waves, func(i, j int) bool { return out.Waves[i].Wave < out.Waves[j].Wave })
	return out, dedupe(notes)
}

func dedupe(s []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range s {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

func key(v Violation) string {
	vms := append([]string(nil), v.VMs...)
	sort.Strings(vms)
	return v.Code + "|" + fmt.Sprint(v.Wave) + "|" + strings.Join(vms, ",")
}

// WhatIf validates the plan, applies the moves, validates again, and reports
// which violations the move resolves or introduces and how downtime changes.
func WhatIf(inv model.Inventory, assessments []model.Assessment, e Edited, moves []Move) (WhatIfResult, error) {
	if len(moves) == 0 {
		return WhatIfResult{}, fmt.Errorf("what-if needs at least one move")
	}
	if len(moves) > maxMoves {
		return WhatIfResult{}, fmt.Errorf("what-if accepts at most %d moves", maxMoves)
	}
	after, notes := ApplyMoves(inv, e, moves)
	r := WhatIfResult{Moves: moves, Before: Validate(inv, assessments, e), After: Validate(inv, assessments, after), Edited: after, Notes: notes,
		Resolved: []Violation{}, Introduced: []Violation{}}
	was, now := map[string]bool{}, map[string]bool{}
	for _, v := range r.Before.Violations {
		was[key(v)] = true
	}
	for _, v := range r.After.Violations {
		now[key(v)] = true
		if !was[key(v)] {
			r.Introduced = append(r.Introduced, v)
		}
	}
	for _, v := range r.Before.Violations {
		if !now[key(v)] {
			r.Resolved = append(r.Resolved, v)
		}
	}
	r.DowntimeDelta = r.After.Plan.Totals.DowntimeMinutes - r.Before.Plan.Totals.DowntimeMinutes
	r.WindowsDelta = r.After.Plan.Totals.Windows - r.Before.Plan.Totals.Windows
	return r, nil
}

// Text renders the validation for a terminal.
func (v Validation) Text() string {
	var b strings.Builder
	state := "PASS"
	if !v.OK {
		state = "FAIL"
	}
	fmt.Fprintf(&b, "%s: %d error(s), %d warning(s); %d VMs in %d waves, %d min downtime, %d window(s)\n", state, v.Errors, v.Warnings, v.Plan.Totals.VMs, len(v.Plan.Waves), v.Plan.Totals.DowntimeMinutes, v.Plan.Totals.Windows)
	for _, x := range v.Violations {
		fmt.Fprintf(&b, "  [%s] %s: %s\n", x.Severity, x.Code, x.Message)
	}
	return b.String()
}

// Text renders the what-if comparison for a terminal.
func (r WhatIfResult) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "What-if: %d move(s)\n", len(r.Moves))
	for _, m := range r.Moves {
		fmt.Fprintf(&b, "  move %s -> wave %d\n", m.VM, m.ToWave)
	}
	for _, n := range r.Notes {
		fmt.Fprintf(&b, "  note: %s\n", n)
	}
	fmt.Fprintf(&b, "Downtime %+d min, windows %+d\n", r.DowntimeDelta, r.WindowsDelta)
	fmt.Fprintf(&b, "Resolved (%d):\n", len(r.Resolved))
	for _, x := range r.Resolved {
		fmt.Fprintf(&b, "  [%s] %s: %s\n", x.Severity, x.Code, x.Message)
	}
	fmt.Fprintf(&b, "Introduced (%d):\n", len(r.Introduced))
	for _, x := range r.Introduced {
		fmt.Fprintf(&b, "  [%s] %s: %s\n", x.Severity, x.Code, x.Message)
	}
	b.WriteString("After: " + r.After.Text())
	return b.String()
}
