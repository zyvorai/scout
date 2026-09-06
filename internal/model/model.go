// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package model

import "time"

type Inventory struct {
	GeneratedAt time.Time         `json:"generatedAt"`
	Source      string            `json:"source"`
	Environment string            `json:"environment"`
	VMs         []VM              `json:"vms"`
	Connections []Connection      `json:"connections,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type VM struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	OS             string            `json:"os"`
	CPUs           int               `json:"cpus"`
	MemoryMiB      int64             `json:"memoryMiB"`
	PowerState     string            `json:"powerState"`
	Firmware       string            `json:"firmware"`
	SecureBoot     bool              `json:"secureBoot"`
	TPM            bool              `json:"tpm"`
	Encrypted      bool              `json:"encrypted"`
	Snapshots      int               `json:"snapshots"`
	ToolsStatus    string            `json:"toolsStatus"`
	GPU            bool              `json:"gpu"`
	SRIOV          bool              `json:"sriov"`
	USBPassthrough bool              `json:"usbPassthrough"`
	PCIPassthrough bool              `json:"pciPassthrough"`
	SharedDisks    bool              `json:"sharedDisks"`
	RDM            bool              `json:"rdm"`
	Disks          []Disk            `json:"disks,omitempty"`
	NICs           []NIC             `json:"nics,omitempty"`
	Tags           map[string]string `json:"tags,omitempty"`
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
}

type Assessment struct {
	VMID     string    `json:"vmId"`
	VMName   string    `json:"vmName"`
	Score    int       `json:"score"`
	Status   string    `json:"status"`
	Findings []Finding `json:"findings"`
	Wave     int       `json:"wave"`
}

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
