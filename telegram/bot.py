"""Telegram bridge for Local Pilot.

A thin long-polling bot: it does Telegram I/O and voice transcription only. All
auth, project selection, and agent runs live in the Local Pilot backend, which
the bot reaches over HTTP. The bot token is read from the backend (set in the
app's Settings), not from the environment, so connecting a Telegram account is
done entirely from the web UI. Settings stays the live source of truth: the
bridge watches the token and rebinds to a new bot within seconds of a change.

Commands:
  /start [code]  greet, or link this chat when a code is passed (deep link)
  /link <code>   link this chat using a code from Settings -> Connect Telegram
  /projects      pick a project to work in (tap a button)
  /chat          switch to no-project chat mode
  /status        show the linked name, selected project, and mode
  /mode plan|ask|auto  plan is read-only; ask pauses for approval; auto runs through
  /clear         forget this chat's history and start a fresh thread
  /help          list the commands

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
from telegram.error import Conflict, InvalidToken
from telegram.ext import (
    Application,
    ApplicationBuilder,
    CallbackQueryHandler,
    CommandHandler,
    ContextTypes,
    MessageHandler,
    filters,
)

from transcribe import transcribe, warmup

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
# httpx logs every request line at INFO, which puts the bot token (it sits in the
# Telegram API URL) in the log and buries our own lines under poll traffic.
logging.getLogger("httpx").setLevel(logging.WARNING)
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


def describe(exc: Exception) -> str:
    """A readable line for a failed backend call.

    httpx's timeout and connect errors stringify to an empty string, so the
    obvious f"...: {exc}" renders as a bare "Failed:" with nothing after it -
    which is exactly what a stalled backend looked like in the chat."""
    if isinstance(exc, httpx.TimeoutException):
        return f"Local Pilot did not answer in time ({type(exc).__name__}). Is it still running?"
    if isinstance(exc, httpx.ConnectError):
        return f"Cannot reach Local Pilot at {BACKEND_URL} - is `pilot web` running?"
    if isinstance(exc, httpx.HTTPStatusError):
        body = (exc.response.text or "").strip()[:200]
        return f"Local Pilot returned {exc.response.status_code}" + (f": {body}" if body else "")
    detail = str(exc).strip()
    return f"{type(exc).__name__}: {detail}" if detail else type(exc).__name__


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


async def safe_edit(message, text: str, reply_markup=None) -> None:
    """Edit a message, truncating to Telegram's limit and falling back to a fresh
    reply if the edit is rejected (e.g. identical text)."""
    text = text or "(no response)"
    if len(text) > MAX_MESSAGE:
        text = text[:MAX_MESSAGE] + "\n... (truncated)"
    try:
        await message.edit_text(text, reply_markup=reply_markup)
    except Exception:
        try:
            await message.reply_text(text, reply_markup=reply_markup)
        except Exception as exc:
            log.warning("could not deliver reply: %s", exc)


# Ask-mode approval buttons, shown when a project run pauses on a mutating op.
_CONFIRM_KB = InlineKeyboardMarkup(
    [
        [
            InlineKeyboardButton("✅ Approve", callback_data="cf:approve"),
            InlineKeyboardButton("✅ All", callback_data="cf:approve_always"),
            InlineKeyboardButton("❌ Decline", callback_data="cf:decline"),
        ]
    ]
)


POLL_INTERVAL = 3.0  # seconds between progress polls (safe for Telegram edit limits)
MAX_POLL_FAILURES = 5  # give up on a run after this many failed progress polls

# A per-chat run token: a newer message bumps it so a stale progress loop stops
# editing the old status message.
_poll_gen: dict[int, int] = {}


async def edit_progress(message, text: str, last: str) -> str:
    """Edit the status message only when the text changed; swallow edit errors so
    an unchanged frame never spams a fresh message. Returns the text shown."""
    if text == last:
        return last
    try:
        await message.edit_text(text[:MAX_MESSAGE])
    except Exception:
        pass
    return text


def _progress_text(snap: dict) -> str:
    lines = ["⚙️ Working..."]
    current = snap.get("current")
    if current:
        lines.append(f"→ {current}")
    for a in snap.get("activity", []):
        lines.append(a)
    return "\n".join(lines)


async def show_confirm(message, snap: dict) -> None:
    confirm = snap.get("confirm") or {}
    parts = [confirm.get("prompt", "Approve this action?")]
    if confirm.get("diff"):
        parts.append(confirm["diff"])
    parts.append("Approve?")
    await safe_edit(message, "\n\n".join(p for p in parts if p), reply_markup=_CONFIRM_KB)


async def drive(chat_id: int, message, snap: dict) -> None:
    """Render a run to completion: live progress while running, an approval prompt
    on pause, the final reply when done. Non-blocking - other commands run mid-run."""
    gen = _poll_gen.get(chat_id, 0) + 1
    _poll_gen[chat_id] = gen
    last = ""
    fails = 0
    while True:
        status = snap.get("status")
        if status == "paused":
            await show_confirm(message, snap)
            return
        if snap.get("reply") is not None or status == "done":
            await safe_edit(message, snap.get("reply") or "(done)")
            return
        if status == "idle":
            # The backend has no run for this chat: it restarted, or another
            # message superseded this one. Say so instead of a bare "(done)".
            await safe_edit(message, "That run is no longer tracked - Local Pilot restarted. Send it again.")
            return
        last = await edit_progress(message, _progress_text(snap), last)
        await asyncio.sleep(POLL_INTERVAL)
        if _poll_gen.get(chat_id) != gen:
            return  # superseded by a newer message
        try:
            snap = await api_get("/telegram/progress", chat_id=chat_id)
            fails = 0
        except Exception as exc:
            fails += 1
            log.warning("progress poll failed (%d/%d): %r", fails, MAX_POLL_FAILURES, exc)
            if fails >= MAX_POLL_FAILURES:
                # Without this the loop polls a dead backend forever and the
                # chat just sits on "Working..." with no explanation.
                await safe_edit(message, f"Lost contact with Local Pilot. {describe(exc)}")
                return
            snap = {"status": "running", "activity": [], "current": ""}
        if _poll_gen.get(chat_id) != gen:
            return


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
    """Kick off a run on the backend and drive its live progress to completion."""
    payload = {**chat_meta(update), "text": text}
    chat_id = update.effective_chat.id
    try:
        snap = await api_post("/telegram/message", payload, timeout=60)
    except Exception as exc:
        log.warning("/telegram/message failed: %r", exc)
        await safe_edit(status_message, describe(exc))
        return
    if not snap.get("authorized", True):
        await safe_edit(status_message, snap.get("reply") or "(not linked)")
        return
    await drive(chat_id, status_message, snap)


async def on_confirm(update: Update, ctx: ContextTypes.DEFAULT_TYPE) -> None:
    """Handle an ask-mode approval button: relay the decision, then resume driving
    the run (another approval, more work, or the final reply)."""
    query = update.callback_query
    await query.answer()
    decision = query.data.split(":", 1)[1]
    chat_id = query.message.chat.id
    await safe_edit(query.message, "⚙️ Working...")  # drop the buttons while it runs
    try:
        snap = await api_post("/telegram/confirm", {"chat_id": chat_id, "decision": decision}, timeout=60)
    except Exception as exc:
        log.warning("/telegram/confirm failed: %r", exc)
        await safe_edit(query.message, describe(exc))
        return
    await drive(chat_id, query.message, snap)


async def do_link(update: Update, code: str) -> None:
    try:
        result = await api_post("/telegram/link/verify", {"code": code.strip(), **chat_meta(update)})
    except Exception as exc:
        log.warning("/link failed: %r", exc)
        await update.message.reply_text(f"Link failed. {describe(exc)}")
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
            "Usage: /link <code> - get the code in Pilot → Settings → Connect Telegram."
        )
        return
    await do_link(update, ctx.args[0])


async def cmd_projects(update: Update, ctx: ContextTypes.DEFAULT_TYPE) -> None:
    meta = chat_meta(update)
    try:
        result = await api_get("/telegram/projects", chat_id=meta["chat_id"])
    except Exception as exc:
        log.warning("/projects failed: %r", exc)
        await update.message.reply_text(f"Could not load projects. {describe(exc)}")
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
        log.warning("project select failed: %r", exc)
        await query.edit_message_text(f"Could not select that project. {describe(exc)}")
        return
    if result.get("cleared"):
        await query.edit_message_text("Switched to chat mode (no project). Send me anything.")
    else:
        await query.edit_message_text(
            f"Working in: {result.get('name')}\n{result.get('path')}\n\n"
            "Send a prompt or a voice note - I'll work on the files directly.\n"
            "Mode: /mode plan|ask|auto (ask pauses for your approval)."
        )


async def cmd_chat(update: Update, ctx: ContextTypes.DEFAULT_TYPE) -> None:
    try:
        await api_post("/telegram/select", {"chat_id": chat_meta(update)["chat_id"], "project_id": None})
    except Exception as exc:
        log.warning("/chat failed: %r", exc)
        await update.message.reply_text(describe(exc))
        return
    await update.message.reply_text("Chat mode (no project). Ask me anything.")


async def cmd_status(update: Update, ctx: ContextTypes.DEFAULT_TYPE) -> None:
    meta = chat_meta(update)
    try:
        result = await api_get("/telegram/status", chat_id=meta["chat_id"])
    except Exception as exc:
        log.warning("/status failed: %r", exc)
        await update.message.reply_text(f"Could not read status. {describe(exc)}")
        return
    if not result.get("authorized"):
        await update.message.reply_text("Not linked. Send /link <code> to connect this chat.")
        return
    project = result.get("project") or "none (chat mode)"
    await update.message.reply_text(f"Project: {project}\nMode: {result.get('mode', 'auto')}")


async def cmd_mode(update: Update, ctx: ContextTypes.DEFAULT_TYPE) -> None:
    mode = ctx.args[0].lower() if ctx.args else ""
    if mode not in ("plan", "ask", "auto"):
        await update.message.reply_text(
            "Usage: /mode plan|ask|auto\n"
            "• plan - read-only; it writes a plan, changes nothing\n"
            "• ask - pauses for your approval before each file/command\n"
            "• auto - runs straight through and applies changes"
        )
        return
    try:
        await api_post("/telegram/mode", {"chat_id": chat_meta(update)["chat_id"], "mode": mode})
    except Exception as exc:
        log.warning("/mode failed: %r", exc)
        await update.message.reply_text(describe(exc))
        return
    await update.message.reply_text(f"Mode set to {mode}.")


HELP_TEXT = (
    "Local Pilot - remote agent\n\n"
    "Commands:\n"
    "/projects - pick a project to work in\n"
    "/chat - no-project chat mode\n"
    "/status - show project + mode\n"
    "/mode plan|ask|auto - set how it runs\n"
    "   • plan: read-only, writes a plan\n"
    "   • ask: pauses for your approval (buttons)\n"
    "   • auto: runs straight through, applies changes\n"
    "/clear - forget this chat's history, start a fresh thread\n"
    "/link <code> - link this chat (code from Pilot → Settings)\n"
    "/help - this message\n\n"
    "No project selected → I just chat. Pick a project and I work on its files "
    "directly. Send text or a voice note. Commands still work while I'm working."
)


async def cmd_help(update: Update, ctx: ContextTypes.DEFAULT_TYPE) -> None:
    await update.message.reply_text(HELP_TEXT)


async def cmd_clear(update: Update, ctx: ContextTypes.DEFAULT_TYPE) -> None:
    chat_id = update.effective_chat.id
    _poll_gen[chat_id] = _poll_gen.get(chat_id, 0) + 1  # stop any live progress loop
    try:
        await api_post("/telegram/clear", {"chat_id": chat_id})
    except Exception as exc:
        log.warning("/clear failed: %r", exc)
        await update.message.reply_text(describe(exc))
        return
    await update.message.reply_text("Cleared - fresh thread, history forgotten.")


async def handle_text(update: Update, ctx: ContextTypes.DEFAULT_TYPE) -> None:
    text = update.message.text
    if not text:
        return
    status = await update.message.reply_text("⚙️ Working...")
    await send_to_agent(update, text, status)


async def handle_voice(update: Update, ctx: ContextTypes.DEFAULT_TYPE) -> None:
    voice = update.message.voice
    if not voice:
        return
    status = await update.message.reply_text("Transcribing...")
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
        log.warning("voice transcription failed: %r", exc)
        await safe_edit(status, f"Transcription failed. {describe(exc)}")
        return
    if not transcript.strip():
        await safe_edit(status, "Could not understand the voice message.")
        return
    await safe_edit(status, f"You said: {transcript}\n\nWorking...")
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
    log.info("Waiting for a Telegram bot token - set it in Pilot → Settings.")
    while True:
        token, enabled = fetch_token()
        if token and enabled:
            return token
        time.sleep(5)


TOKEN_POLL_INTERVAL = 3.0  # seconds between checks for a token change in Settings

# Strong refs to the running token watchers, so they are not garbage collected.
_watchers: set[asyncio.Task] = set()

# Set by watch_token so main() can tell an intentional rebind from a shutdown.
# run_polling also returns on SIGTERM/SIGINT, and without this flag the loop
# would restart polling and make the bridge unkillable by anything but SIGKILL.
_rebind = False


async def watch_token(app: Application, token: str) -> None:
    """Stop polling as soon as Settings swaps or disables the bot token, so main()
    rebinds to the new bot. Without this the bridge would keep polling the bot it
    started with and a token change in the UI would appear to do nothing."""
    global _rebind
    while True:
        await asyncio.sleep(TOKEN_POLL_INTERVAL)
        if not app.running:
            return  # this app is gone; main() has already moved on
        try:
            config = await api_get("/telegram/config")
        except Exception:
            continue  # backend restarting: keep the current bot alive
        current, enabled = config.get("bot_token", ""), bool(config.get("enabled"))
        if not enabled:
            log.info("Telegram disabled in Settings. Stopping the bridge.")
        elif current and current != token:
            log.info("Bot token changed in Settings. Rebinding to the new bot.")
        else:
            continue
        _rebind = True
        app.stop_running()
        return


async def on_error(update: object, ctx: ContextTypes.DEFAULT_TYPE) -> None:
    """Log a failed update as one line instead of a traceback."""
    if isinstance(ctx.error, Conflict):
        log.error("Another process is polling this bot token - stop the other bridge.")
        return
    log.error("Handler error: %s", ctx.error)


def _warm_whisper() -> None:
    try:
        warmup()
        log.info("Voice transcription model ready.")
    except Exception as exc:
        log.warning("Could not preload the transcription model: %r", exc)


def build_app(token: str) -> Application:
    """An Application with every handler wired, plus the token watcher."""

    async def post_init(app: Application) -> None:
        # A plain task, not app.create_task: PTB warns about tasks made before the
        # app is running, and the watcher already stops itself when the app does.
        task = asyncio.create_task(watch_token(app, token))
        _watchers.add(task)
        task.add_done_callback(_watchers.discard)
        # Load the Whisper model now, off the loop, so the first voice note is
        # transcribed in seconds instead of waiting on a cold model load.
        asyncio.get_running_loop().run_in_executor(None, _warm_whisper)

    # concurrent_updates: handle each update in its own task so slash commands
    # (e.g. /status, /mode) still respond while a run is polling for progress.
    app = (
        ApplicationBuilder()
        .token(token)
        .concurrent_updates(True)
        .post_init(post_init)
        .build()
    )
    app.add_handler(CommandHandler("start", cmd_start))
    app.add_handler(CommandHandler("help", cmd_help))
    app.add_handler(CommandHandler("clear", cmd_clear))
    app.add_handler(CommandHandler("link", cmd_link))
    app.add_handler(CommandHandler("projects", cmd_projects))
    app.add_handler(CommandHandler("chat", cmd_chat))
    app.add_handler(CommandHandler("status", cmd_status))
    app.add_handler(CommandHandler("mode", cmd_mode))
    app.add_handler(CallbackQueryHandler(on_pick, pattern=r"^pick:"))
    app.add_handler(CallbackQueryHandler(on_confirm, pattern=r"^cf:"))
    app.add_handler(MessageHandler(filters.TEXT & ~filters.COMMAND, handle_text))
    app.add_handler(MessageHandler(filters.VOICE, handle_voice))
    app.add_error_handler(on_error)
    return app


def main() -> None:
    """Serve the bot Settings currently names, rebinding whenever it changes."""
    global _rebind
    while True:
        token = wait_for_token()
        _rebind = False
        try:
            app = build_app(token)
            log.info("Bot started. Polling...")
            # close_loop=False: the event loop is reused when we rebind, so it
            # must survive run_polling returning.
            app.run_polling(close_loop=False)
        except InvalidToken:
            # A polling Conflict never lands here: PTB retries that internally and
            # reports it through the error handler.
            log.error("Telegram rejected this token. Waiting for a new one in Settings.")
            time.sleep(5)
            continue
        if not _rebind:
            log.info("Shutting down.")  # Ctrl-C or SIGTERM, not a token change
            return
        log.info("Polling stopped. Re-reading the token from Settings.")


if __name__ == "__main__":
    main()
