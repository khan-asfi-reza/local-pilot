"""The unified project registry (projects.json in the shared data dir). The Go
harness and this backend both own this file, so its shape is a cross-language
contract."""

import json
import os

from core import projects


def test_same_path_registers_once(project_dir):
    first = projects.upsert(project_dir, source="web")
    second = projects.upsert(project_dir, source="telegram")

    assert first["id"] == second["id"], "one folder must map to one stable id"
    assert len(projects.list_projects()) == 1
    assert projects.get(first["id"])["path"] == os.path.realpath(project_dir)


def test_record_matches_the_go_schema(project_dir):
    rec = projects.upsert(project_dir)

    with open(projects.store_path()) as f:
        stored = json.load(f)

    assert isinstance(stored, list)
    assert set(rec) >= {"id", "path", "name", "source", "created_at", "last_opened"}
    assert os.path.isabs(rec["path"]), "the terminal resolves projects by absolute path"
    assert rec["name"] == os.path.basename(os.path.realpath(project_dir))


def test_listing_is_most_recently_opened_first(tmp_path):
    projects.clear()
    a = tmp_path / "alpha"
    b = tmp_path / "beta"
    a.mkdir()
    b.mkdir()

    projects.upsert(str(a))
    projects.upsert(str(b))
    projects.upsert(str(a))  # re-open alpha, which should float to the top

    listed = projects.list_projects()
    assert [p["name"] for p in listed] == ["alpha", "beta"]


def test_remove_and_clear_only_touch_the_registry(tmp_path):
    projects.clear()
    folder = tmp_path / "gamma"
    folder.mkdir()
    (folder / "main.py").write_text("print(1)\n")
    rec = projects.upsert(str(folder))

    projects.remove(rec["id"])
    assert projects.get(rec["id"]) is None
    assert (folder / "main.py").exists(), "removing a project must never delete files"

    projects.upsert(str(folder))
    assert projects.clear() == 1
    assert projects.list_projects() == []
    assert (folder / "main.py").exists()


def test_unknown_id_is_none():
    assert projects.get("not-a-real-id") is None
