.PHONY: build build-dev push up down init-env release unrelease dev-init test lint format check format-imports

PROJECT_NAME := docker-coredns-sync
IMAGE := ghcr.io/auto-dns/$(PROJECT_NAME)
VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo latest)

# Default to prod build
build:
	docker build -t $(IMAGE):$(VERSION) --target release -f ./Dockerfile .

# Dev container build (optional)
build-dev:
	docker build -t $(IMAGE):dev --target dev -f ./Dockerfile .

# Push image
push:
	docker push $(IMAGE):$(VERSION)

# Start Docker Compose (prod image)
up:
	docker compose up -d

down:
	docker compose down

# Create empty .env if not present
init-env:
	@if [ ! -f .env ]; then echo "# Created .env file" > .env; fi

# Tag and push for release
release:
	@if [ -z "$(VERSION)" ]; then echo "VERSION is not set"; exit 1; fi
	git tag v$(VERSION)
	git push origin v$(VERSION)

unrelease:
	@if [ -z "$(VERSION)" ]; then echo "VERSION is not set"; exit 1; fi
	git tag -d v$(VERSION)
	git push --delete origin v$(VERSION)

dev-init:
	@touch config.yaml
	@mkdir -p .devcontainer
	@mkdir -p .devcontainer/etcd
	@touch .devcontainer/config.yaml

test:
	pytest tests

lint:
	ruff check src tests
	mypy src tests

format:
	black src tests

check: lint test

format-imports:
	ruff format src tests
