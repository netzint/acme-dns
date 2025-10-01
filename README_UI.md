# ACME-DNS with Web UI

This repository includes both the acme-dns server and a web UI for managing domains.

## Quick Start

### Using Docker Compose (Recommended)

```bash
# Clone repository
git clone https://github.com/netzint/acme-dns.git
cd acme-dns

# Start both services
docker-compose -f docker-compose.all.yml up -d
```

Services:
- **acme-dns server**: Port 53 (DNS) and 443 (API)
- **Web UI**: Port 80

Access the UI at: http://localhost

### Using Pre-built Images

```bash
# Pull images
docker pull ghcr.io/netzint/acme-dns:latest
docker pull ghcr.io/netzint/acme-dns-ui:latest

# Run acme-dns server
docker run -d \
  --name acme-dns \
  -p 53:53/tcp -p 53:53/udp \
  -p 443:443 \
  -v $(pwd)/config:/etc/acme-dns:ro \
  -v $(pwd)/data:/var/lib/acme-dns \
  ghcr.io/netzint/acme-dns:latest

# Run UI
docker run -d \
  --name acme-dns-ui \
  -p 80:80 \
  -e ACME_DNS_API_URL=https://your-acme-dns-server \
  ghcr.io/netzint/acme-dns-ui:latest
```

## UI Features

- 🔐 Login protection
- 📋 Domain overview with all registered domains from database
- ➕ Register new domains
- 📊 Server status display
- 📝 Copy credentials with one click
- 🗑️ Delete domains
- 💾 Local storage for passwords

## Configuration

### UI Login Credentials

Edit `ui/src/app/config/app.config.ts`:
```typescript
export const appConfig = {
  auth: {
    username: 'admin',
    password: 'your-secure-password'
  }
};
```

### API Endpoint

Set via environment variable:
```bash
ACME_DNS_API_URL=https://acme-dns.example.com
```

## Development

### Build UI locally
```bash
cd ui
npm install
npm start
# Open http://localhost:4200
```

### Build Docker images locally
```bash
# Build server
docker build -t acme-dns .

# Build UI
docker build -t acme-dns-ui ./ui
```

## API Endpoints

The acme-dns server includes these endpoints:

- `POST /register` - Register new domain
- `POST /update` - Update TXT record
- `GET /health` - Health check
- `GET /domains` - List all domains (requires X-Api-Key header)

## Architecture

```
┌─────────────┐         ┌──────────────┐         ┌──────────────┐
│   Browser   │────────▶│    UI        │────────▶│  acme-dns    │
│             │  :80    │   (nginx)    │  /api   │   server     │
└─────────────┘         └──────────────┘         └──────────────┘
                              │                          │
                              ▼                          ▼
                        ┌──────────┐              ┌──────────┐
                        │  Angular │              │  SQLite  │
                        │   App    │              │    DB    │
                        └──────────┘              └──────────┘
```

## Security

- UI requires login (credentials in config)
- Passwords are never transmitted from server
- API endpoints use authentication headers
- CORS configured for production use

## License

MIT