"""Telegram runs. Telegram cannot stream, so a run executes in the background and
the bot polls a snapshot every few seconds and edits one status message in place.
These tests drive that snapshot machine end to end."""

import asyncio

from services import tg_runs


def patch_harness(monkeypatch, url):
    import services.harness_client as harness_client
    import services.agent_runner as agent_runner

    monkeypatch.setattr(agent_runner, "HARNESS_URL", url)
    monkeypatch.setattr(harness_client, "HARNESS_URL", url)


async def wait_done(chat_id, tries=200):
    for _ in range(tries):
        snap = tg_runs.progress(chat_id)
        if snap["status"] == "done":
            return snap
        await asyncio.sleep(0.02)
    return tg_runs.progress(chat_id)


def test_an_idle_chat_has_no_run():
    assert tg_runs.progress(4242) == {"status": "idle", "pending": False}


def test_a_run_reports_progress_then_a_final_reply(project_dir, fake_harness, monkeypatch):
    events = [
        {"type": "text", "content": "[scaffolding backend/ - fastapi]\n"},
        {"type": "tool_call", "tool": "write_file", "info": "app/main.py"},
        {"type": "tool_result", "tool": "write_file", "info": "ok",
         "diff": {"path": "app/main.py", "added": 12, "removed": 0}},
        {"type": "text", "content": "Created the service.\n"},
        {"type": "done"},
    ]

    async def scenario():
        first = await tg_runs.start(101, project_dir, [{"role": "user", "content": "build it"}],
                                    model=None, mode="auto", sid=None)
        final = await wait_done(101)
        await tg_runs.cancel(101)
        return first, final

    with fake_harness(events) as harness:
        patch_harness(monkeypatch, harness.url)
        first, final = asyncio.run(scenario())

    assert first["status"] in {"running", "done"}
    assert first["session_id"], "the snapshot must expose the session id for the web IDE"

    assert final["status"] == "done"
    reply = final["reply"]
    assert "Created the service." in reply
    assert "scaffolding backend/ - fastapi" not in reply.split("steps:")[0], \
        "status markers belong in the step list, not in the prose"
    assert "steps:" in reply
    assert "app/main.py (+12 -0)" in reply, "a file edit should be reported as a step"


def test_the_reply_is_plain_ascii(project_dir, fake_harness, monkeypatch):
    """Telegram renders dashes and ellipses inconsistently, so the bridge
    normalises the model's punctuation."""
    events = [{"type": "text", "content": "Done — the API is live…"}, {"type": "done"}]

    async def scenario():
        await tg_runs.start(102, project_dir, [{"role": "user", "content": "go"}], None, "auto", None)
        snap = await wait_done(102)
        await tg_runs.cancel(102)
        return snap

    with fake_harness(events) as harness:
        patch_harness(monkeypatch, harness.url)
        snap = asyncio.run(scenario())

    assert "—" not in snap["reply"] and "…" not in snap["reply"]
    assert "Done - the API is live..." in snap["reply"]


def test_ask_mode_pauses_for_an_approval_button(project_dir, fake_harness, monkeypatch):
    events = [
        {"type": "text", "content": "About to edit\n"},
        {"type": "confirm", "id": "c1", "tool": "edit_file", "summary": "edit calc.py (+1 -1)",
         "diff": {"path": "calc.py", "added": 1, "removed": 1, "hunks": [
             {"lines": [{"op": "remove", "text": "return a - b"}, {"op": "add", "text": "return a + b"}]}]}},
    ]

    async def scenario():
        await tg_runs.start(103, project_dir, [{"role": "user", "content": "fix calc"}], None, "ask", None)
        for _ in range(200):
            snap = tg_runs.progress(103)
            if snap["pending"]:
                break
            await asyncio.sleep(0.02)
        resumed = await tg_runs.resume(103, "approve")
        await tg_runs.cancel(103)
        return snap, resumed

    with fake_harness(events, hold=True) as harness:
        patch_harness(monkeypatch, harness.url)
        paused, resumed = asyncio.run(scenario())
        delivered = list(harness.confirms)

    assert paused["status"] == "paused" and paused["pending"] is True
    confirm = paused["confirm"]
    assert "edit calc.py" in confirm["prompt"]
    assert "-return a - b" in confirm["diff"] and "+return a + b" in confirm["diff"]

    assert resumed["pending"] is False, "approving must clear the pending prompt"
    assert delivered and delivered[0]["id"] == "c1" and delivered[0]["decision"] == "approve", \
        "the decision must reach the harness that is blocked on it"


def test_a_new_message_supersedes_the_previous_run(project_dir, fake_harness, monkeypatch):
    async def scenario():
        await tg_runs.start(104, project_dir, [{"role": "user", "content": "first"}], None, "auto", None)
        first_sid = tg_runs.progress(104)["session_id"]
        await tg_runs.start(104, project_dir, [{"role": "user", "content": "second"}], None, "auto", None)
        second = tg_runs.progress(104)
        await tg_runs.cancel(104)
        return first_sid, second

    with fake_harness([{"type": "text", "content": "ok"}, {"type": "done"}]) as harness:
        patch_harness(monkeypatch, harness.url)
        first_sid, second = asyncio.run(scenario())

    assert second["session_id"] != first_sid, "one in-flight run per chat"
    assert tg_runs.progress(104) == {"status": "idle", "pending": False}


def test_resuming_an_idle_chat_is_harmless():
    assert asyncio.run(tg_runs.resume(999, "approve")) == {"status": "idle", "pending": False}


def test_activity_is_capped_so_a_status_message_stays_small(project_dir, fake_harness, monkeypatch):
    events = [{"type": "tool_result", "tool": "read_file", "info": f"file{i}.py"} for i in range(40)]
    events.append({"type": "done"})

    async def scenario():
        await tg_runs.start(105, project_dir, [{"role": "user", "content": "look"}], None, "auto", None)
        snap = await wait_done(105)
        await tg_runs.cancel(105)
        return snap

    with fake_harness(events) as harness:
        patch_harness(monkeypatch, harness.url)
        snap = asyncio.run(scenario())

    assert len(snap["activity"]) <= tg_runs.MAX_ACTIVITY
