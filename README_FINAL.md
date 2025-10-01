# ACME-DNS mit Web UI - Finale Struktur

## 📁 Übersicht der finalen Dateien

### Wichtige Dateien die BEHALTEN werden:

```
acme-dns/
├── Dockerfile                    # Standard acme-dns Server Image
├── Dockerfile.combined           # Kombiniertes Image mit UI
├── docker-compose.combined.yml  # Docker Compose für kombiniertes Image
├── docker-compose.yml           # Original Docker Compose
├── README_COMBINED.md           # Dokumentation für kombinierte Lösung
├── .github/
│   └── workflows/
│       └── docker-combined.yml  # GitHub Workflow für kombiniertes Image
├── main.go                      # Modifiziert mit UI Support
├── api.go                       # Mit /domains Endpunkt
├── db.go                        # Mit GetAllDomains() Funktion
├── types.go                     # Mit erweitertem Interface
├── acmetxt.go                   # Mit Fulldomain Feld
└── ui/                          # Angular UI Projekt
    ├── Dockerfile               # Standalone UI Image
    ├── src/
    │   └── app/
    │       ├── config/
    │       │   └── app.config.ts    # Login Credentials
    │       ├── environments/
    │       │   ├── environment.ts      # API URL Konfiguration
    │       │   └── environment.prod.ts # Production API URL
    │       └── services/
    │           └── acme-dns.service.ts # API Service mit direkten Calls
    └── [weitere UI Dateien]
```

## 🚀 Deployment Optionen

### Option 1: Kombiniertes Image (EMPFOHLEN)
**Ein Container mit Server + UI**

```bash
# Image bauen
docker build -f Dockerfile.combined -t ghcr.io/netzint/acme-dns:combined .

# Starten
docker run -d \
  -p 53:53/tcp -p 53:53/udp \
  -p 80:80 \
  -v $(pwd)/config:/etc/acme-dns:ro \
  -v $(pwd)/data:/var/lib/acme-dns \
  ghcr.io/netzint/acme-dns:combined
```

**Zugriff:**
- UI: http://localhost/ui/
- API: http://localhost/health, /register, etc.

### Option 2: Separate Container
**Server und UI getrennt**

```bash
# Nur wenn du UI und Server getrennt willst
docker-compose up -d
```

## 🔧 GitHub Actions

### Aktiver Workflow:
- `.github/workflows/docker-combined.yml` - Baut kombiniertes Image

## 📝 Konfiguration

### UI Login (ui/src/app/config/app.config.ts):
```typescript
username: 'admin'
password: 'admin123'  // ÄNDERN!
```

### API Konfiguration (config.cfg):
```ini
[api]
ip = "0.0.0.0"
port = "80"
```

## 🗑️ Gelöschte Dateien

Folgende Dateien wurden entfernt (nicht mehr benötigt):
- Alle alternativen docker-compose Dateien
- Alle alternativen Dockerfiles
- Alle alternativen Workflows
- Nginx Konfigurationen
- Temporäre Hilfsdateien

## ✅ Finale Lösung

- **Ein Repository**: github.com/netzint/acme-dns
- **Ein Image**: ghcr.io/netzint/acme-dns:combined
- **Ein Container**: Server + UI zusammen
- **Keine Proxy**: Direkter Zugriff
- **Keine CORS**: Same Origin