"""Deterministic, seeded bid-request generator (synthetic traffic only).

The same (seed, count) always yields byte-identical requests in the same order.
All randomness comes from one ``random.Random(seed)``; global random state is
never touched. Money is handled as integer micro-USD and written into the JSON
text as an exact decimal, never through a float.

The expected outcomes recorded on each request assume the campaign set in
``testdata/campaigns_sim.json`` (see the CAMPAIGN MODEL note below).

CAMPAIGN MODEL (testdata/campaigns_sim.json), CPM in USD:
  sim-usa-android  3.0  geo USA        os android  300x250
  sim-usa-ios      2.5  geo USA        os ios      300x250
  sim-can-gbr      1.5  geo CAN, GBR   any os      300x250, 728x90
  sim-paused       9.0  paused (never eligible)
Every active campaign is geo-targeted on purpose: a campaign with no targeting
would always pass the geo/OS filters, and the bidder reports the reason of the
campaign that got furthest, so geo/OS no-bids could never be produced.
"""

from __future__ import annotations

import hashlib
import json
import random
from dataclasses import dataclass

# --- categories -----------------------------------------------------------

VALID = "valid"
BELOW_FLOOR = "below_floor"
NON_MATCHING = "non_matching"
INVALID = "invalid"
TIGHT_TMAX = "tight_tmax"

# Fixed percentage mix (must sum to 100). Order matters for reproducibility.
MIX: tuple[tuple[str, int], ...] = (
    (VALID, 60),
    (BELOW_FLOOR, 10),
    (NON_MATCHING, 15),
    (INVALID, 10),
    (TIGHT_TMAX, 5),
)

TIGHT_TMAX_MS = 1
NORMAL_TMAX_MS = (80, 100, 120, 150)

# --- world knowledge: must match testdata/campaigns_sim.json --------------

MICROS_PER_USD = 1_000_000
FLOOR_STEP = 10_000  # floors are whole cents


@dataclass(frozen=True)
class Profile:
    """A (geo, OS, size) combination and the winning CPM it should get."""

    countries: tuple[str, ...]
    oses: tuple[str, ...]
    sizes: tuple[tuple[int, int], ...]
    win_cpm_micros: int


ALL_OSES = ("Android", "iOS", "Windows", "Linux")
PROFILES: tuple[Profile, ...] = (
    Profile(("USA",), ("Android",), ((300, 250),), 3_000_000),  # sim-usa-android
    Profile(("USA",), ("iOS",), ((300, 250),), 2_500_000),  # sim-usa-ios
    Profile(("CAN", "GBR"), ALL_OSES, ((300, 250), (728, 90)), 1_500_000),  # sim-can-gbr
)
MAX_CPM_MICROS = 3_000_000  # highest CPM among eligible campaigns
UNKNOWN_SIZES = ((160, 600), (970, 250), (468, 60))  # no campaign has these
UNKNOWN_COUNTRIES = ("DEU", "FRA", "JPN", "BRA")  # no campaign targets these

DOMAINS = ("news.example.com", "sports.example.org", "blog.example.net", "shop.example.com")
BUNDLES = ("com.example.game", "com.example.news", "com.example.fit")
UAS = {
    "Android": "Dalvik/2.1.0 (Linux; Android 14)",
    "iOS": "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)",
    "Windows": "Mozilla/5.0 (Windows NT 10.0; Win64; x64)",
    "Linux": "Mozilla/5.0 (X11; Linux x86_64)",
}

_FLOOR_SENTINEL = "@@FLOOR@@"


# --- data model -----------------------------------------------------------


@dataclass(frozen=True)
class SimRequest:
    index: int
    category: str
    kind: str  # finer label within the category, e.g. "size_mismatch"
    body: str  # exact text POSTed to the bidder
    expected_status: int
    expected_nobid_reason: str | None = None  # for 204
    expected_error_code: str | None = None  # for 400
    expected_price_micros: int | None = None  # for 200
    tmax_ms: int | None = None


# --- helpers --------------------------------------------------------------


def format_micros(micros: int) -> str:
    """Exact decimal text for a micro-USD amount: 1_500_000 -> '1.5'."""
    if micros < 0:
        return "-" + format_micros(-micros)
    whole, frac = divmod(micros, MICROS_PER_USD)
    if frac == 0:
        return str(whole)
    return f"{whole}.{frac:06d}".rstrip("0")


def allocate_counts(count: int) -> dict[str, int]:
    """Split ``count`` across MIX exactly (largest-remainder method)."""
    base = {name: count * pct // 100 for name, pct in MIX}
    remainders = sorted(
        ((count * pct % 100, -i, name) for i, (name, pct) in enumerate(MIX)),
        reverse=True,
    )
    for _, _, name in remainders[: count - sum(base.values())]:
        base[name] += 1
    return base


def requests_sha256(requests: list[SimRequest]) -> str:
    h = hashlib.sha256()
    for r in requests:
        h.update(r.body.encode("utf-8"))
        h.update(b"\n")
    return h.hexdigest()


def _dumps(obj: dict, floor_micros: int | None) -> str:
    """Compact JSON; the bid floor is spliced in as exact decimal text."""
    text = json.dumps(obj, separators=(",", ":"))
    if floor_micros is not None:
        text = text.replace(json.dumps(_FLOOR_SENTINEL), format_micros(floor_micros))
    return text


# --- generation -----------------------------------------------------------


class Generator:
    def __init__(self, seed: int):
        self.seed = seed
        self.rng = random.Random(seed)

    def generate(self, count: int) -> list[SimRequest]:
        counts = allocate_counts(count)
        categories = [name for name, _ in MIX for _ in range(counts[name])]
        self.rng.shuffle(categories)
        builders = {
            VALID: self._valid,
            BELOW_FLOOR: self._below_floor,
            NON_MATCHING: self._non_matching,
            INVALID: self._invalid,
            TIGHT_TMAX: self._tight_tmax,
        }
        return [builders[cat](i) for i, cat in enumerate(categories)]

    # -- request body construction

    def _request_id(self, index: int) -> str:
        return f"sim-{self.seed}-{index:05d}"

    def _base(
        self,
        index: int,
        country: str,
        os_name: str,
        size: tuple[int, int],
        floor_micros: int | None,
        tmax_ms: int | None,
    ) -> dict:
        """A valid request. ``floor_micros`` None means the field is omitted."""
        rng = self.rng
        imp: dict = {"id": "1", "banner": {"w": size[0], "h": size[1]}}
        if floor_micros is not None:
            imp["bidfloor"] = _FLOOR_SENTINEL
            if rng.random() < 0.3:
                imp["bidfloorcur"] = "USD"
        body: dict = {"id": self._request_id(index), "imp": [imp]}
        if tmax_ms is not None:
            body["tmax"] = tmax_ms
        if rng.random() < 0.7:
            body["site"] = {
                "id": f"site-{rng.randrange(1, 50)}",
                "domain": rng.choice(DOMAINS),
                "page": f"https://{rng.choice(DOMAINS)}/p/{rng.randrange(1, 1000)}",
            }
        else:
            body["app"] = {"id": f"app-{rng.randrange(1, 50)}", "bundle": rng.choice(BUNDLES)}
        body["device"] = {
            "ua": UAS[os_name],
            "ip": f"203.0.113.{rng.randrange(1, 255)}",
            "devicetype": 4 if os_name in ("Android", "iOS") else 2,
            "os": os_name,
            "geo": {"country": country},
        }
        return body

    def _pick_profile(self) -> tuple[Profile, str, str, tuple[int, int]]:
        rng = self.rng
        p = rng.choice(PROFILES)
        return p, rng.choice(p.countries), rng.choice(p.oses), rng.choice(p.sizes)

    def _normal_tmax(self) -> int | None:
        return None if self.rng.random() < 0.1 else self.rng.choice(NORMAL_TMAX_MS)

    def _winning_floor(self, cpm_micros: int) -> int | None:
        """A floor the winner can meet (<= its CPM, equality allowed), or None."""
        if self.rng.random() < 0.2:
            return None  # field omitted: floor defaults to 0
        return self.rng.randrange(0, cpm_micros + 1, FLOOR_STEP)

    # -- categories

    def _make_valid(self, index: int, category: str, tmax_ms: int | None) -> SimRequest:
        p, country, os_name, size = self._pick_profile()
        floor = self._winning_floor(p.win_cpm_micros)
        body = _dumps(self._base(index, country, os_name, size, floor, tmax_ms), floor)
        return SimRequest(
            index, category, "bid", body, 200,
            expected_price_micros=p.win_cpm_micros, tmax_ms=tmax_ms,
        )

    def _valid(self, index: int) -> SimRequest:
        return self._make_valid(index, VALID, self._normal_tmax())

    def _tight_tmax(self, index: int) -> SimRequest:
        # The bidder does not enforce tmax; the exchange does. The request is
        # valid and still gets a normal 200. Lateness is measured client-side.
        return self._make_valid(index, TIGHT_TMAX, TIGHT_TMAX_MS)

    def _below_floor(self, index: int) -> SimRequest:
        _, country, os_name, size = self._pick_profile()
        floor = self.rng.randrange(MAX_CPM_MICROS + FLOOR_STEP, 20 * MICROS_PER_USD + 1, FLOOR_STEP)
        tmax = self._normal_tmax()
        body = _dumps(self._base(index, country, os_name, size, floor, tmax), floor)
        return SimRequest(
            index, BELOW_FLOOR, "floor_above_bid", body, 204,
            expected_nobid_reason="floor_above_bid", tmax_ms=tmax,
        )

    def _non_matching(self, index: int) -> SimRequest:
        rng = self.rng
        kind = rng.choice(("size_mismatch", "geo_mismatch", "os_mismatch"))
        if kind == "size_mismatch":
            # Matches sim-usa-android on geo/OS so size is the furthest failure.
            country, os_name, size = "USA", "Android", rng.choice(UNKNOWN_SIZES)
        elif kind == "geo_mismatch":
            country, os_name, size = rng.choice(UNKNOWN_COUNTRIES), rng.choice(ALL_OSES), (300, 250)
        else:  # os_mismatch: USA passes geo of the USA campaigns, Windows/Linux fails OS
            country, os_name, size = "USA", rng.choice(("Windows", "Linux")), (300, 250)
        floor = rng.randrange(0, MICROS_PER_USD + 1, FLOOR_STEP)
        tmax = self._normal_tmax()
        body = _dumps(self._base(index, country, os_name, size, floor, tmax), floor)
        return SimRequest(
            index, NON_MATCHING, kind, body, 204, expected_nobid_reason=kind, tmax_ms=tmax
        )

    # Each breaker mutates a valid request dict; value = (expected code, mutator).
    def _invalid(self, index: int) -> SimRequest:
        rng = self.rng
        kind = rng.choice(INVALID_KINDS)
        floor = 500_000
        tmax = self._normal_tmax()
        body = self._base(index, "USA", "Android", (300, 250), floor, tmax)
        floor_for_text: int | None = floor

        if kind == "malformed_json":
            text = _dumps(body, floor)
            text = text[: rng.randrange(10, len(text) - 1)]  # truncated mid-document
            return SimRequest(index, INVALID, kind, text, 400,
                              expected_error_code="malformed_json", tmax_ms=tmax)
        if kind == "missing_id":
            del body["id"]
            code = "missing_field"
        elif kind == "id_wrong_type":
            body["id"] = rng.randrange(1, 10**6)
            code = "invalid_type"
        elif kind == "multiple_imps":
            body["imp"].append({"id": "2", "banner": {"w": 300, "h": 250}})
            code = "invalid_imp_count"
        elif kind == "non_usd_floor":
            body["imp"][0]["bidfloorcur"] = rng.choice(("EUR", "GBP", "JPY"))
            code = "unsupported_currency"
        elif kind == "negative_floor":
            body["imp"][0]["bidfloor"] = _FLOOR_SENTINEL
            floor_for_text = -rng.randrange(FLOOR_STEP, MICROS_PER_USD, FLOOR_STEP)
            code = "invalid_value"
        elif kind == "zero_banner_width":
            body["imp"][0]["banner"]["w"] = 0
            code = "invalid_value"
        elif kind == "site_and_app":
            body["site"] = {"id": "s"}
            body["app"] = {"id": "a"}
            code = "invalid_inventory"
        else:  # pragma: no cover - INVALID_KINDS and branches kept in sync by tests
            raise AssertionError(kind)
        text = _dumps(body, floor_for_text)
        return SimRequest(index, INVALID, kind, text, 400,
                          expected_error_code=code, tmax_ms=tmax)


INVALID_KINDS = (
    "malformed_json",
    "missing_id",
    "id_wrong_type",
    "multiple_imps",
    "non_usd_floor",
    "negative_floor",
    "zero_banner_width",
    "site_and_app",
)


def generate(seed: int, count: int) -> list[SimRequest]:
    """The single entry point: deterministic for a given (seed, count)."""
    if count < 0:
        raise ValueError("count must be >= 0")
    return Generator(seed).generate(count)
