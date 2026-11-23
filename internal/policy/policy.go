// Package policy applies an organisation policy file (scout-policy/1) to a set
// of assessments: per-rule severity and penalty overrides, disabled rules, and
// waivers that carry a reason and an expiry date.
//
// Design rules:
//   - A policy never hides a decision. Disabled rules, downgraded severities and
//     waived findings are all listed in the Report, and waived findings stay in
//     the assessment under "waived".
//   - A waiver must name a rule, a VM selector, a reason and an expiry date, and
//     the date may not be more than MaxWaiverDays ahead, so no finding is waived
//     forever. An expired waiver stops applying and is reported.
//   - Unknown fields, unknown rule ids and malformed values are errors, not
//     warnings: a typo in a rule id must not silently waive nothing.
package policy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/zyvorai/scout/internal/engine"
	"github.com/zyvorai/scout/internal/model"
)

const (
	// Schema is the only accepted value of the file's "schema" field.
	Schema = "scout-policy/1"
	// MaxFileBytes bounds the file size read from disk.
	MaxFileBytes = 1 << 20
	// MaxWaivers and MaxRuleOverrides bound the size of a policy.
	MaxWaivers       = 5000
	MaxRuleOverrides = 500
	// MaxWaiverDays is how far ahead a waiver may expire.
	MaxWaiverDays = 366
	// MinReasonLen is the shortest accepted waiver reason, in characters.
	MinReasonLen = 10
	maxFieldLen  = 500
	dateLayout   = "2006-01-02"
)

// Policy is the decoded policy file.
type Policy struct {
	Schema  string              `json:"schema"`
	Name    string              `json:"name,omitempty"`
	Rules   map[string]RuleRule `json:"rules,omitempty"`
	Waivers []Waiver            `json:"waivers,omitempty"`
}

// RuleRule is one per-rule override (the type name avoids a clash with engine.Rule).
type RuleRule struct {
	Disabled bool           `json:"disabled,omitempty"`
	Severity model.Severity `json:"severity,omitempty"`
	Penalty  *int           `json:"penalty,omitempty"`
	Reason   string         `json:"reason,omitempty"`
}

// Waiver sets aside one rule on the VMs its selector matches until Expires.
type Waiver struct {
	Rule    string `json:"rule"`
	VM      string `json:"vm"`
	Reason  string `json:"reason"`
	Owner   string `json:"owner,omitempty"`
	Expires string `json:"expires"`
}

// Parse decodes and validates a policy. now anchors the expiry-horizon check.
func Parse(data []byte, now time.Time) (Policy, error) {
	var p Policy
	if len(data) > MaxFileBytes {
		return p, fmt.Errorf("policy is larger than %d bytes", MaxFileBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return p, fmt.Errorf("policy is not valid scout-policy/1 JSON: %w", err)
	}
	if dec.More() {
		return p, fmt.Errorf("policy has trailing data after the JSON object")
	}
	return p, p.Validate(now)
}

// Load reads and validates a policy file.
func Load(file string, now time.Time) (Policy, error) {
	f, err := os.Open(file)
	if err != nil {
		return Policy{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
	if err != nil {
		return Policy{}, err
	}
	return Parse(data, now)
}

// Validate reports every problem it finds, joined into one error.
func (p Policy) Validate(now time.Time) error {
	var errs []string
	add := func(format string, a ...any) { errs = append(errs, fmt.Sprintf(format, a...)) }
	if p.Schema != Schema {
		add("schema must be %q (got %q)", Schema, p.Schema)
	}
	if err := checkText("name", p.Name, true); err != nil {
		add("%v", err)
	}
	known := map[string]bool{}
	for _, id := range engine.KnownRuleIDs() {
		known[id] = true
	}
	if len(p.Rules) > MaxRuleOverrides {
		add("more than %d rule overrides", MaxRuleOverrides)
	}
	ids := make([]string, 0, len(p.Rules))
	for id := range p.Rules {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		r := p.Rules[id]
		if !known[id] {
			add("rules[%q]: unknown rule id (run `scout policy rules` for the list)", id)
			continue
		}
		switch r.Severity {
		case "", model.SeverityInfo, model.SeverityWarning, model.SeverityBlocker:
		default:
			add("rules[%q]: severity must be info, warning or blocker", id)
		}
		if r.Penalty != nil && (*r.Penalty < 0 || *r.Penalty > 100) {
			add("rules[%q]: penalty must be between 0 and 100", id)
		}
		if r.Disabled && (r.Severity != "" || r.Penalty != nil) {
			add("rules[%q]: disabled cannot be combined with severity or penalty", id)
		}
		if !r.Disabled && r.Severity == "" && r.Penalty == nil {
			add("rules[%q]: override changes nothing (set disabled, severity or penalty)", id)
		}
		if r.Disabled || r.Severity != "" {
			if len([]rune(strings.TrimSpace(r.Reason))) < MinReasonLen {
				add("rules[%q]: a reason of at least %d characters is required to disable a rule or change its severity", id, MinReasonLen)
			}
		}
		if err := checkText(fmt.Sprintf("rules[%q].reason", id), r.Reason, true); err != nil {
			add("%v", err)
		}
	}
	if len(p.Waivers) > MaxWaivers {
		add("more than %d waivers", MaxWaivers)
	}
	for i, w := range p.Waivers {
		at := fmt.Sprintf("waivers[%d]", i)
		if !known[w.Rule] {
			add("%s: unknown rule id %q", at, w.Rule)
		}
		if strings.TrimSpace(w.VM) == "" {
			add("%s: vm selector is required", at)
		} else if w.VM == "*" {
			add("%s: vm \"*\" waives the rule for every VM; use rules[%q].disabled with a reason instead", at, w.Rule)
		} else if _, err := path.Match(w.VM, "x"); err != nil {
			add("%s: vm selector %q is not a valid pattern", at, w.VM)
		}
		if len([]rune(strings.TrimSpace(w.Reason))) < MinReasonLen {
			add("%s: a reason of at least %d characters is required", at, MinReasonLen)
		}
		for field, v := range map[string]string{"vm": w.VM, "reason": w.Reason, "owner": w.Owner} {
			if err := checkText(at+"."+field, v, true); err != nil {
				add("%v", err)
			}
		}
		exp, err := time.Parse(dateLayout, w.Expires)
		switch {
		case err != nil:
			add("%s: expires must be a date like 2026-12-31", at)
		case exp.After(now.AddDate(0, 0, MaxWaiverDays)):
			add("%s: expires %s is more than %d days away; waivers must be reviewed at least yearly", at, w.Expires, MaxWaiverDays)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("invalid policy:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

// checkText rejects control characters (they could forge report lines) and
// over-long values.
func checkText(field, v string, allowEmpty bool) error {
	if v == "" && allowEmpty {
		return nil
	}
	if len([]rune(v)) > maxFieldLen {
		return fmt.Errorf("%s is longer than %d characters", field, maxFieldLen)
	}
	for _, r := range v {
		if unicode.IsControl(r) {
			return fmt.Errorf("%s contains a control character", field)
		}
	}
	return nil
}

// Report lists everything a policy did, so none of it is silent.
type Report struct {
	Policy         string          `json:"policy,omitempty"`
	Disabled       []RuleChange    `json:"disabledRules,omitempty"`
	SeverityChange []SeverityShift `json:"severityChanges,omitempty"`
	Waived         []WaivedRef     `json:"waived,omitempty"`
	Expired        []ExpiredRef    `json:"expiredWaivers,omitempty"`
	Unused         []UnusedRef     `json:"unusedWaivers,omitempty"`
	Expiring       []ExpiringRef   `json:"expiringSoon,omitempty"`
	Counts         Counts          `json:"counts"`
}

// Counts summarises the report.
type Counts struct {
	FindingsDropped  int `json:"findingsDropped"`
	FindingsChanged  int `json:"findingsChanged"`
	FindingsWaived   int `json:"findingsWaived"`
	BlockersWaived   int `json:"blockersWaived"`
	BlockersLowered  int `json:"blockersLowered"`
	ExpiredWaivers   int `json:"expiredWaivers"`
	UnusedWaivers    int `json:"unusedWaivers"`
	ExpiringWithin30 int `json:"expiringWithin30Days"`
}

// RuleChange records findings removed because their rule is disabled.
type RuleChange struct {
	Rule     string `json:"rule"`
	Reason   string `json:"reason"`
	Findings int    `json:"findings"`
}

// SeverityShift records findings whose severity or penalty a rule override changed.
type SeverityShift struct {
	Rule     string         `json:"rule"`
	From     model.Severity `json:"from"`
	To       model.Severity `json:"to"`
	Reason   string         `json:"reason,omitempty"`
	Findings int            `json:"findings"`
}

// WaivedRef records one waived finding.
type WaivedRef struct {
	VM       string         `json:"vm"`
	Rule     string         `json:"rule"`
	Severity model.Severity `json:"severity"`
	Reason   string         `json:"reason"`
	Owner    string         `json:"owner,omitempty"`
	Expires  string         `json:"expires"`
}

// ExpiredRef is a waiver whose date has passed; it no longer applies.
type ExpiredRef struct {
	Rule    string `json:"rule"`
	VM      string `json:"vm"`
	Expires string `json:"expires"`
	Matched int    `json:"findingsNowActive"`
}

// UnusedRef is a waiver that matched no finding (a stale or mistyped selector).
type UnusedRef struct {
	Rule string `json:"rule"`
	VM   string `json:"vm"`
}

// ExpiringRef is an active waiver expiring within 30 days.
type ExpiringRef struct {
	Rule    string `json:"rule"`
	VM      string `json:"vm"`
	Expires string `json:"expires"`
	InDays  int    `json:"inDays"`
}

// Apply returns new assessments with the policy applied and a report. The input
// slice is not modified. now decides which waivers have expired.
func Apply(in []model.Assessment, p Policy, now time.Time) ([]model.Assessment, Report) {
	today := now.UTC().Truncate(24 * time.Hour)
	rep := Report{Policy: p.Name}
	out := make([]model.Assessment, len(in))
	disabled := map[string]int{}
	shifted := map[string]*SeverityShift{}
	waiverHits := make([]int, len(p.Waivers))
	expiredHits := make([]int, len(p.Waivers))

	for i, a := range in {
		cp := a
		cp.Findings = nil
		cp.Waived = append([]model.WaivedFinding(nil), a.Waived...)
		changed := false
		for _, f := range a.Findings {
			if ov, ok := p.Rules[f.RuleID]; ok && ov.Disabled {
				disabled[f.RuleID]++
				changed = true
				continue
			}
			if ov, ok := p.Rules[f.RuleID]; ok {
				from := f.Severity
				if ov.Severity != "" {
					f.Severity = ov.Severity
				}
				if ov.Penalty != nil {
					f.Penalty = *ov.Penalty
				}
				if f.Severity != from {
					key := f.RuleID + "|" + string(from) + "|" + string(f.Severity)
					s := shifted[key]
					if s == nil {
						s = &SeverityShift{Rule: f.RuleID, From: from, To: f.Severity, Reason: ov.Reason}
						shifted[key] = s
					}
					s.Findings++
					if from == model.SeverityBlocker {
						rep.Counts.BlockersLowered++
					}
				}
				if f.Severity != from || ov.Penalty != nil {
					rep.Counts.FindingsChanged++
					changed = true
				}
			}
			wi := matchWaiver(p.Waivers, f.RuleID, a, today)
			if wi >= 0 {
				exp, _ := time.Parse(dateLayout, p.Waivers[wi].Expires)
				if exp.Before(today) {
					expiredHits[wi]++
				} else {
					w := p.Waivers[wi]
					waiverHits[wi]++
					cp.Waived = append(cp.Waived, model.WaivedFinding{Finding: f, WaivedBy: waiverName(w), Reason: w.Reason, Expires: w.Expires})
					rep.Waived = append(rep.Waived, WaivedRef{VM: a.VMName, Rule: f.RuleID, Severity: f.Severity, Reason: w.Reason, Owner: w.Owner, Expires: w.Expires})
					rep.Counts.FindingsWaived++
					if f.Severity == model.SeverityBlocker {
						rep.Counts.BlockersWaived++
					}
					changed = true
					continue
				}
			}
			cp.Findings = append(cp.Findings, f)
		}
		if cp.Findings == nil {
			cp.Findings = []model.Finding{}
		}
		if changed {
			engine.Rescore(&cp)
		}
		out[i] = cp
	}

	for id, n := range disabled {
		rep.Disabled = append(rep.Disabled, RuleChange{Rule: id, Reason: p.Rules[id].Reason, Findings: n})
		rep.Counts.FindingsDropped += n
	}
	sort.Slice(rep.Disabled, func(i, j int) bool { return rep.Disabled[i].Rule < rep.Disabled[j].Rule })
	for _, s := range shifted {
		rep.SeverityChange = append(rep.SeverityChange, *s)
	}
	sort.Slice(rep.SeverityChange, func(i, j int) bool {
		a, b := rep.SeverityChange[i], rep.SeverityChange[j]
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		return a.From < b.From
	})
	sort.Slice(rep.Waived, func(i, j int) bool {
		a, b := rep.Waived[i], rep.Waived[j]
		if a.VM != b.VM {
			return a.VM < b.VM
		}
		return a.Rule < b.Rule
	})
	for i, w := range p.Waivers {
		exp, _ := time.Parse(dateLayout, w.Expires)
		switch {
		case exp.Before(today):
			rep.Expired = append(rep.Expired, ExpiredRef{Rule: w.Rule, VM: w.VM, Expires: w.Expires, Matched: expiredHits[i]})
		case waiverHits[i] == 0:
			rep.Unused = append(rep.Unused, UnusedRef{Rule: w.Rule, VM: w.VM})
		}
		if !exp.Before(today) {
			if days := int(exp.Sub(today).Hours() / 24); days <= 30 {
				rep.Expiring = append(rep.Expiring, ExpiringRef{Rule: w.Rule, VM: w.VM, Expires: w.Expires, InDays: days})
			}
		}
	}
	rep.Counts.ExpiredWaivers = len(rep.Expired)
	rep.Counts.UnusedWaivers = len(rep.Unused)
	rep.Counts.ExpiringWithin30 = len(rep.Expiring)
	return out, rep
}

func waiverName(w Waiver) string {
	if w.Owner != "" {
		return w.Owner
	}
	return "policy"
}

// matchWaiver returns the index of the first waiver for rule whose selector
// matches the VM's id or name, or -1. Expired waivers are still returned so the
// caller can report the findings that became active again.
func matchWaiver(ws []Waiver, rule string, a model.Assessment, today time.Time) int {
	best := -1
	for i, w := range ws {
		if w.Rule != rule {
			continue
		}
		if ok, _ := path.Match(w.VM, a.VMName); !ok {
			if ok2, _ := path.Match(w.VM, a.VMID); !ok2 {
				continue
			}
		}
		exp, err := time.Parse(dateLayout, w.Expires)
		if err != nil {
			continue
		}
		// Prefer a live waiver over an expired one for the same finding.
		if best < 0 {
			best = i
			continue
		}
		bexp, _ := time.Parse(dateLayout, ws[best].Expires)
		if bexp.Before(today) && !exp.Before(today) {
			best = i
		}
	}
	return best
}
