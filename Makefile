.PHONY: up down test test-go test-py sim bench

COMPOSE = docker compose --env-file .env -f deploy/docker-compose.yml
PYTHON ?= python

# Start PostgreSQL and Redis (creates .env from .env.example if missing).
up:
	@test -f .env || cp .env.example .env
	$(COMPOSE) up -d --wait

down:
	$(COMPOSE) down

test: test-go test-py

test-go:
	cd bidder && go vet ./... && go test ./...

test-py:
	cd simulator && $(PYTHON) -m pytest

# Run the simulator (placeholder until Milestone 1).
sim:
	cd simulator && PYTHONPATH=src $(PYTHON) -m simulator

# Benchmarks arrive in Milestone 4; never report numbers that were not measured.
bench:
	@echo "bench: not implemented until Milestone 4"
