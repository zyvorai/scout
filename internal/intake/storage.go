// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package intake

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/zyvorai/scout/internal/model"
)

// storageFile is the documented on-disk shape. It is not a live Ceph connection.
type storageFile struct {
	Ceph *struct {
		Pools []model.Pool `json:"pools"`
	} `json:"ceph,omitempty"`
	NFS *struct {
		Exports []model.Export `json:"exports"`
	} `json:"nfs,omitempty"`
	ZFS *struct {
		Datasets []model.Dataset `json:"datasets"`
	} `json:"zfs,omitempty"`
	Kubernetes        *model.KubernetesStorage `json:"kubernetes,omitempty"`
	GrowthBytesPerDay *int64                   `json:"growthBytesPerDay,omitempty"`
}

// MergeStorage reads storage.json and folds it into the inventory.
// Datastores and hosts already parsed from RVTools are kept.
func MergeStorage(inv *model.Inventory, path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read storage file: %w", err)
	}
	var file storageFile
	dec := json.NewDecoder(bytesReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&file); err != nil {
		return fmt.Errorf("storage.json: %w", err)
	}
	if inv.Storage == nil {
		inv.Storage = &model.StorageEstate{}
	}
	if file.Ceph != nil {
		inv.Storage.CephPools = append(inv.Storage.CephPools, file.Ceph.Pools...)
	}
	if file.NFS != nil {
		inv.Storage.NFSExports = append(inv.Storage.NFSExports, file.NFS.Exports...)
	}
	if file.ZFS != nil {
		inv.Storage.ZFSDatasets = append(inv.Storage.ZFSDatasets, file.ZFS.Datasets...)
	}
	if file.Kubernetes != nil {
		inv.Storage.Kubernetes = file.Kubernetes
	}
	if file.GrowthBytesPerDay != nil {
		if *file.GrowthBytesPerDay < 0 {
			return fmt.Errorf("growthBytesPerDay must be >= 0")
		}
		inv.Storage.GrowthBytesPerDay = *file.GrowthBytesPerDay
	}
	return nil
}
