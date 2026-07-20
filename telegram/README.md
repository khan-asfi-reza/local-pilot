# Telegram Voice Bot

A Telegram bot that accepts text and voice messages, forwards them to the local
Go harness agent, and replies with the agent's response.

## Prerequisites

- Python 3.10+
- [ffmpeg](https://ffmpeg.org/) installed and on PATH
- A Telegram bot token (create one via [@BotFather](https://t.me/BotFather))
- The Go harness server running on port 9000

## Setup

```bash
cd telegram
python -m venv .venv
source .venv/bin/activate   # Windows: .venv\Scripts\activate
pip install -r requirements.txt
cp .env.example .env        # then fill in your token
export TELEGRAM_BOT_TOKEN="your-token-here"
```

The base Whisper model (~150 MB) downloads automatically on first transcription.

## Running

```bash
python bot.py
```

## Notes

- The harness server serializes runs with a global mutex (`runMu`), so only one
  agent turn executes at a time across all chats.
- Voice messages are transcribed via `faster-whisper` (base model) after OGG-to-WAV
  conversion through ffmpeg.
- Per-chat history is kept in memory (last 20 exchanges). Restarting the bot clears it.
