import type { AcmeDomain, ServerInfo } from './types';

/**
 * Generators for the copy/paste blocks shown in the domain details dialog.
 * They are pure functions so the same text can be rendered and copied.
 */

const PLACEHOLDER_PASSWORD = '<PASSWORT — über "Neue Zugangsdaten" erzeugen>';

function password(domain: AcmeDomain): string {
  return domain.credentials_available && domain.password ? domain.password : PLACEHOLDER_PASSWORD;
}

/** Strips a wildcard prefix and a trailing dot from user input. */
export function normalizeDomain(input: string): string {
  return input.trim().toLowerCase().replace(/^\*\./, '').replace(/\.$/, '');
}

/** The DNS record the domain owner has to create, in zone file notation. */
export function cnameRecord(domain: AcmeDomain, targetDomain: string): string {
  const name = `_acme-challenge.${normalizeDomain(targetDomain)}.`;
  return `${name}\tIN\tCNAME\t${domain.fulldomain}.`;
}

/** Just the two values, for DNS panels that ask for name and target separately. */
export function cnameFields(domain: AcmeDomain, targetDomain: string): { name: string; target: string } {
  return {
    name: `_acme-challenge.${normalizeDomain(targetDomain)}`,
    target: `${domain.fulldomain}.`,
  };
}

/**
 * The account file lego (and therefore Traefik) reads from
 * ACME_DNS_STORAGE_PATH. The top level key is the certificate domain.
 */
export function legoStorageJson(domain: AcmeDomain, targetDomain: string, server: ServerInfo): string {
  const entry = {
    [normalizeDomain(targetDomain)]: {
      fulldomain: domain.fulldomain,
      subdomain: domain.subdomain,
      username: domain.username,
      password: password(domain),
      server_url: server.api_base_url,
    },
  };
  return JSON.stringify(entry, null, 2);
}

/** A docker-compose service block wiring Traefik to this acme-dns server. */
export function traefikCompose(domain: AcmeDomain, targetDomain: string, server: ServerInfo): string {
  const cert = normalizeDomain(targetDomain);
  return `# docker-compose.yml
services:
  traefik:
    image: traefik:v3
    environment:
      - ACME_DNS_API_BASE=${server.api_base_url}
      - ACME_DNS_STORAGE_PATH=/letsencrypt/acme-dns.json
    volumes:
      # acme-dns.json enthält den Block aus dem Reiter "lego / acme-dns.json"
      - ./letsencrypt:/letsencrypt
      - /var/run/docker.sock:/var/run/docker.sock:ro
    command:
      - --providers.docker=true
      - --entrypoints.websecure.address=:443
      - --certificatesresolvers.acmedns.acme.email=admin@${cert}
      - --certificatesresolvers.acmedns.acme.storage=/letsencrypt/acme.json
      - --certificatesresolvers.acmedns.acme.dnschallenge=true
      - --certificatesresolvers.acmedns.acme.dnschallenge.provider=acme-dns

  # Beispiel: Dienst, der das Zertifikat nutzt
  whoami:
    image: traefik/whoami
    labels:
      - traefik.enable=true
      - traefik.http.routers.whoami.rule=Host(\`${cert}\`)
      - traefik.http.routers.whoami.entrypoints=websecure
      - traefik.http.routers.whoami.tls.certresolver=acmedns`;
}

/** The equivalent static configuration for a traefik.yml based setup. */
export function traefikStatic(domain: AcmeDomain, targetDomain: string, server: ServerInfo): string {
  const cert = normalizeDomain(targetDomain);
  return `# traefik.yml (statische Konfiguration)
certificatesResolvers:
  acmedns:
    acme:
      email: admin@${cert}
      storage: /letsencrypt/acme.json
      dnsChallenge:
        provider: acme-dns

# Zusätzlich als Umgebungsvariablen setzen:
#   ACME_DNS_API_BASE=${server.api_base_url}
#   ACME_DNS_STORAGE_PATH=/letsencrypt/acme-dns.json`;
}

/** certbot with joohoi's acme-dns-auth.py hook. */
export function certbotSnippet(domain: AcmeDomain, targetDomain: string, server: ServerInfo): string {
  const cert = normalizeDomain(targetDomain);
  return `# 1. Zugangsdaten in /etc/letsencrypt/acmedns.json ablegen:
{
  "${cert}": {
    "fulldomain": "${domain.fulldomain}",
    "subdomain": "${domain.subdomain}",
    "username": "${domain.username}",
    "password": "${password(domain)}",
    "allowfrom": []
  }
}

# 2. In acme-dns-auth.py den Server eintragen:
#    ACMEDNS_URL = "${server.api_base_url}"

# 3. Zertifikat anfordern:
certbot certonly \\
  --manual \\
  --preferred-challenges dns \\
  --manual-auth-hook /etc/letsencrypt/acme-dns-auth.py \\
  --debug-challenges \\
  -d ${cert} \\
  -d '*.${cert}'`;
}

/** A raw /update call, useful to confirm the credentials work at all. */
export function curlSnippet(domain: AcmeDomain, server: ServerInfo): string {
  return `# Schreibt einen Test-TXT-Wert (muss exakt 43 Zeichen lang sein)
curl -X POST ${server.api_base_url}/update \\
  -H "X-Api-User: ${domain.username}" \\
  -H "X-Api-Key: ${password(domain)}" \\
  -d '{"subdomain": "${domain.subdomain}", "txt": "___VALIDATION_TOKEN_43_ZEICHEN_LANG_______"}'

# Danach prüfen, ob der Wert ankommt:
dig +short TXT _acme-challenge.${normalizeDomain(domain.domain_name)}`;
}
