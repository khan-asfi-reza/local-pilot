"""The Telegram bot process itself. It is deliberately thin - Telegram I/O and
nothing else - so what is worth testing is how it renders a run: one status
message edited in place, an approval keyboard when the agent pauses, and the
final reply."""

import asyncio
import importlib.util
import sys
import types
from pathlib import Path

import pytest

BOT_PATH = Path(__file__).resolve().parents[2] / "telegram" / "bot.py"


def _stub_telegram_sdk():
    """Minimal stand-ins for the python-telegram-bot symbols bot.py imports.

    The bot process has its own virtualenv, so the SDK is not installed
    everywhere the suite runs. Its rendering logic - status frames, approval
    keyboards, truncation - is ours and worth testing regardless, and these value
    holders are all that logic touches.
    """
    telegram = types.ModuleType("telegram")

    class InlineKeyboardButton:
        def __init__(self, text, callback_data=None):
            self.text = text
            self.callback_data = callback_data

    class InlineKeyboardMarkup:
        def __init__(self, inline_keyboard):
            self.inline_keyboard = inline_keyboard

    class Update:
        pass

    telegram.InlineKeyboardButton = InlineKeyboardButton
    telegram.InlineKeyboardMarkup = InlineKeyboardMarkup
    telegram.Update = Update

    error = types.ModuleType("telegram.error")

    class Conflict(Exception):
        pass

    class InvalidToken(Exception):
        pass

    error.Conflict = Conflict
    error.InvalidToken = InvalidToken

    ext = types.ModuleType("telegram.ext")

    class _Any:
        def __init__(self, *args, **kwargs):
            pass

        def __getattr__(self, name):
            return _Any()

    class ContextTypes:
        DEFAULT_TYPE = object

    for name in ("Application", "ApplicationBuilder", "CallbackQueryHandler",
                 "CommandHandler", "MessageHandler"):
        setattr(ext, name, _Any)
    ext.ContextTypes = ContextTypes
    ext.filters = _Any()

    telegram.error = error
    telegram.ext = ext
    return {"telegram": telegram, "telegram.error": error, "telegram.ext": ext}


def load_bot():
    """Import telegram/bot.py by path.

    The repository has a telegram/ folder, which shadows the installed
    python-telegram-bot package whenever the repo root is on sys.path, so the
    root is taken off the path for the duration. The local voice transcriber is
    stubbed too: it pulls in a large speech model that has no place in a unit
    test.
    """
    if "transcribe" not in sys.modules:
        stub = types.ModuleType("transcribe")
        stub.transcribe = lambda path: "transcribed text"
        stub.warmup = lambda: None
        sys.modules["transcribe"] = stub

    repo_root = str(BOT_PATH.parents[1])
    saved_path = list(sys.path)
    saved_modules = {k: sys.modules.pop(k) for k in list(sys.modules)
                     if k == "telegram" or k.startswith("telegram.")}
    sys.path[:] = [p for p in sys.path if p not in ("", ".", repo_root)]
    try:
        import telegram  # noqa: F401  (the real SDK, when it is installed)
    except ImportError:
        sys.modules.update(_stub_telegram_sdk())
    try:
        spec = importlib.util.spec_from_file_location("pilot_telegram_bot", BOT_PATH)
        module = importlib.util.module_from_spec(spec)
        sys.modules[spec.name] = module
        spec.loader.exec_module(module)
        return module
    finally:
        sys.path[:] = saved_path
        for key in [k for k in list(sys.modules) if k == "telegram" or k.startswith("telegram.")]:
            del sys.modules[key]
        sys.modules.update(saved_modules)


bot = load_bot()


class FakeMessage:
    """Stands in for a Telegram message: records edits and replies, and can be
    told to reject an edit the way Telegram does for identical text."""

    def __init__(self, reject_edits=False):
        self.edits = []
        self.replies = []
        self.markups = []
        self.reject_edits = reject_edits

    async def edit_text(self, text, reply_markup=None):
        if self.reject_edits:
            raise RuntimeError("message is not modified")
        self.edits.append(text)
        self.markups.append(reply_markup)

    async def reply_text(self, text, reply_markup=None):
        self.replies.append(text)


class FakeUser:
    def __init__(self):
        self.id = 42
        self.username = "khan"
        self.full_name = "Khan Asfi Reza"


class FakeChat:
    id = 909


class FakeUpdate:
    effective_chat = FakeChat()
    effective_user = FakeUser()


def test_chat_meta_carries_the_fields_the_backend_keys_a_link_on():
    meta = bot.chat_meta(FakeUpdate())
    assert meta == {"chat_id": 909, "tg_user_id": 42, "tg_username": "khan",
                    "display_name": "Khan Asfi Reza"}


def test_a_long_reply_is_truncated_to_telegrams_limit():
    msg = FakeMessage()
    asyncio.run(bot.safe_edit(msg, "x" * 9000))

    assert len(msg.edits[0]) <= bot.MAX_MESSAGE + len("\n... (truncated)")
    assert msg.edits[0].endswith("... (truncated)")


def test_an_empty_reply_still_says_something():
    msg = FakeMessage()
    asyncio.run(bot.safe_edit(msg, ""))
    assert msg.edits[0] == "(no response)"


def test_a_rejected_edit_falls_back_to_a_fresh_reply():
    msg = FakeMessage(reject_edits=True)
    asyncio.run(bot.safe_edit(msg, "hello"))

    assert msg.edits == []
    assert msg.replies == ["hello"]


def test_an_unchanged_progress_frame_is_not_re_sent():
    """Telegram rate-limits edits, so an identical frame must be skipped."""
    msg = FakeMessage()

    shown = asyncio.run(bot.edit_progress(msg, "same", "same"))
    assert shown == "same" and msg.edits == []

    shown = asyncio.run(bot.edit_progress(msg, "new", "same"))
    assert shown == "new" and msg.edits == ["new"]


def test_progress_text_shows_the_action_in_flight_and_recent_steps():
    text = bot._progress_text({"current": "write_file app/main.py",
                               "activity": ["- scaffolding backend", "check app/main.py"]})

    lines = text.splitlines()
    assert lines[0].endswith("Working...")
    assert lines[1] == "> write_file app/main.py".replace(">", "→")
    assert lines[2:] == ["- scaffolding backend", "check app/main.py"]


def test_an_approval_prompt_shows_the_diff_and_three_buttons():
    msg = FakeMessage()
    snap = {"confirm": {"prompt": "edit_file: edit calc.py", "diff": "-return a - b\n+return a + b"}}

    asyncio.run(bot.show_confirm(msg, snap))

    assert "edit calc.py" in msg.edits[0]
    assert "-return a - b" in msg.edits[0]
    assert msg.edits[0].endswith("Approve?")
    buttons = msg.markups[0].inline_keyboard[0]
    assert [b.callback_data for b in buttons] == ["cf:approve", "cf:approve_always", "cf:decline"]


def test_drive_stops_on_a_pause_and_asks_for_approval():
    msg = FakeMessage()
    snap = {"status": "paused", "confirm": {"prompt": "shell_run: pytest", "diff": ""}}

    asyncio.run(bot.drive(1, msg, snap))

    assert "pytest" in msg.edits[-1]
    assert msg.markups[-1] is not None


def test_drive_renders_the_final_reply_when_the_run_is_done():
    msg = FakeMessage()

    asyncio.run(bot.drive(2, msg, {"status": "done", "reply": "Created app.py."}))

    assert msg.edits[-1] == "Created app.py."


def test_drive_polls_while_running_then_finishes(monkeypatch):
    msg = FakeMessage()
    snapshots = [
        {"status": "running", "current": "write_file app.py", "activity": []},
        {"status": "done", "reply": "All set."},
    ]

    async def fake_get(path, **params):
        assert path == "/telegram/progress"
        return snapshots.pop(0)

    monkeypatch.setattr(bot, "api_get", fake_get)
    monkeypatch.setattr(bot, "POLL_INTERVAL", 0.01)

    asyncio.run(bot.drive(3, msg, {"status": "running", "current": "", "activity": []}))

    assert msg.edits[-1] == "All set."
    assert any("Working" in e for e in msg.edits), "the user should see live progress, not a frozen message"


def test_a_newer_message_supersedes_an_in_flight_progress_loop(monkeypatch):
    """A second question must not have its status message overwritten by the
    previous run's poll loop."""
    msg = FakeMessage()
    monkeypatch.setattr(bot, "POLL_INTERVAL", 0.01)

    async def fake_get(path, **params):
        bot._poll_gen[4] = bot._poll_gen.get(4, 0) + 1  # a newer message arrives
        return {"status": "running", "activity": [], "current": ""}

    monkeypatch.setattr(bot, "api_get", fake_get)

    asyncio.run(bot.drive(4, msg, {"status": "running", "activity": [], "current": ""}))

    assert not any(e == "All set." for e in msg.edits)


def test_the_help_text_documents_every_command():
    for command in ["/link", "/projects", "/chat", "/status", "/mode", "/clear", "/help"]:
        assert command in bot.HELP_TEXT
