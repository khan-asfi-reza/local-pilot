# Telegram bridge

Control Local Pilot from Telegram. The bot is a thin bridge: it does Telegram
I/O and voice transcription only, and talks to the Local Pilot **backend** over
HTTP. All auth, project selection, and agent runs happen in the backend.

The bot token is stored in the app database (set it in the web UI under
**Settings → Telegram**), not in an env file. Connecting a Telegram account is
done from the web UI's **Connect Telegram** flow.

## Prerequisites

- Python 3.10+
- [ffmpeg](https://ffmpeg.org/) on PATH (for voice notes)
- A Telegram bot token from [@BotFather](https://t.me/BotFather)
- The Local Pilot backend running (started by `pilot web`, on port 8182)

## Setup

```bash
cd telegram
python -m venv .venv
source .venv/bin/activate   # Windows: .venv\Scripts\activate
pip install -r requirements.txt
```

The base Whisper model (~150 MB) downloads automatically on first transcription.

## Running

`pilot web` launches the bot automatically. To run it standalone:

```bash
python bot.py            # BACKEND_URL defaults to http://localhost:8182
```

The bot idles until a token is set (and enabled) in **Settings → Telegram**,
then starts polling.

## Connecting a chat

1. In the web UI: **Settings → Telegram** → paste your bot token → **Connect Telegram**.
2. Tap the deep link (or send `/link <code>` to the bot).
3. `/projects` to pick a project, or `/chat` for no-project chat. Send text or a voice note.

## Commands

| Command | What it does |
|---|---|
| `/start [code]` | Greet, or link this chat when a code is passed (deep link) |
| `/link <code>` | Link this chat using a code from Settings → Connect Telegram |
| `/projects` | Pick a project to work in (tap a button) |
| `/chat` | Switch to no-project chat mode |
| `/status` | Show the linked name, selected project, and mode |
| `/mode ask\|auto` | `ask` pauses on mutating ops; `auto` runs straight through |

## Notes

- Only **linked (authorized)** chats can control the pilot. Linking requires a
  code minted in the local web UI.
- Selecting a project grants the agent **full file/shell access** to that folder.
  Default mode is `auto` (no per-action confirmation); use `/mode ask` to pause.
- Conversations persist under `<project>/.pilot/sessions` (shared with the web
  Code IDE and terminal), so a chat can continue across bot restarts.
- The harness serializes runs with a global mutex, so one agent turn runs at a
  time across all chats.
