# acme-dns mit Verwaltungsoberfläche

Fork von [joohoi/acme-dns](https://github.com/joohoi/acme-dns) mit einer Weboberfläche,
die den kompletten Weg von „ich brauche ein Zertifikat" bis „die DNS-Einträge stimmen"
abbildet.

Der Fork folgt der Paketstruktur des Originals (`pkg/acmedns`, `pkg/api`,
`pkg/database`, `pkg/nameserver`); die Verwaltung liegt in `pkg/api/admin*.go`,
`pkg/api/dnscheck.go` und `pkg/api/staticui.go`. Dadurch lassen sich
Upstream-Änderungen weiterhin per `git merge upstream/master` übernehmen.

Der DNS- und der ACME-Teil sind unverändert: jeder acme-dns-Client (Traefik/lego,
certbot, acme.sh, acme-dns-client) spricht weiter mit `/register` und `/update`.

## Was die Oberfläche macht

1. **Domain anlegen** — erzeugt eine Subdomain samt Zugangsdaten.
2. **Daten ausgeben** — den CNAME für die Zone und fertige Konfigurationsblöcke für
   Traefik, lego, certbot und `curl`.
3. **Prüfen** — fragt den CNAME über öffentliche Resolver ab und testet danach, ob ein
   frisch geschriebener TXT-Wert wirklich unter `_acme-challenge.<domain>` auflösbar ist.
   Das ist derselbe Weg, den eine Zertifizierungsstelle geht, also fällt auch eine kaputte
   Delegierung auf.

## Schnellstart

```bash
mkdir -p config data
cp config.cfg config/config.cfg      # und anpassen
# Das Image läuft als uid 1000, beide Verzeichnisse müssen ihm gehören:
sudo chown -R 1000:1000 config data
sudo chmod 600 config/config.cfg

docker compose -f docker-compose.combined.yml up -d
```

Die Oberfläche liegt danach auf `/`, die API unverändert auf `/register`, `/update`
und `/health`.

Der Healthcheck steht bewusst in der Compose-Datei und nicht im Image: mit
`tls = "letsencrypt"` antwortet der Server nur per HTTPS und legt nur ein Zertifikat
für die eigene Domain vor. Der `extra_hosts`-Eintrag lässt diesen Namen im Container
auf `127.0.0.1` zeigen, damit die Probe ohne `--no-check-certificate` auskommt.

## Konfiguration

Die Verwaltung ist **standardmäßig aus**. Ohne `auth.admin_user` und
`auth.admin_password_hash` läuft acme-dns wie das Original — es gibt dann weder
`/api/admin/*` noch eine Oberfläche.

### Admin-Zugang einrichten

```bash
# bcrypt-Hash erzeugen (liest das Passwort von stdin)
docker run --rm -i ghcr.io/netzint/acme-dns:combined -hashpw
```

```ini
[auth]
admin_user = "admin"
admin_password_hash = "$2a$12$..."
session_ttl_hours = 12
credentials_key = "ein langes zufälliges Geheimnis"
```

`credentials_key` schaltet die **wiederherstellbare Passwortspeicherung** ein: jedes
erzeugte API-Passwort wird zusätzlich AES-256-GCM-verschlüsselt abgelegt, damit die
Oberfläche es später noch einmal anzeigen kann.

> **Sicherheitshinweis:** Wer Konfigurationsdatei *und* Datenbank hat, kann damit alle
> API-Passwörter lesen. Die Konfigurationsdatei gehört mit `chmod 0600` auf den Host und
> nicht ins Image. Ohne `credentials_key` speichert acme-dns nur bcrypt-Hashes; verlorene
> Passwörter lassen sich dann ausschließlich über „Neue Zugangsdaten" ersetzen.

Passt auch ohne den Schlüssel: der Knopf „Neue Zugangsdaten" erzeugt jederzeit ein neues
Passwort für dieselbe Subdomain — der einmal eingetragene CNAME bleibt gültig.

### DNS-Prüfung

```ini
[dnscheck]
resolvers = ["1.1.1.1:53", "8.8.8.8:53", "9.9.9.9:53"]
txt_timeout_seconds = 30
```

Der vollständige Test schreibt kurzzeitig einen Zufallswert in die Registrierung. Läuft
zeitgleich eine echte Zertifikatsausstellung für dieselbe Registrierung, kann das den
gerade gesetzten Challenge-Wert überschreiben — acme-dns hält zwei TXT-Slots vor, der
zweite bleibt erhalten. Wer das ausschließen will, nutzt in der Oberfläche den Schalter
„Nur CNAME prüfen".

### Eigenes Zertifikat per Let's Encrypt

acme-dns kann das Zertifikat für seine eigene API selbst holen; die DNS-01-Challenge
beantwortet es über den eingebauten Challenge-Provider, weil es für seine eigene Zone
autoritativ ist.

```ini
[api]
port = "443"
tls = "letsencrypt"
notification_email = "admin@example.org"
acme_cache_dir = "/var/lib/acme-dns/api-certs"
```

`acme_cache_dir` muss auf einem persistenten Volume liegen, sonst wird bei jedem
Neustart ein neues Zertifikat beantragt und das Rate-Limit von Let's Encrypt greift.

## API

### Unverändert gegenüber dem Original

| Methode | Pfad | Auth |
| --- | --- | --- |
| `POST` | `/register` | keine (per `disable_registration` abschaltbar) |
| `POST` | `/update` | `X-Api-User` / `X-Api-Key` |
| `GET` | `/health` | keine |

### Verwaltung

Alle Endpunkte unter `/api/admin/` außer `/login` erwarten
`Authorization: Bearer <token>`.

| Methode | Pfad | Zweck |
| --- | --- | --- |
| `POST` | `/api/admin/login` | Zugangsdaten gegen Sitzungstoken tauschen |
| `POST` | `/api/admin/logout` | Token verwerfen |
| `GET` | `/api/admin/session` | Token prüfen |
| `GET` | `/api/admin/server` | Domain und Basis-URL für die Vorlagen |
| `GET` | `/api/admin/domains` | alle Registrierungen inkl. Zugangsdaten |
| `POST` | `/api/admin/domains` | Registrierung anlegen |
| `POST` | `/api/admin/domains/:subdomain/name` | Domain-Bezeichnung ändern |
| `POST` | `/api/admin/domains/:subdomain/rotate` | neues Passwort erzeugen |
| `DELETE` | `/api/admin/domains/:subdomain` | Registrierung löschen |
| `POST` | `/api/admin/dnscheck` | CNAME und TXT-Weg prüfen |

Sitzungen liegen nur im Arbeitsspeicher: ein Neustart meldet alle Admins ab.
Nach fünf Fehlversuchen ist eine Quell-IP 15 Minuten gesperrt.

## Client-Konfiguration

Die Oberfläche erzeugt diese Blöcke fertig ausgefüllt. Zum Nachschlagen:

### Traefik / lego

```yaml
environment:
  - ACME_DNS_API_BASE=https://acme-dns.example.org
  - ACME_DNS_STORAGE_PATH=/letsencrypt/acme-dns.json
command:
  - --certificatesresolvers.acmedns.acme.dnschallenge=true
  - --certificatesresolvers.acmedns.acme.dnschallenge.provider=acme-dns
```

`acme-dns.json` bildet Zertifikatsdomain auf Registrierung ab:

```json
{
  "example.org": {
    "fulldomain": "<uuid>.acme-dns.example.org",
    "subdomain": "<uuid>",
    "username": "<uuid>",
    "password": "…",
    "server_url": "https://acme-dns.example.org"
  }
}
```

### certbot

`acme-dns-auth.py` liest dieselben Felder aus `/etc/letsencrypt/acmedns.json`,
dort heißt der Schlüssel `allowfrom` statt `server_url`.

## Entwicklung

```bash
# Backend (kein cgo nötig, der SQLite-Treiber ist pure Go)
go build -o acme-dns . && ./acme-dns -c config.cfg

# Tests
go test ./...

# UI mit Hot Reload gegen ein laufendes Backend
cd ui && npm install && npm start
```

Für den lokalen Betrieb ohne Docker zeigt `api.ui_path` in der `config.cfg` auf das
Bauverzeichnis der UI:

```ini
[api]
ui_path = "./ui/dist/acme-dns-ui/browser"
```

## Datenbank

Die Tabelle `records` wird beim Start automatisch auf Version 3 migriert (neue Spalten
`DomainName`, `CreatedAt`, `UpdatedAt`, `EncPassword`). Bestehende Registrierungen
bleiben erhalten, haben aber kein wiederherstellbares Passwort — die Oberfläche
kennzeichnet sie und bietet die Rotation an.

Der SQLite-Treiber ist seit dem Upstream-Merge `glebarez/go-sqlite` statt
`mattn/go-sqlite3`. Das Dateiformat ist identisch, bestehende Datenbanken werden
unverändert weiterverwendet. Die Engine heißt jetzt `sqlite`; ein altes
`engine = "sqlite3"` wird beim Start mit einer Warnung automatisch umgesetzt.

## Upstream-Änderungen übernehmen

```bash
git remote add upstream https://github.com/joohoi/acme-dns.git
git fetch upstream
git merge upstream/master
```
