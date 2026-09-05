# Deploy Scout

Scout runs in your environment — binary, Podman/Docker, Kubernetes, or remote systemd. Not a hosted SaaS.

| Target | Command |
|---|---|
| Local binary | `make build && ./bin/scout serve --file scout.json` |
| Podman / Docker Compose | `make container-up` |
| Podman Quadlet | `deploy/podman/scout.container` |
| Remote systemd | `./scripts/deploy-remote.sh <host> [user]` |
| Kubernetes | `kubectl apply -k deploy/k8s` |
| Helm | `helm upgrade --install scout deploy/helm/scout -n scout --create-namespace` |

Default binary bind: `127.0.0.1:18447`. Container / remote packs listen on `:18447`.

## Local

```bash
make build
./bin/scout scan --source demo --out scout.json
./bin/scout serve --file scout.json
```

## Podman / Docker

Runtime is auto-detected (`podman` preferred when both are installed). Override with `CTR=docker`.

```bash
./scripts/container.sh which          # show CTR + compose frontend
make container-up                     # compose up --build (detached)
make container-smoke                  # API smoke against localhost:18447
make container-down

# One-shot container (no compose)
./scripts/container.sh run ./scout.json

# Manual equivalents
podman compose up --build
docker compose up --build
CTR=podman make image
```

Environment knobs:

| Variable | Default | Purpose |
|---|---|---|
| `CTR` | auto | `podman` or `docker` |
| `COMPOSE` | auto | e.g. `podman compose` |
| `IMAGE` | `ghcr.io/zyvorai/scout:0.1.0` | Image tag |
| `SCOUT_PORT` | `18447` | Host port |
| `SCOUT_INVENTORY_DIR` | `./sample` | Compose bind mount |

Rootless Podman tips:

- Prefer `./scripts/container.sh run` — it stages a `0644` inventory, adds SELinux `:z`, and uses `--userns=keep-id`.
- Binding host port 18447 is rootless-friendly; override with `SCOUT_PORT` if needed.
- Compose mounts `./sample` with `:z`; inventory files must be readable by UID 65532 (mode `0644`).

### Quadlet (systemd + Podman)

```bash
podman build -t ghcr.io/zyvorai/scout:0.1.0 .
sudo mkdir -p /etc/scout
sudo cp sample/inventory.json /etc/scout/inventory.json
sudo cp deploy/podman/scout.container /etc/containers/systemd/
sudo systemctl daemon-reload
sudo systemctl start scout
```

Rootless: install the unit under `~/.config/containers/systemd/` and use `systemctl --user`.

## Remote host (systemd binary)

Same pattern as Chimera: cross-compile locally, install over SSH, start `scout.service`, then smoke-test the live URL. No Go toolchain on the target.

```bash
./scripts/deploy-remote.sh 212.8.248.187 sus
SCOUT_INVENTORY_SRC=./scout.json ./scripts/deploy-remote.sh 212.8.248.187 sus
SCOUT_URL=http://212.8.248.187:18447 ./scripts/smoke-remote.sh
./scripts/deploy-remote.sh 212.8.248.187 sus --uninstall
```

## Kubernetes (Kustomize)

```bash
kubectl apply -k deploy/k8s
kubectl -n scout port-forward svc/scout 18447:18447
```

Optional Ingress: `deploy/k8s/ingress.yaml` (enable only with your own TLS and auth).

## Helm

```bash
helm upgrade --install scout deploy/helm/scout -n scout --create-namespace \
  --set-file inventory.json=./scout.json
```

## Security

Bind localhost for local binary use. Container and remote deploys listen on all interfaces inside their network namespace — put TLS and auth at the edge before exposing beyond a trusted network. See [SECURITY.md](../SECURITY.md).
