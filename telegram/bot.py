"""Telegram voice bot.

Long-polling bot that accepts text and voice messages, forwards them to the
Go harness, and streams the reply back. Voice messages are transcribed via
faster-whisper before being sent to the agent.

Requires:
  TELEGRAM_BOT_TOKEN  - Bot token from @BotFather
  HARNESS_URL         - Full /run endpoint (default http://localhost:9000/run)

Run:  python bot.py
"""

from __future__ import annotations

import asyncio
import json
import logging
import os
import tempfile
from typing import Any

import httpx
from telegram import Update
from telegram.ext import (
    ApplicationBuilder,
    CommandHandler,
    ContextTypes,
    MessageHandler,
    filters,
)

from transcribe import transcribe

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger(__name__)

TELEGRAM_BOT_TOKEN = os.getenv("TELEGRAM_BOT_TOKEN", "")
HARNESS_URL = os.getenv("HARNESS_URL", "http://localhost:9000/run")

# Serialise harness calls so only one turn runs at a time (matches Go server runMu).
_run_lock = asyncio.Lock()

# Per-chat message history (in-memory). Each value is a list of harness messages.
_chat_history: dict[int, list[dict[str, Any]]] = {}


def _get_history(chat_id: int) -> list[dict[str, Any]]:
    if chat_id not in _chat_history:
        _chat_history[chat_id] = []
    return _chat_history[chat_id]


def _build_payload(messages: list[dict[str, Any]]) -> dict[str, Any]:
    return {
        "messages": messages,
        "allowed_tools": ["code_run", "web_search"],
        "working_directory": "",
    }


def _call_harness_sync(messages: list[dict[str, Any]]) -> str:
    """Send messages to the harness synchronously and collect the full reply text."""
    payload = _build_payload(messages)
    parts: list[str] = []
    with httpx.Client(timeout=None) as client:
        with client.stream("POST", HARNESS_URL, json=payload) as resp:
            resp.raise_for_status()
            for line in resp.iter_lines():
                if not line.strip():
                    continue
                ev = json.loads(line)
                if ev.get("type") == "text":
                    parts.append(ev.get("content", ""))
                elif ev.get("type") in ("done", "error"):
                    break
    return "".join(parts)


async def _call_harness(messages: list[dict[str, Any]]) -> str:
    """Run the synchronous harness call in a thread to avoid blocking the event loop."""
    loop = asyncio.get_running_loop()
    return await loop.run_in_executor(None, _call_harness_sync, messages)


def _download_file_sync(file_path: str, dest: str) -> None:
    """Download a Telegram file synchronously via httpx."""
    with httpx.Client(timeout=60) as client:
        resp = client.get(file_path)
        resp.raise_for_status()
        with open(dest, "wb") as f:
            f.write(resp.content)


async def _transcribe_async(ogg_path: str) -> str:
    """Run the synchronous whisper transcription in a thread."""
    loop = asyncio.get_running_loop()
    return await loop.run_in_executor(None, transcribe, ogg_path)


async def _cap_history(chat_id: int) -> None:
    """Trim per-chat history to the last 20 exchanges (40 messages)."""
    history = _chat_history.get(chat_id, [])
    if len(history) > 40:
        _chat_history[chat_id] = history[-40:]


# --- Handlers ---

async def cmd_start(update: Update, ctx: ContextTypes.DEFAULT_TYPE) -> None:
    await update.message.reply_text(
        "Send me a text message or a voice note and I will forward it to the agent."
    )


async def handle_text(update: Update, ctx: ContextTypes.DEFAULT_TYPE) -> None:
    chat_id = update.effective_chat.id
    user_text = update.message.text
    if not user_text:
        return

    history = _get_history(chat_id)
    history.append({"role": "user", "content": user_text})

    status_msg = await update.message.reply_text("Thinking...")
    try:
        async with _run_lock:
            reply = await _call_harness(history)
    except Exception as exc:
        reply = f"Harness error: {exc}"

    history.append({"role": "assistant", "content": reply})
    await _cap_history(chat_id)

    await status_msg.edit_text(reply)


async def handle_voice(update: Update, ctx: ContextTypes.DEFAULT_TYPE) -> None:
    chat_id = update.effective_chat.id
    voice = update.message.voice
    if not voice:
        return

    status_msg = await update.message.reply_text("Transcribing voice...")
    try:
        tg_file = await ctx.bot.get_file(voice.file_id)
        with tempfile.NamedTemporaryFile(suffix=".ogg", delete=False) as tmp:
            ogg_path = tmp.name
        try:
            _download_file_sync(tg_file.file_path, ogg_path)
            transcript = await _transcribe_async(ogg_path)
        finally:
            if os.path.exists(ogg_path):
                os.unlink(ogg_path)
    except Exception as exc:
        await status_msg.edit_text(f"Transcription failed: {exc}")
        return

    if not transcript.strip():
        await status_msg.edit_text("Could not understand the voice message.")
        return

    await status_msg.edit_text(f"You said: {transcript}\n\nThinking...")

    history = _get_history(chat_id)
    history.append({"role": "user", "content": transcript})

    try:
        async with _run_lock:
            reply = await _call_harness(history)
    except Exception as exc:
        reply = f"Harness error: {exc}"

    history.append({"role": "assistant", "content": reply})
    await _cap_history(chat_id)

    await status_msg.edit_text(reply)


def main() -> None:
    if not TELEGRAM_BOT_TOKEN:
        raise SystemExit("Set TELEGRAM_BOT_TOKEN env var first.")

    app = ApplicationBuilder().token(TELEGRAM_BOT_TOKEN).build()
    app.add_handler(CommandHandler("start", cmd_start))
    app.add_handler(MessageHandler(filters.TEXT & ~filters.COMMAND, handle_text))
    app.add_handler(MessageHandler(filters.VOICE, handle_voice))

    log.info("Bot started. Polling...")
    app.run_polling()


if __name__ == "__main__":
    main()
