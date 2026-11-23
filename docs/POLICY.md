# Policy file: waivers and rule overrides

`scout assess --policy policy.json` applies an organisation policy on top of the built-in rules. Use it when a finding is real but accepted ("this VM keeps its RDM, tracked in CHG-1234") or when a rule does not fit your environment.

A policy never hides a decision:

- Waived findings leave the score and status but stay in the output under `waived`, with who waived them, why, and when the waiver ends.
- Disabled rules, lowered severities and waivers are all listed in the `policy` section of the JSON and summarised on stderr.
- A waiver cannot last forever: `expires` is required and may be at most 366 days away. An expired waiver stops applying, and the findings it covered become active again and are reported.
- Unknown rule ids, unknown fields and malformed values are errors, so a typo cannot silently waive nothing.

## Format (`scout-policy/1`)

```json
{
  "schema": "scout-policy/1",
  "name": "Q4 migration policy",
  "rules": {
    "rdm-disk": { "severity": "warning", "reason": "RDMs are migrated with the storage team's own tool" },
    "secure-boot": { "penalty": 2 },
    "legacy-nic": { "disabled": true, "reason": "All guests get VirtIO NICs during cutover anyway" }
  },
  "waivers": [
    {
      "rule": "encrypted-vm",
      "vm": "web-*",
      "reason": "Vendor appliance, decrypted at cutover (CHG-1234)",
      "owner": "platform-team",
      "expires": "2027-01-31"
    }
  ]
}
```

| Field | Meaning |
|---|---|
| `rules.<id>.disabled` | Drop every finding of the rule. Needs `reason` (at least 10 characters). Cannot be combined with `severity` or `penalty`. |
| `rules.<id>.severity` | `info`, `warning` or `blocker`. Needs `reason`. Lowering a blocker is counted in the report (`blockersLowered`). |
| `rules.<id>.penalty` | Score penalty 0 to 100, replacing the rule's default. |
| `waivers[].rule` | A built-in rule id (`scout policy rules` lists them). |
| `waivers[].vm` | Exact VM name or id, or a `path.Match` pattern such as `web-*`. A bare `*` is rejected: disable the rule instead, with a reason. |
| `waivers[].reason` | At least 10 characters. Shown wherever the waiver is shown. |
| `waivers[].owner` | Optional. Shown as `waivedBy`. |
| `waivers[].expires` | `YYYY-MM-DD`, required. The waiver is valid through that day. |

Limits: 1 MiB file, 5,000 waivers, 500 rule overrides, 500 characters per text field, no control characters.

## Commands

```text
scout policy check policy.json     validate; lists expired waivers and those ending within 30 days (exit 1 if any have expired)
scout policy rules                 print the rule ids a policy may name
scout assess --file scout.json --policy policy.json [--format json|sarif|csv] [--fail-on-expired]
```

`--fail-on-expired` makes `assess` exit 1 when a waiver has expired or matched no finding, so a pipeline notices stale waivers. SARIF and CSV exports contain only the findings that still count; the JSON output carries the `waived` entries and the `policy` report. Because `scout diff` compares assessment JSON, a diff between two policy-applied assessments compares what still counts.

## What the report says

```json
"policy": {
  "counts": { "findingsWaived": 1, "blockersWaived": 1, "blockersLowered": 0, "findingsDropped": 0,
              "expiredWaivers": 0, "unusedWaivers": 1, "expiringWithin30Days": 0 },
  "waived": [ { "vm": "web-01", "rule": "encrypted-vm", "severity": "blocker", "reason": "...", "expires": "2027-01-31" } ],
  "unusedWaivers": [ { "rule": "encrypted-vm", "vm": "ghost-*" } ]
}
```

An unused waiver usually means a renamed VM or a mistyped selector.

## Limits

- The policy is applied by the `assess` command. The dashboard (`scout serve`), the HTML report and `scout plan` do not apply it yet, so they still show every finding.
- Waivers match the VM's name or id, not tags or folders.
- Estate-level findings (duplicate MAC, shared disks across waves) can be waived like any other finding, but the waiver applies to the VM that carries the finding.
- Dates use UTC days. A waiver is a record of an accepted risk, not a technical control: Scout does not check that the stated reason is true.
