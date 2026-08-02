"""App Builder API — Replit-style multi-project. Each project is an isolated Vite
React+Tailwind app with its own dev server, previewed through the gateway on
:6969/<id>/. The agent (and the Source tab) edit files under the project's src/,
and Vite hot-reloads."""

import asyncio
import json
from datetime import datetime, timezone
from typing import Any, AsyncIterator

import httpx
from fastapi import APIRouter, HTTPException, Request
from fastapi.responses import Response, StreamingResponse

from services.harness_client import HARNESS_URL
from services import builder_projects as bp

router = APIRouter(prefix="/builder")

# The build instructions live in the internal "app-builder" skill (injected
# silently). This prompt stays tiny.
_BUILDER_INSTRUCTION = (
    "Build the app the user asks for, following the stack guidance in your "
    "instructions exactly. Write React components as files in the current "
    "directory. Three rules that are easy to get wrong, so hold to them: "
    "(1) EVERY control must actually work — a category/filter/tab/search must "
    "change which items render by filtering the array from state, not just "
    "highlight the active chip; no dead buttons. "
    "(2) Icons are lucide-react components ONLY — never use an emoji as an icon "
    "or decoration anywhere. "
    "(3) For ANY photo, call the search_images tool and use a url it returns in "
    "the <img> — NEVER type or guess an image URL from memory (a hand-written "
    "unsplash/pexels/picsum link 404s and shows a broken image), and never fake a "
    "photo with an empty coloured box. "
    "The dev server is ALREADY running and hot-reloads on save — never try to "
    "build, run, serve, test, or preview the app; there is no such tool and no "
    "need. Build exactly the subject the user asked for. Write each file once and "
    "completely; do NOT rewrite files you already wrote to polish them (it usually "
    "breaks them). When the files are written, reply with one short line. The app "
    "to build:"
)


_MAX_ERRORS = 8
_MAX_ERROR_CHARS = 600


def _console_block(errors: list[Any]) -> str:
    """Render preview runtime errors as a prompt block, newest last."""
    lines: list[str] = []
    for e in errors[-_MAX_ERRORS:]:
        if not isinstance(e, dict):
            continue
        text = str(e.get("text", "")).strip()
        if not text:
            continue
        if len(text) > _MAX_ERROR_CHARS:
            text = text[:_MAX_ERROR_CHARS] + " …(truncated)"
        count = e.get("count") or 1
        level = e.get("level", "error")
        prefix = f"[{level}]" + (f" x{count}" if count > 1 else "")
        lines.append(f"{prefix} {text}")
    if not lines:
        return ""
    return (
        "\n\nThe running preview reported these runtime errors (captured from the "
        "browser console — you cannot and must not run the app yourself; this IS "
        "the run output). Read the files they point at and fix the cause:\n"
        + "\n".join(lines)
    )


@router.on_event("startup")
def _startup() -> None:
    # Best-effort: bring the gateway up so previews resolve immediately.
    try:
        bp.ensure_gateway()
    except Exception:
        pass


@router.get("/projects")
def list_projects() -> dict[str, Any]:
    return {"projects": bp.list_projects()}


@router.post("/projects", status_code=200)
async def create_project(request: Request) -> dict[str, Any]:
    body = await request.json()
    name = (body.get("name") or "").strip()
    prompt = body.get("prompt", "")
    now = datetime.now(timezone.utc).isoformat()
    entry = bp.create_project(name=name, prompt=prompt, created=now)
    bp.ensure_server(entry["id"])
    return {"id": entry["id"], "name": entry["name"]}


@router.get("/projects/{pid}")
def get_project(pid: str, request: Request) -> dict[str, Any]:
    meta = bp.get_meta(pid)
    if not meta:
        raise HTTPException(status_code=404, detail="project not found")
    running = bp.ensure_server(pid)
    host = request.url.hostname or "127.0.0.1"
    return {
        "id": pid,
        "name": meta["name"],
        "url": bp.preview_url(pid, host),
        "running": running,
        "files": bp.list_files(pid),
        "messages": bp.load_messages(pid),  # restore the build log on reload
    }


@router.post("/projects/{pid}/run")
def run_project(pid: str, request: Request) -> dict[str, Any]:
    if not bp.get_meta(pid):
        raise HTTPException(status_code=404, detail="project not found")
    running = bp.ensure_server(pid)
    host = request.url.hostname or "127.0.0.1"
    return {"url": bp.preview_url(pid, host), "running": running}


@router.post("/projects/{pid}/rename")
async def rename_project(pid: str, request: Request) -> dict[str, Any]:
    body = await request.json()
    m = bp.rename_project(pid, body.get("name", ""))
    if not m:
        raise HTTPException(status_code=404, detail="project not found")
    return {"id": m["id"], "name": m["name"]}


@router.post("/projects/{pid}/stop")
def stop_project(pid: str) -> dict[str, Any]:
    bp.stop_server(pid)
    return {"ok": True}


@router.delete("/projects/{pid}")
def delete_project(pid: str) -> dict[str, Any]:
    bp.delete_project(pid)
    return {"ok": True}


@router.get("/projects/{pid}/file")
def read_file(pid: str, path: str) -> dict[str, Any]:
    if not bp.get_meta(pid):
        raise HTTPException(status_code=404, detail="project not found")
    try:
        return {"path": path, "content": bp.read_file(pid, path)}
    except ValueError:
        raise HTTPException(status_code=400, detail="bad path")
    except FileNotFoundError:
        raise HTTPException(status_code=404, detail="file not found")


@router.put("/projects/{pid}/file")
async def write_file(pid: str, request: Request) -> dict[str, Any]:
    if not bp.get_meta(pid):
        raise HTTPException(status_code=404, detail="project not found")
    body = await request.json()
    try:
        bp.write_file(pid, body["path"], body.get("content", ""))
    except ValueError:
        raise HTTPException(status_code=400, detail="bad path")
    return {"ok": True}


@router.get("/projects/{pid}/export")
def export_project(pid: str):
    meta = bp.get_meta(pid)
    if not meta:
        raise HTTPException(status_code=404, detail="project not found")
    data = bp.zip_project(pid)
    safe_name = "".join(c if c.isalnum() or c in "-_" else "-" for c in meta["name"]) or "app"
    return Response(
        content=data,
        media_type="application/zip",
        headers={"Content-Disposition": f'attachment; filename="{safe_name}.zip"'},
    )


@router.post("/projects/{pid}/generate")
async def generate(pid: str, request: Request) -> StreamingResponse:
    if not bp.get_meta(pid):
        raise HTTPException(status_code=404, detail="project not found")
    body = await request.json()
    prompt = body.get("prompt", "")
    model = body.get("model")
    history = body.get("history", [])
    bp.ensure_server(pid)

    # Root at the project so `npm install` finds package.json + node_modules; the
    # skill keeps the model editing src/ and off the config files.
    workdir = bp.project_dir(pid)
    messages: list[dict[str, Any]] = []
    for entry in history:
        role = entry.get("role", "user")
        content = entry.get("content", "")
        if role in ("user", "assistant") and content:
            messages.append({"role": role, "content": content})
    console_block = _console_block(body.get("console_errors") or [])
    messages.append({"role": "user", "content": f"{_BUILDER_INSTRUCTION}\n\n{prompt}{console_block}"})

    # Clean conversation for the persisted build log (no instruction prefix).
    log_convo = [
        {"role": e.get("role"), "content": e.get("content", "")}
        for e in history
        if e.get("role") in ("user", "assistant") and e.get("content")
    ]
    log_convo.append({"role": "user", "content": prompt})

    async def event_stream() -> AsyncIterator[str]:
        answer_parts: list[str] = []
        payload: dict[str, Any] = {
            "messages": messages,
            "working_directory": workdir,
            "full_access": True,
            # File tools + a NARROW npm_install (no general shell): the model can add
            # packages but cannot run arbitrary commands. full_access is only to
            # enable the file tools; the explicit allowlist blocks shell_run/serve.
            "allowed_tools": ["read_file", "write_file", "edit_file", "list_dir", "npm_install", "search_images"],
            "inject_skills": ["app-builder"],
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
                        etype = event.get("type")
                        if etype == "text":
                            answer_parts.append(event.get("content", ""))
                        yield f"data: {json.dumps(event)}\n\n"
                        if etype in ("done", "error"):
                            break
            # Report the files so the Source tab lists them; preview hot-reloads.
            yield f"data: {json.dumps({'type': 'files', 'files': bp.list_files(pid)})}\n\n"
        except asyncio.CancelledError:
            pass  # client reloaded/navigated away — still persist below
        except Exception as exc:
            yield f"data: {json.dumps({'type': 'error', 'message': str(exc)})}\n\n"
        finally:
            # Persist the build log so the conversation survives reload — even if
            # the client disconnected mid-build.
            answer = "".join(answer_parts).strip()
            if answer:
                log_convo.append({"role": "assistant", "content": answer})
            bp.save_messages(pid, log_convo)

    return StreamingResponse(event_stream(), media_type="text/event-stream")
