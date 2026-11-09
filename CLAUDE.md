# MiniDSP — Project Context for Claude Code

## What this is
MiniDSP is a local-first, end-to-end **Demand-Side Platform (DSP)** simulation built as a portfolio project
for Ads Infrastructure / Backend SDE (Software Development Engineer) internships.
It accepts OpenRTB-style (Open Real-Time Bidding) bid requests, evaluates eligible campaigns, computes bids,
simulates a first-price auction, processes win/loss/billing notices, enforces budgets, and benchmarks strategies.

This is a SIMULATION. It never connects to a real ad exchange. All spend, competitors and outcomes are synthetic.
Never write code, docs or README text that claims real business results.

The full plan is in `docs/plan.md`. Read it before starting any milestone.

## My role
I (the human) must be able to explain every design decision in an interview.
- Before writing non-trivial code, propose a plan and wait for my approval.
- After finishing a module, give me a short explanation: what it does, key design choices, trade-offs, and
  what could go wrong. Point out anything I should study more deeply.
- Prefer simple, readable code over clever code.

## Architecture
- `simulator/` — Python 3.11: deterministic exchange traffic generator, first-price auction, notice sender
- `bidder/` — Go (standard `net/http` only): `/openrtb2/bid` endpoint, eligibility, scoring, strategy, budget gate
- `worker/` — event inbox processing, state machine, ledger settlement, expiry, reconciliation
- `ml/` — Python + scikit-learn: Avazu sampling, logistic regression training, weight export
- `dashboard/` — Streamlit reading DuckDB over Parquet exports
- `bench/` — k6 scripts and benchmark reports
- `deploy/` — Docker Compose (PostgreSQL, Redis, services)

Storage roles:
- PostgreSQL = durable source of truth (campaigns, creatives, bid records, event inbox, settlement ledger)
- Redis + Lua = hot budget state and atomic reservations only

## Hard domain rules (do not violate)
1. Money is integer **micro-USD**. Never use floats for money. OpenRTB prices are CPM (Cost Per Mille);
   per-impression cost = CPM / 1000, converted explicitly.
2. One banner impression per request; USD only; first-price auction. These are documented simplifications.
3. A win notice is NOT a charge. Charge only on an explicit billing event.
4. Budget reservation is atomic (Redis Lua), keyed by campaign ID + bid ID, reserving the maximum possible cost.
   Charge or release exactly once on terminal state.
5. Bid state machine: BIDDED → WON | LOST; WON → BILLABLE | EXPIRED. Reject or queue invalid transitions.
6. Every event has an immutable unique ID. Ledger uses unique constraints; duplicates must never double-charge.
7. Redis unavailable → fail closed (no new bids). Never lose durable history.
8. No per-request remote ML calls. The model runs in-process in Go.

## Tech constraints
Do NOT add Kafka, Kubernetes, gRPC, a feature store, a separate inference service, or new major dependencies
without asking me first and giving a reason backed by profiling or a concrete need.

## Working rules
- Work one milestone (or one sub-task) at a time. Do not jump ahead.
- Write tests alongside code. For every bug fix, add a regression test.
- Keep all randomness seeded and reproducible.
- `make` targets: `make up`, `make test`, `make sim`, `make bench`. Keep them working.
- Commit in small, logical steps with clear messages.
- Never fabricate benchmark numbers. Only report measured results, with config, commit SHA and machine info.
- When unsure about an OpenRTB 2.6 field or an ads-domain behavior, say so instead of guessing.
