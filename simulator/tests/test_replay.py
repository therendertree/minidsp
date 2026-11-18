import json
import shutil
import threading
from http.server import BaseHTTPRequestHandler, HTTPServer

import pytest

from simulator.generator import generate
from simulator.replay import fetch_nobid_counters, replay
from simulator.spawn import spawned_bidder
from simulator.summary import build_report


class _Stub(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *a):  # keep test output quiet
        pass

    def _send(self, status, body=b"", headers=()):
        self.send_response(status)
        for k, v in headers:
            self.send_header(k, v)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        self.rfile.read(int(self.headers["Content-Length"]))
        n = self.server.calls = getattr(self.server, "calls", 0) + 1
        if n == 1:
            body = json.dumps({"id": "x", "seatbid": [{"bid": [{"price": 2.5}]}]}).encode()
            self._send(200, body, [("Content-Type", "application/json")])
        elif n == 2:
            self._send(204, headers=[("X-Nobid-Reason", "size_mismatch")])
        elif n == 3:
            self._send(400, json.dumps({"error": {"code": "missing_field", "field": "id"}}).encode())
        else:
            self._send(200, b"not json")

    def do_GET(self):
        self._send(200, json.dumps({"size_mismatch": 1}).encode())


@pytest.fixture
def stub_url():
    srv = HTTPServer(("127.0.0.1", 0), _Stub)
    t = threading.Thread(target=srv.serve_forever, daemon=True)
    t.start()
    yield f"http://127.0.0.1:{srv.server_address[1]}"
    srv.shutdown()


def test_replay_parses_each_response_kind(stub_url):
    results = replay(generate(1, 4), stub_url)
    assert [r.status for r in results] == [200, 204, 400, 200]
    assert results[0].price_micros == 2_500_000
    assert results[1].nobid_reason == "size_mismatch"
    assert results[2].error_code == "missing_field"
    assert results[3].error and "unparseable" in results[3].error  # bad 200 body is flagged
    assert fetch_nobid_counters(stub_url) == {"size_mismatch": 1}


def test_replay_records_connection_failure_without_raising():
    results = replay(generate(1, 2), "http://127.0.0.1:1")  # nothing listens on port 1
    assert len(results) == 2 and all(r.status is None and r.error for r in results)


def test_replay_rejects_non_http_url():
    with pytest.raises(ValueError):
        replay(generate(1, 1), "https://example.com")


@pytest.mark.skipif(shutil.which("go") is None, reason="Go toolchain not installed")
def test_end_to_end_against_real_bidder_is_deterministic():
    requests = generate(42, 200)
    outcomes = []
    for _ in range(2):  # two fresh bidders, same traffic
        with spawned_bidder() as url:
            results = replay(requests, url)
            counters = fetch_nobid_counters(url)
        report = build_report(seed=42, sha256="x", requests=requests, results=results,
                              bidder_desc="test", server_counters=counters, counters_are_fresh=True)
        assert report.ok, report.text
        outcomes.append([(r.status, r.nobid_reason, r.error_code, r.price_micros) for r in results])
    assert outcomes[0] == outcomes[1]
