# Transcription via faster-whisper.
# ffmpeg must be installed and on PATH.
# The Whisper model downloads on first run (~150 MB for "base").
#
# Usage:
#   from transcribe import transcribe
#   text = transcribe("/path/to/voice.ogg")

import os
import subprocess
import tempfile

from faster_whisper import WhisperModel

whisper_model: WhisperModel | None = None


def get_model() -> WhisperModel:
    global whisper_model
    if whisper_model is None:
        whisper_model = WhisperModel("base", device="cpu", compute_type="int8")
    return whisper_model


def warmup() -> None:
    """Load the model ahead of the first voice note. A cold start downloads
    ~150 MB and takes minutes, during which the chat shows only "Transcribing..."."""
    get_model()


def transcribe(ogg_path: str) -> str:
    """Transcribe an OGG voice file to text.

    Converts OGG to WAV via ffmpeg (must be installed), then runs the base
    Whisper model. The model is loaded once on first call.
    """
    with tempfile.NamedTemporaryFile(suffix=".wav", delete=False) as tmp:
        wav_path = tmp.name
    try:
        subprocess.run(
            ["ffmpeg", "-y", "-i", ogg_path, "-ar", "16000", "-ac", "1", wav_path],
            check=True,
            capture_output=True,
        )
        segments, info = get_model().transcribe(wav_path)
        parts = [seg.text for seg in segments]
        return " ".join(parts).strip()
    finally:
        if os.path.exists(wav_path):
            os.unlink(wav_path)
