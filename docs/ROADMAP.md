# Roadmap

This roadmap describes likely open-source work and is not a release commitment.

## Discovery

- Richer vSphere VM hardware, disks, NICs, guest OS, snapshots, firmware, TPM, and device inventory
- libvirt connector
- OpenStack connector
- Hyper-V connector
- OVF/OVA and RVTools import adapters

## Assessment

- Versioned rule packs
- Target profiles for KubeVirt, plain libvirt/KVM, and selected cloud targets
- Guest driver readiness
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
- Maintenance-window constraints
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
