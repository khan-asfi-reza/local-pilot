"""Unified code-project registry, shared by web Code, the terminal, and Telegram.

Stored as one JSON file in the global config dir so every runtime (Python and Go)
reads/writes the same list. Bare minimum per project: a stable id, the absolute
path, a name, a source, and timestamps."""

import json
import os
import threading
import uuid as uuidlib
from datetime import datetime, timezone

from core.appdir import data_dir

_lock = threading.Lock()


def store_path() -> str:
    return os.path.join(data_dir(), "projects.json")


def _now() -> str:
    return datetime.now(timezone.utc).isoformat()


def _load() -> list[dict]:
    try:
        with open(store_path()) as f:
            data = json.load(f)
            return data if isinstance(data, list) else []
    except Exception:
        return []


def _save(items: list[dict]) -> None:
    os.makedirs(data_dir(), exist_ok=True)
    tmp = store_path() + ".tmp"
    with open(tmp, "w") as f:
        json.dump(items, f, indent=2)
    os.replace(tmp, store_path())  # atomic, so a concurrent reader never sees half a file


def list_projects() -> list[dict]:
    return sorted(_load(), key=lambda p: p.get("last_opened", ""), reverse=True)


def get(pid: str) -> dict | None:
    return next((p for p in _load() if p.get("id") == pid), None)


def upsert(path: str, name: str | None = None, source: str = "web") -> dict:
    """Register (or refresh) a project by absolute path. The id is stable per path,
    so all three entry points converge on the same record."""
    rp = os.path.realpath(path)
    with _lock:
        items = _load()
        for p in items:
            if p.get("path") == rp:
                p["last_opened"] = _now()
                if name:
                    p["name"] = name
                _save(items)
                return p
        rec = {
            "id": str(uuidlib.uuid4()),
            "path": rp,
            "name": name or os.path.basename(rp.rstrip(os.sep)) or rp,
            "source": source,
            "created_at": _now(),
            "last_opened": _now(),
        }
        items.append(rec)
        _save(items)
        return rec


def remove(pid: str) -> None:
    with _lock:
        _save([p for p in _load() if p.get("id") != pid])


def clear() -> int:
    """Forget every registered project. Only the registry is emptied — no files on
    disk are touched, so a cleared project reappears the next time it is opened."""
    with _lock:
        n = len(_load())
        _save([])
        return n
