import asyncio
import json
import os
import re
import uuid
from typing import Any, AsyncIterator

from fastapi import APIRouter, Depends, HTTPException, Request
from fastapi.responses import HTMLResponse, StreamingResponse
from sqlmodel import Session

from core.appdir import sandbox_for
from core.database import get_session, init_db
from schemas.builder import BuilderSession
from services.harness_client import stream_harness_turn

router = APIRouter(prefix="/builder")

_bg_tasks: set = set()

# Maps code fence languages to file extensions.
_LANG_EXT = {
    "html": "html",
    "htm": "html",
    "css": "css",
    "javascript": "js",
    "js": "js",
    "jsx": "jsx",
    "typescript": "ts",
    "ts": "ts",
    "tsx": "tsx",
    "python": "py",
    "py": "py",
    "json": "json",
}

# Pattern: ```lang  filename.ext  (optional filename after the language tag)
_CODE_BLOCK_RE = re.compile(
    r"```(\w+)(?:\s+([^\s\n]+))?\s*\n(.*?)```",
    re.DOTALL,
)

# Default filename when the model doesn't specify one.
_DEFAULT_NAMES = {
    "html": "index.html",
    "css": "style.css",
    "js": "script.js",
    "jsx": "App.jsx",
    "ts": "index.ts",
    "tsx": "App.tsx",
    "py": "main.py",
    "json": "data.json",
}

_MIME_TYPES = {
    ".html": "text/html; charset=utf-8",
    ".htm": "text/html; charset=utf-8",
    ".css": "text/css; charset=utf-8",
    ".js": "text/javascript; charset=utf-8",
    ".mjs": "text/javascript; charset=utf-8",
    ".json": "application/json; charset=utf-8",
    ".png": "image/png",
    ".jpg": "image/jpeg",
    ".jpeg": "image/jpeg",
    ".gif": "image/gif",
    ".svg": "image/svg+xml",
    ".ico": "image/x-icon",
    ".woff": "font/woff",
    ".woff2": "font/woff2",
    ".ttf": "font/ttf",
}

_BUILDER_INSTRUCTION = (
    "You are building a web app. Output the complete source code as fenced code blocks.\n\n"
    "RULES:\n"
    "- Use ```html index.html for the main HTML page (must link to CSS/JS files).\n"
    "- Use ```css style.css for styles.\n"
    "- Use ```js script.js for JavaScript.\n"
    "- Always start with index.html. Put the filename right after the triple backtick language tag.\n"
    "- Output ONLY code blocks. No explanations, no markdown text outside blocks.\n"
    "- Make the app visually polished: use modern CSS (flexbox/grid, rounded corners, shadows, gradients).\n"
    "- Use a clean color palette: white/light backgrounds, dark text, accent colors for interactive elements.\n"
    "- Include proper viewport meta tag and responsive design.\n"
)


def _spawn(coro) -> None:
    task = asyncio.create_task(coro)
    _bg_tasks.add(task)
    task.add_done_callback(_bg_tasks.discard)


@router.on_event("startup")
def startup_event() -> None:
    init_db()


def _sandbox_for(session_id: str) -> str:
    return sandbox_for(f"builder-{session_id}")


def _walk_files(root: str) -> list[dict[str, str]]:
    """Walk the sandbox directory and return [{path, content}] for every file."""
    files = []
    for dirpath, _, filenames in os.walk(root):
        for name in filenames:
            full = os.path.join(dirpath, name)
            rel = os.path.relpath(full, root)
            try:
                with open(full, "r", errors="replace") as f:
                    content = f.read()
                files.append({"path": rel, "content": content})
            except Exception:
                pass
    return files


def _extract_files(text: str, sandbox: str) -> list[str]:
    """Parse fenced code blocks from text and write them to the sandbox. Returns
    the list of filenames written."""
    written: list[str] = []
    for match in _CODE_BLOCK_RE.finditer(text):
        lang = match.group(1).lower()
        explicit_name = match.group(2)
        code = match.group(3)

        ext = _LANG_EXT.get(lang, lang)
        if explicit_name:
            filename = explicit_name
        else:
            filename = _DEFAULT_NAMES.get(ext, f"output.{ext}")

        # Avoid writing the builder system prefix or trivially small blocks.
        if len(code.strip()) < 10:
            continue

        filepath = os.path.join(sandbox, filename)
        os.makedirs(os.path.dirname(filepath) if os.path.dirname(filepath) else sandbox, exist_ok=True)
        with open(filepath, "w") as f:
            f.write(code)
        written.append(filename)
    return written


@router.post("/sessions", status_code=200)
async def create_session(request: Request, session: Session = Depends(get_session)) -> dict[str, Any]:
    body = await request.json()
    prompt = body.get("prompt", "")
    model = body.get("model")
    bs = BuilderSession(id=str(uuid.uuid4()), prompt=prompt, model=model)
    session.add(bs)
    session.commit()
    session.refresh(bs)
    os.makedirs(_sandbox_for(bs.id), exist_ok=True)
    return {"id": bs.id}


@router.post("/sessions/{session_id}/generate")
async def generate(session_id: str, request: Request, session: Session = Depends(get_session)) -> StreamingResponse:
    bs = session.get(BuilderSession, session_id)
    if not bs:
        raise HTTPException(status_code=404, detail="session not found")

    body = await request.json()
    prompt = body.get("prompt", "")
    history = body.get("history", [])
    workdir = _sandbox_for(session_id)

    full_prompt = f"{_BUILDER_INSTRUCTION}\n\n{prompt}"

    messages: list[dict[str, Any]] = []
    # Replay compact conversation history for context on follow-ups.
    for entry in history:
        role = entry.get("role", "user")
        content = entry.get("content", "")
        if role in ("user", "assistant") and content:
            messages.append({"role": role, "content": content})
    messages.append({"role": "user", "content": full_prompt})

    async def event_stream() -> AsyncIterator[str]:
        accumulated_text = ""
        try:
            async for event in stream_harness_turn(
                messages,
                working_directory=workdir,
                model=bs.model,
            ):
                event_type = event.get("type")
                if event_type == "text":
                    accumulated_text += event.get("content", "")
                yield f"data: {json.dumps(event)}\n\n"
                if event_type in ("done", "error"):
                    break
        except asyncio.CancelledError:
            return
        except Exception as exc:
            error_event = {"type": "error", "message": str(exc)}
            yield f"data: {json.dumps(error_event)}\n\n"

        # After the stream completes, extract code blocks and write files.
        if accumulated_text:
            written = _extract_files(accumulated_text, workdir)
            if written:
                files_event = {"type": "files", "files": written}
                yield f"data: {json.dumps(files_event)}\n\n"

    return StreamingResponse(event_stream(), media_type="text/event-stream")


@router.get("/sessions/{session_id}/preview/{file_path:path}")
async def preview_file(session_id: str, file_path: str, session: Session = Depends(get_session)):
    bs = session.get(BuilderSession, session_id)
    if not bs:
        raise HTTPException(status_code=404, detail="session not found")
    workdir = _sandbox_for(session_id)
    # Default to index.html when no path is given.
    if not file_path:
        file_path = "index.html"
    # Prevent directory traversal.
    resolved = os.path.normpath(os.path.join(workdir, file_path))
    if not resolved.startswith(workdir):
        raise HTTPException(status_code=403, detail="forbidden")
    if not os.path.isfile(resolved):
        raise HTTPException(status_code=404, detail="file not found")
    ext = os.path.splitext(resolved)[1].lower()
    content_type = _MIME_TYPES.get(ext, "application/octet-stream")
    with open(resolved, "r", errors="replace") as f:
        content = f.read()
    if ext in (".html", ".htm"):
        return HTMLResponse(content=content)
    from fastapi.responses import Response
    return Response(content=content, media_type=content_type)


@router.get("/sessions/{session_id}/preview")
async def preview(session_id: str, session: Session = Depends(get_session)):
    """Fallback: serve index.html for the preview iframe."""
    return await preview_file(session_id, "index.html", session)


@router.get("/sessions/{session_id}/files")
async def get_files(session_id: str, session: Session = Depends(get_session)) -> dict[str, Any]:
    bs = session.get(BuilderSession, session_id)
    if not bs:
        raise HTTPException(status_code=404, detail="session not found")
    workdir = _sandbox_for(session_id)
    files = _walk_files(workdir)
    return {"files": files}
