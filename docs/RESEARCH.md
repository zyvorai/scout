# Research: what migration assessment tools check

Notes from a survey of VMware-to-KVM/KubeVirt assessment tooling, used to choose the rules added after the first 14. Snapshot date: 2026-11. Claims come from vendor docs, release notes and community reports; vendor docs differ between versions, so verify against the release you target.

## What the tools check

### Red Hat Migration Toolkit for Virtualization (Forklift)
A validation service evaluates each VM with Open Policy Agent (Rego) rules and attaches concerns with category Critical, Warning or Information. Custom rules load from a ConfigMap. Documented concerns include TPM (warning), disks on datastores that no longer exist (critical), illegal or locked disks. Critical does not always mean blocked.
Warm migration additionally needs VMware Tools, changed block tracking (CBT) on the VM and every disk, and works only for vSphere/RHV sources. Warm migration chains one CBT snapshot per precopy cycle, with a documented cap of 28 (some pages say 32). On Windows, VSS and the VMware Snapshot Provider must not be disabled.
- https://docs.redhat.com/en/documentation/migration_toolkit_for_virtualization/2.11/html/planning_your_migration_to_red_hat_openshift_virtualization/assembly_cold-warm-migration_mtv
- https://docs.redhat.com/en/documentation/migration_toolkit_for_virtualization/2.11/html/migrating_your_virtual_machines_to_red_hat_openshift_virtualization/assembly_troubleshooting-migration_mtv
- https://docs.redhat.com/en/documentation/migration_toolkit_for_virtualization/2.9/html-single/installing_and_using_the_migration_toolkit_for_virtualization/index

### virt-v2v
Supported guests are a Red Hat table (RHEL 5-10, Windows 8 through Server 2025, varying by host version). Known failure causes: Windows Fast Startup or hibernation enabled, missing virtio-win drivers on the conversion host, encrypted VMs in offline mode, many or old snapshots, guests that were not cleanly shut down, and old virt-v2v releases crashing on newer Windows inspection. It does not reconfigure guest networking, so NIC names change. SLES/Btrfs guests can be slow.
- https://access.redhat.com/articles/1351473
- https://github.com/harvester/harvester/issues/11860
- https://trilio.io/vmware-migration/virt-v2v/

### Proxmox VE import wizard
Tested against ESXi 6.5 to 8.0; importing through vCenter is much slower than from the host; snapshots slow the import; live import still has downtime; ESXi API rate limiting causes failures on parallel disk reads.
- https://pve.proxmox.com/wiki/Migrate_to_Proxmox_VE

### Azure Migrate (VMware assessment)
Readiness per VM: Ready, Conditionally ready, Not ready, Unknown. Inputs: OS endorsed or not, boot type (BIOS/UEFI), disk count and per-disk size limits, unidentifiable OS ("Other") not assessed. Import-based assessments treat a missing boot type as BIOS.
- https://learn.microsoft.com/en-us/azure/migrate/concepts-assessment-calculation?view=migrate-classic
- https://learn.microsoft.com/en-us/azure/migrate/review-assessment?view=migrate

### RVTools, Live Optics, AWS Application Discovery
RVTools `vInfo` carries `HW version`, `CBT`, firmware, tools status and OS; `vDisk` carries disk mode; `vNetwork` carries adapter type. Live Optics and AWS Application Discovery add performance history for right-sizing and dependency data. I did not retrieve their column specifications (see caveats).

## What Scout had and lacked (before this change)

Had: encryption, RDM, shared disk, TPM, Secure Boot, snapshot count, GPU/PCI, SR-IOV, USB, tools health, firmware, a small legacy-OS list, Windows VirtIO evidence gap, aged/orphan snapshots, per-VM cutover estimate, dependency waves.

Lacked: guest OS conversion support, virtual hardware version, CBT state (warm vs cold), the warm-migration snapshot cap, independent disk mode, legacy NIC models, any notion of downtime by wave, concurrency or maintenance windows, and machine-readable export for CI.

## Ranked ideas

| # | Idea | Status |
|---|---|---|
| 1 | Warm/cold downtime and maintenance-window planner with concurrency and exclusive groups | Built (`internal/plan`) |
| 2 | Target-compatibility rules: guest OS support, hardware version, CBT, snapshot cap, independent disks, legacy NICs | Built (6 rules) |
| 3 | SARIF and CSV export for CI gating and spreadsheets, with CSV formula-injection protection | Built |
| 4 | RVTools column mapping for `HW version`, `CBT`, disk mode, NIC adapter | Built |
| 5 | Duplicate MAC / UUID / name, missing datastore and network, shared-disk groups across waves (needs a cross-VM rule interface) | Built: `EstateRule` interface, 6 rules (duplicate IP is not built: IPs change at cutover and are often placeholders) |
| 6 | Windows Fast Startup / hibernation, VSS state | Rejected: not present in any inventory export; needs guest inspection |
| 7 | Disk size limits per target storage class (Azure-style) | Rejected: KubeVirt limits depend on the CSI driver, not a fixed number |
| 8 | Live Optics / vCenter JSON import | Still deferred: formats not verified. Instead built a self-defined CSV format and `scout-inventory/1` JSON with a schema file |
| 9 | Datastore-missing and illegal/locked disk concerns (MTV) | Missing datastore built as a cross-reference against the storage evidence; locked/illegal disk status still deferred (not in RVTools) |
| 10 | Pluggable custom rules (Rego/ConfigMap style) | Deferred: Scout has no third-party dependencies, so it would need its own DSL |

## Rule thresholds and why

- `guest-os-support`: blocker for Solaris, NetWare, OS/2, macOS and DOS (no virt-v2v/KubeVirt path); warning for Windows XP/2000/NT/Vista/2003/9x/ME (outside virtio-win), BSD (not converted by virt-v2v), and unidentified guest types (basis `not-measured`). This is Scout's own approximation of the Red Hat table, not a copy of it.
- `hw-version`: below 7, predating CBT.
- `cbt-disabled`: info only, and only when CBT is explicitly false on a powered-on VM; unknown stays silent.
- `snapshot-cbt-limit`: warns from 24 snapshots, a margin below the documented cap of 28.
- `independent-disk`: independent disks are excluded from snapshots, so snapshot-based warm copy cannot capture them.
- `legacy-nic`: vmxnet, vmxnet2, flexible and pcnet32 have no KVM equivalent.

## Caveats

- Web research was done through search summaries; the MTV snapshot cap is reported as 28 in most pages and 32 in one. Scout uses 28.
- The planner assumes a fixed link efficiency (0.7) and a 5 percent final delta for warm migration. These are defaults, adjustable per request, not measurements.
- Live Optics, AWS Application Discovery and Harvester import details were not retrieved.
