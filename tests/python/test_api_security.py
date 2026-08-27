"""The trust boundary. There are no accounts and no API keys: the whole security
model is that only a process on this machine may drive a full-access agent over
the user's real files. These tests pin that boundary down."""

import os

import pytest
from fastapi.testclient import TestClient


@pytest.fixture(scope="module")
def app():
    import main as main_mod

    return main_mod.create_app()


def local(app):
    return TestClient(app, client=("127.0.0.1", 51000))


def remote(app):
    return TestClient(app, client=("192.168.10.42", 51000))


GUARDED = ["/code/tree?root=/tmp", "/profile", "/telegram/config", "/telegram/projects"]


@pytest.mark.parametrize("path", GUARDED)
def test_agent_routes_are_refused_from_the_network(app, path):
    resp = remote(app).get(path)
    assert resp.status_code == 403, f"{path} was reachable from the LAN"
    assert resp.json()["detail"] == "localhost only"


def test_agent_routes_reject_a_cross_site_page(app, clean_db):
    """A malicious page in the user's browser is still on localhost, so the peer
    address alone is not enough. Sec-Fetch-Site is set by the browser and cannot
    be forged by the page."""
    client = local(app)

    assert client.get("/profile").status_code == 200
    assert client.get("/profile", headers={"sec-fetch-site": "same-site"}).status_code == 200

    blocked = client.get("/profile", headers={"sec-fetch-site": "cross-site"})
    assert blocked.status_code == 403
    assert blocked.json()["detail"] == "cross-site request rejected"


def test_the_chat_api_is_not_behind_the_local_only_guard(app, clean_db):
    """The sandboxed chat API has no file access, so it stays usable from another
    device on the LAN. Only the full-access surfaces are locked down."""
    assert remote(app).get("/health").status_code == 200


def test_reading_a_file_cannot_escape_the_project(app, project_dir, tmp_path):
    secret = tmp_path / "secret.txt"
    secret.write_text("private")
    client = local(app)

    for path in ["../secret.txt", "../../etc/passwd", "sub/../../secret.txt"]:
        resp = client.get("/code/file", params={"root": project_dir, "path": path})
        assert resp.status_code == 400, f"{path} was served"
        assert resp.json()["detail"] == "path escapes project root"


def test_writing_a_file_cannot_escape_the_project(app, project_dir, tmp_path):
    target = tmp_path / "outside.txt"
    client = local(app)

    resp = client.put("/code/file", json={"root": project_dir, "path": "../outside.txt", "content": "pwned"})

    assert resp.status_code == 400
    assert not target.exists()


def test_a_file_inside_the_project_round_trips(app, project_dir):
    client = local(app)

    assert client.put("/code/file", json={"root": project_dir, "path": "src/app.py",
                                          "content": "print(1)\n"}).status_code == 200
    resp = client.get("/code/file", params={"root": project_dir, "path": "src/app.py"})

    assert resp.status_code == 200
    assert resp.json()["content"] == "print(1)\n"


def test_a_huge_file_is_refused_rather_than_streamed_into_the_editor(app, project_dir):
    big = os.path.join(project_dir, "big.log")
    with open(big, "w") as f:
        f.write("x" * (300 * 1024))

    resp = local(app).get("/code/file", params={"root": project_dir, "path": "big.log"})

    assert resp.status_code == 413


def test_the_file_tree_hides_vendored_and_generated_directories(app, project_dir):
    for rel in ["src/app.py", "node_modules/left-pad/index.js", ".git/config", "__pycache__/app.pyc"]:
        full = os.path.join(project_dir, rel)
        os.makedirs(os.path.dirname(full), exist_ok=True)
        open(full, "w").write("x")

    resp = local(app).get("/code/tree", params={"root": project_dir})

    assert resp.status_code == 200
    names = {node["name"] for node in resp.json()["tree"]}
    assert "src" in names
    assert not names & {"node_modules", ".git", "__pycache__"}


def test_the_tree_endpoint_rejects_a_path_that_is_not_a_directory(app, project_dir):
    resp = local(app).get("/code/tree", params={"root": os.path.join(project_dir, "nope")})
    assert resp.status_code == 400
