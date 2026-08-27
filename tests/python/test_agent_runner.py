"""The shared full-access run path. Web Code and Telegram both go through it, so
this is where session persistence, event mirroring and concurrency are decided."""

import asyncio

from core import sessions
from services import agent_runner, run_bus


def patch_harness(monkeypatch, url):
    import services.harness_client as harness_client

    monkeypatch.setattr(agent_runner, "HARNESS_URL", url)
    monkeypatch.setattr(harness_client, "HARNESS_URL", url)


async def collect(root, messages, **kwargs):
    return [event async for event in agent_runner.run_full_access(root, messages, **kwargs)]


def test_a_run_streams_events_and_persists_the_thread(project_dir, fake_harness, monkeypatch):
    events = [
        {"type": "text", "content": "Creating "},
        {"type": "tool_call", "tool": "write_file", "info": "app.py"},
        {"type": "tool_result", "tool": "write_file", "info": "12 bytes written"},
        {"type": "text", "content": "app.py."},
        {"type": "done"},
    ]
    with fake_harness(events) as harness:
        patch_harness(monkeypatch, harness.url)
        got = asyncio.run(collect(project_dir, [{"role": "user", "content": "create app.py"}],
                                  model="qwen3.5:4b", mode="auto"))

        sent = harness.requests[0]

    assert got[0]["type"] == "session", "the session id must arrive before anything else"
    sid = got[0]["id"]
    assert [e["type"] for e in got[1:]] == ["text", "tool_call", "tool_result", "text", "done"]

    assert sent["full_access"] is True, "a project run must be full access, not sandboxed"
    assert sent["working_directory"] == project_dir
    assert sent["mode"] == "auto"
    assert sent["model"] == "qwen3.5:4b"

    saved = sessions.load(project_dir, sid)
    assert saved is not None, "the conversation was not persisted for the other surfaces to resume"
    assert saved["messages"][0]["content"] == "create app.py"
    assert saved["messages"][-1] == {"role": "assistant", "content": "Creating app.py."}


def test_a_run_is_mirrored_to_watchers_of_the_same_project(project_dir, fake_harness, monkeypatch):
    """A Telegram run must show up live in the web Code IDE, tagged with its
    session so concurrent threads on one project stay separable."""
    async def scenario():
        watcher = run_bus.subscribe(project_dir)
        events = await collect(project_dir, [{"role": "user", "content": "hi"}])
        mirrored = []
        while not watcher.empty():
            mirrored.append(watcher.get_nowait())
        run_bus.unsubscribe(project_dir, watcher)
        return events, mirrored

    with fake_harness([{"type": "text", "content": "hello"}, {"type": "done"}]) as harness:
        patch_harness(monkeypatch, harness.url)
        events, mirrored = asyncio.run(scenario())

    sid = events[0]["id"]
    assert mirrored, "nothing was mirrored to the bus"
    assert all(m.get("sid") == sid for m in mirrored), "mirrored events must carry their session id"
    assert mirrored[-1]["type"] == "done", "watchers must be told the run ended"


def test_two_runs_on_one_project_stay_separate_threads(project_dir, fake_harness, monkeypatch):
    async def scenario():
        first, second = await asyncio.gather(
            collect(project_dir, [{"role": "user", "content": "first task"}]),
            collect(project_dir, [{"role": "user", "content": "second task"}]),
        )
        return first[0]["id"], second[0]["id"]

    with fake_harness([{"type": "text", "content": "ok"}, {"type": "done"}]) as harness:
        patch_harness(monkeypatch, harness.url)
        sid_a, sid_b = asyncio.run(scenario())

    assert sid_a != sid_b
    listed = {s["id"] for s in sessions.list_sessions(project_dir)}
    assert {sid_a, sid_b} <= listed, "concurrent runs must each persist their own thread"


def test_resuming_appends_to_the_same_thread(project_dir, fake_harness, monkeypatch):
    with fake_harness([{"type": "text", "content": "second answer"}, {"type": "done"}]) as harness:
        patch_harness(monkeypatch, harness.url)
        sid = sessions.new_id()
        sessions.save(project_dir, sid, [
            {"role": "user", "content": "first question"},
            {"role": "assistant", "content": "first answer"},
        ])
        history = sessions.load(project_dir, sid)["messages"]
        history.append({"role": "user", "content": "second question"})

        asyncio.run(collect(project_dir, history, session_id=sid))

    saved = sessions.load(project_dir, sid)
    assert [m["content"] for m in saved["messages"]] == [
        "first question", "first answer", "second question", "second answer",
    ]


def test_a_harness_outage_is_reported_not_swallowed(project_dir, monkeypatch):
    patch_harness(monkeypatch, "http://127.0.0.1:1/run")

    got = asyncio.run(collect(project_dir, [{"role": "user", "content": "hi"}]))

    assert got[0]["type"] == "session"
    assert got[-1]["type"] == "error", "an unreachable harness must surface an error event"
    assert got[-1]["message"]


def test_the_no_project_chat_path_stays_sandboxed(fake_harness, monkeypatch):
    with fake_harness([{"type": "text", "content": "A closure captures scope."}, {"type": "done"}]) as harness:
        patch_harness(monkeypatch, harness.url)
        reply = asyncio.run(agent_runner.run_sandboxed_chat([{"role": "user", "content": "what is a closure?"}]))
        sent = harness.requests[0]

    assert reply == "A closure captures scope."
    assert not sent.get("full_access"), "chat mode must never request full file access"
    assert sent.get("allowed_tools") == ["code_run", "web_search"]
