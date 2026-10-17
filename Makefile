SHELL := /usr/bin/env bash

PNPM ?= pnpm
GO ?= go
PYTHON ?= python3
COMMIT_SHA ?= $(shell git rev-parse --verify HEAD)
BUILD_TIME ?= unknown

.PHONY: bootstrap dev-backend dev-frontend format check-format lint test test-race typecheck contracts content docs secrets dependencies smoke check-go check-frontend check-data check build manifest clean

bootstrap:
	$(PNPM) --dir frontend install --frozen-lockfile

dev-backend:
	cd backend && $(GO) run ./cmd/server

dev-frontend:
	$(PNPM) --dir frontend dev

format:
	cd backend && $(GO) fmt ./...
	$(PNPM) --dir frontend format

check-format:
	@test -z "$$(gofmt -l backend)" || (gofmt -l backend && exit 1)
	$(PNPM) --dir frontend format:check

lint:
	cd backend && $(GO) vet ./...
	$(PNPM) --dir frontend lint

test:
	cd backend && $(GO) test ./...
	$(PNPM) --dir frontend test

test-race:
	cd backend && $(GO) test -race ./...

typecheck:
	$(PNPM) --dir frontend typecheck

contracts:
	$(PYTHON) scripts/validate_contracts.py

content:
	$(PYTHON) scripts/validate_decks.py --require-reviewed

docs:
	bash scripts/check-docs.sh

secrets:
	$(PYTHON) scripts/check_secrets.py

dependencies:
	cd backend && $(GO) mod verify
	$(PNPM) --dir frontend audit --audit-level high

smoke:
	bash tests/smoke/local_backend.sh

check-go:
	@test -z "$$(gofmt -l backend)" || (gofmt -l backend && exit 1)
	cd backend && $(GO) vet ./...
	cd backend && $(GO) test -race ./...

check-frontend:
	$(PNPM) --dir frontend format:check
	$(PNPM) --dir frontend lint
	$(PNPM) --dir frontend typecheck
	$(PNPM) --dir frontend test
	$(PNPM) --dir frontend build

check-data: contracts content docs secrets

check: check-go check-frontend check-data

build:
	mkdir -p build/artifacts
	cd backend && CGO_ENABLED=0 $(GO) build -buildvcs=false -trimpath -ldflags "-s -w -X github.com/dKorialstrasz/response-online/backend/internal/buildinfo.Commit=$(COMMIT_SHA) -X github.com/dKorialstrasz/response-online/backend/internal/buildinfo.BuiltAt=$(BUILD_TIME)" -o ../build/artifacts/response-server ./cmd/server
	$(PNPM) --dir frontend build
	tar --sort=name --mtime='UTC 1970-01-01' --owner=0 --group=0 --numeric-owner -C frontend/dist -czf build/artifacts/frontend.tar.gz .

manifest: build
	$(PYTHON) scripts/build_manifest.py --commit "$(COMMIT_SHA)" --output build/build-manifest.json

clean:
	rm -rf build/artifacts build/build-manifest.json frontend/dist


