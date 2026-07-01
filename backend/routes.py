import asyncio
import json
import socket
import uuid
from datetime import datetime, timezone
from typing import Any, AsyncIterator, List
from urllib.parse import urlparse

import httpx
from fastapi import APIRouter, Depends, HTTPException, Request
from fastapi.responses import StreamingResponse
from sqlmodel import Session, select

from database import get_session, init_db
from harness_client import HARNESS_URL, stream_harness_turn
from schemas import Message as DBMessage
from schemas import Thread

router = APIRouter()


def _now() -> datetime:
    return datetime.now(timezone.utc)


def _message_to_payload(message: DBMessage) -> dict[str, Any]:
    return {
        "role": message.role,
        "content": message.content,
        "tool_calls": json.loads(message.tool_calls or "[]"),
        "tool_call_id": message.tool_call_id,
        "name": message.name,
    }


def _build_harness_messages(messages: List[DBMessage]) -> list[dict[str, Any]]:
    return [_message_to_payload(message) for message in messages]


def _serialize_message(message: DBMessage) -> dict[str, Any]:
    return {
        "id": message.id,
        "thread_id": message.thread_id,
        "role": message.role,
        "content": message.content,
        "tool_calls": json.loads(message.tool_calls or "[]"),
        "tool_call_id": message.tool_call_id,
        "name": message.name,
        "created_at": message.created_at.isoformat(),
    }


def _persist_message(session: Session, thread_id: str, role: str, content: str, *, tool_calls: list[dict[str, Any]] | None = None, tool_call_id: str | None = None, name: str | None = None) -> DBMessage:
    message = DBMessage(
        id=str(uuid.uuid4()),
        thread_id=thread_id,
        role=role,
        content=content,
        tool_calls=json.dumps(tool_calls or []),
        tool_call_id=tool_call_id,
        name=name,
    )
    session.add(message)
    session.commit()
    session.refresh(message)
    return message


def _persist_tool_event(session: Session, thread_id: str, event: dict[str, Any]) -> DBMessage:
    return _persist_message(session, thread_id, "tool", json.dumps(event), name=event.get("tool"))


@router.on_event("startup")
def startup_event() -> None:
    init_db()


@router.get("/health")
def health() -> dict[str, Any]:
    try:
        parsed = urlparse(HARNESS_URL)
        host = parsed.hostname or "localhost"
        port = parsed.port or 80
        with socket.create_connection((host, port), timeout=1.5):
            reachable = True
    except OSError:
        reachable = False
    return {"ok": reachable, "harness": reachable}


@router.post("/threads", status_code=200)
def create_thread(session: Session = Depends(get_session)) -> dict[str, Any]:
    thread = Thread(id=str(uuid.uuid4()), title="New thread")
    session.add(thread)
    session.commit()
    session.refresh(thread)
    return {"thread": {"id": thread.id, "title": thread.title, "created_at": thread.created_at.isoformat(), "updated_at": thread.updated_at.isoformat()}}


@router.get("/threads")
def list_threads(session: Session = Depends(get_session)) -> list[dict[str, Any]]:
    threads = session.exec(select(Thread).order_by(Thread.updated_at.desc())).all()
    return [{"id": thread.id, "title": thread.title, "created_at": thread.created_at.isoformat(), "updated_at": thread.updated_at.isoformat()} for thread in threads]


@router.get("/threads/{thread_id}")
def get_thread(thread_id: str, session: Session = Depends(get_session)) -> dict[str, Any]:
    thread = session.get(Thread, thread_id)
    if not thread:
        raise HTTPException(status_code=404, detail="thread not found")
    messages = session.exec(select(DBMessage).where(DBMessage.thread_id == thread_id).order_by(DBMessage.created_at.asc())).all()
    return {
        "thread": {"id": thread.id, "title": thread.title, "created_at": thread.created_at.isoformat(), "updated_at": thread.updated_at.isoformat()},
        "messages": [_serialize_message(message) for message in messages],
    }


@router.delete("/threads/{thread_id}")
def delete_thread(thread_id: str, session: Session = Depends(get_session)) -> dict[str, Any]:
    thread = session.get(Thread, thread_id)
    if not thread:
        raise HTTPException(status_code=404, detail="thread not found")
    session.delete(thread)
    for message in session.exec(select(DBMessage).where(DBMessage.thread_id == thread_id)).all():
        session.delete(message)
    session.commit()
    return {"deleted": True}


@router.post("/threads/{thread_id}/messages")
async def send_message(thread_id: str, request: Request, session: Session = Depends(get_session)) -> StreamingResponse:
    body = await request.json()
    content = body.get("content", "")
    if not content:
        raise HTTPException(status_code=400, detail="content is required")

    thread = session.get(Thread, thread_id)
    if not thread:
        raise HTTPException(status_code=404, detail="thread not found")

    if not thread.title.strip():
        thread.title = content[:40]
        session.add(thread)
        session.commit()

    _persist_message(session, thread_id, "user", content)

    history = session.exec(select(DBMessage).where(DBMessage.thread_id == thread_id).order_by(DBMessage.created_at.asc())).all()
    harness_messages = _build_harness_messages(history)

    async def event_stream() -> AsyncIterator[str]:
        assistant_content_parts: list[str] = []
        try:
            async for event in stream_harness_turn(harness_messages):
                event_type = event.get("type")
                if event_type == "text":
                    content_piece = event.get("content", "")
                    assistant_content_parts.append(content_piece)
                    yield f"event: {event_type}\ndata: {json.dumps(event)}\n\n"
                elif event_type in {"tool_call", "tool_result"}:
                    _persist_tool_event(session, thread_id, event)
                    yield f"event: {event_type}\ndata: {json.dumps(event)}\n\n"
                elif event_type == "error":
                    message_content = event.get("message", "Harness error")
                    yield f"event: {event_type}\ndata: {json.dumps(event)}\n\n"
                    _persist_message(session, thread_id, "assistant", message_content)
                    thread.updated_at = _now()
                    session.add(thread)
                    session.commit()
                    break
                elif event_type == "done":
                    assistant_text = "".join(assistant_content_parts)
                    _persist_message(session, thread_id, "assistant", assistant_text)
                    thread.updated_at = _now()
                    session.add(thread)
                    session.commit()
                    yield f"event: {event_type}\ndata: {json.dumps(event)}\n\n"
                    break
                else:
                    yield f"event: {event_type}\ndata: {json.dumps(event)}\n\n"
        except asyncio.CancelledError:
            return
        except Exception as exc:
            error_event = {"type": "error", "message": str(exc)}
            yield f"event: error\ndata: {json.dumps(error_event)}\n\n"
            _persist_message(session, thread_id, "assistant", str(exc))
            thread.updated_at = _now()
            session.add(thread)
            session.commit()

    return StreamingResponse(event_stream(), media_type="text/event-stream")
