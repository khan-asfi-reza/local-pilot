"""Owner profile + Telegram linking, stored in the global SQLite DB.

One Profile row holds the machine owner's name and the Telegram bot config
(token moved here from env). Each linked Telegram chat is a TelegramLink row —
its own selected project, session, and mode — so several people can each drive
the local pilot from their own chat. LinkCode rows are short-lived one-time
codes minted by the local frontend and redeemed through the bot."""

import datetime as dt
import secrets

from sqlmodel import Session, select

from core.database import get_engine
from schemas.profile import PROFILE_ID, LinkCode, Profile, TelegramLink

LINK_CODE_TTL = dt.timedelta(minutes=10)


def _now() -> dt.datetime:
    return dt.datetime.utcnow()


def get_profile() -> Profile:
    """Return the single owner profile, creating an empty one on first access."""
    with Session(get_engine()) as session:
        profile = session.get(Profile, PROFILE_ID)
        if profile is None:
            profile = Profile(id=PROFILE_ID)
            session.add(profile)
            session.commit()
            session.refresh(profile)
        return profile


def save_name(name: str) -> Profile:
    with Session(get_engine()) as session:
        profile = session.get(Profile, PROFILE_ID) or Profile(id=PROFILE_ID)
        profile.name = name.strip()
        profile.onboarded = True
        profile.updated_at = _now()
        session.add(profile)
        session.commit()
        session.refresh(profile)
        return profile


def save_telegram_settings(bot_token: str | None = None, enabled: bool | None = None,
                           bot_username: str | None = None) -> Profile:
    with Session(get_engine()) as session:
        profile = session.get(Profile, PROFILE_ID) or Profile(id=PROFILE_ID)
        if bot_token is not None:
            profile.telegram_bot_token = bot_token.strip()
        if bot_username is not None:
            profile.telegram_bot_username = bot_username.strip()
        if enabled is not None:
            profile.telegram_enabled = enabled
        profile.updated_at = _now()
        session.add(profile)
        session.commit()
        session.refresh(profile)
        return profile


def telegram_config() -> dict:
    """Token + enabled flag for the bot supervisor to poll."""
    profile = get_profile()
    return {"bot_token": profile.telegram_bot_token, "enabled": profile.telegram_enabled}


def public_profile() -> dict:
    """Profile view for the frontend — never leaks the raw token to a GET all
    callers can hit; the token lives behind /telegram/settings instead."""
    profile = get_profile()
    return {
        "name": profile.name,
        "onboarded": profile.onboarded,
        "telegram": {
            "enabled": profile.telegram_enabled,
            "configured": bool(profile.telegram_bot_token),
            "bot_username": profile.telegram_bot_username,
            "links": list_links(),
        },
    }


def new_link_code() -> dict:
    """Mint a one-time link code (10-minute TTL) and a t.me deep link for it."""
    code = secrets.token_hex(4)
    expires = _now() + LINK_CODE_TTL
    with Session(get_engine()) as session:
        session.add(LinkCode(code=code, expires_at=expires))
        session.commit()
    profile = get_profile()
    deep_link = ""
    if profile.telegram_bot_username:
        deep_link = f"https://t.me/{profile.telegram_bot_username}?start={code}"
    return {"code": code, "deep_link": deep_link, "expires_in": int(LINK_CODE_TTL.total_seconds())}


def verify_link_code(code: str, chat_id: int, tg_user_id: int | None = None,
                     tg_username: str | None = None, display_name: str = "") -> bool:
    """Redeem a code: consume it and authorize the chat. Returns False if the
    code is unknown, already used, or expired."""
    with Session(get_engine()) as session:
        entry = session.get(LinkCode, code.strip())
        if entry is None or entry.used or entry.expires_at < _now():
            return False
        entry.used = True
        session.add(entry)
        link = session.get(TelegramLink, chat_id) or TelegramLink(chat_id=chat_id)
        link.tg_user_id = tg_user_id
        link.tg_username = tg_username
        link.display_name = display_name or link.display_name
        link.authorized = True
        link.last_seen = _now()
        session.add(link)
        session.commit()
        return True


def get_link(chat_id: int) -> TelegramLink | None:
    with Session(get_engine()) as session:
        return session.get(TelegramLink, chat_id)


def ensure_link(chat_id: int, tg_user_id: int | None = None, tg_username: str | None = None,
                display_name: str = "") -> TelegramLink:
    """Return the chat's link, creating an unauthorized row for a new chat and
    refreshing its last_seen / display fields."""
    with Session(get_engine()) as session:
        link = session.get(TelegramLink, chat_id) or TelegramLink(chat_id=chat_id)
        if tg_user_id is not None:
            link.tg_user_id = tg_user_id
        if tg_username is not None:
            link.tg_username = tg_username
        if display_name:
            link.display_name = display_name
        link.last_seen = _now()
        session.add(link)
        session.commit()
        session.refresh(link)
        return link


def set_selected_project(chat_id: int, project_id: str | None, session_id: str | None) -> None:
    with Session(get_engine()) as session:
        link = session.get(TelegramLink, chat_id)
        if link is None:
            return
        link.selected_project_id = project_id
        link.active_session_id = session_id
        session.add(link)
        session.commit()


def set_session(chat_id: int, session_id: str) -> None:
    with Session(get_engine()) as session:
        link = session.get(TelegramLink, chat_id)
        if link is None:
            return
        link.active_session_id = session_id
        session.add(link)
        session.commit()


def set_mode(chat_id: int, mode: str) -> None:
    with Session(get_engine()) as session:
        link = session.get(TelegramLink, chat_id)
        if link is None:
            return
        link.mode = mode
        session.add(link)
        session.commit()


def list_links() -> list[dict]:
    with Session(get_engine()) as session:
        links = session.exec(select(TelegramLink)).all()
    return [
        {
            "chat_id": link.chat_id,
            "display_name": link.display_name,
            "tg_username": link.tg_username,
            "authorized": link.authorized,
            "last_seen": link.last_seen.isoformat() if link.last_seen else None,
        }
        for link in links
    ]


def revoke_link(chat_id: int) -> None:
    with Session(get_engine()) as session:
        link = session.get(TelegramLink, chat_id)
        if link is not None:
            session.delete(link)
            session.commit()
