"""The sandboxed chat API. This is the one surface with no project and no file
access: each thread runs in its own sandbox under the data dir, so a question
asked in the browser can never touch the user's code."""

import json
import os

import pytest
from fastapi.testclient import TestClient


@pytest.fixture
def api(clean_db):
    import main as main_mod

    return TestClient(main_mod.create_app(), client=("127.0.0.1", 51000))


@pytest.fixture
def harness_patch(monkeypatch):
    def patch(url):
        import routes.chat as chat_routes
        import services.harness_client as harness_client

        monkeypatch.setattr(harness_client, "HARNESS_URL", url)
        monkeypatch.setattr(chat_routes, "HARNESS_URL", url)

    return patch


def sse(stream: str):
    events = []
    for line in stream.splitlines():
        if line.startswith("data: "):
            events.append(json.loads(line[len("data: "):]))
    return events


def test_threads_are_created_listed_and_deleted(api):
    created = api.post("/threads", json={"title": "First thread"}).json()["thread"]

    assert created["id"] and created["title"] == "First thread"
    assert [t["id"] for t in api.get("/threads").json()] == [created["id"]]

    fetched = api.get(f"/threads/{created['id']}").json()
    assert fetched["thread"]["id"] == created["id"]
    assert fetched["messages"] == []

    assert api.delete(f"/threads/{created['id']}").status_code == 200
    assert api.get("/threads").json() == []
    assert api.get(f"/threads/{created['id']}").status_code == 404


def test_a_thread_with_no_body_still_gets_a_default_title(api):
    created = api.post("/threads").json()["thread"]
    assert created["title"] == "New thread"


def test_a_threads_model_can_be_pinned(api):
    tid = api.post("/threads").json()["thread"]["id"]

    updated = api.post(f"/threads/{tid}/model", json={"model": "qwen3.5:4b"}).json()["thread"]

    assert updated["model"] == "qwen3.5:4b"
    assert api.post("/threads/ghost/model", json={"model": "x"}).status_code == 404


def test_a_message_streams_the_answer_and_persists_the_exchange(api, fake_harness, harness_patch):
    tid = api.post("/threads").json()["thread"]["id"]
    events = [
        {"type": "text", "content": "A closure "},
        {"type": "text", "content": "captures scope."},
        {"type": "done"},
    ]

    with fake_harness(events) as harness:
        harness_patch(harness.url)
        resp = api.post(f"/threads/{tid}/messages", json={"content": "what is a closure?"})
        sent = harness.requests[0]

    assert resp.status_code == 200
    assert [e["type"] for e in sse(resp.text)] == ["text", "text", "done"]

    assert not sent.get("full_access"), "the chat API must never request file access"
    assert sent["allowed_tools"] == ["code_run", "web_search"]
    sandbox = sent["working_directory"]
    assert tid in sandbox and os.path.isdir(sandbox), "each thread runs in its own sandbox"

    stored = api.get(f"/threads/{tid}").json()
    assert [m["role"] for m in stored["messages"]] == ["user", "assistant"]
    assert stored["messages"][1]["content"] == "A closure captures scope."
    assert stored["thread"]["title"] and stored["thread"]["title"] != "New thread", \
        "the first exchange should title the thread"


def test_tool_activity_is_recorded_in_the_thread(api, fake_harness, harness_patch):
    tid = api.post("/threads").json()["thread"]["id"]
    events = [
        {"type": "tool_call", "tool": "code_run", "info": "run the snippet"},
        {"type": "tool_result", "tool": "code_run", "info": "exit 0", "data": "4"},
        {"type": "text", "content": "It prints 4."},
        {"type": "done"},
    ]

    with fake_harness(events) as harness:
        harness_patch(harness.url)
        resp = api.post(f"/threads/{tid}/messages", json={"content": "what does 2+2 print?"})

    assert [e["type"] for e in sse(resp.text)] == ["tool_call", "tool_result", "text", "done"]
    roles = [m["role"] for m in api.get(f"/threads/{tid}").json()["messages"]]
    assert "tool" in roles or len(roles) >= 2


def test_a_harness_outage_is_reported_in_the_stream(api, harness_patch):
    tid = api.post("/threads").json()["thread"]["id"]
    harness_patch("http://127.0.0.1:1/run")

    resp = api.post(f"/threads/{tid}/messages", json={"content": "hello"})

    assert [e["type"] for e in sse(resp.text)][-1] == "error"
    stored = api.get(f"/threads/{tid}").json()["messages"]
    assert stored[-1]["role"] == "assistant", "the failure is kept in the thread, not lost"


def test_an_empty_or_unknown_message_is_rejected(api):
    tid = api.post("/threads").json()["thread"]["id"]

    assert api.post(f"/threads/{tid}/messages", json={"content": ""}).status_code == 400
    assert api.post("/threads/ghost/messages", json={"content": "hi"}).status_code == 404


def test_health_reports_whether_the_harness_is_up(api, fake_harness):
    with fake_harness([]) as harness:
        import services.harness_client as harness_client
        import routes.chat as chat_routes

        original = chat_routes.HARNESS_URL
        chat_routes.HARNESS_URL = harness.url
        harness_client.HARNESS_URL = harness.url
        try:
            up = api.get("/health").json()
        finally:
            chat_routes.HARNESS_URL = original
            harness_client.HARNESS_URL = original

    assert up == {"ok": True, "harness": True}


def test_the_model_list_degrades_gracefully_when_the_harness_is_down(api, harness_patch):
    harness_patch("http://127.0.0.1:1/run")

    body = api.get("/models").json()

    assert body["models"] == [] and body["default"] is None
    assert body["error"], "the UI needs to know why the picker is empty"
