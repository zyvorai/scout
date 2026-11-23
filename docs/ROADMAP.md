# Roadmap

This roadmap describes likely public work and is not a release commitment.

## Discovery

- RVTools `.xlsx` import (`vInfo`, `vDisk`, `vSnapshot`, `vHost`, `vDatastore`, `vPartition`, `vNetwork`) — see [IMPORT.md](IMPORT.md)
- Richer vSphere VM hardware, disks, NICs, guest OS, snapshots, firmware, TPM, and device inventory
- libvirt connector
- OpenStack connector
- Hyper-V connector
- OVF/OVA import adapters

## Assessment

- Versioned rule packs
- Target profiles for KubeVirt, plain libvirt/KVM, and selected cloud targets
- Guest driver readiness
- Guest OS support, hardware version, CBT, independent disk and legacy NIC rules (shipped; see RESEARCH.md for what remains)
- Windows VirtIO readiness
- Storage conversion estimates
- Network policy translation findings
- Custom organization rules

## Dependency intelligence

- Flow import from PacketWolf/eBPF collectors
- Firewall-flow and NetFlow import
- DNS/application dependency enrichment
- Confidence scores and observation windows

## Planning

- Capacity-aware wave packing
- Maintenance-window, concurrency and exclusive-group constraints with warm/cold downtime estimates (shipped, `scout plan`)
- Affinity/anti-affinity preservation
- Site/rack/failure-domain constraints
- Estimated transfer time by datastore and link capacity

## Packaging and deploy

- Single binary + Docker / Podman (Compose + Quadlet)
- Remote systemd deploy (`scripts/deploy-remote.sh`) + API smoke (`scripts/smoke-remote.sh`)
- Helm chart (`deploy/helm/scout`)
- Kustomize manifests (`deploy/k8s`)

## Integration

- Signed assessment bundle format
- Webhooks
- OpenAPI specification
- Export adapter for migration orchestrators such as Transiva
- CI-friendly policy gate mode
