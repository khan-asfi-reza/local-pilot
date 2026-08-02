from __future__ import annotations

import datetime as dt

from sqlmodel import Field, SQLModel

PROFILE_ID = "default"


class Profile(SQLModel, table=True):
    id: str = Field(default=PROFILE_ID, primary_key=True)
    name: str = ""
    telegram_bot_token: str = ""
    telegram_bot_username: str = ""
    telegram_enabled: bool = False
    onboarded: bool = False
    created_at: dt.datetime = Field(default_factory=dt.datetime.utcnow)
    updated_at: dt.datetime = Field(default_factory=dt.datetime.utcnow)


class TelegramLink(SQLModel, table=True):
    chat_id: int = Field(primary_key=True)
    tg_user_id: int | None = None
    tg_username: str | None = None
    display_name: str = ""
    authorized: bool = False
    selected_project_id: str | None = None
    active_session_id: str | None = None
    mode: str = "auto"
    model: str | None = None
    created_at: dt.datetime = Field(default_factory=dt.datetime.utcnow)
    last_seen: dt.datetime = Field(default_factory=dt.datetime.utcnow)


class LinkCode(SQLModel, table=True):
    code: str = Field(primary_key=True)
    expires_at: dt.datetime
    used: bool = False
