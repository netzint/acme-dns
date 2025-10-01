#!/bin/sh
set -e

# Default API URL if not provided
ACME_DNS_API_URL=${ACME_DNS_API_URL:-"https://acme-dns.netzint.de"}

# Replace environment variables in nginx config
envsubst '${ACME_DNS_API_URL}' < /etc/nginx/conf.d/nginx-template.conf > /etc/nginx/conf.d/default.conf

# Remove template
rm -f /etc/nginx/conf.d/nginx-template.conf

echo "ACME-DNS UI starting with API URL: ${ACME_DNS_API_URL}"

# Start nginx
exec nginx -g 'daemon off;'