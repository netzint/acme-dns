# GitHub Actions für acme-dns Docker Image

## Übersicht

Dieses Repository enthält GitHub Actions Workflows zum automatischen Bauen und Veröffentlichen von Docker Images in der GitHub Container Registry (ghcr.io) und optional auf Docker Hub.

## Workflows

### 1. `docker-build.yml` (Einfacher Workflow)
- **Trigger:** Push auf main/master, Tags, Pull Requests
- **Features:**
  - Multi-Platform Build (amd64, arm64, arm/v7)
  - Automatisches Tagging
  - GitHub Container Registry Push

### 2. `docker-release.yml` (Erweiterter Workflow)
- **Trigger:** Push auf main/master/develop, Tags, Pull Requests, manuell
- **Features:**
  - Multi-Platform Build
  - Security Scanning mit Trivy
  - Container Signierung mit Cosign
  - SBOM Generation
  - Automatische GitHub Releases bei Tags
  - Optional: Docker Hub Push

## Setup

### 1. Repository Permissions
Gehe zu Settings → Actions → General → Workflow permissions:
- ✅ Read and write permissions
- ✅ Allow GitHub Actions to create and approve pull requests

### 2. Docker Hub (Optional)
Wenn du auch auf Docker Hub pushen möchtest:
1. Gehe zu Settings → Secrets and variables → Actions
2. Füge folgende Secrets hinzu:
   - `DOCKERHUB_USERNAME`: Dein Docker Hub Benutzername
   - `DOCKERHUB_TOKEN`: Dein Docker Hub Access Token

### 3. Workflow aktivieren
1. Pushe die `.github/workflows/` Dateien zu deinem Repository
2. Die Workflows werden automatisch aktiviert

## Verwendung

### Image pullen von GitHub Container Registry:
```bash
# Latest
docker pull ghcr.io/netzint/acme-dns:latest

# Specific version
docker pull ghcr.io/netzint/acme-dns:v1.0.0

# Branch build
docker pull ghcr.io/netzint/acme-dns:main
```

### Docker Compose verwenden:
```bash
# Mit ghcr.io Image
docker-compose -f docker-compose.ghcr.yml up -d
```

## Image Tags

Die folgenden Tags werden automatisch erstellt:

- `latest` - Neuester Build vom main/master Branch
- `v1.0.0` - Semantic Version Tags
- `1.0` - Major.Minor Version
- `1` - Major Version
- `main` - Branch Name
- `main-abc123` - Branch + Short SHA
- `20240101-abc123def` - Datum + SHA (nur main branch)

## Features

### Architecture Support
Images werden für folgende Architektur gebaut:
- `linux/amd64` (x86_64)

### Security
- **Trivy Scanning:** Automatische Vulnerability Scans
- **Cosign Signing:** Kryptographische Signatur der Images
- **SBOM:** Software Bill of Materials Generation
- **Non-root User:** Container läuft als non-root user

### Optimierungen
- **Multi-stage Build:** Kleinere finale Images
- **Layer Caching:** Schnellere Builds durch GitHub Actions Cache
- **Health Checks:** Eingebaute Gesundheitsprüfung

## Manueller Trigger

Du kannst den Workflow auch manuell triggern:
1. Gehe zu Actions → Docker Build and Release
2. Klicke auf "Run workflow"
3. Wähle Branch und optionales Tag
4. Klicke auf "Run workflow"

## Dockerfile

Der Workflow nutzt `Dockerfile.improved` mit:
- Multi-stage Build
- Non-root User
- Health Check
- Optimierte Layer
- Security Best Practices

## Monitoring

### Build Status
- Siehe Actions Tab im GitHub Repository
- Badge für README: `![Build](https://github.com/netzint/acme-dns/actions/workflows/docker-release.yml/badge.svg)`

### Security Alerts
- Vulnerabilities werden im Security Tab angezeigt
- Automatische Dependabot Alerts

## Troubleshooting

### Build fehlgeschlagen
1. Prüfe die Logs im Actions Tab
2. Stelle sicher, dass alle Dateien korrekt sind
3. Prüfe die Permissions

### Push fehlgeschlagen
1. Prüfe die Repository Permissions
2. Für Docker Hub: Prüfe die Secrets
3. Stelle sicher, dass der Namespace korrekt ist

### Image zu groß
- Verwende den optimierten `Dockerfile.improved`
- Prüfe unnötige Dependencies
- Nutze Multi-stage Build

## Links

- [GitHub Container Registry Docs](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)
- [Docker Buildx](https://docs.docker.com/buildx/working-with-buildx/)
- [Cosign](https://docs.sigstore.dev/cosign/overview/)
- [Trivy](https://aquasecurity.github.io/trivy/)