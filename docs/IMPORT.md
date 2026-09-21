<!-- Copyright 2026 Zyvor AI Labs · https://zyvor.dev -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# Local import

`scout import` reads files that are already on the operator's machine. It does not dial Ceph, NFS, ZFS, or Kubernetes, and it does not upload the export.

```bash
./bin/scout import \
  --rvtools RVTools.xlsx \
  --storage storage.json \
  --dr dr.yaml \
  --assumptions assumptions.json \
  --out scout.json
```

The inventory is written mode `0600`. Warnings for missing RVTools sheets go to stderr. Templates are skipped.

## RVTools

The workbook is `.xlsx`. Scout reads `vInfo`, `vDisk`, `vSnapshot`, `vHost`, `vDatastore`, `vPartition`, and `vNetwork`. A missing sheet is a warning, not a guessed value. VirtIO driver state is not in RVTools, so Windows guests are marked for GuestKit inspection instead of being scored as ready.

## storage.json

Produce this on the customer network with `scripts/collect-storage.sh`. Scout only reads the file.

```json
{
  "ceph": {"pools": [{"name": "rbd", "usedBytes": 100, "maxBytes": 1000}]},
  "nfs": {"exports": [{"server": "nas", "path": "/export", "sizeBytes": 1000, "usedBytes": 100}]},
  "zfs": {"datasets": [{"name": "tank/vm", "usedBytes": 100, "availBytes": 900}]},
  "kubernetes": {
    "storageClasses": ["rook-ceph-block"],
    "persistentVolumes": 2,
    "persistentVolumeClaims": 2,
    "allocatableCPU": "16",
    "allocatableMemoryBytes": 0
  },
  "growthBytesPerDay": 1073741824
}
```

Without `growthBytesPerDay`, capacity runway is reported as unknown. Free percent is still shown when capacity was measured.

## dr.yaml

Version 1 accepts only these flat keys. It does not parse Veeam or Rubrik files.

```yaml
lastSuccessfulBackup: 2026-09-01T00:00:00Z
targetRPOMinutes: 15
lastTestRestore: 2026-06-01T00:00:00Z
offsiteCopy: true
```

If the last test restore is older than 90 days, the DR score drops. If the file is omitted, the score is unknown.

## assumptions.json

Customer figures are required for a complete three-year comparison. Omitted Zyvor commercial keys use these defaults: `$129` per core per year, `$15,000` minimum annual contract, a three-year term, and migration services for the first 100 VMs. `includedDiskGiB` has no default. Until it is set, the report says the migration-services cap is incomplete.

```json
{
  "cores": 64,
  "vmwareAnnual": 180000,
  "storageAnnual": 40000,
  "supportAnnual": 20000,
  "migrationCost": 50000,
  "linkMbps": 1000,
  "cutoverMinutes": 15,
  "pricePerCoreYear": 129,
  "minimumAnnual": 15000,
  "termYears": 3,
  "includedMigrationVMs": 100,
  "includedDiskGiB": 10240
}
```

`linkMbps` is used only to estimate cutover minutes: disk bytes divided by that rate, plus `cutoverMinutes`. The result is labeled an estimate.

## Reports

```bash
./bin/scout report \
  --file scout.json \
  --out report.html \
  --executive executive.html \
  --pdf executive.pdf \
  --workbook workbook.zip
```

The PDF is a short executive document. The workbook is a zip of CSV tables. Both stay on disk.
