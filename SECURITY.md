# Security

Please do not open public issues for suspected vulnerabilities. Report security issues privately to the maintainers through the repository's GitHub Security Advisories feature.

Scout is designed to run inside a customer's environment. Credentials are held only in memory and are never written to reports. The web server binds to `127.0.0.1:18447` by default. Use a reverse proxy with authentication and TLS before exposing it beyond localhost.

Scout deploy examples (binary, Docker, Helm, Kustomize): [docs/DEPLOY.md](docs/DEPLOY.md).
