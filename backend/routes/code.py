import asyncio
import json
import os
import uuid
from typing import Any, AsyncIterator

import httpx
from fastapi import APIRouter, Depends, HTTPException, Request
from fastapi.responses import StreamingResponse
from sqlmodel import select

from core.database import get_session, init_db
from schemas.project import Project
from services.harness_client import HARNESS_URL

router = APIRouter(prefix="/code", dependencies=[])

SKIP_DIRS = {".git", "node_modules", "dist", ".venv", "__pycache__"}


def local_only(request: Request) -> None:
    host = request.client.host if request.client else ""
    if host not in {"127.0.0.1", "::1", "localhost"}:
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
    target = os.path.realpath(path) if path else os.path.expanduser("~")
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
def list_projects(session=Depends(get_session)) -> dict[str, Any]:
    projects = session.exec(select(Project).order_by(Project.created_at.desc())).all()
    return {
        "projects": [
            {
                "id": p.id,
                "path": p.path,
                "name": p.name,
                "created_at": p.created_at.isoformat(),
            }
            for p in projects
        ]
    }


@router.post("/projects", status_code=200)
async def create_project(request: Request, session=Depends(get_session)) -> dict[str, Any]:
    body = await request.json()
    path = body.get("path", "")
    resolved = os.path.realpath(path)
    if not os.path.isdir(resolved):
        raise HTTPException(status_code=400, detail="directory does not exist")
    existing = session.exec(select(Project).where(Project.path == resolved)).first()
    if existing:
        return {"project": {"id": existing.id, "path": existing.path, "name": existing.name, "created_at": existing.created_at.isoformat()}}
    project = Project(id=str(uuid.uuid4()), path=resolved, name=os.path.basename(resolved))
    session.add(project)
    session.commit()
    session.refresh(project)
    return {"project": {"id": project.id, "path": project.path, "name": project.name, "created_at": project.created_at.isoformat()}}


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


@router.post("/agent")
async def code_agent(request: Request) -> StreamingResponse:
    body = await request.json()
    root = body["root"]
    model = body.get("model")
    messages = body.get("messages", [])
    resolved_root = os.path.realpath(root)
    if not os.path.isdir(resolved_root):
        raise HTTPException(status_code=400, detail="not a directory")

    async def event_stream() -> AsyncIterator[str]:
        payload = {
            "messages": messages,
            "working_directory": resolved_root,
            "full_access": True,
        }
        if model:
            payload["model"] = model
        try:
            async with httpx.AsyncClient(timeout=None) as client:
                async with client.stream("POST", HARNESS_URL, json=payload) as resp:
                    resp.raise_for_status()
                    async for line in resp.aiter_lines():
                        if not line.strip():
                            continue
                        event = json.loads(line)
                        etype = event.get("type", "text")
                        yield f"event: {etype}\ndata: {json.dumps(event)}\n\n"
                        if etype in {"done", "error"}:
                            break
        except asyncio.CancelledError:
            return
        except Exception as exc:
            yield f"event: error\ndata: {json.dumps({'type': 'error', 'message': str(exc)})}\n\n"

    return StreamingResponse(event_stream(), media_type="text/event-stream")
