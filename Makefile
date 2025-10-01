.PHONY: build run test clean docker-build docker-run docker-push help

# Variables
BINARY_NAME=acme-dns
DOCKER_IMAGE=acme-dns
DOCKER_TAG=latest
REGISTRY=ghcr.io
NAMESPACE=$(shell git config --get user.name | tr '[:upper:]' '[:lower:]')
FULL_IMAGE=$(REGISTRY)/$(NAMESPACE)/$(DOCKER_IMAGE):$(DOCKER_TAG)

# Go build flags
GOFLAGS=-ldflags="-w -s -X main.version=$$(git describe --tags --always --dirty) -X main.commit=$$(git rev-parse HEAD)"

# Default target
all: build

## help: Show this help message
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'

## build: Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	go build $(GOFLAGS) -o $(BINARY_NAME) .

## run: Run the application
run: build
	./$(BINARY_NAME) -c config.cfg

## test: Run tests
test:
	go test -v ./...

## clean: Clean build files
clean:
	@echo "Cleaning..."
	go clean
	rm -f $(BINARY_NAME)

## docker-build: Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build -f Dockerfile.improved -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

## docker-build-multi: Build multi-platform Docker image (currently only amd64)
docker-build-multi:
	@echo "Building Docker image for amd64..."
	docker buildx build \
		--platform linux/amd64 \
		-f Dockerfile.improved \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) \
		.

## docker-run: Run Docker container
docker-run:
	@echo "Running Docker container..."
	docker run -d \
		--name $(BINARY_NAME) \
		-p 53:53/tcp \
		-p 53:53/udp \
		-p 80:80 \
		-v $$(pwd)/config.cfg:/etc/acme-dns/config.cfg:ro \
		-v $$(pwd)/data:/var/lib/acme-dns \
		$(DOCKER_IMAGE):$(DOCKER_TAG)

## docker-stop: Stop and remove Docker container
docker-stop:
	@echo "Stopping Docker container..."
	docker stop $(BINARY_NAME) || true
	docker rm $(BINARY_NAME) || true

## docker-logs: Show Docker container logs
docker-logs:
	docker logs -f $(BINARY_NAME)

## docker-push: Push Docker image to registry
docker-push:
	@echo "Pushing Docker image to $(FULL_IMAGE)..."
	docker tag $(DOCKER_IMAGE):$(DOCKER_TAG) $(FULL_IMAGE)
	docker push $(FULL_IMAGE)

## docker-compose-up: Start with docker-compose
docker-compose-up:
	docker-compose -f docker-compose.yml up -d

## docker-compose-down: Stop with docker-compose
docker-compose-down:
	docker-compose -f docker-compose.yml down

## docker-compose-logs: Show docker-compose logs
docker-compose-logs:
	docker-compose -f docker-compose.yml logs -f

## install: Install binary to /usr/local/bin
install: build
	@echo "Installing $(BINARY_NAME) to /usr/local/bin..."
	sudo cp $(BINARY_NAME) /usr/local/bin/
	sudo chmod +x /usr/local/bin/$(BINARY_NAME)

## uninstall: Remove binary from /usr/local/bin
uninstall:
	@echo "Removing $(BINARY_NAME) from /usr/local/bin..."
	sudo rm -f /usr/local/bin/$(BINARY_NAME)

## fmt: Format Go code
fmt:
	go fmt ./...

## vet: Run go vet
vet:
	go vet ./...

## lint: Run golangci-lint
lint:
	golangci-lint run

## deps: Download dependencies
deps:
	go mod download
	go mod tidy

## update-deps: Update dependencies
update-deps:
	go get -u ./...
	go mod tidy

## version: Show version information
version:
	@echo "Version: $$(git describe --tags --always --dirty)"
	@echo "Commit: $$(git rev-parse HEAD)"
	@echo "Date: $$(date -u +%Y-%m-%dT%H:%M:%SZ)"