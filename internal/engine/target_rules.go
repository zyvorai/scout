// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package engine

import (
	"fmt"
	"strings"

	"github.com/zyvorai/scout/internal/model"
)

// warmSnapshotCap is the number of CBT snapshots a warm migration can chain
// (Red Hat MTV documents 28; some releases say 32, so the lower value is used).
const warmSnapshotCap = 28

// targetRules are checks derived from virt-v2v, Forklift/MTV and Proxmox import
// behaviour. See docs/RESEARCH.md for the sources.
func targetRules() []Rule {
	return []Rule{
		ruleFunc{"guest-os-support", ruleGuestOS},
		ruleFunc{"hw-version", func(vm model.VM) *model.Finding {
			if vm.HWVersion <= 0 || vm.HWVersion >= 7 {
				return nil
			}
			return finding("hw-version", model.SeverityWarning, "Very old virtual hardware version", fmt.Sprintf("VM uses virtual hardware version %d. Versions below 7 predate changed block tracking and use legacy devices.", vm.HWVersion), "Upgrade virtual hardware (after a snapshot) or plan a cold conversion with a test clone.", 5)
		}},
		ruleFunc{"cbt-disabled", func(vm model.VM) *model.Finding {
			if vm.CBTEnabled == nil || *vm.CBTEnabled || !poweredOn(vm.PowerState) {
				return nil
			}
			return finding("cbt-disabled", model.SeverityInfo, "Changed block tracking is disabled", "Warm migration needs CBT on the VM and every disk. Without it the whole disk is copied during downtime (cold migration).", "Enable CBT (ctkEnabled) in advance if the VM must be migrated with minimal downtime.", 2)
		}},
		ruleFunc{"snapshot-cbt-limit", func(vm model.VM) *model.Finding {
			if vm.Snapshots < warmSnapshotCap-4 {
				return nil
			}
			detail := fmt.Sprintf("VM has %d snapshots. Warm migration creates a CBT snapshot per precopy cycle and fails at about %d.", vm.Snapshots, warmSnapshotCap)
			if vm.Snapshots >= warmSnapshotCap {
				detail = fmt.Sprintf("VM already has %d snapshots, at or above the warm-migration cap of about %d.", vm.Snapshots, warmSnapshotCap)
			}
			return finding("snapshot-cbt-limit", model.SeverityWarning, "Snapshot count near the warm-migration cap", detail, "Consolidate snapshots before starting a warm migration.", 10)
		}},
		ruleFunc{"independent-disk", func(vm model.VM) *model.Finding {
			var persistent, nonpersistent int
			for _, d := range vm.Disks {
				m := strings.ToLower(d.Mode)
				switch {
				case strings.Contains(m, "independent") && strings.Contains(m, "nonpersistent"):
					nonpersistent++
				case strings.Contains(m, "independent"):
					persistent++
				}
			}
			if persistent+nonpersistent == 0 {
				return nil
			}
			detail := fmt.Sprintf("%d independent-persistent and %d independent-nonpersistent disks. Independent disks are excluded from snapshots, so warm migration cannot track them and a consistent copy needs the VM powered off.", persistent, nonpersistent)
			if nonpersistent > 0 {
				detail += " Non-persistent disks discard writes at power off; confirm the data worth keeping."
			}
			return finding("independent-disk", model.SeverityWarning, "Independent disk mode", detail, "Switch disks to dependent mode, or migrate cold with the VM powered off.", 10)
		}},
		ruleFunc{"legacy-nic", func(vm model.VM) *model.Finding {
			var legacy []string
			for _, n := range vm.NICs {
				t := strings.ToLower(strings.TrimSpace(n.Type))
				if t == "vmxnet" || t == "vmxnet2" || strings.HasPrefix(t, "vmxnet2") || t == "flexible" || t == "pcnet32" {
					legacy = append(legacy, t)
				}
			}
			if len(legacy) == 0 {
				return nil
			}
			return finding("legacy-nic", model.SeverityInfo, "Legacy VMware NIC model", fmt.Sprintf("NIC model(s) %s have no direct KVM equivalent and are remapped to virtio or e1000.", strings.Join(legacy, ", ")), "Expect a NIC name and driver change; verify guest network configuration after conversion.", 2)
		}},
	}
}

func poweredOn(ps string) bool {
	p := strings.ToLower(ps)
	return !strings.Contains(p, "off") && !strings.Contains(p, "suspend")
}

// ruleGuestOS compares the guest OS name with what virt-v2v and virtio-win can
// convert. Names that cannot be identified are flagged as not measured.
func ruleGuestOS(vm model.VM) *model.Finding {
	o := strings.ToLower(strings.TrimSpace(vm.OS))
	if o == "" {
		return nil // an absent OS is not evidence; legacy rules stay silent too
	}
	for _, t := range []string{"solaris", "netware", "os/2", "darwin", "macos", "mac os", "os x", "msdos", "ms-dos"} {
		if strings.Contains(o, t) {
			return finding("guest-os-support", model.SeverityBlocker, "Guest OS cannot be converted", fmt.Sprintf("%q is not supported by virt-v2v or KubeVirt guest tooling.", vm.OS), "Rebuild or replace the workload; do not schedule it in a conversion wave.", 25)
		}
	}
	for _, t := range []string{"windows xp", "windows 2000", "windows nt", "windows vista", "windows server 2003", "windows 95", "windows 98", "windows me"} {
		if strings.Contains(o, t) {
			return finding("guest-os-support", model.SeverityWarning, "Guest OS is out of virtio-win support", fmt.Sprintf("%q predates the Windows versions supported by virt-v2v and virtio-win.", vm.OS), "Plan a manual P2V-style rebuild or an unsupported best-effort conversion on an emulated disk and NIC.", 20)
		}
	}
	if strings.Contains(o, "bsd") {
		return finding("guest-os-support", model.SeverityWarning, "BSD guest not covered by virt-v2v", "virt-v2v does not officially convert BSD guests; they usually run on KVM but need manual driver and boot checks.", "Test-convert a clone using the disk-copy path and verify boot and network.", 12)
	}
	if strings.Contains(o, "other") && !strings.Contains(o, "linux") {
		f := finding("guest-os-support", model.SeverityWarning, "Guest OS not identified", fmt.Sprintf("Guest OS is recorded as %q, so conversion support cannot be checked.", vm.OS), "Confirm the guest OS and update the VM's guest type before planning.", 5)
		f.Basis = model.BasisNotMeasured
		return f
	}
	return nil
}
