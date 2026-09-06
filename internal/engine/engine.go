// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/zyvorai/scout/internal/model"
)

type Rule interface {
	ID() string
	Evaluate(model.VM) *model.Finding
}

type ruleFunc struct {
	id string
	fn func(model.VM) *model.Finding
}

func (r ruleFunc) ID() string                          { return r.id }
func (r ruleFunc) Evaluate(vm model.VM) *model.Finding { return r.fn(vm) }

func DefaultRules() []Rule {
	return []Rule{
		ruleFunc{"encrypted-vm", func(vm model.VM) *model.Finding {
			if !vm.Encrypted {
				return nil
			}
			return finding("encrypted-vm", model.SeverityBlocker, "Encrypted VM", "Encrypted VM configuration requires a controlled decrypt/re-encrypt migration workflow.", "Decrypt or export through an approved migration workflow before conversion.", 30)
		}},
		ruleFunc{"rdm-disk", func(vm model.VM) *model.Finding {
			has := vm.RDM
			for _, d := range vm.Disks {
				has = has || d.RDM
			}
			if !has {
				return nil
			}
			return finding("rdm-disk", model.SeverityBlocker, "Raw Device Mapping detected", "RDM-backed storage cannot be converted like a normal virtual disk.", "Replace the RDM with a virtual disk or map an equivalent target block device before cutover.", 30)
		}},
		ruleFunc{"shared-disk", func(vm model.VM) *model.Finding {
			has := vm.SharedDisks
			for _, d := range vm.Disks {
				has = has || d.Shared
			}
			if !has {
				return nil
			}
			return finding("shared-disk", model.SeverityBlocker, "Shared disk detected", "Shared virtual disks often participate in clustered applications and require coordinated migration.", "Model the clustered workload explicitly and recreate shared storage semantics on the target platform.", 25)
		}},
		ruleFunc{"tpm", func(vm model.VM) *model.Finding {
			if !vm.TPM {
				return nil
			}
			return finding("tpm", model.SeverityWarning, "Virtual TPM enabled", "vTPM state is platform-specific and needs special handling during migration.", "Export recovery material and recreate the TPM device on the target before first boot.", 12)
		}},
		ruleFunc{"secure-boot", func(vm model.VM) *model.Finding {
			if !vm.SecureBoot {
				return nil
			}
			return finding("secure-boot", model.SeverityInfo, "Secure Boot enabled", "Secure Boot is supported by modern KVM stacks but keys and firmware mode should be preserved.", "Use UEFI on the target and validate signed boot components before cutover.", 0)
		}},
		ruleFunc{"many-snapshots", func(vm model.VM) *model.Finding {
			if vm.Snapshots <= 3 {
				return nil
			}
			penalty := 5
			if vm.Snapshots > 10 {
				penalty = 10
			}
			return finding("many-snapshots", model.SeverityWarning, "Snapshot chain needs consolidation", fmt.Sprintf("VM has %d snapshots, which increases conversion time and rollback complexity.", vm.Snapshots), "Consolidate old snapshots and retain only the recovery points required for the migration window.", penalty)
		}},
		ruleFunc{"gpu", func(vm model.VM) *model.Finding {
			if !vm.GPU && !vm.PCIPassthrough {
				return nil
			}
			return finding("gpu", model.SeverityWarning, "GPU or PCI passthrough detected", "Direct device assignment depends on target hardware, IOMMU layout, and driver compatibility.", "Reserve equivalent target hardware and validate passthrough before scheduling the migration wave.", 15)
		}},
		ruleFunc{"sriov", func(vm model.VM) *model.Finding {
			has := vm.SRIOV
			for _, n := range vm.NICs {
				has = has || n.SRIOV
			}
			if !has {
				return nil
			}
			return finding("sriov", model.SeverityWarning, "SR-IOV networking detected", "SR-IOV virtual functions are tied to the source networking configuration.", "Provision compatible SR-IOV resources and network attachment definitions on the target.", 12)
		}},
		ruleFunc{"usb", func(vm model.VM) *model.Finding {
			if !vm.USBPassthrough {
				return nil
			}
			return finding("usb", model.SeverityWarning, "USB passthrough detected", "Attached USB devices cannot be assumed to exist on the target host.", "Remove the dependency or map an equivalent device before migration.", 10)
		}},
		ruleFunc{"tools", func(vm model.VM) *model.Finding {
			s := strings.ToLower(strings.TrimSpace(vm.ToolsStatus))
			if s == "" || s == "running" || s == "ok" || s == "guesttoolsrunning" {
				return nil
			}
			return finding("tools", model.SeverityWarning, "Guest tools are not healthy", "Guest tooling is useful for reliable shutdown, IP discovery, and application-consistent migration steps.", "Repair or update guest tools before the migration window.", 5)
		}},
		ruleFunc{"firmware", func(vm model.VM) *model.Finding {
			f := strings.ToLower(vm.Firmware)
			if f == "" || f == "bios" || f == "uefi" || f == "efi" {
				return nil
			}
			return finding("firmware", model.SeverityWarning, "Unknown firmware mode", "The source firmware mode could not be mapped confidently.", "Confirm BIOS/UEFI mode and configure the target VM to match.", 8)
		}},
		ruleFunc{"legacy-os", func(vm model.VM) *model.Finding {
			o := strings.ToLower(vm.OS)
			legacy := []string{"windows server 2008", "windows 7", "centos 6", "rhel 6", "ubuntu 14.04"}
			for _, token := range legacy {
				if strings.Contains(o, token) {
					return finding("legacy-os", model.SeverityWarning, "Legacy guest operating system", "The guest OS is old enough that VirtIO drivers and supportability need validation.", "Test-convert a clone and validate boot, storage, network, and application behavior before production migration.", 10)
				}
			}
			return nil
		}},
	}
}

func finding(id string, severity model.Severity, title, detail, recommendation string, penalty int) *model.Finding {
	return &model.Finding{RuleID: id, Severity: severity, Title: title, Detail: detail, Recommendation: recommendation, Penalty: penalty}
}

func Assess(inv model.Inventory, rules []Rule) []model.Assessment {
	if rules == nil {
		rules = DefaultRules()
	}
	out := make([]model.Assessment, 0, len(inv.VMs))
	for _, vm := range inv.VMs {
		score := 100
		findings := make([]model.Finding, 0)
		blocker := false
		warning := false
		for _, r := range rules {
			if f := r.Evaluate(vm); f != nil {
				findings = append(findings, *f)
				score -= f.Penalty
				if f.Severity == model.SeverityBlocker {
					blocker = true
				}
				if f.Severity == model.SeverityWarning {
					warning = true
				}
			}
		}
		if score < 0 {
			score = 0
		}
		status := "ready"
		if blocker {
			status = "blocked"
		} else if score < 90 || warning {
			status = "review"
		}
		sort.SliceStable(findings, func(i, j int) bool {
			order := map[model.Severity]int{model.SeverityBlocker: 0, model.SeverityWarning: 1, model.SeverityInfo: 2}
			if order[findings[i].Severity] != order[findings[j].Severity] {
				return order[findings[i].Severity] < order[findings[j].Severity]
			}
			return findings[i].RuleID < findings[j].RuleID
		})
		out = append(out, model.Assessment{VMID: vm.ID, VMName: vm.Name, Score: score, Status: status, Findings: findings})
	}
	return out
}

func Summary(inv model.Inventory, assessments []model.Assessment) model.Summary {
	var s model.Summary
	s.TotalVMs = len(inv.VMs)
	var score int
	for _, vm := range inv.VMs {
		s.TotalCPU += vm.CPUs
		s.TotalMemoryGiB += vm.MemoryMiB / 1024
		for _, d := range vm.Disks {
			s.TotalStorageGiB += d.SizeGiB
		}
	}
	for _, a := range assessments {
		score += a.Score
		switch a.Status {
		case "ready":
			s.Ready++
		case "review":
			s.Review++
		case "blocked":
			s.Blocked++
		}
		if a.Wave > s.Waves {
			s.Waves = a.Wave
		}
	}
	if len(assessments) > 0 {
		s.AverageScore = score / len(assessments)
	}
	return s
}
