# Changelog

## Unreleased

### Features
- Six new readiness rules (14 -> 20): `guest-os-support`, `hw-version`, `cbt-disabled`, `snapshot-cbt-limit`, `independent-disk`, `legacy-nic`. New optional inventory fields `hwVersion`, `cbtEnabled`, disk `mode`, NIC `type`, populated from RVTools.
- Downtime and maintenance-window planner: `scout plan`, `GET/POST /api/v1/plan`.
- SARIF 2.1.0 and findings CSV export: `scout assess --format sarif|csv`, `/api/v1/sarif`, `/api/v1/findings.csv`.
- Research notes: docs/RESEARCH.md.

- Estate-level rules: a second rule interface (`engine.EstateRule`) that sees the whole inventory and the assigned waves, with six rules (20 -> 26): `duplicate-mac`, `duplicate-uuid`, `duplicate-name`, `missing-datastore`, `missing-network`, `shared-disk-split`. `GET /api/v1/estate/findings`, `estateFindings` in `scout assess`/`analyze`. New optional inventory fields: VM `uuid`, disk `backing`, top-level `networks`. VMs sharing a backing disk are now assigned to the same wave. `engine.Evaluate` is the single assess pipeline.

- Wave-plan validator and what-if: `scout plan --validate plan.json [--move vm=wave] [--format text]`, `--draft`, `POST /api/v1/plan/validate`, `GET /api/v1/plan/draft`. Format `scout-plan/1` (see docs/API.md). Exits 1 on errors.

- Import hardening and two documented generic formats: `scout import --csv` (header contract) and `--scout-json` (`scout-inventory/1`, schema in docs/scout-inventory-1.schema.json), with `--validate` for line-numbered, capped error reports (exit 1 on errors). Samples: sample/inventory.csv, sample/estate-issues.json. RVTools import now keeps `VM UUID` and the disk `Path`.

- Readiness diff: `scout diff old.json new.json [--format text] [--fail-on regression,new-blocker,score-drop=N]` and `POST /api/v1/diff`.

- Dashboard: estate findings, plan check (validator and what-if) and readiness diff sections in the embedded UI. No build step; all server data is escaped.

### Fixes
- Inventory decode rejects trailing data after the JSON document and unsupported `schema` values.
- `POST /api/v1/plan`, `/api/v1/analyze` and the new endpoints reject trailing data after the JSON document.
- Workbook and findings CSV cells beginning with `=`, `+`, `-`, `@` are now neutralised (spreadsheet formula injection via VM names).

### Dependency and toolchain upgrades
- Go: `go.mod` directive 1.23 -> 1.26; CI now tests Go 1.26.x and 1.27.x; Dockerfile build image `golang:1.27.4-alpine`, runtime `distroless/static-debian13:nonroot`.
- GitHub Actions: checkout v7, setup-go v7, setup-node v7, upload-pages-artifact v5, deploy-pages v5.
- Website: all npm dependencies updated to latest (react/react-dom 19.3, @mdx-js/react 3.1, clsx 2.1, prism-react-renderer 2.4, @types/react 19.3); `npm ci && npm run build` passes.
- README Go badge and requirement updated to 1.26+.

### Held back
- `typescript` stays at `~6.0.2` (latest is 7.0.x): `@docusaurus/tsconfig` still sets the `baseUrl` option removed in TypeScript 7, so `npm run typecheck` fails. Revisit when Docusaurus ships a TS7-compatible tsconfig.
- Scout/chart/image version `0.1.0` is the product version, not a dependency pin, so it is unchanged.
- Pre-existing, unrelated: docs/DEPLOY.md links `../SECURITY.md`, which the Docusaurus build reports as a broken link (warning only).
