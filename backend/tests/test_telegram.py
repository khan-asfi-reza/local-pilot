import json
import sys
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

from fastapi.testclient import TestClient

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))


def make_fake_harness():
    """A harness stub that streams a fixed reply for both the safe and full-access
    paths (unlike the chat test's stub, it does not assert on allowed_tools)."""
    class Handler(BaseHTTPRequestHandler):
        def do_POST(self):
            length = int(self.headers.get("Content-Length", "0"))
            self.rfile.read(length)
            self.send_response(200)
            self.send_header("Content-Type", "application/x-ndjson")
            self.end_headers()
            for event in [{"type": "text", "content": "Hello"}, {"type": "done"}]:
                self.wfile.write((json.dumps(event) + "\n").encode())
                self.wfile.flush()

        def log_message(self, *args, **kwargs):
            return

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    return server, thread


def test_telegram_flow(tmp_path, monkeypatch):
    server, thread = make_fake_harness()
    harness_url = f"http://127.0.0.1:{server.server_address[1]}/run"
    monkeypatch.setenv("HOME", str(tmp_path))  # isolate ~/.localpilot (projects.json, sandbox)
    monkeypatch.setenv("DATABASE_URL", f"sqlite:///{tmp_path / 'db.sqlite'}")
    monkeypatch.setenv("HARNESS_URL", harness_url)

    import main as main_mod
    import routes.profile as profile_routes
    import services.agent_runner as agent_runner
    import services.harness_client as harness_client
    from core import projects

    # Point the already-imported HARNESS_URL bindings at the fake server.
    monkeypatch.setattr(agent_runner, "HARNESS_URL", harness_url)
    monkeypatch.setattr(harness_client, "HARNESS_URL", harness_url)

    # Don't hit the real Telegram API for the bot username.
    async def fake_username(token):
        return "testbot"

    monkeypatch.setattr(profile_routes, "fetch_bot_username", fake_username)
    # Let the TestClient (host "testclient") pass the localhost guards.
    monkeypatch.setattr(main_mod, "_LOCAL_HOSTS", main_mod._LOCAL_HOSTS | {"testclient"})

    app = main_mod.create_app()
    app.dependency_overrides[profile_routes.local_only] = lambda: None

    with TestClient(app) as client:
        # Onboarding: fresh profile is not onboarded; saving a name onboards it.
        assert client.get("/profile").json()["onboarded"] is False
        client.post("/profile", json={"name": "Asfi"})
        prof = client.get("/profile").json()
        assert prof["onboarded"] and prof["name"] == "Asfi"

        # Telegram settings persist the token; the bot config endpoint returns it.
        settings = client.post("/telegram/settings", json={"bot_token": "123:abc", "enabled": True}).json()
        assert settings["configured"] and settings["bot_username"] == "testbot"
        cfg = client.get("/telegram/config").json()
        assert cfg["bot_token"] == "123:abc" and cfg["enabled"] is True

        # Link a chat with a one-time code; reusing the code fails.
        code = client.post("/telegram/link/start").json()["code"]
        assert client.post("/telegram/link/verify",
                           json={"code": code, "chat_id": 111, "display_name": "Asfi K"}).json()["ok"] is True
        assert client.post("/telegram/link/verify",
                           json={"code": code, "chat_id": 111}).json()["ok"] is False
        links = client.get("/profile").json()["telegram"]["links"]
        assert any(link["chat_id"] == 111 and link["authorized"] for link in links)

        # An unlinked chat is rejected.
        assert client.post("/telegram/message", json={"chat_id": 999, "text": "hi"}).json()["authorized"] is False

        # A linked chat with no project selected → sandboxed chat reply.
        reply = client.post("/telegram/message", json={"chat_id": 111, "text": "hi"}).json()
        assert reply["authorized"] is True and reply["reply"] == "Hello"

        # Select a project → full-access run; the session persists under the project.
        proj_dir = tmp_path / "myproj"
        proj_dir.mkdir()
        rec = projects.upsert(str(proj_dir), source="test")
        sel = client.post("/telegram/select", json={"chat_id": 111, "project_id": rec["id"]}).json()
        assert sel["name"] == "myproj"
        run = client.post("/telegram/message", json={"chat_id": 111, "text": "edit the readme"}).json()
        assert run["authorized"] is True and run["reply"] == "Hello"
        assert (proj_dir / ".pilot" / "sessions").is_dir()

    server.shutdown()
    thread.join(timeout=2)
