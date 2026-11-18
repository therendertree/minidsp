import json
from collections import Counter
from decimal import Decimal

import pytest

from simulator.generator import (
    BELOW_FLOOR, INVALID, INVALID_KINDS, MIX, NON_MATCHING, TIGHT_TMAX, TIGHT_TMAX_MS, VALID,
    allocate_counts, format_micros, generate, requests_sha256,
)


def test_same_seed_is_byte_identical():
    a, b = generate(42, 1000), generate(42, 1000)
    assert [r.body for r in a] == [r.body for r in b]
    assert a == b
    assert requests_sha256(a) == requests_sha256(b)


def test_different_seed_differs():
    assert requests_sha256(generate(1, 200)) != requests_sha256(generate(2, 200))


def test_prefix_is_not_required_to_match_but_count_changes_output_length():
    assert len(generate(7, 0)) == 0
    assert len(generate(7, 13)) == 13


def test_exact_category_counts_for_1000():
    counts = Counter(r.category for r in generate(42, 1000))
    assert counts == {VALID: 600, BELOW_FLOOR: 100, NON_MATCHING: 150, INVALID: 100, TIGHT_TMAX: 50}


@pytest.mark.parametrize("count", [0, 1, 7, 99, 101, 333, 1000, 1001])
def test_allocation_sums_to_count_and_is_close_to_mix(count):
    alloc = allocate_counts(count)
    assert sum(alloc.values()) == count
    for name, pct in MIX:
        assert abs(alloc[name] - count * pct / 100) < 1


def test_request_ids_unique_and_prefixed_with_seed():
    reqs = generate(5, 300)
    ids = [json.loads(r.body)["id"] for r in reqs if r.kind not in ("malformed_json", "missing_id", "id_wrong_type")]
    assert len(ids) == len(set(ids))
    assert all(i.startswith("sim-5-") for i in ids)


def test_all_bodies_are_json_except_malformed():
    for r in generate(42, 1000):
        if r.kind == "malformed_json":
            with pytest.raises(json.JSONDecodeError):
                json.loads(r.body)
        else:
            json.loads(r.body)


def test_tight_tmax_requests_use_1ms_and_expect_a_bid():
    tight = [r for r in generate(42, 1000) if r.category == TIGHT_TMAX]
    assert tight
    for r in tight:
        assert r.tmax_ms == TIGHT_TMAX_MS == 1
        assert json.loads(r.body)["tmax"] == 1
        assert r.expected_status == 200


def test_other_categories_do_not_use_tight_tmax():
    for r in generate(42, 1000):
        if r.category != TIGHT_TMAX:
            assert r.tmax_ms != 1


def test_below_floor_floors_exceed_every_campaign_cpm():
    reqs = [r for r in generate(42, 1000) if r.category == BELOW_FLOOR]
    for r in reqs:
        floor = json.loads(r.body, parse_float=Decimal)["imp"][0]["bidfloor"]
        assert floor > 3  # highest eligible CPM in testdata/campaigns_sim.json
        assert (r.expected_status, r.expected_nobid_reason) == (204, "floor_above_bid")


def test_valid_floor_never_exceeds_expected_price():
    for r in generate(42, 1000):
        if r.expected_status == 200:
            imp = json.loads(r.body, parse_float=Decimal)["imp"][0]
            floor = Decimal(str(imp.get("bidfloor", 0)))
            assert floor * 1_000_000 <= r.expected_price_micros


def test_non_matching_covers_size_geo_and_os():
    kinds = Counter(r.kind for r in generate(42, 1000) if r.category == NON_MATCHING)
    assert set(kinds) == {"size_mismatch", "geo_mismatch", "os_mismatch"}
    for r in generate(42, 1000):
        if r.category == NON_MATCHING:
            assert r.expected_nobid_reason == r.kind


def test_invalid_covers_every_kind_with_a_distinct_expectation():
    reqs = [r for r in generate(42, 1000) if r.category == INVALID]
    assert {r.kind for r in reqs} == set(INVALID_KINDS)
    assert all(r.expected_status == 400 and r.expected_error_code for r in reqs)


def test_floors_are_written_as_exact_decimals_not_floats():
    # No float artifacts like 0.30000000000000004 anywhere in the traffic.
    for r in generate(42, 1000):
        assert "000000000" not in r.body


@pytest.mark.parametrize("micros,text", [
    (0, "0"), (1, "0.000001"), (10_000, "0.01"), (1_500_000, "1.5"),
    (3_000_000, "3"), (3_010_000, "3.01"), (-500_000, "-0.5"),
])
def test_format_micros(micros, text):
    assert format_micros(micros) == text


def test_negative_count_rejected():
    with pytest.raises(ValueError):
        generate(1, -1)
