"""Compare observed responses with expectations and render the run summary."""

from __future__ import annotations

from collections import Counter
from dataclasses import dataclass

from simulator.generator import MIX, TIGHT_TMAX, SimRequest, format_micros
from simulator.replay import Result

MAX_MISMATCHES_SHOWN = 10


def check(req: SimRequest, res: Result) -> str | None:
    """Return a description of the first way ``res`` differs from ``req``'s expectation."""
    if res.error is not None:
        return f"transport/parse error: {res.error}"
    if res.status != req.expected_status:
        return f"status {res.status}, expected {req.expected_status}"
    if req.expected_status == 200 and res.price_micros != req.expected_price_micros:
        return (f"price {_fmt(res.price_micros)}, expected {_fmt(req.expected_price_micros)}")
    if req.expected_status == 204 and res.nobid_reason != req.expected_nobid_reason:
        return f"no-bid reason {res.nobid_reason!r}, expected {req.expected_nobid_reason!r}"
    if req.expected_status == 400 and res.error_code != req.expected_error_code:
        return f"error code {res.error_code!r}, expected {req.expected_error_code!r}"
    return None


def _fmt(micros: int | None) -> str:
    return "none" if micros is None else format_micros(micros)


def percentile(sorted_values: list[float], pct: float) -> float:
    """Nearest-rank percentile of an already sorted, non-empty list."""
    rank = max(1, -(-len(sorted_values) * pct // 100))  # ceil
    return sorted_values[int(rank) - 1]


@dataclass(frozen=True)
class Report:
    text: str
    mismatches: int
    errors: int

    @property
    def ok(self) -> bool:
        return self.mismatches == 0 and self.errors == 0


def build_report(
    *,
    seed: int,
    sha256: str,
    requests: list[SimRequest],
    results: list[Result],
    bidder_desc: str,
    server_counters: dict[str, int] | None,
    counters_are_fresh: bool,
) -> Report:
    lines: list[str] = []
    out = lines.append
    out("MiniDSP simulation replay (synthetic traffic only)")
    out(f"  seed={seed}  requests={len(requests)}  requests_sha256={sha256[:16]}")
    out(f"  bidder: {bidder_desc}")
    out("")

    problems: list[tuple[str, str]] = []  # (label, message)
    problem_idx: set[int] = set()
    for req, res in zip(requests, results, strict=True):
        msg = check(req, res)
        if msg:
            problems.append((f"#{req.index} {req.category}/{req.kind}", msg))
            problem_idx.add(req.index)
    errors = sum(1 for r in results if r.error is not None)

    out("HTTP status codes")
    for status, n in sorted(Counter(r.status for r in results if r.status is not None).items()):
        out(f"  {status}  {n:>5}")
    if errors:
        out(f"  (no usable response / unparseable: {errors})")
    out("")

    observed_reasons = Counter(r.nobid_reason for r in results if r.status == 204)
    out("No-bid reasons (X-Nobid-Reason header)")
    for reason, n in sorted(observed_reasons.items(), key=lambda kv: (-kv[1], str(kv[0]))):
        out(f"  {str(reason):<18}{n:>5}")
    if not observed_reasons:
        out("  (none)")
    out("")

    out("400 error codes")
    codes = Counter(r.error_code for r in results if r.status == 400)
    for code, n in sorted(codes.items(), key=lambda kv: (-kv[1], str(kv[0]))):
        out(f"  {str(code):<22}{n:>5}")
    if not codes:
        out("  (none)")
    out("")

    out("Expected vs observed, per category")
    out(f"  {'category':<14}{'n':>6}{'ok':>7}{'mismatch':>10}")
    by_cat: dict[str, list[bool]] = {name: [] for name, _ in MIX}
    for req in requests:
        by_cat[req.category].append(req.index not in problem_idx)
    for name, _ in MIX:
        oks = by_cat[name]
        out(f"  {name:<14}{len(oks):>6}{sum(oks):>7}{len(oks) - sum(oks):>10}")
    out("")

    if server_counters is not None:
        out("Bidder /debug/nobid counters")
        for reason, n in sorted(server_counters.items()):
            out(f"  {reason:<18}{n:>5}")
        if not server_counters:
            out("  (none)")
        if counters_are_fresh:
            seen = {str(k): v for k, v in observed_reasons.items()}
            if server_counters != seen:
                problems.append(("counters", f"server {server_counters} != observed {seen}"))
                out("  MISMATCH: counters differ from observed headers")
            else:
                out("  matches observed headers")
        else:
            out("  (external bidder: counters may include earlier traffic; not compared)")
        out("")

    if problems:
        out(f"RESULT: FAIL  ({len(problems)} problem(s))")
        for label, msg in problems[:MAX_MISMATCHES_SHOWN]:
            out(f"  {label}: {msg}")
        if len(problems) > MAX_MISMATCHES_SHOWN:
            out(f"  ... and {len(problems) - MAX_MISMATCHES_SHOWN} more")
    else:
        out("RESULT: PASS  (every response matched its expectation)")
    out("")

    # Everything below depends on this machine's wall clock: informational only.
    out("Informational: measured on this machine, NOT deterministic, NOT a benchmark")
    timed = sorted(r.elapsed_ms for r in results if r.error is None)
    if timed:
        out(f"  round-trip ms  p50={percentile(timed, 50):.2f}  p95={percentile(timed, 95):.2f}  max={timed[-1]:.2f}")
    late: Counter[str] = Counter()
    with_tmax: Counter[str] = Counter()
    for req, res in zip(requests, results, strict=True):
        if req.tmax_ms is not None and res.error is None:
            with_tmax[req.category] += 1
            if res.elapsed_ms > req.tmax_ms:
                late[req.category] += 1
    out("  responses slower than the request's tmax (an exchange would discard these;")
    out("  the bidder itself does not enforce tmax)")
    for name, _ in MIX:
        if with_tmax[name]:
            tag = "  <- tmax=1ms by design" if name == TIGHT_TMAX else ""
            out(f"    {name:<14}{late[name]:>4} / {with_tmax[name]:<4}{tag}")

    return Report("\n".join(lines) + "\n", mismatches=len(problems), errors=errors)
