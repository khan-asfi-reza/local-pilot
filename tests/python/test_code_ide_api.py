"""The Code IDE surface: the endpoints the browser drives to open a project,
edit its files, run the agent over them, and resume a thread. Everything here is
full access to real files on the machine, so the failure cases matter as much as
the happy paths."""

import base64
import json
import os

import pytest
from fastapi.testclient import TestClient

from core import projects, sessions


@pytest.fixture
def api(clean_db):
    import main as main_mod

    projects.clear()
    return TestClient(main_mod.create_app(), client=("127.0.0.1", 51000))


@pytest.fixture
def harness_patch(monkeypatch):
    def patch(url):
        import services.agent_runner as agent_runner
        import services.harness_client as harness_client

        monkeypatch.setattr(agent_runner, "HARNESS_URL", url)
        monkeypatch.setattr(harness_client, "HARNESS_URL", url)

    return patch


def test_opening_a_folder_registers_it_once(api, project_dir):
    first = api.post("/code/projects", json={"path": project_dir})
    second = api.post("/code/projects", json={"path": project_dir})

    assert first.status_code == 200
    assert first.json()["project"]["id"] == second.json()["project"]["id"]
    assert len(api.get("/code/projects").json()["projects"]) == 1


def test_opening_a_folder_that_does_not_exist_is_rejected(api, tmp_path):
    resp = api.post("/code/projects", json={"path": str(tmp_path / "ghost")})
    assert resp.status_code == 400


def test_creating_a_new_project_makes_the_folder(api, tmp_path):
    resp = api.post("/code/projects/new", json={"name": "bookmarks", "location": str(tmp_path)})

    assert resp.status_code == 200
    assert os.path.isdir(tmp_path / "bookmarks")
    assert resp.json()["project"]["name"] == "bookmarks"


@pytest.mark.parametrize("name", ["../escape", "a/b", "..", "", "-rf"])
def test_a_new_project_name_cannot_be_a_path(api, tmp_path, name):
    resp = api.post("/code/projects/new", json={"name": name, "location": str(tmp_path)})
    assert resp.status_code == 400


def test_creating_a_project_over_a_non_empty_folder_is_refused(api, tmp_path):
    (tmp_path / "taken").mkdir()
    (tmp_path / "taken" / "file.txt").write_text("x")

    resp = api.post("/code/projects/new", json={"name": "taken", "location": str(tmp_path)})

    assert resp.status_code == 409


def test_forgetting_a_project_leaves_the_files_alone(api, project_dir):
    open(os.path.join(project_dir, "keep.py"), "w").write("print(1)\n")
    pid = api.post("/code/projects", json={"path": project_dir}).json()["project"]["id"]

    assert api.delete("/code/projects", params={"id": pid}).json()["removed"] == 1
    assert api.delete("/code/projects", params={"id": pid}).status_code == 404
    assert os.path.exists(os.path.join(project_dir, "keep.py"))


def test_a_source_document_is_kept_with_its_project(api, project_dir):
    api.post("/code/projects", json={"path": project_dir})
    payload = base64.b64encode(b"# Bookmarks PRD").decode()

    resp = api.post("/code/projects/doc", json={"root": project_dir, "filename": "prd.md",
                                                "content_b64": payload})

    assert resp.json()["path"] == ".pilot/prd.md"
    assert open(os.path.join(project_dir, ".pilot", "prd.md")).read() == "# Bookmarks PRD"


def test_a_document_cannot_be_dropped_into_an_unregistered_folder(api, tmp_path):
    resp = api.post("/code/projects/doc", json={"root": str(tmp_path), "filename": "x.md",
                                                "content_b64": base64.b64encode(b"x").decode()})
    assert resp.status_code == 403


def test_creating_renaming_and_deleting_entries(api, project_dir):
    assert api.post("/code/fs/entry", json={"root": project_dir, "path": "src", "type": "dir"}).status_code == 200
    assert api.post("/code/fs/entry", json={"root": project_dir, "path": "src/app.py"}).status_code == 200
    assert os.path.isfile(os.path.join(project_dir, "src", "app.py"))

    duplicate = api.post("/code/fs/entry", json={"root": project_dir, "path": "src/app.py"})
    assert duplicate.status_code == 409

    renamed = api.patch("/code/fs/entry", json={"root": project_dir, "path": "src/app.py",
                                                "new_path": "src/main.py"})
    assert renamed.status_code == 200
    assert os.path.isfile(os.path.join(project_dir, "src", "main.py"))

    assert api.request("DELETE", "/code/fs/entry",
                       params={"root": project_dir, "path": "src"}).status_code == 200
    assert not os.path.exists(os.path.join(project_dir, "src"))


def test_entry_operations_cannot_escape_the_project(api, project_dir, tmp_path):
    outside = tmp_path / "victim.txt"
    outside.write_text("keep me")

    assert api.post("/code/fs/entry", json={"root": project_dir, "path": "../evil.txt"}).status_code == 400
    assert api.request("DELETE", "/code/fs/entry",
                       params={"root": project_dir, "path": "../victim.txt"}).status_code == 400
    assert outside.exists()


def test_the_project_root_cannot_be_deleted(api, project_dir):
    resp = api.request("DELETE", "/code/fs/entry", params={"root": project_dir, "path": "."})
    assert resp.status_code == 400
    assert os.path.isdir(project_dir)


def test_browsing_lists_visible_directories_only(api, tmp_path):
    (tmp_path / "visible").mkdir()
    (tmp_path / ".hidden").mkdir()
    (tmp_path / "file.txt").write_text("x")

    resp = api.get("/code/browse", params={"path": str(tmp_path)})

    body = resp.json()
    assert [d["name"] for d in body["dirs"]] == ["visible"]
    assert body["parent"] == os.path.dirname(os.path.realpath(str(tmp_path)))
    assert api.get("/code/browse", params={"path": str(tmp_path / "nope")}).status_code == 400


def test_uploading_a_plain_document_returns_its_text(api):
    resp = api.post("/code/extract", files={"file": ("brief.md", b"# Brief\nBuild a bookmarks app.", "text/markdown")})

    body = resp.json()
    assert body["filename"] == "brief.md"
    assert "Build a bookmarks app." in body["text"]


def test_running_the_agent_streams_events_and_saves_the_thread(api, project_dir, fake_harness, harness_patch):
    events = [
        {"type": "text", "content": "Adding the endpoint."},
        {"type": "tool_result", "tool": "write_file", "info": "app.py"},
        {"type": "done"},
    ]
    with fake_harness(events) as harness:
        harness_patch(harness.url)
        resp = api.post("/code/agent", json={"root": project_dir, "mode": "auto",
                                             "messages": [{"role": "user", "content": "add an endpoint"}]})
        stream = resp.text
        sent = harness.requests[0]

    assert resp.status_code == 200
    types = [line[len("event: "):] for line in stream.splitlines() if line.startswith("event: ")]
    assert types == ["session", "text", "tool_result", "done"]
    assert sent["full_access"] is True

    payloads = [json.loads(line[len("data: "):]) for line in stream.splitlines() if line.startswith("data: ")]
    sid = payloads[0]["id"]
    listed = api.get("/code/sessions", params={"root": project_dir}).json()["sessions"]
    assert [s["id"] for s in listed] == [sid]


def test_the_agent_endpoint_validates_its_inputs(api, project_dir, tmp_path):
    bad_root = api.post("/code/agent", json={"root": str(tmp_path / "ghost"), "messages": []})
    assert bad_root.status_code == 400

    bad_session = api.post("/code/agent", json={"root": project_dir, "messages": [],
                                                "session_id": "../../escape"})
    assert bad_session.status_code == 400


def test_sessions_can_be_listed_read_and_deleted(api, project_dir):
    sid = sessions.new_id()
    sessions.save(project_dir, sid, [{"role": "user", "content": "hello"}])

    listed = api.get("/code/sessions", params={"root": project_dir}).json()["sessions"]
    assert [s["id"] for s in listed] == [sid]

    loaded = api.get("/code/session", params={"root": project_dir, "id": sid}).json()
    assert loaded["messages"][0]["content"] == "hello"

    assert api.get("/code/session", params={"root": project_dir, "id": "aaaaaaaaaaaa"}).status_code == 404
    assert api.request("DELETE", "/code/session", params={"root": project_dir, "id": sid}).json()["ok"] is True
    assert api.get("/code/sessions", params={"root": project_dir}).json()["sessions"] == []
