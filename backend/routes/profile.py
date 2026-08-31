"""Owner profile + Telegram control routes.

Two audiences, both localhost-only (see the app middleware and local_only):
the frontend (Settings + onboarding + Connect flow) and the Telegram bot (the
thin bridge that fetches its token, redeems link codes, and relays messages to
the full-access agent). All the state lives in core.profile; the actual agent
runs are in services.agent_runner."""

import asyncio
import os

import httpx
from fastapi import APIRouter, Depends, HTTPException, Request

from core import profile, projects, sessions
from core.appdir import sandbox_for
from core.database import init_db
from services import tg_runs

LINK_HELP = (
    "You are not linked yet. Open Pilot on your computer, go to Settings → "
    "Connect Telegram, then send me /link <code> (or tap the deep link)."
)


def local_only(request: Request) -> None:
    host = request.client.host if request.client else ""
    if host not in {"127.0.0.1", "::1", "localhost"}:
        raise HTTPException(status_code=403, detail="localhost only")


router = APIRouter(dependencies=[Depends(local_only)])


@router.on_event("startup")
def startup_event() -> None:
    init_db()


async def fetch_bot_username(token: str) -> str:
    """Ask Telegram for the bot's @username (best effort; also validates token)."""
    try:
        async with httpx.AsyncClient(timeout=10) as client:
            resp = await client.get(f"https://api.telegram.org/bot{token}/getMe")
            resp.raise_for_status()
            return resp.json().get("result", {}).get("username", "")
    except Exception:
        return ""


# Frontend-facing routes (Settings, onboarding, Connect flow).

@router.get("/profile")
def get_profile() -> dict:
    return profile.public_profile()


@router.post("/profile")
async def save_profile(request: Request) -> dict:
    body = await request.json()
    name = (body.get("name") or "").strip()
    if not name:
        raise HTTPException(status_code=400, detail="name required")
    profile.save_name(name)
    return profile.public_profile()


@router.get("/telegram/settings")
def telegram_settings() -> dict:
    p = profile.get_profile()
    return {
        "enabled": p.telegram_enabled,
        "configured": bool(p.telegram_bot_token),
        "bot_username": p.telegram_bot_username,
        "bot_token": p.telegram_bot_token,
    }


@router.post("/telegram/settings")
async def save_telegram_settings(request: Request) -> dict:
    body = await request.json()
    token = body.get("bot_token")
    enabled = body.get("enabled")
    username = None
    if token:
        username = await fetch_bot_username(token.strip())
    profile.save_telegram_settings(bot_token=token, enabled=enabled, bot_username=username)
    p = profile.get_profile()
    return {
        "enabled": p.telegram_enabled,
        "configured": bool(p.telegram_bot_token),
        "bot_username": p.telegram_bot_username,
    }


@router.post("/telegram/link/start")
def telegram_link_start() -> dict:
    return profile.new_link_code()


@router.delete("/telegram/link/{chat_id}")
def telegram_link_revoke(chat_id: int) -> dict:
    profile.revoke_link(chat_id)
    return {"ok": True}


# Bot-facing routes (the thin Telegram bridge is the only caller).

@router.get("/telegram/config")
def telegram_config() -> dict:
    return profile.telegram_config()


@router.post("/telegram/link/verify")
async def telegram_link_verify(request: Request) -> dict:
    body = await request.json()
    ok = profile.verify_link_code(
        code=(body.get("code") or ""),
        chat_id=int(body["chat_id"]),
        tg_user_id=body.get("tg_user_id"),
        tg_username=body.get("tg_username"),
        display_name=body.get("display_name", ""),
    )
    return {"ok": ok, "name": profile.get_profile().name}


@router.get("/telegram/projects")
def telegram_projects() -> dict:
    # Same unified registry the web Code IDE lists, newest-opened first.
    return {"projects": projects.list_projects()}


@router.get("/telegram/status")
def telegram_status(chat_id: int) -> dict:
    link = profile.get_link(chat_id)
    if link is None:
        return {"authorized": False, "project": None, "mode": "auto"}
    proj = projects.get(link.selected_project_id) if link.selected_project_id else None
    return {"authorized": link.authorized, "project": proj["name"] if proj else None, "mode": link.mode}


@router.post("/telegram/select")
async def telegram_select(request: Request) -> dict:
    body = await request.json()
    chat_id = int(body["chat_id"])
    pid = body.get("project_id") or None
    name, path = "", ""
    new_sid = sessions.new_id()
    if pid:
        proj = projects.get(pid)
        if not proj:
            raise HTTPException(status_code=404, detail="project not found")
        name, path = proj["name"], proj["path"]
        # Seed a tagged thread so it shows in the Code IDE (marked telegram) right
        # away, and the chat's display name titles it.
        link = profile.get_link(chat_id)
        who = (link.display_name or link.tg_username or str(chat_id)) if link else str(chat_id)
        try:
            sessions.ensure_stub(path, new_sid, source="telegram", title=f"Telegram · {who}")
        except Exception:
            pass
    # A fresh session per selection, so switching context starts a clean thread.
    profile.set_selected_project(chat_id, pid, new_sid)
    return {"name": name, "path": path, "cleared": pid is None}


@router.post("/telegram/mode")
async def telegram_mode(request: Request) -> dict:
    body = await request.json()
    mode = body.get("mode", "auto")
    if mode not in ("plan", "ask", "auto"):
        raise HTTPException(status_code=400, detail="mode must be plan, ask, or auto")
    profile.set_mode(int(body["chat_id"]), mode)
    return {"ok": True, "mode": mode}


@router.post("/telegram/message")
async def telegram_message(request: Request) -> dict:
    body = await request.json()
    chat_id = int(body["chat_id"])
    text = (body.get("text") or "").strip()
    link = profile.ensure_link(
        chat_id,
        tg_user_id=body.get("tg_user_id"),
        tg_username=body.get("tg_username"),
        display_name=body.get("display_name", ""),
    )
    if not link.authorized:
        return {"authorized": False, "reply": LINK_HELP, "status": "done", "pending": False}
    if not text:
        return {"authorized": True, "reply": "(empty message)", "status": "done", "pending": False}

    proj = projects.get(link.selected_project_id) if link.selected_project_id else None
    sid = link.active_session_id or sessions.new_id()

    # With a project: the same full-access agent the web Code IDE runs, in the
    # chat's mode (plan/auto run straight through; ask pauses on the first
    # mutating op and returns pending=True, resumed via /telegram/confirm).
    # Without one: a sandboxed, safe-tools chat whose history lives under the
    # global sandbox dir so the conversation carries across messages.
    # Both go through tg_runs, so a slow model streams progress to the bot
    # instead of holding this request open until the bot's timeout fires.
    root = proj["path"] if proj else sandbox_for(f"tg-{chat_id}")
    prior = await asyncio.to_thread(sessions.load, root, sid) or {}
    history = list(prior.get("messages", []))
    history.append({"role": "user", "content": text})
    snap = await tg_runs.start(
        chat_id, root, history, model=link.model, mode=link.mode, sid=sid,
        sandboxed=proj is None,
    )
    profile.set_session(chat_id, snap.get("session_id") or sid)
    return {"authorized": True, **snap}


@router.get("/telegram/progress")
def telegram_progress(chat_id: int) -> dict:
    """Latest snapshot of a chat's in-flight project run, polled by the bot to
    show live activity, an approval prompt, or the final reply."""
    return tg_runs.progress(chat_id)


@router.post("/telegram/clear")
async def telegram_clear(request: Request) -> dict:
    """Forget this chat's conversation: stop any in-flight run and start a fresh
    session (a new thread). If a project is selected, the new thread is seeded as
    a telegram-tagged one so it shows in the Code IDE."""
    body = await request.json()
    chat_id = int(body["chat_id"])
    await tg_runs.cancel(chat_id)
    link = profile.get_link(chat_id)
    new_sid = sessions.new_id()
    if link and link.selected_project_id:
        proj = projects.get(link.selected_project_id)
        if proj:
            who = link.display_name or link.tg_username or str(chat_id)
            try:
                sessions.ensure_stub(proj["path"], new_sid, source="telegram", title=f"Telegram · {who}")
            except Exception:
                pass
    profile.set_session(chat_id, new_sid)
    return {"ok": True}


@router.post("/telegram/confirm")
async def telegram_confirm(request: Request) -> dict:
    """Deliver an ask-mode decision (Approve / Approve all / Decline) from a button
    tap to the paused run for this chat; the run then carries on."""
    body = await request.json()
    chat_id = int(body["chat_id"])
    decision = body.get("decision", "decline")
    if decision not in ("approve", "approve_always", "decline"):
        raise HTTPException(status_code=400, detail="bad decision")
    snap = await tg_runs.resume(chat_id, decision, body.get("feedback", ""))
    if snap.get("session_id"):
        profile.set_session(chat_id, snap["session_id"])
    return {"authorized": True, **snap}
