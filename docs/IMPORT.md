<!-- Copyright 2026 Zyvor AI Labs · https://zyvor.dev -->
<!-- SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0 -->

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

## Generic formats and validation

Two formats that Scout defines itself, so they can be specified exactly. Scout does not import Live Optics or vCenter JSON exports: their formats were not verified from primary sources and are not guessed.

```bash
./bin/scout import --csv inventory.csv --environment prod --out scout.json
./bin/scout import --scout-json inventory.json --out scout.json
./bin/scout import --csv inventory.csv --validate        # check only, writes nothing
```

`--validate` prints every problem as `file: line N: [error|warning] path: message` on stderr and exits 1 if there is any error, so it works as a CI gate. At most 100 problems are listed. Without `--validate`, an import with errors writes nothing. Exactly one of `--rvtools`, `--csv`, `--scout-json`, `--file` is accepted. All reads are capped at 64 MiB.

### CSV header contract

One VM per row; UTF-8 (a BOM is accepted); RFC 4180 quoting; blank rows are skipped. Header names are case-insensitive. Only `name` is required. An unknown column is an error (a typo should not silently drop data); columns starting with `x_` are ignored. Limits: 64 columns, 4096 bytes per field, 200,000 rows.

| Column | Meaning |
|---|---|
| `name` | required |
| `id` | unique id; defaults to `name` (then names must be unique, a duplicate is an error) |
| `uuid`, `os`, `power_state`, `firmware` (`bios`/`uefi`), `tools_status` | text |
| `cpus`, `memory_mib`, `snapshots`, `hw_version` | non-negative integers; blank means unknown/zero |
| `cbt_enabled` | boolean; blank means unknown (not false) |
| `secure_boot`, `tpm`, `encrypted`, `gpu`, `sriov`, `usb_passthrough`, `pci_passthrough`, `shared_disks`, `rdm` | boolean; blank is false |
| `disk_gib` | `;`-separated disk sizes in GiB, for example `100;50` |
| `disk_datastores`, `disk_backings` | `;`-separated, one entry per disk in `disk_gib` (use `a;;c` for gaps) |
| `networks`, `macs`, `ips` | `;`-separated, one entry per NIC; the lists given must have equal length |

Booleans are `true/false/yes/no/y/n/t/f/1/0`. Lists are position-aligned: the second entry of `disk_datastores` belongs to the second size in `disk_gib`. [`sample/inventory.csv`](../sample/inventory.csv) is an example.

### `scout-inventory/1` JSON

The normal inventory document with a required `"schema": "scout-inventory/1"` member. The machine-readable contract is [`scout-inventory-1.schema.json`](scout-inventory-1.schema.json) (JSON Schema 2020-12). Unknown members, trailing data and other schema versions are rejected. Beyond the schema, Scout checks: unique ids, non-empty names, non-negative numbers, MAC and IP syntax, VLAN 0-4094, ports 0-65535, connection endpoints that exist, and string and list size limits. Line numbers: syntax and type errors give the line of the failure, an unknown member the line of its first occurrence, and semantic problems the line where the offending VM object starts (connection problems have no line). [`sample/estate-issues.json`](../sample/estate-issues.json) triggers every estate-level rule.

The optional `networks` list and `storage.datastores` enable the `missing-network` and `missing-datastore` checks; `uuid`, NIC `mac` and disk `backing` feed `duplicate-uuid`, `duplicate-mac` and `shared-disk-split`.

## RVTools

The workbook is `.xlsx`. Scout reads `vInfo`, `vDisk`, `vSnapshot`, `vHost`, `vDatastore`, `vPartition`, and `vNetwork`. A missing sheet is a warning, not a guessed value. `vInfo` columns `HW version` and `CBT`, `vInfo` `VM UUID`, `vDisk` `Path` (the backing file used to find shared disks, read when present), `vDisk` `Disk Mode`, and `vNetwork` `Adapter` feed the hardware-version, CBT, independent-disk and legacy-NIC rules; when a column is absent the rule stays silent. VirtIO driver state is not in RVTools, so Windows guests are marked for GuestKit inspection instead of being scored as ready.

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
