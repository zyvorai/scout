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

## `GET /api/v1/graph`

Returns normalized graph nodes and edges suitable for another UI or planning system.

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
      "firmware": "uefi",
      "toolsStatus": "running",
      "disks": [{"id":"disk-1","sizeGiB":100,"format":"vmdk"}]
    }
  ],
  "connections": [
    {"from":"vm-1","to":"vm-2","protocol":"tcp","port":5432}
  ]
}
```

Unknown JSON fields are rejected by file import to catch schema mistakes early.
