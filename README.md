<div align="center">

# Scout

[![CI](https://github.com/zyvorai/scout/actions/workflows/ci.yml/badge.svg)](https://github.com/zyvorai/scout/actions/workflows/ci.yml)
[![License: Zyvor Production](https://img.shields.io/badge/license-Zyvor%20Production%20Licence%20v2.0-blue.svg)](LICENSE)
[![Version](https://img.shields.io/badge/version-0.1.0-informational)](cmd/scout/main.go)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)](go.mod)
[![Docs](https://img.shields.io/badge/Docs-zyvorai.github.io%2Fscout-0071e3)](https://zyvorai.github.io/scout/)

[![Book a demo](https://img.shields.io/badge/Book_a_demo-0071e3?style=for-the-badge)](https://zyvor.dev/schedule?utm_source=github&utm_medium=scout&utm_campaign=readme_hero)
[![30-day PoC](https://img.shields.io/badge/30--day_PoC-000000?style=for-the-badge)](https://zyvor.dev/poc?utm_source=github&utm_medium=scout&utm_campaign=readme_hero)
[![Quickstart](https://img.shields.io/badge/Quickstart_with_one_binary-7d7aff?style=for-the-badge)](#quickstart)

![Scout — migration discovery and readiness](docs/social/scout-hero-dark.jpg)

### Know every blocker. Before you migrate.

**Migration discovery and readiness before migration risk.** Zyvor Scout is a read-only assessment tool for teams planning migrations from virtualized estates to KVM/KubeVirt and other open platforms. It discovers or imports VM inventory, evaluates compatibility, explains blockers, maps dependencies, groups workloads into migration waves, and serves a polished local dashboard from one Go binary.

**26 readiness rules** (20 per-VM + 6 estate-level) · **Ready · Review · Blocked** · **Migration waves** · **Read-only, files stay local** · **One Go binary, no third-party dependencies**

📖 **[Read the full docs](https://zyvorai.github.io/scout/)** — quickstart, import formats, architecture, and API.

</div>

> Scout assesses and plans. It does **not** modify source workloads or execute migrations.

---

## Why Scout

| When this happens… | Scout gives you… |
|---|---|
| The migration plan lives in a spreadsheet that hides the hard parts | Explainable findings for RDM/shared disks, vTPM, Secure Boot, passthrough devices, snapshot chains and legacy guests |
| Nobody can say which VMs are ready, only which ones look ready | Every VM starts at 100; each deduction has a rule ID, severity, explanation and recommendation, ending in Ready, Review or Blocked |
| Moving one VM breaks the app it talks to | A dependency graph from the inventory's connections, and migration waves of workloads that cut over together |
| The RVTools export cannot leave the customer network | `scout import` reads RVTools, storage, DR and TCO files locally; nothing is uploaded |
| An assessment tool that writes to vCenter is a non-starter | Read-only discovery; vCenter credentials stay in memory and are never written to inventory or report |
| Leadership wants a summary, engineers want the detail | A portable HTML report, an executive PDF, a workbook export and a JSON API from the same assessment |

Migration projects often begin with spreadsheets that hide the hard parts: RDM/shared disks, vTPM, Secure Boot, passthrough devices, snapshot chains, legacy guests, and application dependencies. Scout turns those facts into an explainable readiness score and an execution-oriented plan.

![Capabilities at a glance: Discover, Assess, Plan, Operate](docs/ux/readme-capabilities.jpg)

---

## See it live

![Zyvor Scout dashboard](docs/scout-dashboard.png)

The embedded dashboard served by `scout serve` on `http://127.0.0.1:18447`.

---

## How it fits together

![Inventory in, a migration plan out](docs/ux/readme-how-it-works.jpg)

```text
       discovery / JSON import / RVTools
                │
                ▼
           Inventory Model
                │
        ┌───────┴────────┐
        ▼                ▼
  Compatibility       Dependency
     Engine              Graph
        │                │
        └───────┬────────┘
                ▼
        Migration Waves
                │
        ┌───────┴──────────┐
        ▼                  ▼
   JSON API            HTML Report
        │
        ▼
 Embedded Web Dashboard
```

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for package boundaries and extension points.

---

## Quickstart

Requires Go 1.26+.

```bash
git clone https://github.com/zyvorai/scout.git
cd scout
make build

# Generate a safe demo inventory
./bin/scout scan --source demo --out scout.json

# Print machine-readable assessment
./bin/scout assess --file scout.json

# Open the local dashboard
./bin/scout serve --file scout.json
```

Visit `http://127.0.0.1:18447`.

```bash
./bin/scout report --file scout.json --out report.html
./bin/scout report --file scout.json --executive executive.html --pdf executive.pdf --workbook workbook.zip
```

---

## Capabilities

### Discover

- Demo inventory for safe local evaluation
- VMware vCenter REST discovery (session auth, no guessed fields)
- Strict JSON inventory import, a documented CSV format and a `scout-inventory/1` JSON schema, with `scout import --validate` for line-numbered errors
- Local RVTools / storage / DR / TCO inputs ([docs/IMPORT.md](docs/IMPORT.md)) — files stay on the machine that runs Scout

### Assess

- Explainable compatibility rule engine
- Ready / Review / Blocked classification
- Rules for encryption, RDM, shared disks, vTPM, Secure Boot, snapshots, GPU/PCI/USB/SR-IOV passthrough, guest tools, firmware, legacy OS, guest OS conversion support, hardware version, CBT/warm-migration limits, independent disks, legacy NICs
- Estate-level rules that see the whole inventory: duplicate MAC / UUID / name, VMs on datastores or networks missing from the inventory, shared-disk groups split across waves

### Plan

- Dashboard sections for estate-level findings, the wave-plan validator with what-if, and the readiness diff (all escaped, no build step)
- Dependency graph of connected workloads
- Migration waves that cut over together
- Downtime and maintenance-window planner (`scout plan`): warm vs cold copy time, max concurrency, exclusive groups
- Wave-plan validator and what-if (`scout plan --validate plan.json [--move vm=N]`): dependency order, exclusive groups, concurrency, window overruns, shared disks, with the VMs involved
- Read-only readiness diff (`scout diff old.json new.json --fail-on regression`): verdict changes, rules that started or stopped firing, score deltas, CI exit status
- SARIF 2.1.0 and spreadsheet-safe CSV findings export (`scout assess --format sarif|csv`)
- Portable HTML assessment report, executive PDF, workbook export

### Operate

- Zyvor-branded responsive dashboard embedded in one binary
- JSON API for integrations
- Localhost-by-default binding + security headers
- Docker / Podman (Compose + Quadlet), remote systemd deploy, Helm + Kustomize
- Unit and API tests, GitHub Actions CI

## VMware vCenter discovery

Dependency-free vCenter REST connector: API session auth, VM enumeration. v0.1.0 treats fields not returned by the basic VM endpoint as unknown rather than guessing them. Richer hardware/guest collection is on the roadmap.

```bash
export VCENTER_URL='https://vcenter.example.com'
export VCENTER_USERNAME='administrator@vsphere.local'
export VCENTER_PASSWORD='...'

./bin/scout scan --source vmware --out scout.json

# Lab / private PKI only — do not use in production
./bin/scout scan --source vmware --insecure --out scout.json
```

Prefer environment variables so passwords do not land in shell history.

## How scoring works

Every VM begins at 100. Rules emit one of three finding levels:

- **info** — context worth preserving; no readiness penalty by default
- **warning** — needs review or remediation
- **blocker** — excludes the workload from migration waves until resolved

Per-VM rules live in `internal/engine/engine.go` and `target_rules.go`; estate-level rules (which see every VM) live in `internal/engine/estate_rules.go`. Both are intentionally simple to extend.

## API

When `scout serve` is running:

| Method | Endpoint | Purpose |
|---|---|---|
| GET | `/api/v1/healthz` | Liveness |
| GET | `/api/v1/inventory` | Source inventory |
| GET | `/api/v1/assessments` | Per-VM readiness |
| GET | `/api/v1/summary` | Aggregate readiness |
| GET | `/api/v1/graph` | Dependency graph |
| GET | `/api/v1/estate/findings` | Estate-level (cross-VM) findings grouped by rule |
| GET | `/api/v1/estate` | Capacity, DR, architecture, TCO |
| GET/POST | `/api/v1/plan` | Downtime and maintenance-window plan under constraints |
| POST | `/api/v1/plan/validate` | Validate an edited wave plan, optional what-if moves |
| GET | `/api/v1/plan/draft` | Automatic waves as an editable plan |
| POST | `/api/v1/diff` | Compare an older assessment with the current one |
| GET | `/api/v1/sarif` | Findings as SARIF 2.1.0 |
| GET | `/api/v1/findings.csv` | Findings as spreadsheet-safe CSV |
| GET | `/api/v1/report` | Portable HTML report |
| POST | `/api/v1/analyze` | Analyze supplied inventory without persisting it |

See [docs/API.md](docs/API.md).

## Deploy

```bash
# Podman or Docker (auto-detects)
make container-up

# Remote systemd (cross-compile + SSH + smoke)
./scripts/deploy-remote.sh <host> [user] --port 19726

# Smoke an existing instance
SCOUT_URL=http://<host>:19726 ./scripts/smoke-remote.sh

# Kubernetes
kubectl apply -k deploy/k8s
helm upgrade --install scout deploy/helm/scout -n scout --create-namespace \
  --set-file inventory.json=./scout.json
```

Details: [docs/DEPLOY.md](docs/DEPLOY.md).

## Security model

- Source-side operations are read-only
- vCenter credentials stay in memory — never written to inventory or report
- Server binds to `127.0.0.1:18447` by default
- Conservative timeouts and an 8 MiB analysis-body limit
- CSP, frame, MIME-sniffing, referrer, and permissions headers on the dashboard
- Put Scout behind authenticated TLS termination before exposing it to a network

Read [SECURITY.md](SECURITY.md) before reporting vulnerabilities.

## Development

```bash
make test && make vet && make build
```

## Docs

| Doc | Topic |
|---|---|
| [zyvorai.github.io/scout](https://zyvorai.github.io/scout/) | Product docs |
| [docs/IMPORT.md](docs/IMPORT.md) | RVTools, storage, DR, TCO inputs |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Package boundaries |
| [docs/API.md](docs/API.md) | HTTP API |
| [docs/DEPLOY.md](docs/DEPLOY.md) | Container, remote, Helm |
| [docs/ROADMAP.md](docs/ROADMAP.md) | Next milestones |

Social assets: [docs/social/](docs/social/).

---

## Maturity

Scout is at **v0.1.0**. v0.1.0 treats vCenter fields not returned by the basic VM endpoint as unknown rather than guessing them; richer hardware/guest collection is on the roadmap.

The scope boundary is **discovery + assessment + dependency planning**. Execution systems can consume Scout's JSON without making Scout responsible for cutover or source mutation.

Planned: richer vSphere hardware collection, libvirt/OpenStack/Hyper-V discovery, guest inspection adapters, traffic-derived dependencies, rule packs, signed assessment bundles, orchestrator export adapters.

See [docs/ROADMAP.md](docs/ROADMAP.md).

---

## Part of the Zyvor stack

| Product | Role next to Scout |
|---|---|
| **Scout** | Read-only discovery, readiness scoring and wave planning |
| **[Transiva](https://github.com/zyvorai/zyvor-transiva)** | Discovers and exports VMs from VMware vSphere and Nutanix AHV, after the plan |
| **[hyper2kvm](https://github.com/zyvorai/zyvor-h2kvm)** | VM migration into KVM |
| **[GuestKit](https://github.com/zyvorai/zyvor-guestkit)** | Guest inspection; Scout marks Windows guests from RVTools for GuestKit inspection of VirtIO state |
| **[Zorvia](https://github.com/zyvorai/zyvor-zorvia)** | KubeVirt VM platform, a migration target |

Scout's JSON is the hand-off point; execution systems consume it without making Scout responsible for cutover. → [zyvor.dev](https://zyvor.dev)

---

## Support

A 30-day trial is available on request from sales@zyvor.dev (a paid evaluation Order at no charge); production and any other use needs a paid Order under the Zyvor Production Licence v2.0. If it helps you, you can donate any amount you like: <https://rzp.io/rzp/utwdYh3>. A donation is voluntary and supports development and maintenance. It is not a purchase: it buys no support, SLA, features, priority or license, and it is not tax-deductible (Zyvor AI Labs Private Limited is a company, not a charity). Support contracts are separate: [sales@zyvor.dev](mailto:sales@zyvor.dev).

## License

Scout is source-available under the **[Zyvor Production Licence v2.0](LICENSE)** (SPDX `LicenseRef-Zyvor-Production-2.0`, also in [LICENSES/](LICENSES/LicenseRef-Zyvor-Production-2.0.txt); see [NOTICE](NOTICE)).

- **Trial:** a 30-day trial is available on request from [sales@zyvor.dev](mailto:sales@zyvor.dev) (a paid evaluation Order at no charge).
- **Production and any other use needs a paid Order** under the Zyvor Production Licence v2.0. Buy / get a quote: [zyvor.dev/buy](https://zyvor.dev/buy) · [sales@zyvor.dev](mailto:sales@zyvor.dev).
- **Price:** INR 49,999 per assessment (one-time) incl. 18% GST in India; US$504 per assessment (one-time) excl. tax elsewhere. Buy: [zyvor.dev/buy](https://zyvor.dev/buy). Orders of USD 5,000 or more and quotes are invoiced in USD through Skydo.

**Zyvor Enterprise** adds what production teams ask for: supported releases, deployment and upgrade guidance, priority incident triage, a named technical contact and 24x7 critical intake. Plans and terms: [docs/SUBSCRIPTION-MODEL.md](docs/SUBSCRIPTION-MODEL.md) · [Pricing](https://zyvor.dev/pricing?utm_source=github&utm_medium=scout&utm_campaign=readme_license) · [Buy / get a quote](https://zyvor.dev/buy) · [sales@zyvor.dev](mailto:sales@zyvor.dev).

Read [SECURITY.md](SECURITY.md) before reporting vulnerabilities. Contributing: [CONTRIBUTING.md](CONTRIBUTING.md).

---

<div align="center">

### Find the blockers before your cutover does

[![Book a demo](https://img.shields.io/badge/Book_a_demo-0071e3?style=for-the-badge)](https://zyvor.dev/schedule?utm_source=github&utm_medium=scout&utm_campaign=readme_footer)
[![30-day PoC](https://img.shields.io/badge/Start_a_30--day_PoC-000000?style=for-the-badge)](https://zyvor.dev/poc?utm_source=github&utm_medium=scout&utm_campaign=readme_footer)
[![Pricing](https://img.shields.io/badge/Pricing-1d1d1f?style=for-the-badge)](https://zyvor.dev/pricing?utm_source=github&utm_medium=scout&utm_campaign=readme_footer)
[![Contact sales](https://img.shields.io/badge/Contact_sales-7d7aff?style=for-the-badge)](mailto:sales@zyvor.dev?subject=Scout)
[![Star on GitHub](https://img.shields.io/github/stars/zyvorai/scout?style=for-the-badge&logo=github&label=Star&color=2997ff)](https://github.com/zyvorai/scout)

</div>

## Optional: Gateway API front door (Setura)

Besides the classic Ingress, the Helm chart can expose the service through a [Setura](https://github.com/zyvorai/zyvor-setura-gateway) Gateway API front door. It is off by default and the Ingress is unchanged.

```sh
# once per cluster: install the Setura controller
setura gateway install --image ghcr.io/zyvorai/zyvor-setura-gateway:0.7.0 --namespace setura-system | kubectl apply -f -

# then switch the chart from Ingress to Gateway (enable one, not both)
helm upgrade --install scout deploy/helm/scout --set gateway.enabled=true --set ingress.enabled=false
```

The Gateway and HTTPRoute mirror the Ingress hosts, paths and backends. Set `gateway.tls.enabled=true` for an HTTPS listener.
