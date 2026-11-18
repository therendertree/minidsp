"""Replay generated requests against a running bidder over HTTP (stdlib only)."""

from __future__ import annotations

import http.client
import json
import time
from dataclasses import dataclass
from decimal import Decimal
from urllib.parse import urlsplit

from simulator.generator import MICROS_PER_USD, SimRequest

BID_PATH = "/openrtb2/bid"
NOBID_REASON_HEADER = "X-Nobid-Reason"


@dataclass(frozen=True)
class Result:
    index: int
    status: int | None  # None if no HTTP response was received
    nobid_reason: str | None  # from X-Nobid-Reason on 204
    error_code: str | None  # from the error body on 400
    price_micros: int | None  # from the bid on 200
    elapsed_ms: float  # wall-clock, includes local HTTP overhead
    error: str | None = None  # transport or response-parsing failure


def _split(base_url: str) -> tuple[str, int]:
    parts = urlsplit(base_url)
    if parts.scheme != "http" or not parts.hostname:
        raise ValueError(f"only plain http URLs are supported, got {base_url!r}")
    return parts.hostname, parts.port or 80


def _parse_price_micros(body: bytes) -> int:
    """Exact micro-USD from the response price (Decimal, never float)."""
    doc = json.loads(body, parse_float=Decimal)
    price = Decimal(str(doc["seatbid"][0]["bid"][0]["price"]))
    micros = price * MICROS_PER_USD
    if micros != micros.to_integral_value():
        raise ValueError(f"price {price} has sub-micro precision")
    return int(micros)


def _parse_error_code(body: bytes) -> str:
    return json.loads(body)["error"]["code"]


def replay(requests: list[SimRequest], base_url: str, timeout: float = 5.0) -> list[Result]:
    """POST every request, in order, over one keep-alive connection."""
    host, port = _split(base_url)
    conn = http.client.HTTPConnection(host, port, timeout=timeout)
    results: list[Result] = []
    for req in requests:
        start = time.perf_counter()
        try:
            conn.request(
                "POST", BID_PATH, body=req.body.encode("utf-8"),
                headers={"Content-Type": "application/json"},
            )
            resp = conn.getresponse()
            data = resp.read()
            elapsed = (time.perf_counter() - start) * 1000
            results.append(_to_result(req.index, resp.status, resp.getheader(NOBID_REASON_HEADER), data, elapsed))
        except (OSError, http.client.HTTPException) as exc:
            elapsed = (time.perf_counter() - start) * 1000
            results.append(Result(req.index, None, None, None, None, elapsed, error=f"{type(exc).__name__}: {exc}"))
            conn.close()  # reconnects automatically on the next request
    conn.close()
    return results


def _to_result(index: int, status: int, nobid: str | None, data: bytes, elapsed: float) -> Result:
    try:
        if status == 200:
            return Result(index, status, None, None, _parse_price_micros(data), elapsed)
        if status == 400:
            return Result(index, status, None, _parse_error_code(data), None, elapsed)
        if status == 204:
            return Result(index, status, nobid, None, None, elapsed)
    except (ValueError, KeyError, IndexError, TypeError) as exc:
        return Result(index, status, None, None, None, elapsed, error=f"unparseable body: {exc}")
    return Result(index, status, None, None, None, elapsed)


def fetch_nobid_counters(base_url: str, timeout: float = 5.0) -> dict[str, int] | None:
    """The bidder's in-memory no-bid counters, or None if unavailable."""
    host, port = _split(base_url)
    conn = http.client.HTTPConnection(host, port, timeout=timeout)
    try:
        conn.request("GET", "/debug/nobid")
        resp = conn.getresponse()
        data = resp.read()
        if resp.status != 200:
            return None
        return {str(k): int(v) for k, v in json.loads(data).items()}
    except (OSError, http.client.HTTPException, ValueError):
        return None
    finally:
        conn.close()
