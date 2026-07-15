from __future__ import annotations

import json
import os
import uuid
from typing import Any, AsyncIterator

from fastapi import APIRouter, Depends, HTTPException, Request
from fastapi.responses import FileResponse, StreamingResponse
from sqlmodel import Session

from core.appdir import data_dir
from core.database import get_session, init_db
from services.harness_client import stream_harness_turn
from schemas.builder import BuilderSession

router = APIRouter(prefix="/builder")

BUILDER_SYSTEM_PROMPT = """\
You are an App Builder agent. The user gives you a brief prompt describing a web \
app. Your job is to produce a SINGLE, SELF-CONTAINED index.html file that \
implements the app.

Rules:
- Use React 18 via CDN (unpkg) and Babel standalone for JSX transform. No build step.
- Put ALL CSS in a <style> tag inside the HTML. Use modern, clean styling.
- Put ALL JavaScript/JSX in a <script type="text/babel"> tag.
- The file must be fully self-contained: no external files, no imports except CDN URLs.
- Make it visually polished: use a clean layout, good spacing, readable fonts.
- If the app needs data, use inline mock data or useState.
- Write the complete file using the write_file tool with filename "index.html".
- Do NOT output any explanation. Just write the file.
"""


def _builder_dir(session_id: str) -> str:
    path = os.path.join(data_dir(), "builder", session_id)
    os.makedirs(path, exist_ok=True)
    return path


def _guard_path(session_dir: str, target: str) -> str:
    resolved = os.path.realpath(os.path.join(session_dir, target))
    if not resolved.startswith(os.path.realpath(session_dir)):
        raise HTTPException(status_code=400, detail="path escapes session directory")
    return resolved


@router.on_event("startup")
def startup_event() -> None:
    init_db()


@router.post("/sessions", status_code=201)
async def create_session(request: Request, session: Session = Depends(get_session)) -> dict[str, Any]:
    body = await request.json()
    prompt = body.get("prompt", "")
    if not prompt:
        raise HTTPException(status_code=400, detail="prompt is required")
    model = body.get("model")
    sid = str(uuid.uuid4())
    bs = BuilderSession(id=sid, prompt=prompt, model=model)
    session.add(bs)
    session.commit()
    session.refresh(bs)
    _builder_dir(sid)
    return {"id": bs.id}


@router.post("/sessions/{sid}/generate")
async def generate(sid: str, request: Request, session: Session = Depends(get_session)) -> StreamingResponse:
    bs = session.get(BuilderSession, sid)
    if not bs:
        raise HTTPException(status_code=404, detail="session not found")
    body = await request.json()
    user_prompt = body.get("prompt", "")
    if not user_prompt:
        raise HTTPException(status_code=400, detail="prompt is required")

    workdir = _builder_dir(sid)
    messages: list[dict[str, Any]] = [
        {"role": "system", "content": BUILDER_SYSTEM_PROMPT},
        {"role": "user", "content": user_prompt},
    ]

    async def event_stream() -> AsyncIterator[str]:
        async for event in stream_harness_turn(messages, working_directory=workdir, model=bs.model):
            yield f"data: {json.dumps(event)}\n\n"

    return StreamingResponse(event_stream(), media_type="text/event-stream")


@router.get("/sessions/{sid}/preview")
def preview(sid: str, session: Session = Depends(get_session)) -> FileResponse:
    bs = session.get(BuilderSession, sid)
    if not bs:
        raise HTTPException(status_code=404, detail="session not found")
    html_path = _guard_path(_builder_dir(sid), "index.html")
    if not os.path.isfile(html_path):
        raise HTTPException(status_code=404, detail="index.html not yet generated")
    return FileResponse(html_path, media_type="text/html")


@router.get("/sessions/{sid}/files")
def list_files(sid: str, session: Session = Depends(get_session)) -> dict[str, Any]:
    bs = session.get(BuilderSession, sid)
    if not bs:
        raise HTTPException(status_code=404, detail="session not found")
    session_dir = _builder_dir(sid)
    files: list[dict[str, str]] = []
    for root, _dirs, filenames in os.walk(session_dir):
        for fname in filenames:
            abs_path = os.path.join(root, fname)
            rel = os.path.relpath(abs_path, session_dir)
            _guard_path(session_dir, rel)
            try:
                with open(abs_path, "r", encoding="utf-8", errors="replace") as f:
                    content = f.read()
            except Exception:
                content = ""
            files.append({"path": rel, "content": content})
    return {"files": files}
