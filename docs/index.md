---
hero:
  eyebrow: MIGRATION READINESS
  title: Zyvor Scout
  lead: >-
    Migration discovery and readiness before migration risk. A read-only
    assessment tool that discovers or imports VM inventory, evaluates
    compatibility, explains blockers, and groups workloads into migration
    waves.
  swatches:
    - {label: "v0.1.0"}
    - {label: "Apache-2.0"}
    - {label: "Go 1.23+"}
  highlights:
    - {value: "12", label: "Compatibility categories checked by built-in rules", footnote: "1"}
    - {value: "7", label: "JSON API endpoints for integrations", footnote: "2"}
    - {value: "3", label: "Finding severities — info, warning, blocker", footnote: "1"}
    - {value: "8 MiB", label: "Max request body enforced on the analyze endpoint", footnote: "2"}
  hub_bands:
    - {icon: "🧩", title: "Architecture", description: "Scout is deliberately a single-binary Go application with no JavaScript build chain.", href: "ARCHITECTURE.md"}
    - {icon: "🔌", title: "API", description: "The API is served by scout serve and is intended for local integrations and migration planning systems.", href: "API.md"}
    - {icon: "🚀", title: "Deploy", description: "Scout runs in your environment — binary, Podman/Docker, Kubernetes, or remote systemd. Not a hosted SaaS.", href: "DEPLOY.md"}
    - {icon: "🗺️", title: "Roadmap", description: "This roadmap describes likely open-source work and is not a release commitment.", href: "ROADMAP.md"}
footnotes:
  - {marker: "1", text: "Built-in rules inspect encryption, RDM, shared disks, vTPM, Secure Boot, snapshot depth, GPU/PCI passthrough, SR-IOV, USB passthrough, guest tools, firmware mode, and legacy operating systems — 12 categories in total. Every VM starts at 100 and rules emit info, warning, or blocker findings.", href: "https://github.com/zyvorai/scout#how-scoring-works", href_label: "See \"How scoring works\" in the README."}
  - {marker: "2", text: "healthz, inventory, assessments, summary, graph, report, and analyze — 7 endpoints. The analyze endpoint caps request bodies at 8 MiB.", href: "API.md", href_label: "See the API guide."}
---

Zyvor Scout is an Apache-2.0, read-only assessment tool for infrastructure
teams planning migrations from virtualized environments to KVM/KubeVirt and
other open platforms. It discovers or imports VM inventory, evaluates
compatibility, explains blockers, maps workload dependencies, groups
workloads into migration waves, and serves a polished local dashboard from
one Go binary.

> Scout assesses and plans. It does **not** modify source workloads or
> execute migrations.

![Zyvor Scout dashboard](scout-dashboard.png)

## Why Scout

Migration projects often begin with spreadsheets that hide the hard parts:
RDM/shared disks, vTPM, Secure Boot, passthrough devices, snapshot chains,
legacy guests, and application dependencies. Scout turns those facts into
an explainable readiness score and an execution-oriented plan.

## Included in v0.1.0

<div class="icon-badge-list" markdown="1">

- 🔍 VMware vCenter REST discovery
- 📥 Strict JSON inventory import
- 🧠 Explainable compatibility rule engine
- 🚦 Ready / Review / Blocked classification
- 🕸️ Dependency graph
- 🌊 Connected-workload migration waves
- 📄 Portable HTML assessment report
- 🔌 JSON API for integrations

</div>

## Quick start

Requires Go 1.23+.

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

## Project direction

The intended open-source boundary is **discovery + assessment + dependency
planning**. Execution systems can consume Scout's JSON output without
making Scout responsible for cutover or source mutation.

See the full [README on GitHub](https://github.com/zyvorai/scout#readme)
for the complete quick start, security model, and license, and the
[Roadmap](ROADMAP.md) for planned work.
