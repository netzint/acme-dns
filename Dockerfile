# Plain acme-dns server without the management UI.
# Use Dockerfile.combined for the image that also serves the web UI.
FROM golang:alpine AS builder
LABEL maintainer="lukas.spitznagel@netzint.de"

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# The sqlite driver is pure Go, so no cgo and no toolchain are needed.
RUN CGO_ENABLED=0 go build -ldflags="-w -s" -o acme-dns .

FROM alpine:latest

RUN apk --no-cache add ca-certificates && update-ca-certificates
RUN mkdir -p /etc/acme-dns /var/lib/acme-dns

COPY --from=builder /build/acme-dns /usr/local/bin/acme-dns

WORKDIR /var/lib/acme-dns
VOLUME ["/etc/acme-dns", "/var/lib/acme-dns"]
ENTRYPOINT ["/usr/local/bin/acme-dns"]
EXPOSE 53 80 443
EXPOSE 53/udp
