# Changelog

## Unreleased

### Dependency and toolchain upgrades
- Go: `go.mod` directive 1.23 -> 1.26; CI now tests Go 1.26.x and 1.27.x; Dockerfile build image `golang:1.27.4-alpine`, runtime `distroless/static-debian13:nonroot`.
- GitHub Actions: checkout v7, setup-go v7, setup-node v7, upload-pages-artifact v5, deploy-pages v5.
- Website: all npm dependencies updated to latest (react/react-dom 19.3, @mdx-js/react 3.1, clsx 2.1, prism-react-renderer 2.4, @types/react 19.3); `npm ci && npm run build` passes.
- README Go badge and requirement updated to 1.26+.

### Held back
- `typescript` stays at `~6.0.2` (latest is 7.0.x): `@docusaurus/tsconfig` still sets the `baseUrl` option removed in TypeScript 7, so `npm run typecheck` fails. Revisit when Docusaurus ships a TS7-compatible tsconfig.
- Scout/chart/image version `0.1.0` is the product version, not a dependency pin, so it is unchanged.
- Pre-existing, unrelated: docs/DEPLOY.md links `../SECURITY.md`, which the Docusaurus build reports as a broken link (warning only).
