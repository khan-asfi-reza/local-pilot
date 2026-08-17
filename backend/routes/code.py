import asyncio
import base64
import html
import io
import json
import os
import re
import shutil
import subprocess
import tempfile
import zipfile
from typing import Any, AsyncIterator
from urllib.parse import urlparse

import httpx
from fastapi import APIRouter, Depends, File, HTTPException, Request, UploadFile, WebSocket, WebSocketDisconnect
from fastapi.responses import StreamingResponse
from starlette.requests import HTTPConnection

from core.database import init_db
from core import projects, sessions
from services import run_bus, terminals
from services.agent_runner import run_full_access
from services.harness_client import HARNESS_URL

router = APIRouter(prefix="/code", dependencies=[])

SKIP_DIRS = {".git", "node_modules", "dist", ".venv", "__pycache__"}

MAX_DOC_BYTES = 200 * 1024  # cap on the text returned by /extract

MAX_RAW_DOC_BYTES = 25 * 1024 * 1024  # cap on a source doc saved into .pilot/

LOCAL_HOSTS = {"127.0.0.1", "::1", "localhost"}

_XML_TAG_RE = re.compile(r"<[^>]+>")


def _strip_xml(s: str) -> str:
    return html.unescape(_XML_TAG_RE.sub("", s))


def _zip_entry(z: zipfile.ZipFile, name: str) -> str:
    with z.open(name) as f:
        return f.read().decode("utf-8", errors="replace")


def _extract_docx(data: bytes) -> str:
    # OOXML: paragraphs/table rows close with </w:p>/</w:tr>; tabs are <w:tab/>.
    with zipfile.ZipFile(io.BytesIO(data)) as z:
        raw = _zip_entry(z, "word/document.xml")
    raw = raw.replace("<w:tab/>", "\t").replace("</w:p>", "\n").replace("</w:tr>", "\n")
    return _strip_xml(raw).strip()


def _extract_pptx(data: bytes) -> str:
    out = []
    with zipfile.ZipFile(io.BytesIO(data)) as z:
        names = sorted(
            n for n in z.namelist() if n.startswith("ppt/slides/slide") and n.endswith(".xml")
        )
        for n in names:
            raw = _zip_entry(z, n).replace("</a:p>", "\n")
            out.append(_strip_xml(raw).strip())
    return "\n\n".join(t for t in out if t).strip()


def _extract_xlsx(data: bytes) -> str:
    with zipfile.ZipFile(io.BytesIO(data)) as z:
        if "xl/sharedStrings.xml" not in z.namelist():
            return ""
        raw = _zip_entry(z, "xl/sharedStrings.xml")
    return _strip_xml(raw.replace("</si>", "\n")).strip()


def _extract_pdf(data: bytes) -> str:
    with tempfile.NamedTemporaryFile(suffix=".pdf") as tmp:
        tmp.write(data)
        tmp.flush()
        if shutil.which("pdftotext"):
            try:
                out = subprocess.run(
                    ["pdftotext", "-layout", tmp.name, "-"], capture_output=True, timeout=30
                )
                if out.returncode == 0:
                    return out.stdout.decode("utf-8", errors="replace").strip()
            except (OSError, subprocess.SubprocessError):
                pass
        try:
            from pdfminer.high_level import extract_text  # optional dependency

            return (extract_text(tmp.name) or "").strip()
        except Exception:
            pass
    return "[Could not extract PDF text: install poppler (brew install poppler) for the pdftotext command, or pip install pdfminer.six]"


def _extract_text(filename: str, data: bytes) -> str:
    ext = os.path.splitext(filename)[1].lower()
    try:
        if ext == ".docx":
            return _extract_docx(data)
        if ext == ".pptx":
            return _extract_pptx(data)
        if ext == ".xlsx":
            return _extract_xlsx(data)
        if ext == ".pdf":
            return _extract_pdf(data)
    except (zipfile.BadZipFile, KeyError) as exc:
        return f"[Could not extract {filename}: {exc}]"
    return data.decode("utf-8", errors="replace")


def local_only(conn: HTTPConnection) -> None:
    # HTTPConnection, not Request: the same guard then covers the terminal websocket,
    # which does not pass through the app's http middleware.
    host = conn.client.host if conn.client else ""
    if host not in LOCAL_HOSTS:
        raise HTTPException(status_code=403, detail="localhost only")


router = APIRouter(prefix="/code", dependencies=[Depends(local_only)])


def safe_join(root: str, rel: str) -> str:
    root_real = os.path.realpath(root)
    target = os.path.realpath(os.path.join(root_real, rel))
    if target != root_real and not target.startswith(root_real + os.sep):
        raise HTTPException(status_code=400, detail="path escapes project root")
    return target


@router.on_event("startup")
def startup_event() -> None:
    init_db()


@router.get("/browse")
def browse(path: str = "") -> dict[str, Any]:
    target = os.path.realpath(os.path.expanduser(path)) if path else os.path.expanduser("~")
    if not os.path.isdir(target):
        raise HTTPException(status_code=400, detail="not a directory")
    parent = os.path.dirname(target)
    if parent == target:
        parent = None
    dirs = []
    try:
        for entry in sorted(os.listdir(target)):
            if entry.startswith("."):
                continue
            full = os.path.join(target, entry)
            if os.path.isdir(full):
                dirs.append({"name": entry, "path": full})
    except PermissionError:
        raise HTTPException(status_code=403, detail="permission denied")
    return {"path": target, "parent": parent, "dirs": dirs}


@router.get("/projects")
def list_projects() -> dict[str, Any]:
    # Unified registry (shared with the terminal + Telegram).
    return {"projects": projects.list_projects()}


@router.post("/projects", status_code=200)
async def create_project(request: Request) -> dict[str, Any]:
    body = await request.json()
    path = body.get("path", "")
    resolved = os.path.realpath(path)
    if not os.path.isdir(resolved):
        raise HTTPException(status_code=400, detail="directory does not exist")
    return {"project": projects.upsert(resolved, source="web")}


@router.delete("/projects")
def clear_projects(id: str | None = None) -> dict[str, Any]:
    """Forget one project (?id=) or the whole recent list. Registry-only: the
    folder and its files stay on disk."""
    if id:
        if not projects.get(id):
            raise HTTPException(status_code=404, detail="unknown project")
        projects.remove(id)
        return {"ok": True, "removed": 1}
    return {"ok": True, "removed": projects.clear()}


@router.post("/projects/new", status_code=200)
async def new_project(request: Request) -> dict[str, Any]:
    """Create <location>/<name> on disk and register it. The brief/PRD is NOT
    written to the project as a file — it rides along to the agent as attached
    context (and the raw source doc is kept under .pilot/ via /projects/doc)."""
    body = await request.json()
    name = (body.get("name") or "").strip()
    location = os.path.realpath(os.path.expanduser(body.get("location") or ""))
    if not name or name in {".", ".."} or "/" in name or "\\" in name or name.startswith("-"):
        raise HTTPException(status_code=400, detail="invalid project name")
    if not os.path.isdir(location):
        raise HTTPException(status_code=400, detail="location does not exist")
    target = os.path.join(location, name)
    if os.path.isfile(target):
        raise HTTPException(status_code=409, detail="a file with that name exists")
    if os.path.isdir(target) and os.listdir(target):
        raise HTTPException(status_code=409, detail="directory exists and is not empty")
    try:
        os.makedirs(target, exist_ok=True)
    except OSError as exc:
        raise HTTPException(status_code=400, detail=str(exc))
    return {"project": projects.upsert(target, name=name, source="web")}


@router.post("/projects/doc", status_code=200)
async def save_project_doc(request: Request) -> dict[str, Any]:
    """Keep an uploaded source document with the project by writing it to <root>/.pilot/."""
    body = await request.json()
    root = os.path.realpath(body.get("root") or "")
    filename = os.path.basename(body.get("filename") or "")
    if not os.path.isdir(root):
        raise HTTPException(status_code=400, detail="not a directory")
    # Only write inside a directory the app already tracks as a project, so a local
    # caller cannot drop files into an arbitrary .pilot/ anywhere on disk.
    if not any(os.path.realpath(p.get("path", "")) == root for p in projects.list_projects()):
        raise HTTPException(status_code=403, detail="root is not a registered project")
    # basename already strips separators; also require a safe, non-dotfile name so
    # the write target is fully predictable.
    if not re.match(r"^[A-Za-z0-9][A-Za-z0-9._-]*$", filename):
        raise HTTPException(status_code=400, detail="invalid filename")
    try:
        data = base64.b64decode(body.get("content_b64") or "", validate=True)
    except (ValueError, TypeError):
        raise HTTPException(status_code=400, detail="invalid base64 content")
    if len(data) > MAX_RAW_DOC_BYTES:
        raise HTTPException(status_code=413, detail="document too large")
    pilot = os.path.join(root, ".pilot")
    os.makedirs(pilot, exist_ok=True)
    with open(os.path.join(pilot, filename), "wb") as f:
        f.write(data)
    return {"ok": True, "path": f".pilot/{filename}"}


@router.post("/fs/entry", status_code=200)
async def create_entry(request: Request) -> dict[str, Any]:
    """Create an empty file or a directory inside the project."""
    body = await request.json()
    path = (body.get("path") or "").strip("/")
    if not path:
        raise HTTPException(status_code=400, detail="path required")
    target = safe_join(body["root"], path)
    if os.path.exists(target):
        raise HTTPException(status_code=409, detail="already exists")
    try:
        if body.get("type") == "dir":
            os.makedirs(target)
        else:
            parent = os.path.dirname(target)
            if parent:
                os.makedirs(parent, exist_ok=True)
            open(target, "x").close()
    except OSError as exc:
        raise HTTPException(status_code=400, detail=str(exc))
    return {"ok": True, "path": path}


@router.patch("/fs/entry")
async def rename_entry(request: Request) -> dict[str, Any]:
    """Rename or move a file/directory within the project."""
    body = await request.json()
    root = body["root"]
    new_path = (body.get("new_path") or "").strip("/")
    if not new_path:
        raise HTTPException(status_code=400, detail="new_path required")
    src = safe_join(root, body["path"])
    dst = safe_join(root, new_path)
    if not os.path.exists(src):
        raise HTTPException(status_code=404, detail="not found")
    if os.path.exists(dst):
        raise HTTPException(status_code=409, detail="target already exists")
    try:
        parent = os.path.dirname(dst)
        if parent:
            os.makedirs(parent, exist_ok=True)
        os.rename(src, dst)
    except OSError as exc:
        raise HTTPException(status_code=400, detail=str(exc))
    return {"ok": True, "path": new_path}


@router.delete("/fs/entry")
def delete_entry(root: str, path: str) -> dict[str, Any]:
    """Delete a file, or a directory and everything under it."""
    target = safe_join(root, path)
    if target == os.path.realpath(root):
        raise HTTPException(status_code=400, detail="cannot delete the project root")
    try:
        if os.path.isdir(target):
            shutil.rmtree(target)
        elif os.path.exists(target):
            os.remove(target)
        else:
            raise HTTPException(status_code=404, detail="not found")
    except OSError as exc:
        raise HTTPException(status_code=400, detail=str(exc))
    return {"ok": True}


def _build_tree(root: str, relative: str = "") -> list[dict[str, Any]]:
    full = os.path.join(root, relative) if relative else root
    nodes = []
    try:
        entries = sorted(os.listdir(full))
    except PermissionError:
        return nodes
    dirs = []
    files = []
    for entry in entries:
        if entry in SKIP_DIRS:
            continue
        entry_full = os.path.join(full, entry)
        entry_rel = os.path.join(relative, entry) if relative else entry
        if os.path.isdir(entry_full):
            dirs.append({"name": entry, "path": entry_rel.replace(os.sep, "/"), "type": "dir", "children": _build_tree(root, entry_rel)})
        else:
            files.append({"name": entry, "path": entry_rel.replace(os.sep, "/"), "type": "file"})
    return dirs + files


@router.get("/tree")
def tree(root: str) -> dict[str, Any]:
    resolved = os.path.realpath(root)
    if not os.path.isdir(resolved):
        raise HTTPException(status_code=400, detail="not a directory")
    return {"root": resolved, "tree": _build_tree(resolved)}


@router.get("/file")
def read_file(root: str, path: str) -> dict[str, Any]:
    target = safe_join(root, path)
    if not os.path.isfile(target):
        raise HTTPException(status_code=404, detail="file not found")
    if os.path.getsize(target) > 200 * 1024:
        raise HTTPException(status_code=413, detail="file too large")
    with open(target, "r", encoding="utf-8", errors="replace") as f:
        content = f.read()
    return {"path": path, "content": content}


@router.put("/file")
async def write_file(request: Request) -> dict[str, Any]:
    body = await request.json()
    root = body["root"]
    path = body["path"]
    content = body["content"]
    target = safe_join(root, path)
    os.makedirs(os.path.dirname(target), exist_ok=True)
    with open(target, "w", encoding="utf-8") as f:
        f.write(content)
    return {"ok": True}


@router.post("/extract")
async def extract_document(file: UploadFile = File(...)) -> dict[str, Any]:
    """Extract plain text from an uploaded document (docx/pdf/pptx/xlsx/txt/md)."""
    filename = os.path.basename(file.filename or "document")
    data = await file.read()
    text = _extract_text(filename, data)
    if len(text) > MAX_DOC_BYTES:
        text = text[:MAX_DOC_BYTES]
    return {"filename": filename, "text": text}


@router.post("/agent")
async def code_agent(request: Request) -> StreamingResponse:
    body = await request.json()
    root = body["root"]
    model = body.get("model")
    messages = body.get("messages", [])
    mode = body.get("mode") or "ask"  # "ask" pauses on mutating ops; default auto
    sid = body.get("session_id") or sessions.new_id()
    if not sessions.is_valid_id(sid):
        raise HTTPException(status_code=400, detail="invalid session id")
    resolved_root = os.path.realpath(root)
    if not os.path.isdir(resolved_root):
        raise HTTPException(status_code=400, detail="not a directory")

    async def event_stream() -> AsyncIterator[str]:
        # run_full_access streams the harness events (session first) and persists
        # the conversation to <root>/.pilot/sessions/<id>.json when it finishes.
        async for event in run_full_access(resolved_root, messages, model=model, mode=mode, session_id=sid):
            etype = event.get("type", "text")
            yield f"event: {etype}\ndata: {json.dumps(event)}\n\n"

    return StreamingResponse(event_stream(), media_type="text/event-stream")


@router.get("/agent/events")
async def code_agent_events(request: Request, root: str) -> StreamingResponse:
    """Watch (read-only) every agent run happening in `root`, including ones
    started from Telegram, so the Code IDE mirrors them live. Unlike /agent this
    starts nothing — it only relays events published to the run bus."""
    resolved_root = os.path.realpath(root)

    async def event_stream() -> AsyncIterator[str]:
        queue = run_bus.subscribe(resolved_root)
        try:
            while True:
                if await request.is_disconnected():
                    break
                try:
                    event = await asyncio.wait_for(queue.get(), timeout=20.0)
                except asyncio.TimeoutError:
                    yield ": keep-alive\n\n"  # comment frame keeps the connection open
                    continue
                etype = event.get("type", "text")
                yield f"event: {etype}\ndata: {json.dumps(event)}\n\n"
        finally:
            run_bus.unsubscribe(resolved_root, queue)

    return StreamingResponse(event_stream(), media_type="text/event-stream")


@router.get("/sessions")
def code_sessions(root: str) -> dict[str, Any]:
    """List a project's conversation sessions (shared with the terminal)."""
    return {"sessions": sessions.list_sessions(os.path.realpath(root))}


@router.get("/session")
def code_session(root: str, id: str) -> dict[str, Any]:
    s = sessions.load(os.path.realpath(root), id)
    if not s:
        raise HTTPException(status_code=404, detail="session not found")
    return s


@router.delete("/session")
def delete_session(root: str, id: str) -> dict[str, Any]:
    """Delete a project's conversation session (shared with the terminal)."""
    return {"ok": sessions.delete(os.path.realpath(root), id)}


@router.get("/terminals")
def list_terminals(root: str) -> dict[str, Any]:
    """Live terminal ids for a project, so a reloaded page can reattach."""
    return {"supported": terminals.supported(), "ids": terminals.list_ids(os.path.realpath(root))}


@router.delete("/terminal")
def kill_terminal(id: str) -> dict[str, Any]:
    return {"ok": terminals.kill(id)}


@router.websocket("/terminal")
async def terminal_ws(ws: WebSocket) -> None:
    """Stream a real shell running in the project directory.

    Output goes down as binary frames (xterm decodes UTF-8 itself); input and
    resize come up as JSON text frames. The shell outlives the socket, so a page
    reload reattaches to the same session instead of killing what is running.
    """
    # A websocket handshake skips the app's http middleware and may skip router
    # dependencies, so guard here directly: this endpoint is arbitrary local code
    # execution. The peer address is the authoritative check (not client-controlled);
    # the Origin is a secondary check and a missing Origin is rejected, not allowed.
    peer = ws.client.host if ws.client else ""
    origin = ws.headers.get("origin")
    if peer not in LOCAL_HOSTS or (urlparse(origin or "").hostname or "") not in LOCAL_HOSTS:
        await ws.close(code=1008)
        return
    if not terminals.supported():
        await ws.close(code=1011)
        return
    tid = ws.query_params.get("id", "")
    resolved = os.path.realpath(ws.query_params.get("root", ""))
    if not tid or not os.path.isdir(resolved):
        await ws.close(code=1008)
        return
    try:
        cols = max(int(ws.query_params.get("cols", 80)), 2)
        rows = max(int(ws.query_params.get("rows", 24)), 1)
    except ValueError:
        cols, rows = 80, 24

    await ws.accept()
    try:
        term = terminals.create(tid, resolved, cols, rows)
    except Exception as exc:
        await ws.send_text(json.dumps({"type": "error", "message": str(exc)}))
        await ws.close(code=1011)
        return

    previous = term.client
    if previous is not None and previous is not ws:
        try:
            await previous.close(code=1000)
        except Exception:
            pass
    term.client = ws
    if term.buffer:
        await ws.send_bytes(bytes(term.buffer))  # replay the scrollback

    try:
        while True:
            msg = await ws.receive_text()
            try:
                data = json.loads(msg)
            except ValueError:
                continue
            kind = data.get("type")
            if kind == "input":
                term.write(data.get("data", ""))
            elif kind == "resize":
                term.resize(int(data.get("cols", cols)), int(data.get("rows", rows)))
            elif kind == "kill":
                term.kill()
                break
    except WebSocketDisconnect:
        pass
    except Exception:
        pass
    finally:
        if term.client is ws:
            term.client = None


@router.post("/agent/confirm")
async def code_agent_confirm(request: Request) -> dict[str, Any]:
    """Relay an ask-mode decision to the harness for a run that is paused waiting
    on the user. The harness matches it by the id from its 'confirm' event."""
    body = await request.json()
    confirm_url = HARNESS_URL.rsplit("/run", 1)[0] + "/confirm"
    payload = {
        "id": body.get("id", ""),
        "decision": body.get("decision", "decline"),
        "feedback": body.get("feedback", ""),
    }
    async with httpx.AsyncClient(timeout=30) as client:
        resp = await client.post(confirm_url, json=payload)
        resp.raise_for_status()
        return resp.json()
