.PHONY: up down test test-go test-py sim bench

COMPOSE = docker compose --env-file .env -f deploy/docker-compose.yml
# Uses the simulator venv (Python 3.11, see .python-version); override with PYTHON=...
# Create it with: py -3.11 -m venv simulator/.venv && simulator/.venv/Scripts/python -m pip install -e "simulator[dev]"
PYTHON ?= .venv/Scripts/python

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
