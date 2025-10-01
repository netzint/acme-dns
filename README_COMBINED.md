# ACME-DNS with Integrated UI

Single container solution with acme-dns server and web UI combined.

## Architecture

```
┌──────────────────┐
│  acme-dns server │
│                  │
│  API Endpoints:  │
│  /register       │
│  /update         │
│  /health         │
│  /domains        │
│                  │
│  Static Files:   │
│  /ui/*           │
│  / → /ui/        │
└──────────────────┘
```

The acme-dns server handles:
- All API endpoints directly
- Serves UI static files from `/ui/*`
- Redirects `/` to `/ui/`
- Returns 404 for unknown paths

## Quick Start

### Using Pre-built Image

```bash
docker run -d \
  --name acme-dns \
  -p 53:53/tcp -p 53:53/udp \
  -p 80:80 \
  -v $(pwd)/config:/etc/acme-dns:ro \
  -v $(pwd)/data:/var/lib/acme-dns \
  ghcr.io/netzint/acme-dns:combined
```

Access:
- **UI**: http://localhost/ui/
- **API**: http://localhost/register, etc.
- **DNS**: Port 53

### Using Docker Compose

```bash
curl -O https://raw.githubusercontent.com/netzint/acme-dns/main/docker-compose.combined.yml
docker-compose -f docker-compose.combined.yml up -d
```

## Building

### Build Combined Docker Image

```bash
docker build -f Dockerfile.combined -t acme-dns:combined .
```

### Build Locally

```bash
# Build backend
go build -o acme-dns .

# Build frontend
cd ui
npm install
npm run build
cd ..

# Copy UI files for serving
sudo mkdir -p /usr/share/acme-dns-ui
sudo cp -r ui/dist/acme-dns-ui/browser/* /usr/share/acme-dns-ui/

# Run
./acme-dns -c config.cfg
```

## How It Works

1. **Single Binary**: The acme-dns server includes code to serve static files
2. **UI Path Detection**: If `/usr/share/acme-dns-ui` exists, UI routes are enabled
3. **API First**: All API endpoints are registered first and have priority
4. **UI Fallback**: `/ui/*` paths serve static files, with Angular routing support
5. **No Proxy Needed**: UI JavaScript calls API endpoints directly on same origin

## Configuration

### UI Credentials

Edit `ui/src/app/config/app.config.ts` before building:

```typescript
export const appConfig = {
  auth: {
    username: 'admin',
    password: 'secure-password'
  }
};
```

### API Configuration

Standard acme-dns configuration in `config.cfg`:

```ini
[general]
listen = "0.0.0.0:53"
domain = "auth.example.com"

[database]
engine = "sqlite3"
connection = "/var/lib/acme-dns/acme-dns.db"

[api]
ip = "0.0.0.0"
port = "80"
```

## API Endpoints

- `POST /register` - Register new domain
- `POST /update` - Update TXT record (requires auth)
- `GET /health` - Health check
- `GET /domains` - List all domains (requires X-Api-Key header)
- `GET /ui/*` - UI static files (if UI is included)
- `GET /` - Redirects to /ui/

## Advantages

✅ **Single Container** - Everything in one image
✅ **No CORS Issues** - Same origin for UI and API
✅ **No Proxy Config** - Direct API access
✅ **Optional UI** - Works without UI files
✅ **Simple Deployment** - One service to manage

## Development

### Run Backend Only
```bash
go run .
```

### Run UI Development Server
```bash
cd ui
npm start
# Proxy to backend: edit proxy.conf.json
```

### Build Everything
```bash
make -f Makefile.all build-all
```

## Security Notes

- UI login credentials are compiled into the frontend
- API endpoints require proper authentication headers
- Consider using HTTPS in production
- Change default passwords before deployment

## License

MIT