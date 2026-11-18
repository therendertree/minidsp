"""`python -m simulator`: generate seeded traffic and replay it against the bidder.

Examples:
    python -m simulator --spawn-bidder              # build + run bidder, replay 1000 requests
    python -m simulator --url http://127.0.0.1:8080 # replay against a bidder you started
    python -m simulator --generate-only --dump out.jsonl
"""

from __future__ import annotations

import argparse
import json
import sys
from collections import Counter
from pathlib import Path

from simulator.generator import MIX, generate, requests_sha256
from simulator.replay import fetch_nobid_counters, replay
from simulator.spawn import DEFAULT_CAMPAIGNS, DEFAULT_REPO_ROOT, SpawnError, spawned_bidder
from simulator.summary import build_report

DEFAULT_URL = "http://127.0.0.1:8080"


def _parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(prog="simulator", description=__doc__.splitlines()[0])
    p.add_argument("--seed", type=int, default=42, help="RNG seed (default 42)")
    p.add_argument("--count", type=int, default=1000, help="number of requests (default 1000)")
    p.add_argument("--url", default=DEFAULT_URL, help=f"bidder base URL (default {DEFAULT_URL})")
    p.add_argument("--spawn-bidder", action="store_true",
                   help="build and run the Go bidder on a free port for this run (ignores --url)")
    p.add_argument("--campaigns", default=DEFAULT_CAMPAIGNS,
                   help=f"campaigns file for --spawn-bidder (default {DEFAULT_CAMPAIGNS})")
    p.add_argument("--repo-root", type=Path, default=DEFAULT_REPO_ROOT, help="repository root")
    p.add_argument("--generate-only", action="store_true", help="generate and describe traffic; send nothing")
    p.add_argument("--dump", type=Path, help="write the generated requests as JSON lines to this file")
    return p


def _dump(path: Path, requests) -> None:
    with open(path, "w", encoding="utf-8", newline="\n") as f:
        for r in requests:
            f.write(json.dumps({
                "index": r.index, "category": r.category, "kind": r.kind,
                "expected_status": r.expected_status,
                "expected_nobid_reason": r.expected_nobid_reason,
                "expected_error_code": r.expected_error_code,
                "expected_price_micros": r.expected_price_micros,
                "tmax_ms": r.tmax_ms, "body": r.body,
            }) + "\n")


def _describe_generation(seed: int, requests) -> str:
    cats = Counter(r.category for r in requests)
    kinds = Counter((r.category, r.kind) for r in requests)
    lines = [f"seed={seed}  requests={len(requests)}  requests_sha256={requests_sha256(requests)[:16]}"]
    for name, _ in MIX:
        lines.append(f"  {name:<14}{cats[name]:>6}")
        for (cat, kind), n in sorted(kinds.items()):
            if cat == name and kind != "bid":
                lines.append(f"      {kind:<20}{n:>5}")
    return "\n".join(lines) + "\n"


def main(argv: list[str] | None = None) -> int:
    args = _parser().parse_args(argv)
    if args.count < 0:
        print("--count must be >= 0", file=sys.stderr)
        return 2

    requests = generate(args.seed, args.count)
    if args.dump:
        _dump(args.dump, requests)

    if args.generate_only:
        print(_describe_generation(args.seed, requests), end="")
        return 0

    sha = requests_sha256(requests)
    try:
        if args.spawn_bidder:
            with spawned_bidder(args.repo_root, args.campaigns) as url:
                desc = f"spawned at {url} with {args.campaigns}"
                results = replay(requests, url)
                counters = fetch_nobid_counters(url)
            fresh = True
        else:
            desc = f"external at {args.url} (campaign set must match testdata/campaigns_sim.json)"
            results = replay(requests, args.url)
            counters = fetch_nobid_counters(args.url)
            fresh = False
    except SpawnError as exc:
        print(f"could not start bidder: {exc}", file=sys.stderr)
        return 2

    report = build_report(
        seed=args.seed, sha256=sha, requests=requests, results=results,
        bidder_desc=desc, server_counters=counters, counters_are_fresh=fresh,
    )
    print(report.text, end="")
    return 0 if report.ok else 1


if __name__ == "__main__":
    raise SystemExit(main())
