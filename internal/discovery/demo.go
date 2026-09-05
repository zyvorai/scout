package discovery

import (
	"context"
	"time"

	"github.com/zyvorai/scout/internal/model"
)

type Demo struct{}

func (Demo) Discover(context.Context) (model.Inventory, error) {
	return model.Inventory{
		GeneratedAt: time.Now().UTC(), Source: "demo", Environment: "Northstar Production",
		Metadata: map[string]string{"datacenter": "DC-PUNE-01", "collector": "scout"},
		VMs: []model.VM{
			{ID: "vm-101", Name: "sap-app-01", OS: "RHEL 9.4", CPUs: 8, MemoryMiB: 16384, PowerState: "POWERED_ON", Firmware: "uefi", SecureBoot: true, Snapshots: 1, ToolsStatus: "running", Disks: []model.Disk{{ID: "disk-1", SizeGiB: 180, Format: "vmdk", Datastore: "vsan-prod"}}, NICs: []model.NIC{{ID: "nic-1", Network: "prod-app", IP: "10.40.1.21", VLAN: 401}}},
			{ID: "vm-102", Name: "postgres-01", OS: "Ubuntu 24.04 LTS", CPUs: 16, MemoryMiB: 32768, PowerState: "POWERED_ON", Firmware: "uefi", SecureBoot: true, Snapshots: 2, ToolsStatus: "running", Disks: []model.Disk{{ID: "disk-1", SizeGiB: 80, Format: "vmdk", Datastore: "vsan-prod"}, {ID: "disk-2", SizeGiB: 600, Format: "vmdk", Datastore: "vsan-db"}}, NICs: []model.NIC{{ID: "nic-1", Network: "prod-db", IP: "10.40.2.15", VLAN: 402}}},
			{ID: "vm-103", Name: "redis-01", OS: "Ubuntu 24.04 LTS", CPUs: 4, MemoryMiB: 8192, PowerState: "POWERED_ON", Firmware: "uefi", Snapshots: 0, ToolsStatus: "running", Disks: []model.Disk{{ID: "disk-1", SizeGiB: 60, Format: "vmdk", Datastore: "vsan-prod"}}, NICs: []model.NIC{{ID: "nic-1", Network: "prod-cache", IP: "10.40.3.12", VLAN: 403}}},
			{ID: "vm-104", Name: "windows-ad-01", OS: "Windows Server 2022", CPUs: 4, MemoryMiB: 8192, PowerState: "POWERED_ON", Firmware: "uefi", SecureBoot: true, TPM: true, Snapshots: 4, ToolsStatus: "running", Disks: []model.Disk{{ID: "disk-1", SizeGiB: 120, Format: "vmdk", Datastore: "vsan-prod"}}, NICs: []model.NIC{{ID: "nic-1", Network: "prod-core", IP: "10.40.10.10", VLAN: 410}}},
			{ID: "vm-105", Name: "sql-prod-01", OS: "Windows Server 2019", CPUs: 16, MemoryMiB: 65536, PowerState: "POWERED_ON", Firmware: "uefi", SecureBoot: true, TPM: true, RDM: true, SharedDisks: true, Snapshots: 14, ToolsStatus: "running", Disks: []model.Disk{{ID: "disk-1", SizeGiB: 160, Format: "vmdk", Datastore: "vsan-prod"}, {ID: "rdm-1", SizeGiB: 1200, Format: "rdm", RDM: true, Shared: true, Datastore: "san-lun-14"}}, NICs: []model.NIC{{ID: "nic-1", Network: "prod-db", IP: "10.40.2.20", VLAN: 402}}},
			{ID: "vm-106", Name: "gpu-worker-01", OS: "Ubuntu 24.04 LTS", CPUs: 24, MemoryMiB: 131072, PowerState: "POWERED_ON", Firmware: "uefi", GPU: true, PCIPassthrough: true, SRIOV: true, ToolsStatus: "running", Disks: []model.Disk{{ID: "disk-1", SizeGiB: 300, Format: "vmdk", Datastore: "nvme-gpu"}}, NICs: []model.NIC{{ID: "nic-1", Network: "ai-fabric", IP: "10.41.1.31", VLAN: 501, SRIOV: true}}},
			{ID: "vm-107", Name: "legacy-erp-01", OS: "CentOS 6.10", CPUs: 4, MemoryMiB: 8192, PowerState: "POWERED_ON", Firmware: "bios", Snapshots: 7, ToolsStatus: "outdated", Disks: []model.Disk{{ID: "disk-1", SizeGiB: 240, Format: "vmdk", Datastore: "legacy-san"}}, NICs: []model.NIC{{ID: "nic-1", Network: "legacy-app", IP: "10.42.1.18", VLAN: 601}}},
			{ID: "vm-108", Name: "api-gw-01", OS: "RHEL 9.4", CPUs: 4, MemoryMiB: 8192, PowerState: "POWERED_ON", Firmware: "uefi", SecureBoot: true, ToolsStatus: "running", Disks: []model.Disk{{ID: "disk-1", SizeGiB: 80, Format: "vmdk", Datastore: "vsan-prod"}}, NICs: []model.NIC{{ID: "nic-1", Network: "prod-edge", IP: "10.40.20.4", VLAN: 420}}},
		},
		Connections: []model.Connection{
			{From: "vm-101", To: "vm-102", Protocol: "tcp", Port: 5432, Bytes: 82910402},
			{From: "vm-101", To: "vm-103", Protocol: "tcp", Port: 6379, Bytes: 12900320},
			{From: "vm-101", To: "vm-108", Protocol: "tcp", Port: 443, Bytes: 9912033},
			{From: "vm-105", To: "vm-104", Protocol: "tcp", Port: 389, Bytes: 920031},
			{From: "vm-108", To: "vm-104", Protocol: "tcp", Port: 53, Bytes: 330021},
		},
	}, nil
}
