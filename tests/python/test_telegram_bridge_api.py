"""The Telegram bridge as the bot sees it: HTTP endpoints on the backend. The bot
is deliberately thin, so every authorisation and routing decision tested here is
the one that actually protects the machine."""

import pytest
from fastapi.testclient import TestClient

from core import profile, projects, sessions


@pytest.fixture
def client(clean_db):
    import main as main_mod
    import services.agent_runner as agent_runner
    import services.harness_client as harness_client

    return TestClient(main_mod.create_app(), client=("127.0.0.1", 51000)), agent_runner, harness_client


def link_chat(chat_id: int) -> None:
    profile.save_telegram_settings(bot_token="t", enabled=True, bot_username="shamsu_bot")
    code = profile.new_link_code()["code"]
    assert profile.verify_link_code(code, chat_id=chat_id, display_name="Khan")


def test_an_unlinked_chat_is_told_how_to_link_and_runs_nothing(client):
    api, _, _ = client

    resp = api.post("/telegram/message", json={"chat_id": 1, "text": "delete everything"})

    body = resp.json()
    assert resp.status_code == 200
    assert body["authorized"] is False
    assert body["status"] == "done"
    assert "/link" in body["reply"], "the refusal should tell the user how to pair"


def test_linking_through_the_bot_authorises_the_chat(client):
    api, _, _ = client
    profile.save_telegram_settings(bot_token="t", enabled=True, bot_username="shamsu_bot")
    code = profile.new_link_code()["code"]

    ok = api.post("/telegram/link/verify", json={"chat_id": 7, "code": code, "display_name": "Khan"})
    assert ok.json()["ok"] is True

    replay = api.post("/telegram/link/verify", json={"chat_id": 8, "code": code})
    assert replay.json()["ok"] is False, "a link code must be single use"

    status = api.get("/telegram/status", params={"chat_id": 7}).json()
    assert status["authorized"] is True
    assert status["project"] is None and status["mode"] == "auto"


def test_selecting_a_project_opens_a_telegram_tagged_thread(client, project_dir):
    api, _, _ = client
    link_chat(11)
    projects.clear()
    rec = projects.upsert(project_dir, source="web")

    resp = api.post("/telegram/select", json={"chat_id": 11, "project_id": rec["id"]})

    assert resp.json()["path"] == rec["path"]
    listed = sessions.list_sessions(project_dir)
    assert len(listed) == 1
    assert listed[0]["source"] == "telegram", "the web Code IDE must see where the thread came from"
    assert "Telegram" in listed[0]["title"]

    status = api.get("/telegram/status", params={"chat_id": 11}).json()
    assert status["project"] == rec["name"]


def test_selecting_an_unknown_project_is_rejected(client):
    api, _, _ = client
    link_chat(12)
    assert api.post("/telegram/select", json={"chat_id": 12, "project_id": "ghost"}).status_code == 404


def test_mode_is_validated_and_stored_per_chat(client):
    api, _, _ = client
    link_chat(13)

    assert api.post("/telegram/mode", json={"chat_id": 13, "mode": "plan"}).json()["mode"] == "plan"
    assert api.get("/telegram/status", params={"chat_id": 13}).json()["mode"] == "plan"
    assert api.post("/telegram/mode", json={"chat_id": 13, "mode": "yolo"}).status_code == 400


def test_a_message_with_no_project_takes_the_sandboxed_chat_path(client, fake_harness, monkeypatch):
    api, agent_runner, harness_client = client
    link_chat(14)

    with fake_harness([{"type": "text", "content": "A closure captures scope."}, {"type": "done"}]) as harness:
        monkeypatch.setattr(agent_runner, "HARNESS_URL", harness.url)
        monkeypatch.setattr(harness_client, "HARNESS_URL", harness.url)
        body = api.post("/telegram/message", json={"chat_id": 14, "text": "what is a closure?"}).json()
        sent = harness.requests[0]

    assert body["authorized"] is True
    assert body["reply"] == "A closure captures scope."
    assert not sent.get("full_access"), "with no project selected the bot must not get file access"


def test_a_message_with_a_project_drives_the_full_access_agent(client, project_dir, fake_harness, monkeypatch):
    api, agent_runner, harness_client = client
    link_chat(15)
    projects.clear()
    rec = projects.upsert(project_dir)
    api.post("/telegram/select", json={"chat_id": 15, "project_id": rec["id"]})

    events = [{"type": "text", "content": "Added the endpoint.\n"}, {"type": "done"}]
    with fake_harness(events) as harness:
        monkeypatch.setattr(agent_runner, "HARNESS_URL", harness.url)
        monkeypatch.setattr(harness_client, "HARNESS_URL", harness.url)
        body = api.post("/telegram/message", json={"chat_id": 15, "text": "add a health endpoint"}).json()
        for _ in range(200):
            snap = api.get("/telegram/progress", params={"chat_id": 15}).json()
            if snap["status"] == "done":
                break
        sent = harness.requests[0]

    assert body["authorized"] is True
    assert sent["full_access"] is True
    assert sent["working_directory"] == rec["path"]
    assert snap["reply"].startswith("Added the endpoint.")


def test_clearing_a_chat_starts_a_new_thread(client, project_dir, fake_harness, monkeypatch):
    api, agent_runner, harness_client = client
    link_chat(16)
    projects.clear()
    rec = projects.upsert(project_dir)
    api.post("/telegram/select", json={"chat_id": 16, "project_id": rec["id"]})
    before = profile.get_link(16).active_session_id

    api.post("/telegram/clear", json={"chat_id": 16})

    after = profile.get_link(16).active_session_id
    assert after != before
    sources = {s["source"] for s in sessions.list_sessions(project_dir)}
    assert sources == {"telegram"}
    assert len(sessions.list_sessions(project_dir)) == 2


def test_a_bad_confirm_decision_is_rejected(client):
    api, _, _ = client
    link_chat(17)
    assert api.post("/telegram/confirm", json={"chat_id": 17, "decision": "maybe"}).status_code == 400


def test_the_bot_reads_its_token_from_the_backend(client):
    api, _, _ = client
    profile.save_telegram_settings(bot_token="123:abc", enabled=True)

    cfg = api.get("/telegram/config").json()

    assert cfg == {"bot_token": "123:abc", "enabled": True}
