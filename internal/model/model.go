// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package model

import "time"

type Inventory struct {
	GeneratedAt     time.Time         `json:"generatedAt"`
	Source          string            `json:"source"`
	Environment     string            `json:"environment"`
	VMs             []VM              `json:"vms"`
	Connections     []Connection      `json:"connections,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	Storage         *StorageEstate    `json:"storage,omitempty"`
	DR              *DRPosture        `json:"dr,omitempty"`
	Assumptions     *Assumptions      `json:"assumptions,omitempty"`
	ImportWarnings  []string          `json:"importWarnings,omitempty"`
	OrphanSnapshots []Snapshot        `json:"orphanSnapshots,omitempty"`
}

type VM struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	OS              string            `json:"os"`
	CPUs            int               `json:"cpus"`
	MemoryMiB       int64             `json:"memoryMiB"`
	PowerState      string            `json:"powerState"`
	Firmware        string            `json:"firmware"`
	SecureBoot      bool              `json:"secureBoot"`
	TPM             bool              `json:"tpm"`
	Encrypted       bool              `json:"encrypted"`
	Snapshots       int               `json:"snapshots"`
	ToolsStatus     string            `json:"toolsStatus"`
	GPU             bool              `json:"gpu"`
	SRIOV           bool              `json:"sriov"`
	USBPassthrough  bool              `json:"usbPassthrough"`
	PCIPassthrough  bool              `json:"pciPassthrough"`
	SharedDisks     bool              `json:"sharedDisks"`
	RDM             bool              `json:"rdm"`
	Disks           []Disk            `json:"disks,omitempty"`
	NICs            []NIC             `json:"nics,omitempty"`
	SnapshotDetails []Snapshot        `json:"snapshotDetails,omitempty"`
	Partitions      []Partition       `json:"partitions,omitempty"`
	Tags            map[string]string `json:"tags,omitempty"`
}

type Disk struct {
	ID        string `json:"id"`
	SizeGiB   int64  `json:"sizeGiB"`
	Format    string `json:"format"`
	Shared    bool   `json:"shared"`
	RDM       bool   `json:"rdm"`
	Datastore string `json:"datastore,omitempty"`
}

type NIC struct {
	ID      string `json:"id"`
	Network string `json:"network"`
	MAC     string `json:"mac,omitempty"`
	IP      string `json:"ip,omitempty"`
	VLAN    int    `json:"vlan,omitempty"`
	SRIOV   bool   `json:"sriov"`
}

type Snapshot struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt,omitempty"`
	SizeBytes int64     `json:"sizeBytes,omitempty"`
	Orphan    bool      `json:"orphan,omitempty"`
	Aged      bool      `json:"aged,omitempty"`
}

type Partition struct {
	Disk          string `json:"disk,omitempty"`
	CapacityBytes int64  `json:"capacityBytes,omitempty"`
	FreeBytes     int64  `json:"freeBytes,omitempty"`
}

type Datastore struct {
	Name      string `json:"name"`
	Capacity  int64  `json:"capacityBytes,omitempty"`
	FreeBytes int64  `json:"freeBytes,omitempty"`
	UsedBytes int64  `json:"usedBytes,omitempty"`
}

type Pool struct {
	Name      string `json:"name"`
	UsedBytes int64  `json:"usedBytes,omitempty"`
	MaxBytes  int64  `json:"maxBytes,omitempty"`
}

type Export struct {
	Server    string `json:"server,omitempty"`
	Path      string `json:"path"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
	UsedBytes int64  `json:"usedBytes,omitempty"`
}

type Dataset struct {
	Name       string `json:"name"`
	UsedBytes  int64  `json:"usedBytes,omitempty"`
	AvailBytes int64  `json:"availBytes,omitempty"`
}

type KubernetesStorage struct {
	StorageClasses         []string `json:"storageClasses,omitempty"`
	PersistentVolumes      int      `json:"persistentVolumes,omitempty"`
	PersistentVolumeClaims int      `json:"persistentVolumeClaims,omitempty"`
	AllocatableCPU         string   `json:"allocatableCPU,omitempty"`
	AllocatableMemoryBytes int64    `json:"allocatableMemoryBytes,omitempty"`
}

type Host struct {
	Name  string `json:"name"`
	Cores int    `json:"cores,omitempty"`
}

// StorageEstate is capacity evidence the customer collected on their own network.
// Scout never dials Ceph, NFS, ZFS, or Kubernetes while reading it.
type StorageEstate struct {
	Datastores        []Datastore        `json:"datastores,omitempty"`
	CephPools         []Pool             `json:"cephPools,omitempty"`
	NFSExports        []Export           `json:"nfsExports,omitempty"`
	ZFSDatasets       []Dataset          `json:"zfsDatasets,omitempty"`
	Kubernetes        *KubernetesStorage `json:"kubernetes,omitempty"`
	Hosts             []Host             `json:"hosts,omitempty"`
	GrowthBytesPerDay int64              `json:"growthBytesPerDay,omitempty"`
	MeasuredHostCores int                `json:"measuredHostCores,omitempty"`
}

// DRPosture is the small backup declaration in dr.yaml. Nil pointers were omitted.
type DRPosture struct {
	LastSuccessfulBackup *time.Time `json:"lastSuccessfulBackup,omitempty"`
	TargetRPOMinutes     *int       `json:"targetRPOMinutes,omitempty"`
	LastTestRestore      *time.Time `json:"lastTestRestore,omitempty"`
	OffsiteCopy          *bool      `json:"offsiteCopy,omitempty"`
}

// Assumptions are the customer's commercial inputs. Nil means the field was not provided.
// Zyvor price defaults apply only when the matching pointer is nil.
type Assumptions struct {
	Cores                *int     `json:"cores,omitempty"`
	VMwareAnnual         *float64 `json:"vmwareAnnual,omitempty"`
	StorageAnnual        *float64 `json:"storageAnnual,omitempty"`
	SupportAnnual        *float64 `json:"supportAnnual,omitempty"`
	MigrationCost        *float64 `json:"migrationCost,omitempty"`
	LinkMbps             *float64 `json:"linkMbps,omitempty"`
	CutoverMinutes       *int     `json:"cutoverMinutes,omitempty"`
	PricePerCoreYear     *float64 `json:"pricePerCoreYear,omitempty"`
	MinimumAnnual        *float64 `json:"minimumAnnual,omitempty"`
	TermYears            *int     `json:"termYears,omitempty"`
	IncludedMigrationVMs *int     `json:"includedMigrationVMs,omitempty"`
	IncludedDiskGiB      *int64   `json:"includedDiskGiB,omitempty"`
}

type Connection struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
	Bytes    int64  `json:"bytes,omitempty"`
}

type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityBlocker Severity = "blocker"
)

type Finding struct {
	RuleID         string   `json:"ruleId"`
	Severity       Severity `json:"severity"`
	Title          string   `json:"title"`
	Detail         string   `json:"detail"`
	Recommendation string   `json:"recommendation,omitempty"`
	Penalty        int      `json:"penalty"`
	// Basis is "measured", "estimated", or "not-measured".
	Basis string `json:"basis,omitempty"`
}

type Assessment struct {
	VMID                    string    `json:"vmId"`
	VMName                  string    `json:"vmName"`
	Score                   int       `json:"score"`
	Status                  string    `json:"status"`
	Findings                []Finding `json:"findings"`
	Wave                    int       `json:"wave"`
	EstimatedCutoverMinutes int       `json:"estimatedCutoverMinutes,omitempty"`
	CutoverBasis            string    `json:"cutoverBasis,omitempty"`
}

const (
	BasisMeasured    = "measured"
	BasisEstimated   = "estimated"
	BasisNotMeasured = "not-measured"
)

type Summary struct {
	TotalVMs        int   `json:"totalVMs"`
	Ready           int   `json:"ready"`
	Review          int   `json:"review"`
	Blocked         int   `json:"blocked"`
	TotalCPU        int   `json:"totalCPU"`
	TotalMemoryGiB  int64 `json:"totalMemoryGiB"`
	TotalStorageGiB int64 `json:"totalStorageGiB"`
	Waves           int   `json:"waves"`
	AverageScore    int   `json:"averageScore"`
}

type GraphNode struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Status string `json:"status"`
	Wave   int    `json:"wave"`
}

type GraphEdge struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
}

type Graph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}
