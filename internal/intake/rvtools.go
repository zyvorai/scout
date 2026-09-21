// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package intake

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/zyvorai/scout/internal/model"
)

// SnapshotAgedAfter is how old a snapshot must be before the report calls it aged.
const SnapshotAgedAfter = 30 * 24 * time.Hour

// expectedSheets are the RVTools tabs this importer knows how to read.
var expectedSheets = []string{"vInfo", "vDisk", "vSnapshot", "vHost", "vDatastore", "vPartition", "vNetwork"}

// FromRVTools maps an RVTools .xlsx export into an inventory.
// Sheets that are absent become import warnings. Fields the workbook does not
// contain, including VirtIO driver state, are left unset rather than guessed.
func FromRVTools(path string, now time.Time) (model.Inventory, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	wb, err := OpenXLSX(path)
	if err != nil {
		return model.Inventory{}, err
	}
	sheets := map[string]Sheet{}
	var warnings []string
	for _, want := range expectedSheets {
		found, ok := findSheet(wb, want)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("%s sheet missing; those fields were not measured", want))
			continue
		}
		sheets[want] = found
	}
	info, ok := sheets["vInfo"]
	if !ok {
		return model.Inventory{}, fmt.Errorf("rvtools export has no vInfo sheet")
	}
	byName := map[string]*model.VM{}
	var order []string
	var skippedTemplates int
	env := ""
	for i, row := range info.Rows {
		if truthy(cell(row, "template")) {
			skippedTemplates++
			continue
		}
		name := cell(row, "vm", "name", "vm name")
		if name == "" {
			continue
		}
		id := cell(row, "vm uuid", "uuid", "object id", "vm id")
		if id == "" {
			id = sanitizeID(name)
		}
		if _, exists := byName[strings.ToLower(name)]; exists {
			id = fmt.Sprintf("%s-%d", id, i+1)
		}
		osName := cell(row, "os according to the vmware tools", "os according to the configuration file", "guest os", "os")
		mem := memoryMiB(row)
		vm := &model.VM{
			ID:          id,
			Name:        name,
			OS:          osName,
			CPUs:        atoi(cell(row, "cpus", "cpu", "# cpu")),
			MemoryMiB:   mem,
			PowerState:  cell(row, "powerstate", "power state"),
			Firmware:    strings.ToLower(cell(row, "firmware")),
			ToolsStatus: cell(row, "tools status", "vmware tools", "guest tools status"),
		}
		if env == "" {
			env = cell(row, "datacenter", "datacenter name")
		}
		byName[strings.ToLower(name)] = vm
		order = append(order, strings.ToLower(name))
	}
	if skippedTemplates > 0 {
		warnings = append(warnings, fmt.Sprintf("skipped %d template rows; templates are not migration workloads", skippedTemplates))
	}
	if len(order) == 0 {
		return model.Inventory{}, fmt.Errorf("rvtools vInfo contained no virtual machines")
	}

	if diskSheet, ok := sheets["vDisk"]; ok {
		for i, row := range diskSheet.Rows {
			vm := byName[strings.ToLower(cell(row, "vm", "vm name"))]
			if vm == nil {
				continue
			}
			sizeMB := number(cell(row, "capacity mb", "capacity mib", "capacity miB", "total mb"))
			disk := model.Disk{
				ID:        firstNonEmpty(cell(row, "disk key", "disk", "controller", "path"), fmt.Sprintf("disk-%d", i+1)),
				SizeGiB:   int64(math.Round(sizeMB / 1024)),
				Format:    "vmdk",
				Datastore: cell(row, "datastore", "datastore name"),
			}
			mode := strings.ToLower(cell(row, "disk mode", "mode"))
			raw := cell(row, "raw lun id", "raw lun", "raw")
			if strings.Contains(mode, "rdm") || (raw != "" && !truthyFalse(raw)) {
				disk.RDM = true
				disk.Format = "rdm"
				vm.RDM = true
			}
			sharing := strings.ToLower(cell(row, "sharing mode", "sharing", "multi-writer", "multiwriter"))
			if strings.Contains(sharing, "multi") || truthy(sharing) {
				disk.Shared = true
				vm.SharedDisks = true
			}
			vm.Disks = append(vm.Disks, disk)
		}
	}

	if partSheet, ok := sheets["vPartition"]; ok {
		for _, row := range partSheet.Rows {
			vm := byName[strings.ToLower(cell(row, "vm", "vm name"))]
			if vm == nil {
				continue
			}
			vm.Partitions = append(vm.Partitions, model.Partition{
				Disk:          cell(row, "disk", "disk key"),
				CapacityBytes: mbToBytes(number(cell(row, "capacity mb", "capacity mib"))),
				FreeBytes:     mbToBytes(number(cell(row, "free mb", "free mib"))),
			})
		}
	}

	if netSheet, ok := sheets["vNetwork"]; ok {
		for i, row := range netSheet.Rows {
			vm := byName[strings.ToLower(cell(row, "vm", "vm name"))]
			if vm == nil {
				continue
			}
			adapter := strings.ToLower(cell(row, "adapter", "nic", "network adapter"))
			nic := model.NIC{
				ID:      firstNonEmpty(cell(row, "mac address", "mac"), fmt.Sprintf("nic-%d", i+1)),
				Network: cell(row, "network", "network name", "port group", "dv port group"),
				MAC:     cell(row, "mac address", "mac"),
				IP:      cell(row, "ip address", "ip", "ipv4 address"),
				VLAN:    atoi(cell(row, "vlan", "vlan id")),
			}
			if strings.Contains(adapter, "sriov") || strings.Contains(strings.ToLower(nic.Network), "sriov") {
				nic.SRIOV = true
				vm.SRIOV = true
			}
			vm.NICs = append(vm.NICs, nic)
		}
	}

	var orphans []model.Snapshot
	if snapSheet, ok := sheets["vSnapshot"]; ok {
		for _, row := range snapSheet.Rows {
			name := cell(row, "vm", "vm name")
			sizeMB := number(firstNonEmpty(cell(row, "size mb (total)", "size mb", "size mib", "snapshot size mb")))
			created := parseTime(cell(row, "date / time utc", "date / time", "date time", "created"))
			snap := model.Snapshot{
				Name:      firstNonEmpty(cell(row, "name", "snapshot"), "snapshot"),
				CreatedAt: created,
				SizeBytes: mbToBytes(sizeMB),
			}
			if !created.IsZero() && now.Sub(created) > SnapshotAgedAfter {
				snap.Aged = true
			}
			vm := byName[strings.ToLower(name)]
			if vm == nil {
				snap.Orphan = true
				orphans = append(orphans, snap)
				continue
			}
			vm.SnapshotDetails = append(vm.SnapshotDetails, snap)
			vm.Snapshots++
		}
	}

	storage := &model.StorageEstate{}
	if ds, ok := sheets["vDatastore"]; ok {
		for _, row := range ds.Rows {
			name := cell(row, "name", "datastore")
			if name == "" {
				continue
			}
			storage.Datastores = append(storage.Datastores, model.Datastore{
				Name:      name,
				Capacity:  mbToBytes(number(cell(row, "capacity mb", "capacity mib"))),
				FreeBytes: mbToBytes(number(cell(row, "free mb", "free mib"))),
				UsedBytes: mbToBytes(number(cell(row, "in use mb", "in use mib", "provisioned mb"))),
			})
		}
	}
	if hosts, ok := sheets["vHost"]; ok {
		for _, row := range hosts.Rows {
			name := cell(row, "host", "name")
			if name == "" {
				continue
			}
			cores := atoi(cell(row, "# cores", "cores", "cpu cores"))
			storage.Hosts = append(storage.Hosts, model.Host{Name: name, Cores: cores})
			storage.MeasuredHostCores += cores
		}
	}
	if len(storage.Datastores) == 0 && len(storage.Hosts) == 0 {
		storage = nil
	}

	vms := make([]model.VM, 0, len(order))
	for _, key := range order {
		vms = append(vms, *byName[key])
	}
	if len(orphans) > 0 {
		warnings = append(warnings, fmt.Sprintf("%d snapshot rows did not match a VM in vInfo and are treated as orphaned", len(orphans)))
	}
	inv := model.Inventory{
		GeneratedAt:     now.UTC(),
		Source:          "rvtools",
		Environment:     firstNonEmpty(env, "RVTools export"),
		VMs:             vms,
		Storage:         storage,
		ImportWarnings:  warnings,
		OrphanSnapshots: orphans,
		Metadata:        map[string]string{"collector": "rvtools"},
	}
	return inv, nil
}

func findSheet(wb Workbook, name string) (Sheet, bool) {
	for k, sheet := range wb.Sheets {
		if strings.EqualFold(k, name) {
			return sheet, true
		}
	}
	return Sheet{}, false
}

func cell(row map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(row[normHeader(k)]); v != "" && v != "-" {
			return v
		}
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func memoryMiB(row map[string]string) int64 {
	if v := cell(row, "memory mb", "memory mib"); v != "" {
		return int64(math.Round(number(v)))
	}
	if v := cell(row, "memory gb"); v != "" {
		return int64(math.Round(number(v) * 1024))
	}
	if v := cell(row, "memory"); v != "" {
		n := number(v)
		if n > 0 && n < 128 {
			return int64(math.Round(n * 1024))
		}
		return int64(math.Round(n))
	}
	return 0
}

func number(s string) float64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	if s == "" {
		return 0
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return n
}

func atoi(s string) int {
	return int(math.Round(number(s)))
}

func mbToBytes(mb float64) int64 {
	if mb <= 0 {
		return 0
	}
	return int64(math.Round(mb * 1024 * 1024))
}

func truthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "yes", "1", "y":
		return true
	default:
		return false
	}
}

func truthyFalse(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "false", "no", "0", "n", "-":
		return true
	default:
		return false
	}
}

func sanitizeID(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else if r == ' ' || r == '_' {
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "vm"
	}
	return b.String()
}

func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006/01/02 15:04:05",
		"2006-01-02",
		"01/02/2006 15:04:05",
		"1/2/2006 15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	if n, err := strconv.ParseFloat(s, 64); err == nil && n > 20000 && n < 80000 {
		// Excel serial day, 1899-12-30 epoch.
		base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
		return base.Add(time.Duration(n * float64(24*time.Hour))).UTC()
	}
	return time.Time{}
}
