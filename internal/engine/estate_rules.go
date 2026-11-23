// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/zyvorai/scout/internal/graph"
	"github.com/zyvorai/scout/internal/model"
)

// maxNamedPeers bounds how many other VMs a finding names, so one hostile or
// huge inventory cannot produce megabyte-sized explanations.
const maxNamedPeers = 5

// EstateContext is what an estate rule sees: the whole inventory plus the
// per-VM assessments with migration waves already assigned.
type EstateContext struct {
	Inventory   model.Inventory
	Assessments []model.Assessment
	// WaveOf maps a VM id to its wave; 0 means held (blocked) or unassigned.
	WaveOf map[string]int
}

// VMFinding attaches a finding to one VM.
type VMFinding struct {
	VMID    string
	Finding model.Finding
}

// EstateRule is the cross-VM counterpart of Rule. It sees the whole inventory,
// so it can detect problems that only exist between VMs. It runs after waves
// are assigned and therefore must not emit blockers (which would have to
// change the waves it was given); ApplyEstateRules downgrades any blocker to a
// warning.
type EstateRule interface {
	ID() string
	Evaluate(EstateContext) []VMFinding
}

type estateRuleFunc struct {
	id string
	fn func(EstateContext) []VMFinding
}

func (r estateRuleFunc) ID() string                           { return r.id }
func (r estateRuleFunc) Evaluate(c EstateContext) []VMFinding { return r.fn(c) }

// DefaultEstateRules returns the built-in cross-VM rules.
func DefaultEstateRules() []EstateRule {
	return []EstateRule{
		estateRuleFunc{"duplicate-mac", ruleDuplicateMAC},
		estateRuleFunc{"duplicate-uuid", ruleDuplicateUUID},
		estateRuleFunc{"duplicate-name", ruleDuplicateName},
		estateRuleFunc{"missing-datastore", ruleMissingDatastore},
		estateRuleFunc{"missing-network", ruleMissingNetwork},
		estateRuleFunc{"shared-disk-split", ruleSharedDiskSplit},
	}
}

// IsEstateRule reports whether id belongs to a built-in estate rule.
func IsEstateRule(id string) bool {
	for _, r := range DefaultEstateRules() {
		if r.ID() == id {
			return true
		}
	}
	return false
}

// Evaluate runs the whole pipeline: per-VM rules, wave assignment, then
// estate rules. Every entry point (CLI, API, report) should use it so they
// agree. Cutover estimates are added afterwards by estate.Annotate.
func Evaluate(inv model.Inventory) []model.Assessment {
	return ApplyEstateRules(inv, graph.AssignWaves(inv, Assess(inv, nil)), nil)
}

// ApplyEstateRules runs estate rules over assessments that already carry
// waves and returns copies with the extra findings merged, rescored and
// re-sorted. Nil rules selects DefaultEstateRules.
func ApplyEstateRules(inv model.Inventory, assessments []model.Assessment, rules []EstateRule) []model.Assessment {
	if rules == nil {
		rules = DefaultEstateRules()
	}
	out := make([]model.Assessment, len(assessments))
	idx := make(map[string]int, len(assessments))
	waveOf := make(map[string]int, len(assessments))
	for i, a := range assessments {
		a.Findings = append([]model.Finding(nil), a.Findings...)
		out[i] = a
		idx[a.VMID] = i
		waveOf[a.VMID] = a.Wave
	}
	ctx := EstateContext{Inventory: inv, Assessments: assessments, WaveOf: waveOf}
	touched := map[int]bool{}
	for _, r := range rules {
		for _, vf := range r.Evaluate(ctx) {
			i, ok := idx[vf.VMID]
			if !ok {
				continue
			}
			f := vf.Finding
			if f.RuleID == "" {
				f.RuleID = r.ID()
			}
			if f.Basis == "" {
				f.Basis = model.BasisMeasured
			}
			if f.Severity == model.SeverityBlocker {
				f.Severity = model.SeverityWarning
			}
			out[i].Findings = append(out[i].Findings, f)
			touched[i] = true
		}
	}
	for i := range touched {
		rescore(&out[i])
	}
	return out
}

// EstateFindingGroup is one estate rule's findings gathered across VMs, for
// reports and the dashboard.
type EstateFindingGroup struct {
	RuleID         string            `json:"ruleId"`
	Severity       model.Severity    `json:"severity"`
	Title          string            `json:"title"`
	Recommendation string            `json:"recommendation,omitempty"`
	Count          int               `json:"count"`
	VMs            []EstateFindingVM `json:"vms"`
}

// EstateFindingVM is one affected VM within an EstateFindingGroup.
type EstateFindingVM struct {
	VMID   string `json:"vmId"`
	VMName string `json:"vmName"`
	Detail string `json:"detail"`
}

// EstateFindings groups the estate-rule findings in assessments by rule.
func EstateFindings(assessments []model.Assessment) []EstateFindingGroup {
	byRule := map[string]*EstateFindingGroup{}
	for _, a := range assessments {
		for _, f := range a.Findings {
			if !IsEstateRule(f.RuleID) {
				continue
			}
			g := byRule[f.RuleID]
			if g == nil {
				g = &EstateFindingGroup{RuleID: f.RuleID, Severity: f.Severity, Title: f.Title, Recommendation: f.Recommendation, VMs: []EstateFindingVM{}}
				byRule[f.RuleID] = g
			}
			g.Count++
			g.VMs = append(g.VMs, EstateFindingVM{VMID: a.VMID, VMName: a.VMName, Detail: f.Detail})
		}
	}
	out := make([]EstateFindingGroup, 0, len(byRule))
	for _, g := range byRule {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RuleID < out[j].RuleID })
	return out
}

// peers renders "a, b, c and N more" for at most maxNamedPeers names.
func peers(names []string) string {
	sort.Strings(names)
	if len(names) <= maxNamedPeers {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(names[:maxNamedPeers], ", "), len(names)-maxNamedPeers)
}

func isHex(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// NormalizeMAC canonicalises a MAC address to lower-case colon form. It
// accepts aa:bb:.., aa-bb-.., aabb.ccdd.eeff and aabbccddeeff. It returns ""
// for anything that is not six octets and for the all-zero and broadcast
// addresses, which are placeholders and not identities.
func NormalizeMAC(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || len(s) > 32 {
		return ""
	}
	var hex string
	if strings.ContainsAny(s, ":-") {
		parts := strings.FieldsFunc(s, func(r rune) bool { return r == ':' || r == '-' })
		if len(parts) != 6 {
			return ""
		}
		for _, p := range parts {
			if len(p) != 2 {
				return ""
			}
			hex += p
		}
	} else {
		hex = strings.ReplaceAll(s, ".", "")
	}
	if len(hex) != 12 || !isHex(hex) || hex == "000000000000" || hex == "ffffffffffff" {
		return ""
	}
	var b strings.Builder
	for i := 0; i < 12; i += 2 {
		if i > 0 {
			b.WriteByte(':')
		}
		b.WriteString(hex[i : i+2])
	}
	return b.String()
}

// duplicates is the shared shape of the identity rules: key each VM, and flag
// every VM whose key is shared with another VM. A VM gets one finding per
// rule; further shared keys extend its detail.
func duplicates(ctx EstateContext, id string, keys func(model.VM) []string, build func(vm model.VM, key string, others []string) model.Finding) []VMFinding {
	owners := map[string][]int{}
	for i, vm := range ctx.Inventory.VMs {
		seen := map[string]bool{}
		for _, k := range keys(vm) {
			if k == "" || seen[k] {
				continue
			}
			seen[k] = true
			owners[k] = append(owners[k], i)
		}
	}
	shared := make([]string, 0, len(owners))
	for k, o := range owners {
		if len(o) > 1 {
			shared = append(shared, k)
		}
	}
	sort.Strings(shared)
	merged := map[int]*model.Finding{}
	var order []int
	for _, k := range shared {
		for _, i := range owners[k] {
			var others []string
			for _, j := range owners[k] {
				if j != i {
					others = append(others, ctx.Inventory.VMs[j].Name)
				}
			}
			fi := build(ctx.Inventory.VMs[i], k, others)
			if prev, ok := merged[i]; ok {
				if len(prev.Detail) < 2048 {
					prev.Detail += " " + fi.Detail
				}
				continue
			}
			fi.RuleID = id
			merged[i] = &fi
			order = append(order, i)
		}
	}
	out := make([]VMFinding, 0, len(order))
	for _, i := range order {
		out = append(out, VMFinding{VMID: ctx.Inventory.VMs[i].ID, Finding: *merged[i]})
	}
	return out
}

func ruleDuplicateMAC(ctx EstateContext) []VMFinding {
	return duplicates(ctx, "duplicate-mac", func(vm model.VM) []string {
		var ks []string
		for _, n := range vm.NICs {
			ks = append(ks, NormalizeMAC(n.MAC))
		}
		return ks
	}, func(vm model.VM, key string, others []string) model.Finding {
		return model.Finding{Severity: model.SeverityWarning, Title: "Duplicate MAC address",
			Detail:         fmt.Sprintf("MAC %s is also used by %s.", key, peers(others)),
			Recommendation: "Give each VM a unique MAC (regenerate the address on the clone) before cutover; duplicate MACs cause intermittent connectivity on the target network.",
			Penalty:        10}
	})
}

func ruleDuplicateUUID(ctx EstateContext) []VMFinding {
	return duplicates(ctx, "duplicate-uuid", func(vm model.VM) []string {
		return []string{strings.ToLower(strings.TrimSpace(vm.UUID))}
	}, func(vm model.VM, key string, others []string) model.Finding {
		return model.Finding{Severity: model.SeverityWarning, Title: "Duplicate VM UUID",
			Detail:         fmt.Sprintf("UUID %s is also used by %s. Copied or cloned VMs that kept their BIOS UUID are indistinguishable to inventory and licensing tools.", key, peers(others)),
			Recommendation: "Regenerate the UUID on the copy (or confirm it is a stale duplicate inventory row) so the target platform can tell the VMs apart.",
			Penalty:        10}
	})
}

func ruleDuplicateName(ctx EstateContext) []VMFinding {
	return duplicates(ctx, "duplicate-name", func(vm model.VM) []string {
		return []string{strings.ToLower(strings.TrimSpace(vm.Name))}
	}, func(vm model.VM, key string, others []string) model.Finding {
		return model.Finding{Severity: model.SeverityWarning, Title: "Duplicate VM name",
			Detail:         fmt.Sprintf("Name %q is shared with %d other VM(s). Name-based plans, exclusive groups and imports can only match the first one.", vm.Name, len(others)),
			Recommendation: "Rename one VM or refer to VMs by id; target platforms usually require unique names per namespace.",
			Penalty:        5}
	})
}

func setOf(items []string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, s := range items {
		m[strings.ToLower(strings.TrimSpace(s))] = true
	}
	return m
}

func ruleMissingDatastore(ctx EstateContext) []VMFinding {
	if ctx.Inventory.Storage == nil || len(ctx.Inventory.Storage.Datastores) == 0 {
		return nil // no datastore list: nothing to cross-check, and nothing is guessed
	}
	var names []string
	for _, d := range ctx.Inventory.Storage.Datastores {
		names = append(names, d.Name)
	}
	known := setOf(names)
	var out []VMFinding
	for _, vm := range ctx.Inventory.VMs {
		missing := map[string]bool{}
		for _, d := range vm.Disks {
			ds := strings.TrimSpace(d.Datastore)
			if ds != "" && !known[strings.ToLower(ds)] {
				missing[ds] = true
			}
		}
		if len(missing) == 0 {
			continue
		}
		list := make([]string, 0, len(missing))
		for m := range missing {
			list = append(list, m)
		}
		out = append(out, VMFinding{VMID: vm.ID, Finding: model.Finding{RuleID: "missing-datastore", Severity: model.SeverityWarning, Title: "Disk on a datastore missing from the inventory",
			Detail:         fmt.Sprintf("Disks reference datastore(s) %s that are not in the storage evidence, so capacity and copy sources cannot be verified.", peers(list)),
			Recommendation: "Re-export with the datastore included (or fix the name), and confirm the datastore is still mounted; a missing or renamed datastore is a common cause of failed conversions.",
			Penalty:        8}})
	}
	return out
}

func ruleMissingNetwork(ctx EstateContext) []VMFinding {
	if len(ctx.Inventory.Networks) == 0 {
		return nil
	}
	known := setOf(ctx.Inventory.Networks)
	var out []VMFinding
	for _, vm := range ctx.Inventory.VMs {
		missing := map[string]bool{}
		for _, n := range vm.NICs {
			nw := strings.TrimSpace(n.Network)
			if nw != "" && !known[strings.ToLower(nw)] {
				missing[nw] = true
			}
		}
		if len(missing) == 0 {
			continue
		}
		list := make([]string, 0, len(missing))
		for m := range missing {
			list = append(list, m)
		}
		out = append(out, VMFinding{VMID: vm.ID, Finding: model.Finding{RuleID: "missing-network", Severity: model.SeverityWarning, Title: "NIC on a network missing from the inventory",
			Detail:         fmt.Sprintf("NICs reference network(s) %s that are not in the declared network list, so a target network mapping cannot be derived.", peers(list)),
			Recommendation: "Add the network to the inventory (or correct the NIC) and define its target network mapping before cutover.",
			Penalty:        8}})
	}
	return out
}

func waveLabel(w int) string {
	if w <= 0 {
		return "held"
	}
	return fmt.Sprintf("wave %d", w)
}

func ruleSharedDiskSplit(ctx EstateContext) []VMFinding {
	name := map[string]string{}
	for _, vm := range ctx.Inventory.VMs {
		name[vm.ID] = vm.Name
	}
	var out []VMFinding
	for _, g := range model.SharedDiskGroups(ctx.Inventory) {
		waves := map[int]bool{}
		for _, id := range g.VMs {
			waves[ctx.WaveOf[id]] = true
		}
		if len(waves) < 2 {
			continue
		}
		for _, id := range g.VMs {
			var others []string
			for _, o := range g.VMs {
				if o != id {
					others = append(others, fmt.Sprintf("%s (%s)", name[o], waveLabel(ctx.WaveOf[o])))
				}
			}
			out = append(out, VMFinding{VMID: id, Finding: model.Finding{RuleID: "shared-disk-split", Severity: model.SeverityWarning, Title: "Shared-disk group split across waves",
				Detail:         fmt.Sprintf("This VM (%s) shares a disk with %s. Members of one shared-disk group cannot be cut over independently.", waveLabel(ctx.WaveOf[id]), peers(others)),
				Recommendation: "Resolve the blocker holding part of the group, then migrate all members in the same wave.",
				Penalty:        15}})
		}
	}
	return out
}
