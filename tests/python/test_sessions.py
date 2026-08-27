"""Conversation sessions: the JSON files the terminal, the web Code IDE and the
Telegram bridge all read and write, so a thread started in one shows up in the
others."""

import json
import os

import pytest

from core import sessions


def test_new_ids_are_unique_and_valid():
    ids = {sessions.new_id() for _ in range(200)}
    assert len(ids) == 200
    assert all(sessions.is_valid_id(sid) for sid in ids)


@pytest.mark.parametrize("bad", [
    "../../../etc/passwd",
    "..",
    "a/b",
    "ABC123",          # uppercase is outside the terminal's id range
    "short",           # under six characters
    "",
    "x" * 65,
    None,
    12345,
])
def test_invalid_ids_are_rejected(bad):
    assert not sessions.is_valid_id(bad)


def test_traversal_ids_cannot_escape_the_sessions_directory(project_dir, tmp_path):
    """A session id is used to build a file path, so it is a path-traversal sink."""
    outside = tmp_path / "stolen.json"
    outside.write_text("{}")

    assert sessions.load(project_dir, "../../stolen") is None
    assert sessions.delete(project_dir, "../../stolen") is False
    assert outside.exists(), "delete() escaped the sessions directory"

    with pytest.raises(ValueError):
        sessions.save(project_dir, "../../stolen", [{"role": "user", "content": "hi"}])


def test_save_and_load_round_trip(project_dir):
    sid = sessions.new_id()
    messages = [
        {"role": "user", "content": "add a health endpoint"},
        {"role": "assistant", "content": "done"},
    ]

    saved = sessions.save(project_dir, sid, messages, model="qwen3.5:4b", mode="auto")

    assert saved["id"] == sid
    assert saved["model"] == "qwen3.5:4b"
    assert saved["mode"] == "auto"
    assert saved["title"] == "add a health endpoint", "the title comes from the first user message"
    assert saved["created_at"] and saved["updated_at"]

    loaded = sessions.load(project_dir, sid)
    assert loaded == saved

    on_disk = os.path.join(project_dir, ".pilot", "sessions", f"{sid}.json")
    assert os.path.isfile(on_disk), "sessions must live where the terminal looks for them"
    assert json.loads(open(on_disk).read())["messages"] == messages


def test_resaving_preserves_creation_time_and_origin(project_dir):
    sid = sessions.new_id()
    sessions.ensure_stub(project_dir, sid, source="telegram", title="from a phone")
    first = sessions.load(project_dir, sid)

    sessions.save(project_dir, sid, [{"role": "user", "content": "hello"}], mode="ask")
    second = sessions.load(project_dir, sid)

    assert second["created_at"] == first["created_at"]
    assert second["source"] == "telegram", "a Telegram thread must not be relabelled as web"
    assert second["title"] == "from a phone", "an existing title is kept"


def test_ensure_stub_is_idempotent(project_dir):
    sid = sessions.new_id()
    first = sessions.ensure_stub(project_dir, sid, source="telegram")
    sessions.save(project_dir, sid, [{"role": "user", "content": "hi"}])
    again = sessions.ensure_stub(project_dir, sid, source="web")

    assert again["source"] == "telegram"
    assert again["messages"], "ensure_stub must not wipe an existing conversation"
    assert again["created_at"] == first["created_at"]


def test_listing_is_newest_first_and_survives_a_corrupt_file(project_dir):
    older = sessions.new_id()
    newer = sessions.new_id()
    sessions.save(project_dir, older, [{"role": "user", "content": "first"}])
    saved = sessions.load(project_dir, older)
    saved["updated_at"] = "2020-01-01T00:00:00Z"
    path = os.path.join(project_dir, ".pilot", "sessions", f"{older}.json")
    with open(path, "w") as f:
        json.dump(saved, f)
    sessions.save(project_dir, newer, [{"role": "user", "content": "second"}])

    # A half-written or hand-edited file must not take the whole list down.
    with open(os.path.join(project_dir, ".pilot", "sessions", "deadbeefcafe.json"), "w") as f:
        f.write("{ not json")

    listed = sessions.list_sessions(project_dir)

    assert [s["id"] for s in listed] == [newer, older]
    assert listed[0]["title"] == "second"


def test_listing_an_unknown_project_is_empty(tmp_path):
    assert sessions.list_sessions(str(tmp_path / "nope")) == []


def test_delete_removes_only_the_named_session(project_dir):
    keep, drop = sessions.new_id(), sessions.new_id()
    sessions.save(project_dir, keep, [{"role": "user", "content": "keep"}])
    sessions.save(project_dir, drop, [{"role": "user", "content": "drop"}])

    assert sessions.delete(project_dir, drop) is True
    assert sessions.delete(project_dir, drop) is False, "deleting twice should report failure"
    assert [s["id"] for s in sessions.list_sessions(project_dir)] == [keep]
