import json
import os
import sys
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

from fastapi.testclient import TestClient

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))


def make_fake_harness_server():
    class Handler(BaseHTTPRequestHandler):
        def do_POST(self):
            length = int(self.headers.get("Content-Length", "0"))
            body = self.rfile.read(length).decode()
            payload = json.loads(body)
            assert payload["allowed_tools"] == ["code_run", "web_search"]
            self.send_response(200)
            self.send_header("Content-Type", "application/x-ndjson")
            self.end_headers()
            for event in [
                {"type": "text", "content": "Hello"},
                {"type": "tool_call", "tool": "code_run", "info": "run tests"},
                {"type": "tool_result", "tool": "code_run", "info": "done", "data": "ok"},
                {"type": "done"},
            ]:
                self.wfile.write((json.dumps(event) + "\n").encode())
                self.wfile.flush()

        def log_message(self, *args, **kwargs):
            return

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    return server, thread


def test_turn_flow_persists_messages_and_streams_events(tmp_path):
    server, thread = make_fake_harness_server()
    harness_url = f"http://127.0.0.1:{server.server_address[1]}/run"
    db_path = tmp_path / "local-pilot-test.db"
    os.environ["HARNESS_URL"] = harness_url
    os.environ["DATABASE_URL"] = f"sqlite:///{db_path}"
    os.environ["PORT"] = "6000"

    from main import create_app

    app = create_app()
    with TestClient(app) as client:
        response = client.post("/threads", json={})
        assert response.status_code == 200
        thread_data = response.json()["thread"]
        thread_id = thread_data["id"]

        with client.stream("POST", f"/threads/{thread_id}/messages", json={"content": "hello"}) as stream:
            body = "".join(stream.iter_text())

        assert "event: text" in body
        assert "Hello" in body
        assert "event: done" in body

        persisted = client.get(f"/threads/{thread_id}")
        payload = persisted.json()
        assert payload["thread"]["id"] == thread_id
        assert len(payload["messages"]) >= 2
        assert any(message["role"] == "user" and message["content"] == "hello" for message in payload["messages"])
        assert any(message["role"] == "assistant" and message["content"] == "Hello" for message in payload["messages"])

    server.shutdown()
    thread.join(timeout=2)
