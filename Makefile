BINARY := scout
PKG := ./...
IMAGE ?= ghcr.io/zyvorai/scout:0.1.0
HELM_CHART := deploy/helm/scout
# Prefer Podman when both are installed; override with CTR=docker.
CTR ?= $(shell command -v podman >/dev/null 2>&1 && echo podman || echo docker)

.PHONY: build test vet run demo report image container-up container-down container-run container-smoke helm-lint kustomize-build smoke deploy-remote clean

build:
	mkdir -p bin
	go build -trimpath -ldflags="-s -w" -o bin/$(BINARY) ./cmd/scout

test:
	go test $(PKG)

vet:
	go vet $(PKG)

run:
	go run ./cmd/scout serve --file sample/inventory.json

demo:
	go run ./cmd/scout scan --source demo --out scout.json

report:
	go run ./cmd/scout report --file sample/inventory.json --out report.html

image:
	./scripts/container.sh build

container-up:
	./scripts/container.sh up --build

container-down:
	./scripts/container.sh down

container-run:
	./scripts/container.sh run

container-smoke:
	./scripts/container.sh smoke

helm-lint:
	helm lint $(HELM_CHART)
	helm template scout $(HELM_CHART) >/dev/null

kustomize-build:
	kubectl kustomize deploy/k8s >/dev/null

smoke:
	./scripts/smoke-remote.sh

deploy-remote:
	./scripts/deploy-remote.sh $(ARGS)

clean:
	rm -rf bin dist coverage.out report.html scout.json
	-./scripts/container.sh down >/dev/null 2>&1 || true
	-./scripts/container.sh stop >/dev/null 2>&1 || true
