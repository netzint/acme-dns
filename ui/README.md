# ACME-DNS UI

Eine Angular-basierte Web-UI für die Verwaltung von ACME-DNS Domains.

## Features

- Login-geschützter Bereich (Credentials in `src/app/config/app.config.ts`)
- Dashboard zur Anzeige aller registrierten Domains
- Neue Domains registrieren
- Domain-Informationen kopieren (Full Domain, Username, Password)
- Domains löschen
- Server-Status Anzeige
- Speicherung der Domains im LocalStorage

## Installation

```bash
cd acme-dns-ui
npm install
```

## Konfiguration

Bearbeite die Datei `src/app/config/app.config.ts`:

```typescript
export const appConfig: AppConfig = {
  auth: {
    username: 'admin',        // Login Username
    password: 'admin123'      // Login Passwort
  },
  acmeDns: {
    apiUrl: 'http://localhost:8080',  // ACME-DNS Server URL
    username: '',
    password: ''
  }
};
```

## Entwicklung

```bash
npm start
```

Die Anwendung läuft dann auf http://localhost:4200/

## CORS-Konfiguration für ACME-DNS

Damit die UI mit dem ACME-DNS Server kommunizieren kann, muss CORS im ACME-DNS Server aktiviert werden. 

Füge folgende Einstellungen in die `config.cfg` des ACME-DNS Servers ein:

```ini
[api]
# CORS Headers
cors_origins = ["http://localhost:4200"]
```

## Build für Produktion

```bash
npm run build
```

Die gebaute Anwendung befindet sich dann im `dist/` Verzeichnis.

## Verwendung

1. Starte den ACME-DNS Server
2. Starte die Angular-Anwendung
3. Logge dich mit den konfigurierten Credentials ein
4. Registriere neue Domains über den "Register New Domain" Button
5. Die Domain-Credentials werden automatisch im LocalStorage gespeichert

## Docker Deployment (optional)

Du kannst die UI auch als Docker Container deployen:

```dockerfile
FROM node:20-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:alpine
COPY --from=builder /app/dist/acme-dns-ui/browser /usr/share/nginx/html
EXPOSE 80
```