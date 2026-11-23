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

## `POST /api/v1/plan/validate` and `GET /api/v1/plan/draft`

Validate a hand-edited wave plan (the `scout-plan/1` format) and optionally run a what-if. `GET /api/v1/plan/draft` returns the automatic wave assignment as an editable plan.

```json
{
  "schema": "scout-plan/1",
  "constraints": {"maxConcurrent": 2, "windowMinutes": 180, "linkMbps": 1000, "exclusiveGroups": [["db-1", "db-2"]]},
  "waves": [
    {"wave": 1, "vms": ["db-1", "db-2"]},
    {"wave": 2, "vms": ["app-1"], "batches": [["app-1"]]}
  ],
  "dependsOn": [{"vm": "app-1", "on": "db-1"}]
}
```

VMs are referenced by id or name (names are case-insensitive; a name shared by several VMs is `ambiguous-vm`, use the id). `batches` are optional: without them Scout packs each wave with the planner, which already honours `maxConcurrent` and exclusive groups; with them they are checked as written. The request body is `{"plan": {...}, "moves": [{"vm": "db-1", "toWave": 2}]}`, capped at 8 MiB, strict (unknown fields and trailing data are 400). The response is `{"validation": {...}}`, plus `"whatIf"` when `moves` is present.

`validation` has `ok` (no errors), `errors`, `warnings`, `violations` and `plan` (the downtime and window arithmetic of the edited plan). Each violation has `code`, `severity`, `wave`, `vms` (at most 100 listed) and `message`:

| Code | Severity | Meaning |
|---|---|---|
| `dependency-order` | error | an inventory connection `from` -> `to` has the caller in an earlier wave than the dependency, or a `dependsOn` is violated. Convention: a VM must not move before the services it calls |
| `dependency-split` | warning | connected VMs are in different waves but the dependency moves first |
| `shared-disk-split` | error | VMs sharing a backing disk are not all in one wave |
| `exclusive-group` | error | an explicit batch cuts over two members of an exclusive group together |
| `concurrency` | error | an explicit batch is larger than `maxConcurrent` |
| `window-overrun` | warning | a batch's estimated downtime exceeds `windowMinutes` |
| `blocked-vm` | error | a blocked VM is scheduled |
| `unplanned-vm` | warning | a non-blocked VM is in no wave |
| `unknown-vm`, `ambiguous-vm`, `duplicate-assignment`, `invalid-wave`, `duplicate-wave`, `batch-mismatch`, `invalid-constraints` | error | the plan file itself is inconsistent |

`whatIf` reports `before` and `after` validations, the violations `resolved` and `introduced` by the moves, `downtimeMinutesDelta`, `windowsDelta`, the resulting `edited` plan and `notes` (for example that explicit batches of a touched wave were dropped and re-packed). Moves of unknown VMs are skipped with a note.

CLI: `scout plan --file scout.json --draft > plan.json`, then `scout plan --file scout.json --validate plan.json [--format text]` (exit status 1 when there are errors, so it works in CI), and `--move db-1=2` (repeatable) for a what-if.

## `POST /api/v1/diff`

Read-only readiness diff. The body is an older assessment (the JSON printed by `scout assess`) or an older inventory file (assessed with the current rules, so a rule change between runs shows up as a change). It is compared with what this server is currently serving. Capped at 8 MiB.

The response is `{"result": ..., "gate": ...}`:

- `old`, `new`: totals (`vms`, `ready`, `review`, `blocked`, `averageScore`) and `averageScoreDelta`.
- `changed`: VMs whose status, score or fired rules differ, with `verdict` (`improved`, `regressed`, `unchanged` for a score-only change), `scoreDelta`, `rulesStarted`, `rulesStopped`. Regressions come first. `matchedBy: "name"` marks VMs whose ids differ between exports and were matched by a name that is unique on both sides.
- `added`, `removed` (VM names; counts in `addedCount`, `removedCount`).
- `rules`: per rule, `oldCount`, `newCount` and the VMs where it `started` or `stopped` firing. Rules with no change are omitted. Lists are capped at 200 names (`startedNotListed`, `stoppedNotListed` give the remainder); counts are exact.
- `gate`: `?failOn=` takes a comma-separated list of `regression` (any VM got worse, ready < review < blocked), `new-blocker` (a blocker rule started firing, or a new VM is blocked) and `score-drop=N` (average score fell by more than N). `gate.failed` is true when any condition holds. Unknown tokens are 400.

CLI: `scout diff old.json new.json [--format text] [--fail-on regression,new-blocker,score-drop=5]`. It exits 1 when the gate fails, so it can gate CI. Either file may be assess output or an inventory.

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
