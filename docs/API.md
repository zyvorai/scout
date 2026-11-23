# HTTP API

The API is served by `scout serve` and is intended for local integrations and migration planning systems.

## `GET /api/v1/healthz`

Returns liveness metadata.

## `GET /api/v1/inventory`

Returns the normalized source inventory used by the current process.

## `GET /api/v1/assessments`

Returns the ordered per-VM readiness assessments including rule findings and assigned migration wave.

## `GET /api/v1/summary`

Returns aggregate VM, CPU, memory, storage, readiness, score, and wave counts.

## `GET /api/v1/estate`

Returns capacity runway, the DR score, the target-architecture recommendation, and the three-year arithmetic. Missing inputs stay unknown. They are not filled in.

## `GET /api/v1/estate/findings`

Returns the estate-level (cross-VM) findings grouped by rule: `ruleId`, `severity`, `title`, `recommendation`, `count`, and the affected `vms` with a per-VM `detail`. The same findings also appear in each VM's entry in `/api/v1/assessments`, count toward its score, and are included in the SARIF and CSV exports, the HTML report, and the `estateFindings` key of `scout assess` and `POST /api/v1/analyze`.

| Rule ID | Severity | Penalty | Fires when |
|---|---|---|---|
| `duplicate-mac` | warning | 10 | two or more VMs have a NIC with the same MAC (any of `aa:bb:..`, `aa-bb-..`, `aabb.ccdd.eeff`, `aabbccddeeff`; all-zero and broadcast are ignored) |
| `duplicate-uuid` | warning | 10 | two or more VMs have the same `uuid` (case-insensitive; empty is ignored) |
| `duplicate-name` | warning | 5 | two or more VMs have the same name (case-insensitive) |
| `missing-datastore` | warning | 8 | a disk names a datastore that is not in `storage.datastores`. Silent when no datastore list exists |
| `missing-network` | warning | 8 | a NIC names a network that is not in the top-level `networks` list. Silent when that list is absent |
| `shared-disk-split` | warning | 15 | VMs that share one backing disk (same datastore and `backing` path) are not all in the same wave, for example because one of them is blocked |

Estate rules run after wave assignment, so they never emit blockers (a blocker returned by a custom estate rule is downgraded to a warning). VMs that share a backing disk are put in the same wave. Without `backing` paths no sharing group is derived; Scout does not guess sharing.

## `GET /api/v1/graph`

Returns normalized graph nodes and edges suitable for another UI or planning system.

## `GET /api/v1/plan` and `POST /api/v1/plan`

Returns a downtime and maintenance-window plan for every non-blocked VM, wave by wave. VMs in a wave are grouped into batches that cut over together.

- Copy time is `data / (linkMbps x efficiency)`; concurrent VMs in a batch share the link.
- A VM with `cbtEnabled: true` is planned as **warm**: only the final delta (`deltaPercent` of its data) is copied during downtime. Everything else, including VMs whose CBT state is unknown, is planned as **cold** and copies all data during downtime.
- A batch is limited to `maxConcurrent` VMs and never contains two VMs from one `exclusiveGroups` entry (ids or names).
- `windows` is the number of maintenance windows needed, packing batches in order; a batch longer than one window takes several.
- Without a link speed, `basis` is `not-measured`, copy time is 0 and a warning says so. Blocked VMs are counted in `totals.skippedBlocked`.

`GET` takes query parameters `linkMbps`, `efficiency`, `deltaPercent`, `maxConcurrent`, `windowMinutes`, `cutoverMinutes`. `POST` takes a JSON body with the same names plus `exclusiveGroups`; unknown fields are rejected. Defaults: `maxConcurrent` 4, `windowMinutes` 240, `efficiency` 0.7, `deltaPercent` 5, `cutoverMinutes` from `assumptions.json` or 15, `linkMbps` from `assumptions.json`.

```bash
curl -sS -X POST http://127.0.0.1:18447/api/v1/plan \
  -d '{"linkMbps":1000,"maxConcurrent":2,"windowMinutes":180,"exclusiveGroups":[["db-1","db-2"]]}'
```

CLI equivalent: `scout plan --file scout.json --link-mbps 1000 --max-concurrent 2 --window 180 --exclusive "db-1,db-2"`. The result is an estimate from disk sizes, not a measurement.

## `GET /api/v1/sarif`

Findings as SARIF 2.1.0 (`application/sarif+json`) for CI and code-scanning tools. blocker maps to `error`, warning to `warning`, info to `note`. Each VM is a logical location. CLI: `scout assess --format sarif`.

## `GET /api/v1/findings.csv`

One row per finding. Cells that begin with `=`, `+`, `-`, `@` or a control character are prefixed with `'` so VM names cannot run as spreadsheet formulas. The workbook CSVs get the same protection. CLI: `scout assess --format csv`.

## `GET /api/v1/report`

Returns a self-contained HTML assessment report.

## `POST /api/v1/analyze`

Accepts a normalized inventory document and returns summary, assessments, and graph without writing the body to disk.

The request body is capped at 8 MiB.

Example:

```bash
curl -sS \
  -H 'content-type: application/json' \
  --data-binary @sample/inventory.json \
  http://127.0.0.1:18447/api/v1/analyze
```

## Inventory shape

```json
{
  "generatedAt": "2026-09-06T00:00:00Z",
  "source": "example",
  "environment": "production",
  "vms": [
    {
      "id": "vm-1",
      "name": "app-01",
      "os": "RHEL 9",
      "cpus": 4,
      "memoryMiB": 8192,
      "uuid": "4200aaaa-0000-0000-0000-000000000001",
      "firmware": "uefi",
      "toolsStatus": "running",
      "hwVersion": 19,
      "cbtEnabled": true,
      "disks": [{"id":"disk-1","sizeGiB":100,"format":"vmdk","mode":"persistent","datastore":"ds1","backing":"[ds1] app-01/app-01.vmdk"}],
      "nics": [{"id":"nic-1","network":"prod","mac":"00:50:56:aa:bb:cc","type":"vmxnet3"}]
    }
  ],
  "networks": ["prod"],
  "connections": [
    {"from":"vm-1","to":"vm-2","protocol":"tcp","port":5432}
  ]
}
```

`uuid`, `hwVersion`, `cbtEnabled` (omit when unknown), disk `mode`, disk `backing`, NIC `mac`, NIC `type` and the top-level `networks` are optional; rules stay silent when they are absent.

Unknown JSON fields are rejected by file import to catch schema mistakes early.
