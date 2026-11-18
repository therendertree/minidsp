"""Build and run the Go bidder as a child process for `make sim` (stdlib only)."""

from __future__ import annotations

import contextlib
import http.client
import os
import socket
import subprocess
import tempfile
import time
from collections.abc import Iterator
from pathlib import Path

# simulator/src/simulator/spawn.py -> repo root is three levels above this package dir.
DEFAULT_REPO_ROOT = Path(__file__).resolve().parents[3]
DEFAULT_CAMPAIGNS = "testdata/campaigns_sim.json"


class SpawnError(RuntimeError):
    pass


def _free_port() -> int:
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def _wait_healthy(port: int, proc: subprocess.Popen, timeout: float = 10.0) -> None:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if proc.poll() is not None:
            raise SpawnError(f"bidder exited early with code {proc.returncode}")
        try:
            conn = http.client.HTTPConnection("127.0.0.1", port, timeout=1)
            conn.request("GET", "/healthz")
            if conn.getresponse().status == 200:
                return
        except (OSError, http.client.HTTPException):
            pass
        time.sleep(0.1)
    raise SpawnError("bidder did not become healthy in time")


@contextlib.contextmanager
def spawned_bidder(
    repo_root: Path = DEFAULT_REPO_ROOT, campaigns: str = DEFAULT_CAMPAIGNS
) -> Iterator[str]:
    """Build the bidder, start it on a free port, yield its base URL, then stop it.

    The campaigns path is resolved against repo_root unless absolute.
    """
    campaigns_path = Path(campaigns)
    if not campaigns_path.is_absolute():
        campaigns_path = repo_root / campaigns_path
    if not campaigns_path.is_file():
        raise SpawnError(f"campaigns file not found: {campaigns_path}")

    with tempfile.TemporaryDirectory(prefix="minidsp-sim-") as tmp:
        binary = Path(tmp) / ("bidder.exe" if os.name == "nt" else "bidder")
        build = subprocess.run(
            ["go", "build", "-o", str(binary), "./cmd/bidder"],
            cwd=repo_root / "bidder", capture_output=True, text=True,
        )
        if build.returncode != 0:
            raise SpawnError(f"go build failed:\n{build.stderr}")

        port = _free_port()
        env = {**os.environ, "BIDDER_ADDR": f"127.0.0.1:{port}", "BIDDER_CAMPAIGNS": str(campaigns_path)}
        log_path = Path(tmp) / "bidder.log"
        with open(log_path, "w", encoding="utf-8") as log:
            proc = subprocess.Popen([str(binary)], env=env, stdout=log, stderr=subprocess.STDOUT)
            try:
                try:
                    _wait_healthy(port, proc)
                except SpawnError as exc:
                    raise SpawnError(f"{exc}\n--- bidder log ---\n{log_path.read_text(encoding='utf-8')}") from None
                yield f"http://127.0.0.1:{port}"
            finally:
                proc.terminate()
                try:
                    proc.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    proc.kill()
                    proc.wait()
