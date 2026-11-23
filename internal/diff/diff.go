// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

// Package diff compares two readiness assessments. It is read-only: it never
// re-scans anything, it only reads two documents and reports what changed.
package diff

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/zyvorai/scout/internal/engine"
	"github.com/zyvorai/scout/internal/inventory"
	"github.com/zyvorai/scout/internal/model"
)

// MaxBytes caps the size of each input document.
const MaxBytes = 64 << 20

// MaxListed caps the VM names listed per rule and per section, so one huge
// estate cannot produce a megabyte-sized report. Counts are always exact.
const MaxListed = 200

var rank = map[string]int{"ready": 0, "review": 1, "blocked": 2}

// Load reads either the JSON printed by `scout assess` (it has an
// "assessments" array) or an inventory file (it has "vms"), which is assessed
// with the current rules. Anything else is an error.
func Load(r io.Reader) ([]model.Assessment, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxBytes {
		return nil, fmt.Errorf("input exceeds %d bytes", MaxBytes)
	}
	var probe struct {
		Assessments []model.Assessment `json:"assessments"`
		VMs         json.RawMessage    `json:"vms"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("not a JSON assessment or inventory: %w", err)
	}
	if len(probe.Assessments) > 0 {
		for i, a := range probe.Assessments {
			if _, ok := rank[a.Status]; !ok {
				return nil, fmt.Errorf("assessments[%d] has unknown status %q", i, a.Status)
			}
			if a.VMID == "" {
				return nil, fmt.Errorf("assessments[%d] has no vmId", i)
			}
		}
		return probe.Assessments, nil
	}
	if len(probe.VMs) > 0 {
		inv, err := inventory.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		return engine.Evaluate(inv), nil
	}
	return nil, fmt.Errorf("input has neither \"assessments\" nor \"vms\"")
}

// VMChange is one VM whose result differs between the two assessments.
type VMChange struct {
	VMID         string   `json:"vmId"`
	VMName       string   `json:"vmName"`
	OldStatus    string   `json:"oldStatus"`
	NewStatus    string   `json:"newStatus"`
	Verdict      string   `json:"verdict"` // improved, regressed, unchanged
	OldScore     int      `json:"oldScore"`
	NewScore     int      `json:"newScore"`
	ScoreDelta   int      `json:"scoreDelta"`
	RulesStarted []string `json:"rulesStarted,omitempty"`
	RulesStopped []string `json:"rulesStopped,omitempty"`
	MatchedBy    string   `json:"matchedBy,omitempty"` // "name" when the ids differ
}

// RuleChange is how often one rule fired before and after.
type RuleChange struct {
	RuleID      string   `json:"ruleId"`
	Severity    string   `json:"severity,omitempty"`
	OldCount    int      `json:"oldCount"`
	NewCount    int      `json:"newCount"`
	Started     []string `json:"started,omitempty"` // VMs where it began firing
	Stopped     []string `json:"stopped,omitempty"` // VMs where it stopped
	StartedMore int      `json:"startedNotListed,omitempty"`
	StoppedMore int      `json:"stoppedNotListed,omitempty"`
}

// Totals is the headline of one side.
type Totals struct {
	VMs          int `json:"vms"`
	Ready        int `json:"ready"`
	Review       int `json:"review"`
	Blocked      int `json:"blocked"`
	AverageScore int `json:"averageScore"`
}

// Result is the full comparison.
type Result struct {
	Old          Totals       `json:"old"`
	New          Totals       `json:"new"`
	ScoreDelta   int          `json:"averageScoreDelta"`
	Improved     int          `json:"improved"`
	Regressed    int          `json:"regressed"`
	Changed      []VMChange   `json:"changed"`
	Added        []string     `json:"added"`
	Removed      []string     `json:"removed"`
	AddedCount   int          `json:"addedCount"`
	RemovedCount int          `json:"removedCount"`
	Rules        []RuleChange `json:"rules"`
	ChangedMore  int          `json:"changedNotListed,omitempty"`
}

func totals(as []model.Assessment) Totals {
	t := Totals{VMs: len(as)}
	sum := 0
	for _, a := range as {
		sum += a.Score
		switch a.Status {
		case "ready":
			t.Ready++
		case "review":
			t.Review++
		case "blocked":
			t.Blocked++
		}
	}
	if len(as) > 0 {
		t.AverageScore = sum / len(as)
	}
	return t
}

func ruleSet(a model.Assessment) map[string]model.Severity {
	m := map[string]model.Severity{}
	for _, f := range a.Findings {
		m[f.RuleID] = f.Severity
	}
	return m
}

func capNames(n []string) ([]string, int) {
	sort.Strings(n)
	if len(n) > MaxListed {
		return n[:MaxListed], len(n) - MaxListed
	}
	return n, 0
}

// Compare diffs old against new. VMs are matched by id; those left over on
// both sides are matched by name when the name is unique on each side, which
// covers re-exports that renumber ids.
func Compare(oldA, newA []model.Assessment) Result {
	res := Result{Old: totals(oldA), New: totals(newA), Changed: []VMChange{}, Added: []string{}, Removed: []string{}, Rules: []RuleChange{}}
	res.ScoreDelta = res.New.AverageScore - res.Old.AverageScore

	oldByID := map[string]int{}
	for i, a := range oldA {
		oldByID[a.VMID] = i
	}
	pairs := map[int]int{} // new index -> old index
	usedOld := map[int]bool{}
	for j, a := range newA {
		if i, ok := oldByID[a.VMID]; ok && !usedOld[i] {
			pairs[j] = i
			usedOld[i] = true
		}
	}
	nameCount := func(as []model.Assessment, skip func(int) bool) map[string][]int {
		m := map[string][]int{}
		for i, a := range as {
			if !skip(i) {
				k := strings.ToLower(strings.TrimSpace(a.VMName))
				m[k] = append(m[k], i)
			}
		}
		return m
	}
	matchedNew := func(j int) bool { _, ok := pairs[j]; return ok }
	oldNames := nameCount(oldA, func(i int) bool { return usedOld[i] })
	newNames := nameCount(newA, matchedNew)
	byName := map[int]bool{}
	for k, ns := range newNames {
		if os := oldNames[k]; k != "" && len(ns) == 1 && len(os) == 1 {
			pairs[ns[0]] = os[0]
			usedOld[os[0]] = true
			byName[ns[0]] = true
		}
	}

	type ruleAgg struct {
		sev              string
		oldN, newN       int
		started, stopped []string
	}
	rules := map[string]*ruleAgg{}
	agg := func(id string) *ruleAgg {
		if rules[id] == nil {
			rules[id] = &ruleAgg{}
		}
		return rules[id]
	}
	for _, a := range oldA {
		for id := range ruleSet(a) {
			agg(id).oldN++
		}
	}
	for j, a := range newA {
		ns := ruleSet(a)
		for id, sev := range ns {
			g := agg(id)
			g.newN++
			g.sev = string(sev)
		}
		i, ok := pairs[j]
		if !ok {
			res.Added = append(res.Added, a.VMName)
			continue
		}
		o := oldA[i]
		os := ruleSet(o)
		var started, stopped []string
		for id := range ns {
			if _, was := os[id]; !was {
				started = append(started, id)
				agg(id).started = append(agg(id).started, a.VMName)
			}
		}
		for id, sev := range os {
			if _, is := ns[id]; !is {
				stopped = append(stopped, id)
				g := agg(id)
				g.stopped = append(g.stopped, a.VMName)
				if g.sev == "" {
					g.sev = string(sev)
				}
			}
		}
		sort.Strings(started)
		sort.Strings(stopped)
		if o.Status == a.Status && o.Score == a.Score && len(started) == 0 && len(stopped) == 0 {
			continue
		}
		c := VMChange{VMID: a.VMID, VMName: a.VMName, OldStatus: o.Status, NewStatus: a.Status, OldScore: o.Score, NewScore: a.Score,
			ScoreDelta: a.Score - o.Score, RulesStarted: started, RulesStopped: stopped, Verdict: "unchanged"}
		switch {
		case rank[a.Status] > rank[o.Status]:
			c.Verdict = "regressed"
			res.Regressed++
		case rank[a.Status] < rank[o.Status]:
			c.Verdict = "improved"
			res.Improved++
		}
		if byName[j] {
			c.MatchedBy = "name"
		}
		res.Changed = append(res.Changed, c)
	}
	for i, a := range oldA {
		if !usedOld[i] {
			res.Removed = append(res.Removed, a.VMName)
		}
	}
	// Regressions first, then by size of score drop, then name.
	sort.SliceStable(res.Changed, func(i, j int) bool {
		a, b := res.Changed[i], res.Changed[j]
		ra, rb := rank[a.NewStatus]-rank[a.OldStatus], rank[b.NewStatus]-rank[b.OldStatus]
		if ra != rb {
			return ra > rb
		}
		if a.ScoreDelta != b.ScoreDelta {
			return a.ScoreDelta < b.ScoreDelta
		}
		return a.VMName < b.VMName
	})
	if len(res.Changed) > MaxListed*5 {
		res.ChangedMore = len(res.Changed) - MaxListed*5
		res.Changed = res.Changed[:MaxListed*5]
	}
	res.AddedCount, res.RemovedCount = len(res.Added), len(res.Removed)
	res.Added, _ = capNames(res.Added)
	res.Removed, _ = capNames(res.Removed)
	ids := make([]string, 0, len(rules))
	for id := range rules {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		g := rules[id]
		if g.oldN == g.newN && len(g.started) == 0 && len(g.stopped) == 0 {
			continue
		}
		rc := RuleChange{RuleID: id, Severity: g.sev, OldCount: g.oldN, NewCount: g.newN}
		rc.Started, rc.StartedMore = capNames(g.started)
		rc.Stopped, rc.StoppedMore = capNames(g.stopped)
		res.Rules = append(res.Rules, rc)
	}
	return res
}

// Gate is the CI verdict for a Result.
type Gate struct {
	Failed  bool     `json:"failed"`
	Reasons []string `json:"reasons"`
}

// Evaluate applies a comma-separated fail-on spec. Tokens:
//
//	regression    any VM whose verdict got worse (ready < review < blocked)
//	new-blocker   any blocker rule that started firing on a VM, or a new VM that is blocked
//	score-drop=N  the average score fell by more than N points
//
// An empty spec never fails.
func Evaluate(r Result, spec string, newA []model.Assessment) (Gate, error) {
	g := Gate{Reasons: []string{}}
	for _, tok := range strings.Split(spec, ",") {
		tok = strings.TrimSpace(tok)
		switch {
		case tok == "":
		case tok == "regression":
			if r.Regressed > 0 {
				g.Reasons = append(g.Reasons, fmt.Sprintf("%d VM(s) got worse", r.Regressed))
			}
		case tok == "new-blocker":
			n := 0
			for _, rc := range r.Rules {
				if rc.Severity == string(model.SeverityBlocker) {
					n += len(rc.Started) + rc.StartedMore
				}
			}
			for _, a := range newA {
				for _, name := range r.Added {
					if a.VMName == name && a.Status == "blocked" {
						n++
					}
				}
			}
			if n > 0 {
				g.Reasons = append(g.Reasons, fmt.Sprintf("%d new blocker finding(s)", n))
			}
		case strings.HasPrefix(tok, "score-drop="):
			n, err := strconv.Atoi(strings.TrimPrefix(tok, "score-drop="))
			if err != nil || n < 0 || n > 100 {
				return g, fmt.Errorf("fail-on %q: want score-drop=N with N between 0 and 100", tok)
			}
			if -r.ScoreDelta > n {
				g.Reasons = append(g.Reasons, fmt.Sprintf("average score fell by %d (limit %d)", -r.ScoreDelta, n))
			}
		default:
			return g, fmt.Errorf("unknown fail-on token %q (want regression, new-blocker, score-drop=N)", tok)
		}
	}
	g.Failed = len(g.Reasons) > 0
	return g, nil
}

// Text renders the result for a terminal.
func (r Result) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "VMs %d -> %d | ready %d -> %d | review %d -> %d | blocked %d -> %d | average score %d -> %d (%+d)\n",
		r.Old.VMs, r.New.VMs, r.Old.Ready, r.New.Ready, r.Old.Review, r.New.Review, r.Old.Blocked, r.New.Blocked, r.Old.AverageScore, r.New.AverageScore, r.ScoreDelta)
	fmt.Fprintf(&b, "%d improved, %d regressed, %d added, %d removed\n", r.Improved, r.Regressed, r.AddedCount, r.RemovedCount)
	for _, c := range r.Changed {
		fmt.Fprintf(&b, "  %-9s %s: %s %d -> %s %d (%+d)", c.Verdict, c.VMName, c.OldStatus, c.OldScore, c.NewStatus, c.NewScore, c.ScoreDelta)
		if len(c.RulesStarted) > 0 {
			fmt.Fprintf(&b, " started: %s", strings.Join(c.RulesStarted, ","))
		}
		if len(c.RulesStopped) > 0 {
			fmt.Fprintf(&b, " stopped: %s", strings.Join(c.RulesStopped, ","))
		}
		b.WriteString("\n")
	}
	for _, c := range r.Rules {
		fmt.Fprintf(&b, "  rule %s: %d -> %d VMs\n", c.RuleID, c.OldCount, c.NewCount)
	}
	return b.String()
}
