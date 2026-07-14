from __future__ import annotations

import datetime as dt

from sqlmodel import Field, SQLModel


class BuilderSession(SQLModel, table=True):
    id: str | None = Field(default=None, primary_key=True)
    prompt: str = ""
    model: str | None = None
    created_at: dt.datetime = Field(default_factory=dt.datetime.utcnow)
    updated_at: dt.datetime = Field(default_factory=dt.datetime.utcnow)
