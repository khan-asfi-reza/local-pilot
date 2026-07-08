from __future__ import annotations

import datetime as dt
from typing import Any

from sqlmodel import Field, SQLModel


class Thread(SQLModel, table=True):
    id: str | None = Field(default=None, primary_key=True)
    title: str = ""
    model: str | None = None
    created_at: dt.datetime = Field(default_factory=dt.datetime.utcnow)
    updated_at: dt.datetime = Field(default_factory=dt.datetime.utcnow)


class Message(SQLModel, table=True):
    id: str | None = Field(default=None, primary_key=True)
    thread_id: str = Field(foreign_key="thread.id")
    role: str
    content: str = ""
    tool_calls: str = "[]"
    tool_call_id: str | None = None
    name: str | None = None
    created_at: dt.datetime = Field(default_factory=dt.datetime.utcnow)


class ThreadRead(SQLModel):
    thread: dict[str, Any]
    messages: list[dict[str, Any]]


class ThreadCreate(SQLModel):
    title: str | None = None
