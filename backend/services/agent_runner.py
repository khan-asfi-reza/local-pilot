"""Shared harness run helpers used by the web Code IDE and the Telegram bridge.

Both drive the same full-access agent in a real project directory and persist the
conversation to <root>/.pilot/sessions/<id>.json (the store the terminal shares),
so a run started from Telegram shows up in the web IDE and vice versa."""

import asyncio
import json
from typing import AsyncIterator

import httpx

from core import sessions
from services import run_bus
from services.harness_client import HARNESS_URL, stream_harness_turn


async def run_full_access(root: str, messages: list[dict], model: str | None = None,
                          mode: str = "ask", session_id: str | None = None) -> AsyncIterator[dict]:
    """Yield parsed harness events for a full-access run in `root`. The first
    event is {'type':'session','id':...}; the conversation is persisted when the
    run finishes so either tool can resume it."""
    sid = session_id or sessions.new_id()
    session_event = {"type": "session", "id": sid}
    # Tag every mirrored event with its session id so a watcher can route it to
    # the right thread (concurrent runs on one project share the bus).
    run_bus.publish(root, {**session_event, "sid": sid})
    yield session_event
    assistant_parts: list[str] = []
    payload: dict = {"messages": messages, "working_directory": root, "full_access": True}
    if model:
        payload["model"] = model
    if mode:
        payload["mode"] = mode
    try:
        async with httpx.AsyncClient(timeout=None) as client:
            async with client.stream("POST", HARNESS_URL, json=payload) as resp:
                resp.raise_for_status()
                async for line in resp.aiter_lines():
                    if not line.strip():
                        continue
                    event = json.loads(line)
                    if event.get("type", "text") == "text":
                        assistant_parts.append(event.get("content", ""))
                    # Mirror every event to watchers of this project (e.g. the web
                    # Code IDE showing a run this Telegram chat started).
                    run_bus.publish(root, {**event, "sid": sid})
                    yield event
                    if event.get("type") in {"done", "error"}:
                        break
    except asyncio.CancelledError:
        return
    except Exception as exc:
        yield {"type": "error", "message": str(exc)}
    finally:
        # Tell watchers the run ended, even if the stream closed without a 'done'.
        run_bus.publish(root, {"type": "done", "sid": sid})
        convo = list(messages)
        answer = "".join(assistant_parts).strip()
        if answer:
            convo.append({"role": "assistant", "content": answer})
        if convo:
            try:
                sessions.save(root, sid, convo, model=model, mode=mode)
            except Exception:
                pass


async def run_full_access_collect(root: str, messages: list[dict], model: str | None = None,
                                  mode: str = "auto", session_id: str | None = None) -> tuple[str, str]:
    """Run a full-access turn and return (reply_text, session_id). Single-shot
    form for the Telegram bridge, which edits one message with the final reply."""
    parts: list[str] = []
    sid = session_id or ""
    async for event in run_full_access(root, messages, model=model, mode=mode, session_id=session_id):
        etype = event.get("type")
        if etype == "session":
            sid = event.get("id", sid)
        elif etype == "text":
            parts.append(event.get("content", ""))
        elif etype == "error":
            parts.append(f"\n[error: {event.get('message', '')}]")
            break
        elif etype == "done":
            break
    return "".join(parts).strip(), sid


async def run_sandboxed_chat(messages: list[dict], model: str | None = None) -> str:
    """Collect the full reply from a sandboxed (safe-tools) run — no project, no
    file access. Used for Telegram's no-project chat mode."""
    parts: list[str] = []
    async for event in stream_harness_turn(messages, working_directory="", model=model):
        etype = event.get("type")
        if etype == "text":
            parts.append(event.get("content", ""))
        elif etype in ("done", "error"):
            if etype == "error":
                parts.append(f"\n[error: {event.get('message', '')}]")
            break
    return "".join(parts).strip()
