"""Shared fixtures for the Local Pilot (Shamsu) Python test suite.

Every test runs against a throwaway HOME and a throwaway SQLite database, so the
suite never reads or writes the developer's real ~/.localpilot data. The backend
package is put on sys.path here because it is an application, not an installed
distribution.
"""

import json
import os
import sys
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[2]
BACKEND = REPO_ROOT / "backend"
# The bot lives in telegram/, whose directory name would shadow the installed
# python-telegram-bot package, so it is loaded by path (see test_telegram_bot.py)
# rather than added to sys.path.
if str(BACKEND) not in sys.path:
    sys.path.insert(0, str(BACKEND))


@pytest.fixture(scope="session", autouse=True)
def isolated_home(tmp_path_factory):
    """Point the shared data dir and database at a temp home for the whole run."""
    home = tmp_path_factory.mktemp("home")
    os.environ["HOME"] = str(home)
    os.environ["LOCALAPPDATA"] = str(home)
    os.environ["XDG_DATA_HOME"] = str(home)
    os.environ["DATABASE_URL"] = f"sqlite:///{home / 'test.db'}"
    yield home


@pytest.fixture
def clean_db(isolated_home):
    """A fresh database file per test, so link codes and profiles never leak."""
    import schemas  # noqa: F401  (registers the chat tables on the metadata)
    import schemas.profile  # noqa: F401  (registers the profile + telegram tables)
    from core.database import init_db

    db = isolated_home / "test.db"
    if db.exists():
        db.unlink()
    init_db()
    yield


@pytest.fixture
def project_dir(tmp_path):
    """An empty project directory, standing in for a user's real code folder."""
    root = tmp_path / "project"
    root.mkdir()
    return str(root)


class FakeHarness:
    """A stub of the Go harness /run endpoint.

    It streams a scripted list of newline-delimited events, exactly as the real
    harness does, and records the request bodies it was sent so a test can assert
    what the backend forwarded. With hold=True the response stays open after the
    last event, the way the real harness does while it waits for an ask-mode
    approval; the /confirm call (or leaving the fixture) releases it.
    """

    def __init__(self, events, hold=False):
        self.events = events
        self.hold = hold
        self.requests = []
        self.confirms = []
        self.released = threading.Event()
        outer = self

        class Handler(BaseHTTPRequestHandler):
            def do_POST(self):
                length = int(self.headers.get("Content-Length", "0"))
                raw = self.rfile.read(length).decode()
                payload = json.loads(raw or "{}")
                if self.path.rstrip("/").endswith("confirm"):
                    outer.confirms.append(payload)
                    outer.released.set()
                    body = json.dumps({"ok": True}).encode()
                    self.send_response(200)
                    self.send_header("Content-Type", "application/json")
                    self.send_header("Content-Length", str(len(body)))
                    self.end_headers()
                    self.wfile.write(body)
                    return
                outer.requests.append(payload)
                self.send_response(200)
                self.send_header("Content-Type", "application/x-ndjson")
                self.end_headers()
                for event in outer.events:
                    self.wfile.write((json.dumps(event) + "\n").encode())
                    self.wfile.flush()
                if outer.hold:
                    outer.released.wait(timeout=10)

            def log_message(self, *args, **kwargs):
                return

        self._server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self._thread = threading.Thread(target=self._server.serve_forever, daemon=True)

    def __enter__(self):
        self._thread.start()
        return self

    def __exit__(self, *exc):
        self.released.set()
        self._server.shutdown()
        self._server.server_close()

    @property
    def url(self) -> str:
        return f"http://127.0.0.1:{self._server.server_address[1]}/run"


@pytest.fixture
def fake_harness():
    """Factory for a scripted harness stub."""
    return FakeHarness
