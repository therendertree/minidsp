# MiniDSP — An End-to-End Real-Time Bidding Engine

*Individual Software Engineering Project · Project plan, November 2026*

- **Student track:** Ads Infrastructure / Backend SDE (Software Development Engineer) Internship
- **Duration:** 10 weeks
- **Effort:** 10–15 hours/week
- **Delivery mode:** local-first

---

## 1. Executive Summary

Build a small, demonstrable demand-side platform (DSP) that accepts OpenRTB-style bid requests, evaluates eligible campaigns, calculates bids, simulates a first-price auction, processes win/loss/billing notifications, enforces advertiser budgets, and compares bidding strategies under reproducible workloads.

This is a self-contained advertising system simulation, not a connection to a live ad exchange. All budgets, clearing prices, competitors and ad outcomes are synthetic unless explicitly sourced from a public dataset. The objective is strong backend/system design evidence for Ads SDE internship applications — not fabricated business results.

### Non-negotiable outcomes

- **A working end-to-end demo by the end of Week 4:** create campaign → bid request → response/no-bid → simulated auction → notices → spending dashboard.
- **A concurrency-safe budget and idempotent settlement subsystem** with tests for duplicate, delayed and out-of-order events.
- **A reproducible benchmarking report:** baseline versus optimized version, including throughput, p50/p95/p99 latency, resource use, reliability and trade-offs.

---

## 2. Scope & Success Criteria

| In scope (MVP) | Explicitly out of scope |
|---|---|
| OpenRTB 2.6 subset; one banner impression per request; USD only | Live exchanges, real advertising spend, advertiser accounts |
| Campaign targeting, fixed/pCTR bids, first-price simulator | Video/native/CTV, multi-impression auctions, private deals |
| Budget reservations, asynchronous notices, ledger, pacing | Financial-grade settlement or globally distributed systems |
| Local dashboard, benchmark suite, public reproducible repository | Kubernetes, managed Kafka, GPU training, multi-region deployment |

**Acceptance rule:** a milestone is complete only when its code is committed, key tests pass, a short demo works from a clean setup, and the student can explain its architecture and design choices.

---

## 3. Architecture & Technology

```
Exchange Simulator (Python)
  → OpenRTB HTTP Bidder (Go)
  → Campaign/Targeting
  → CTR Scorer
  → Bid Strategy
  → Atomic Budget Gate (Redis)
```

The Simulator runs the first-price auction and sends notifications to an event-ingestion endpoint. PostgreSQL stores campaigns, event inbox, bid records and an auditable settlement ledger. A worker/reconciliation job updates durable state. The dashboard reads summarized metrics. k6 independently measures the Bidder.

| Component | Selected tool | Reason / cost |
|---|---|---|
| Low-latency bidder | Go standard `net/http` | Small memory footprint; clear concurrency model; free |
| Simulator + offline ML | Python 3.11; scikit-learn | Fast iteration and CPU-friendly; free |
| Transactional persistence | PostgreSQL (local Docker) | Durable campaign/event/ledger records; free |
| Hot budget state | Redis + Lua | Atomic state transitions; free locally |
| Async event processing | PostgreSQL inbox/outbox + worker | No Kafka infrastructure required |
| Metrics / dashboard | DuckDB + Parquet; Streamlit | Cheap analytics and local demo; free |
| Performance / CI | k6; Go pprof; GitHub Actions | Local load tests; public repo standard Actions free |
| Environment | Docker Compose | One-command local startup; free |

**Technology decision:** do not add Kafka, Kubernetes, gRPC, a managed feature store, or a separate Python inference microservice unless the baseline is fully working and profiling proves a need.

---

## 4. Key Domain Rules

- Bid prices are expressed as CPM in OpenRTB; convert to per-impression cost with explicit fixed-point units (e.g., integer micro-USD).
- A win notice does not automatically mean a billable impression. Settlement occurs on an explicit simulated billing event; impressions and clicks are separately logged.
- One request/one impression and a first-price auction are documented simplifications — not universal RTB behavior.
- Treat timeouts, missing notifications, duplicate events, partial failures, and late events as first-class engineering cases.

---

## 5. Milestone Plan

### Milestone 1 — Protocol & Basic Bidder (Weeks 1–2)

**Goal:** prove that the core bidder responds correctly to realistic protocol-shaped requests without external services.

- Learn the request/response lifecycle and document the supported OpenRTB 2.6 fields (`id`, `imp.id`, `banner`, `bidfloor`, `tmax`, `site`/`app`, `device`, `seatbid.bid`, `impid`, `price`, `crid`).
- Build a deterministic Python exchange traffic generator, including invalid, non-matching, timed-out and above-floor requests.
- Implement Go HTTP `/openrtb2/bid`; validate payloads; campaign eligibility; fixed-CPM bids; HTTP 204 no-bid and HTTP 400 malformed request.
- Create golden request/response fixtures, protocol contract tests, Makefile and minimal Compose setup.

**Exit criteria:** 1,000 deterministic requests replay successfully; all invalid/no-bid cases are tested; an architecture diagram and a 2-minute demo exist.

### Milestone 2 — Full Auction + Budget Integrity (Weeks 3–4)

**Goal:** complete the auction and simulated billing loop. This is the first portfolio-quality checkpoint.

- Implement campaign CRUD/state, creative assignment, eligibility, daily budgets and simple frequency caps; persist business entities in PostgreSQL.
- Build a first-price exchange simulator with synthetic competing bids and an explicit event-state machine: `BIDDED → WON/LOST → BILLABLE or EXPIRED`.
- Implement Redis Lua atomic budget reservation keyed by campaign and unique bid ID; reserve a maximum possible cost, then charge/release on terminal outcomes.
- Persist immutable event IDs and a settlement ledger in PostgreSQL with unique constraints; reject duplicate billing; track pending reservations and expiry.
- Provide a Streamlit dashboard with request count, bid/no-bid reasons, win rate, simulated spend, budget remaining, and auction trace.

**Exit criteria:** a complete 10,000-request simulation runs; two concurrent requests cannot reserve beyond the available budget; ten duplicate billing notifications produce one charge; a restart yields consistent totals after reconciliation.

> **Important design note:** per-bid reservation is intentionally conservative and may suppress bids while auctions are pending. The project must explain this trade-off. Strict global budget guarantees under unbounded late notices or total Redis loss are not claimed; those cases are recorded and reconciled under an explicit notification-window policy.

### Milestone 3 — CTR Scoring, Strategy & Pacing (Weeks 5–7)

**Goal:** compare fixed-price decisions against model-based bidding, without needing a GPU or paid inference endpoint.

- Use a manageable sample of public Avazu CTR data. Document anonymized input features, data rights, sampling method, and the limitations of mapping labels to synthetic campaigns.
- Train a logistic regression baseline; evaluate with time-aware holdout, log loss, AUC and calibration. LightGBM is optional for offline comparison.
- Export the baseline model and feature encoding to the Go process; add Python-versus-Go inference parity tests and define fallback on missing features.
- Implement fixed CPM, value-based pCTR bidding, and a rule-based pacing strategy using budget spent versus elapsed campaign time.
- Replay identical request sequences and random seeds; compare bid rate, win rate, spend, budget utilization, latency, simulated clicks, and pacing smoothness.

**Exit criteria:** all three strategies are reproducibly compared; no strategy is advertised as improving real-world revenue; scoring integrates without per-request remote ML calls.

### Milestone 4 — Reliability, Performance & Portfolio (Weeks 8–10)

**Goal:** demonstrate production-style engineering decisions and empirically measured results.

- Implement a durable notification inbox and retrying worker; support idempotent processing, state-transition validation, expired-reservation cleanup, and Redis/PostgreSQL reconciliation.
- Run failure injection: duplicated billing, missing loss/win notice, delayed bill, worker crash after DB commit/before acknowledgment, Redis unavailable, bidder timeout, and campaign pause.
- Create k6 test scenarios at 100, 300 and 500 requests/second for five minutes each, subject to available hardware. Capture p50/p95/p99, success/no-bid rates, CPU, memory and budget consistency.
- Use Go pprof to identify one measured bottleneck; make a documented change; repeat the same load, dataset, software configuration and random seed.
- Finish README, architecture/sequence diagrams, API examples, setup scripts, model card, benchmark report, short demo video, and two to three evidence-backed resume bullets.

**Exit criteria:** another developer can clone, start, simulate, test and inspect the results; a benchmark report includes raw configuration, before/after numbers, failures and limitations.

---

## 6. Engineering Test Matrix

| Scenario | Required behavior |
|---|---|
| Concurrent bids with insufficient budget | No over-reservation; deterministic no-bid |
| Same billing event delivered 10 times | Exactly one ledger charge within the defined processing workflow |
| Loss follows reservation | Reservation released once |
| Bill arrives before win / repeated win | Validated transition or queued for reconciliation |
| Worker crashes after database commit | Safe replay without duplicate ledger entry |
| Reservation expires without notice | Recover under policy; record late-bill exception if needed |
| Redis unavailable | Fail closed for new bids; preserve durable history |
| Simulated `tmax` exceeded | Exchange discards late response |
| Campaign paused during traffic | Newly evaluated bids are rejected |

---

## 7. Benchmark & Evidence Requirements

**Primary experiment:** test Go bidder-only latency versus end-to-end simulation latency separately. Use 100/300/500 RPS as planned load levels, not guaranteed capacities. A stretch target is p95 < 50 ms at 100 RPS on a documented local machine with Redis enabled, without budget violations; update the target after measuring the baseline.

**Every run must record:** Git commit SHA, OS/CPU/RAM, process counts, test tool version, duration, RPS, active campaigns, dataset/seed, external-service configuration, p50/p95/p99 latency, CPU/memory, response errors, no-bid reasons, and reservation/ledger reconciliation results.

**Optimization examples:** reduce DB reads on the hot path, cache eligible campaigns, reuse JSON buffers, tune connection pools, preload model weights. Do not claim QPS or latency improvements without identical before/after conditions.

---

## 8. Budget & Hosting Plan

| Item / scenario | Expected cost | Rule |
|---|---|---|
| Local Docker + languages + DB + Redis + k6 + dashboard | $0 cloud spend | Preferred for all development and benchmarks |
| GitHub public repo + standard Actions runners | $0 runner charge | Avoid larger runners and paid artifacts |
| Optional 2GB DigitalOcean demo VM | $12/month at full-month use | Bidder + small data services only |
| Optional 4GB DigitalOcean demo VM | $24/month at full-month use | Better for a short full-stack demo; delete after use |
| Domain, managed Kafka, managed Redis, GPU | $0 planned | Do not purchase |

As of November 2026, DigitalOcean lists $12/month for a 1-vCPU/2GB Basic Droplet and $24/month for a 2-vCPU/4GB Basic Droplet, with per-second billing and a minimum charge. A week on a $24/month-equivalent machine is roughly $6 before add-ons/taxes; actual charges depend on time/resources. Set a total external-spend ceiling of $40 and alerts at $10/$25. Delete VM, volumes and snapshots after demos; a stopped VM can still incur storage/VM charges.

No 24/7 public service is required. A GitHub README, recorded demo, static architecture page and reproducible local setup are sufficient for recruiter review. For a temporary public demo, add authentication/rate limits, synthetic-only data, no exposed Redis/PostgreSQL ports and no secrets in Git.

---

## 9. Final Deliverables & Mentor Review

- Public GitHub monorepo with tests, Docker Compose, reproducible seeds and complete setup instructions.
- Working demo: create campaigns → generate bid traffic → simulate winners and billing → inspect budget dashboard.
- Architecture and event-lifecycle diagrams; documented policy for late notices and budget reconciliation.
- Three-strategy experiment and model-quality report (synthetic auction assumptions clearly labeled).
- Performance report with failure tests and one measured before/after optimization.
- Five-minute demonstration video and 15-minute system-design walkthrough.

**Review checkpoints:** Week 2 (protocol), Week 4 (end-to-end correctness), Week 7 (ML/bidding evaluation), Week 10 (failure handling, reproducibility, performance, and interview narrative).

**Suggested scoring:** correctness/auction rules 25%; backend/system design 25%; reliability 20%; performance evidence 15%; ads/ML understanding 10%; presentation 5%. Passing target: at least 80/100, with no failures in budget idempotency or replay tests.

---

## 10. Reference Materials & Cost Sources

- IAB Tech Lab OpenRTB 2.6: https://github.com/InteractiveAdvertisingBureau/openrtb2.x/blob/main/2.6.md
- Redis Lua scripting: https://redis.io/docs/latest/develop/programmability/eval-intro/
- Grafana k6: https://grafana.com/docs/k6/latest/
- GitHub Actions public runner billing: https://docs.github.com/en/actions/reference/runners/github-hosted-runners
- DigitalOcean official Droplet prices: https://www.digitalocean.com/pricing/droplets
- Avazu CTR data: https://www.kaggle.com/c/avazu-ctr-prediction/data
- Redis Streams (optional future extension): https://redis.io/docs/latest/develop/data-types/streams/

---

## Mentor Recommendation

Keep the first four weeks narrow. A correct end-to-end auction with audited budget transitions beats an unfinished microservice architecture. Then add model-based bidding, reliability and measured performance. Use the local machine for nearly all work; spend money only if a live cloud demonstration adds real value.
