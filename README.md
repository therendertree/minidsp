# MiniDSP

A local-first, end-to-end **Demand-Side Platform (DSP) simulation**: OpenRTB-style bid requests, campaign
eligibility, bidding, a simulated first-price auction, win/loss/billing notices, budget enforcement and
benchmarks.

> This is a simulation. It never connects to a real ad exchange. All spend, competitors and outcomes are
> synthetic. See [docs/plan.md](docs/plan.md) for the full plan.

**Status:** repository skeleton only. No business logic yet.

## Layout

| Folder | Purpose |
|---|---|
| `bidder/` | Go bidder (`net/http` only) |
| `simulator/` | Python exchange simulator |
| `worker/` | Event processing and settlement (later) |
| `ml/` | CTR model training (later) |
| `dashboard/` | Streamlit dashboard (later) |
| `bench/` | k6 benchmarks (later) |
| `deploy/` | Docker Compose: PostgreSQL, Redis |

## Quickstart

Requires Go 1.22+, Python 3.11+, make and Docker.

```bash
make up      # start PostgreSQL and Redis
make test    # Go and Python tests
make sim     # run the simulator (placeholder)
```
