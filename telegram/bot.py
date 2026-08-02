"""Telegram bridge for Local Pilot.

A thin long-polling bot: it does Telegram I/O and voice transcription only. All
auth, project selection, and agent runs live in the Local Pilot backend, which
the bot reaches over HTTP. The bot token is read from the backend (set in the
app's Settings), not from the environment, so connecting a Telegram account is
done entirely from the web UI.

Commands:
  /start [code]  greet, or link this chat when a code is passed (deep link)
  /link <code>   link this chat using a code from Settings -> Connect Telegram
  /projects      pick a project to work in (tap a button)
  /chat          switch to no-project chat mode
  /status        show the linked name, selected project, and mode
  /mode ask|auto ask pauses on mutating ops; auto runs straight through

Run:  python bot.py     (BACKEND_URL optional, default http://localhost:8182)
"""

from __future__ import annotations

import asyncio
import logging
import os
import tempfile
import time

import httpx
from telegram import InlineKeyboardButton, InlineKeyboardMarkup, Update
from telegram.ext import (
    ApplicationBuilder,
    CallbackQueryHandler,
    CommandHandler,
    ContextTypes,
    MessageHandler,
    filters,
)

from transcribe import transcribe

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger(__name__)

BACKEND_URL = os.getenv("BACKEND_URL", "http://localhost:8182").rstrip("/")

MAX_MESSAGE = 4000  # Telegram caps messages at 4096; leave room for a truncation note.


async def api_get(path: str, **params) -> dict:
    async with httpx.AsyncClient(timeout=30) as client:
        resp = await client.get(BACKEND_URL + path, params=params or None)
        resp.raise_for_status()
        return resp.json()


async def api_post(path: str, payload: dict, timeout: float | None = 60.0) -> dict:
    async with httpx.AsyncClient(timeout=timeout) as client:
        resp = await client.post(BACKEND_URL + path, json=payload)
        resp.raise_for_status()
        return resp.json()


def chat_meta(update: Update) -> dict:
    """The identity fields the backend keys a link on."""
    chat = update.effective_chat
    user = update.effective_user
    return {
        "chat_id": chat.id,
        "tg_user_id": user.id if user else None,
        "tg_username": user.username if user else None,
        "display_name": (user.full_name if user else "") or "",
    }


async def safe_edit(message, text: str) -> None:
    """Edit a message, truncating to Telegram's limit and falling back to a fresh
    reply if the edit is rejected (e.g. identical text)."""
    text = text or "(no response)"
    if len(text) > MAX_MESSAGE:
        text = text[:MAX_MESSAGE] + "\n… (truncated)"
    try:
        await message.edit_text(text)
    except Exception:
        try:
            await message.reply_text(text)
        except Exception as exc:
            log.warning("could not deliver reply: %s", exc)


def download_file_sync(url: str, dest: str) -> None:
    with httpx.Client(timeout=60) as client:
        resp = client.get(url)
        resp.raise_for_status()
        with open(dest, "wb") as f:
            f.write(resp.content)


async def download_file(url: str, dest: str) -> None:
    loop = asyncio.get_running_loop()
    await loop.run_in_executor(None, download_file_sync, url, dest)


async def transcribe_async(ogg_path: str) -> str:
    loop = asyncio.get_running_loop()
    return await loop.run_in_executor(None, transcribe, ogg_path)


async def send_to_agent(update: Update, text: str, status_message) -> None:
    """Relay a prompt to the backend and show the reply."""
    payload = {**chat_meta(update), "text": text}
    try:
        result = await api_post("/telegram/message", payload, timeout=None)
        reply = result.get("reply", "(no response)")
    except Exception as exc:
        reply = f"Error talking to Local Pilot: {exc}"
    await safe_edit(status_message, reply)


async def do_link(update: Update, code: str) -> None:
    try:
        result = await api_post("/telegram/link/verify", {"code": code.strip(), **chat_meta(update)})
    except Exception as exc:
        await update.message.reply_text(f"Link failed: {exc}")
        return
    if result.get("ok"):
        name = result.get("name") or "there"
        await update.message.reply_text(
            f"Linked! Hi {name}. Send /projects to pick a project, or just start chatting."
        )
    else:
        await update.message.reply_text(
            "That code is invalid or expired. Generate a fresh one in Pilot → Settings → Connect Telegram."
        )


async def cmd_start(update: Update, ctx: ContextTypes.DEFAULT_TYPE) -> None:
    if ctx.args:
        await do_link(update, ctx.args[0])
        return
    await update.message.reply_text(
        "Hi! I connect you to your Local Pilot.\n\n"
        "1. In Pilot → Settings → Connect Telegram, get a code.\n"
        "2. Send /link <code> here (or tap the deep link).\n"
        "3. /projects to pick a project, or just chat.\n\n"
        "You can send text or a voice note."
    )


async def cmd_link(update: Update, ctx: ContextTypes.DEFAULT_TYPE) -> None:
    if not ctx.args:
        await update.message.reply_text(
            "Usage: /link <code> — get the code in Pilot → Settings → Connect Telegram."
        )
        return
    await do_link(update, ctx.args[0])


async def cmd_projects(update: Update, ctx: ContextTypes.DEFAULT_TYPE) -> None:
    meta = chat_meta(update)
    try:
        result = await api_get("/telegram/projects", chat_id=meta["chat_id"])
    except Exception as exc:
        await update.message.reply_text(f"Could not load projects: {exc}")
        return
    projects = result.get("projects", [])
    if not projects:
        await update.message.reply_text("No projects yet. Open a folder in Pilot's Code tool first.")
        return
    buttons = [[InlineKeyboardButton(p["name"], callback_data="pick:" + p["id"])] for p in projects[:20]]
    buttons.append([InlineKeyboardButton("💬 No project (just chat)", callback_data="pick:none")])
    await update.message.reply_text("Pick a project:", reply_markup=InlineKeyboardMarkup(buttons))


async def on_pick(update: Update, ctx: ContextTypes.DEFAULT_TYPE) -> None:
    query = update.callback_query
    await query.answer()
    data = query.data.split(":", 1)[1]
    project_id = None if data == "none" else data
    try:
        result = await api_post(
            "/telegram/select", {"chat_id": query.message.chat.id, "project_id": project_id}
        )
    except Exception as exc:
        await query.edit_message_text(f"Select failed: {exc}")
        return
    if result.get("cleared"):
        await query.edit_message_text("Switched to chat mode (no project). Send me anything.")
    else:
        await query.edit_message_text(
            f"Working in: {result.get('name')}\n{result.get('path')}\n\nSend a prompt or a voice note."
        )


async def cmd_chat(update: Update, ctx: ContextTypes.DEFAULT_TYPE) -> None:
    try:
        await api_post("/telegram/select", {"chat_id": chat_meta(update)["chat_id"], "project_id": None})
    except Exception as exc:
        await update.message.reply_text(f"Failed: {exc}")
        return
    await update.message.reply_text("Chat mode (no project). Ask me anything.")


async def cmd_status(update: Update, ctx: ContextTypes.DEFAULT_TYPE) -> None:
    meta = chat_meta(update)
    try:
        result = await api_get("/telegram/status", chat_id=meta["chat_id"])
    except Exception as exc:
        await update.message.reply_text(f"Could not read status: {exc}")
        return
    if not result.get("authorized"):
        await update.message.reply_text("Not linked. Send /link <code> to connect this chat.")
        return
    project = result.get("project") or "none (chat mode)"
    await update.message.reply_text(f"Project: {project}\nMode: {result.get('mode', 'auto')}")


async def cmd_mode(update: Update, ctx: ContextTypes.DEFAULT_TYPE) -> None:
    mode = ctx.args[0].lower() if ctx.args else ""
    if mode not in ("ask", "auto"):
        await update.message.reply_text("Usage: /mode ask|auto")
        return
    try:
        await api_post("/telegram/mode", {"chat_id": chat_meta(update)["chat_id"], "mode": mode})
    except Exception as exc:
        await update.message.reply_text(f"Failed: {exc}")
        return
    await update.message.reply_text(f"Mode set to {mode}.")


async def handle_text(update: Update, ctx: ContextTypes.DEFAULT_TYPE) -> None:
    text = update.message.text
    if not text:
        return
    status = await update.message.reply_text("Working…")
    await send_to_agent(update, text, status)


async def handle_voice(update: Update, ctx: ContextTypes.DEFAULT_TYPE) -> None:
    voice = update.message.voice
    if not voice:
        return
    status = await update.message.reply_text("Transcribing…")
    try:
        tg_file = await ctx.bot.get_file(voice.file_id)
        with tempfile.NamedTemporaryFile(suffix=".ogg", delete=False) as tmp:
            ogg_path = tmp.name
        try:
            await download_file(tg_file.file_path, ogg_path)
            transcript = await transcribe_async(ogg_path)
        finally:
            if os.path.exists(ogg_path):
                os.unlink(ogg_path)
    except Exception as exc:
        await safe_edit(status, f"Transcription failed: {exc}")
        return
    if not transcript.strip():
        await safe_edit(status, "Could not understand the voice message.")
        return
    await safe_edit(status, f"You said: {transcript}\n\nWorking…")
    await send_to_agent(update, transcript, status)


def fetch_token() -> tuple[str, bool]:
    """Read the bot token + enabled flag from the backend. Returns ("", False)
    when the backend is unreachable or no token is set yet."""
    try:
        with httpx.Client(timeout=10) as client:
            resp = client.get(BACKEND_URL + "/telegram/config")
            resp.raise_for_status()
            data = resp.json()
            return data.get("bot_token", ""), bool(data.get("enabled"))
    except Exception as exc:
        log.warning("waiting for backend at %s: %s", BACKEND_URL, exc)
        return "", False


def wait_for_token() -> str:
    """Block until the backend has an enabled bot token (set in Settings)."""
    log.info("Waiting for a Telegram bot token — set it in Pilot → Settings.")
    while True:
        token, enabled = fetch_token()
        if token and enabled:
            return token
        time.sleep(5)


def main() -> None:
    token = wait_for_token()
    app = ApplicationBuilder().token(token).build()
    app.add_handler(CommandHandler("start", cmd_start))
    app.add_handler(CommandHandler("link", cmd_link))
    app.add_handler(CommandHandler("projects", cmd_projects))
    app.add_handler(CommandHandler("chat", cmd_chat))
    app.add_handler(CommandHandler("status", cmd_status))
    app.add_handler(CommandHandler("mode", cmd_mode))
    app.add_handler(CallbackQueryHandler(on_pick, pattern=r"^pick:"))
    app.add_handler(MessageHandler(filters.TEXT & ~filters.COMMAND, handle_text))
    app.add_handler(MessageHandler(filters.VOICE, handle_voice))

    log.info("Bot started. Polling…")
    app.run_polling()


if __name__ == "__main__":
    main()
