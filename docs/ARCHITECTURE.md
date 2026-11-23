# Architecture

Scout is deliberately a single-binary Go application with no JavaScript build chain. Static dashboard assets are compiled into the executable with `go:embed`.

## Packages

- `cmd/scout` — CLI entry point and commands.
- `internal/model` — stable inventory, finding, assessment, summary, and graph data structures.
- `internal/discovery` — source adapters. The `Discoverer` interface returns the common inventory model.
- `internal/inventory` — strict JSON decode, validation, and persistence.
- `internal/engine` — explainable compatibility rules and readiness scoring. `engine.go` holds the core rules and `target_rules.go` the virt-v2v / Forklift-derived rules (20 in total). `estate_rules.go` holds the second, cross-VM rule interface (6 more rules).
- `internal/plan` — downtime/maintenance-window planner: warm vs cold copy time, concurrency batches, exclusive groups. `validate.go` checks user-edited plans and runs what-ifs.
- `internal/diff` — read-only comparison of two assessments (or inventories) with a CI gate.
- `internal/graph` — dependency graph projection and migration-wave grouping.
- `internal/report` — standalone, portable HTML report generation, workbook CSVs, SARIF 2.1.0 and findings CSV export (all CSV is formula-injection safe).
- `internal/server` — JSON API, embedded dashboard (overview, inventory, dependencies, waves, estate findings, plan check, diff), HTTP hardening. The page loads `app.js` only (CSP `script-src 'self'`); every server-supplied string goes through `esc()`.

## Design principles

### Read-only by design

Discovery should not mutate the source platform. Execution belongs in a separate migration system.

### Unknown is not compatible

Provider adapters should not invent compatibility metadata. If a source API does not expose a detail, leave it unknown and let rules or enrichment stages decide how to handle it.

### Explainability over opaque scoring

A score is the result of explicit findings. Every deduction has a rule ID, severity, human explanation, and recommendation.

### Stable core model

Provider-specific identifiers may be stored in tags, but assessment logic consumes the common VM model. This keeps provider code from leaking throughout the system.

## Discovery extension

Implement:

```go
type Discoverer interface {
    Discover(context.Context) (model.Inventory, error)
}
```

A provider should:

1. authenticate without persisting secrets;
2. enumerate source workloads;
3. normalize only facts it can verify;
4. return deterministic IDs for workload references;
5. avoid modifying the source environment.

## Rule extension

Rules implement:

```go
type Rule interface {
    ID() string
    Evaluate(model.VM) *model.Finding
}
```

A rule returns `nil` when it has nothing to report. Findings can be informational, warning, or blocker severity.

Estate-level rules implement a second interface that sees the whole inventory and the assigned waves:

```go
type EstateRule interface {
    ID() string
    Evaluate(EstateContext) []VMFinding
}
```

`engine.Evaluate(inv)` is the single pipeline every entry point uses: per-VM rules, wave assignment, estate rules, rescoring. Estate rules run after waves are assigned and cannot emit blockers.

## Wave algorithm

v0.1.0 uses connected components in the dependency graph. Non-blocked workloads in one connected component receive the same wave. VMs that share a backing disk file are linked like a dependency. Blocked workloads receive wave `0` and are excluded until remediated.

This is intentionally deterministic. Future schedulers can add target-capacity limits, maintenance windows, application priorities, or topology constraints while preserving the underlying dependency groups.
