from simulator.generator import SimRequest
from simulator.replay import Result
from simulator.summary import build_report, check, percentile


def req(i, status, **kw):
    return SimRequest(i, kw.pop("category", "valid"), kw.pop("kind", "bid"), "{}", status, **kw)


def res(i, status, **kw):
    return Result(i, status, kw.pop("nobid_reason", None), kw.pop("error_code", None),
                  kw.pop("price_micros", None), kw.pop("elapsed_ms", 1.0), kw.pop("error", None))


def test_check_matches_and_mismatches():
    assert check(req(0, 200, expected_price_micros=2_500_000), res(0, 200, price_micros=2_500_000)) is None
    assert "price" in check(req(0, 200, expected_price_micros=2_500_000), res(0, 200, price_micros=1))
    assert "status" in check(req(0, 200, expected_price_micros=1), res(0, 204))
    assert check(req(0, 204, expected_nobid_reason="x"), res(0, 204, nobid_reason="x")) is None
    assert "reason" in check(req(0, 204, expected_nobid_reason="x"), res(0, 204, nobid_reason="y"))
    assert check(req(0, 400, expected_error_code="c"), res(0, 400, error_code="c")) is None
    assert "error code" in check(req(0, 400, expected_error_code="c"), res(0, 400, error_code="d"))
    assert "error" in check(req(0, 200), res(0, None, error="boom"))


def test_percentile_nearest_rank():
    data = [float(i) for i in range(1, 101)]
    assert percentile(data, 50) == 50.0
    assert percentile(data, 95) == 95.0
    assert percentile([3.0], 99) == 3.0


def _report(requests, results, counters=None, fresh=True):
    return build_report(seed=1, sha256="ab" * 32, requests=requests, results=results,
                        bidder_desc="test", server_counters=counters, counters_are_fresh=fresh)


def test_report_pass_and_counts():
    requests = [req(0, 200, expected_price_micros=1), req(1, 204, category="below_floor", kind="floor_above_bid",
                                                          expected_nobid_reason="floor_above_bid")]
    results = [res(0, 200, price_micros=1), res(1, 204, nobid_reason="floor_above_bid")]
    r = _report(requests, results, {"floor_above_bid": 1})
    assert r.ok and r.mismatches == 0
    assert "RESULT: PASS" in r.text and "floor_above_bid" in r.text and "matches observed" in r.text


def test_report_fail_lists_problem():
    requests = [req(0, 200, expected_price_micros=1)]
    r = _report(requests, [res(0, 204, nobid_reason="x")])
    assert not r.ok and r.mismatches == 1
    assert "RESULT: FAIL" in r.text and "#0 valid/bid" in r.text


def test_counters_mismatch_fails_only_when_fresh():
    requests = [req(0, 204, expected_nobid_reason="x")]
    results = [res(0, 204, nobid_reason="x")]
    assert not _report(requests, results, {"x": 5}, fresh=True).ok
    assert _report(requests, results, {"x": 5}, fresh=False).ok


def test_transport_errors_are_counted_and_fail():
    r = _report([req(0, 200)], [res(0, None, error="refused")])
    assert r.errors == 1 and not r.ok
