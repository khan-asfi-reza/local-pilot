"""Per-project conversation sessions, stored the same way the terminal stores
them: JSON files in <project>/.pilot/sessions/<id>.json. Web Code and the
terminal share the format, so either can list and resume the other's sessions.
Bare-minimum message shape matches harness model.Message (role, content, ...)."""

import json
import os
import re
import secrets
import time

_SESSIONS = os.path.join(".pilot", "sessions")

# Session ids are 6-byte hex (see new_id); allow the terminal's range too. Any
# id outside this shape is rejected so it can never traverse out of _dir().
_SID_RE = re.compile(r"^[a-f0-9]{6,64}$")


def _dir(root: str) -> str:
    return os.path.join(root, _SESSIONS)


def is_valid_id(sid: str) -> bool:
    return isinstance(sid, str) and bool(_SID_RE.match(sid))


def _safe_path(root: str, sid: str) -> str:
    if not is_valid_id(sid):
        raise ValueError("invalid session id")
    base = os.path.realpath(_dir(root))
    full = os.path.realpath(os.path.join(base, sid + ".json"))
    if full != base and not full.startswith(base + os.sep):
        raise ValueError("invalid session id")
    return full


def _now() -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def new_id() -> str:
    return secrets.token_hex(6)  # matches the terminal's 6-byte hex ids


def _title(messages: list[dict]) -> str:
    for m in messages:
        if m.get("role") == "user" and (m.get("content") or "").strip():
            t = m["content"].strip()
            return t[:60]
    return ""


def list_sessions(root: str) -> list[dict]:
    """Session summaries for a project, newest first."""
    out: list[dict] = []
    try:
        entries = os.listdir(_dir(root))
    except OSError:
        return out
    for name in entries:
        if not name.endswith(".json"):
            continue
        try:
            with open(os.path.join(_dir(root), name)) as f:
                s = json.load(f)
        except Exception:
            continue
        if s.get("id"):
            out.append({"id": s["id"], "title": s.get("title", ""),
                        "updated_at": s.get("updated_at", ""), "model": s.get("model")})
    out.sort(key=lambda s: s.get("updated_at", ""), reverse=True)
    return out


def load(root: str, sid: str) -> dict | None:
    try:
        with open(_safe_path(root, sid)) as f:
            return json.load(f)
    except Exception:
        return None


def delete(root: str, sid: str) -> bool:
    """Remove a session file. Returns False if it was missing or invalid."""
    try:
        os.remove(_safe_path(root, sid))
        return True
    except (OSError, ValueError):
        return False


def save(root: str, sid: str, messages: list[dict], model: str | None = None, mode: str = "ask") -> dict:
    """Write (or update) a session file, matching the terminal's schema."""
    path = _safe_path(root, sid)
    existing = load(root, sid) or {}
    created = existing.get("created_at") or _now()
    session = {
        "id": sid,
        "model": model or existing.get("model"),
        "mode": mode,
        "title": existing.get("title") or _title(messages),
        "messages": messages,
        "created_at": created,
        "updated_at": _now(),
    }
    os.makedirs(_dir(root), exist_ok=True)
    tmp = path + ".tmp"
    with open(tmp, "w") as f:
        json.dump(session, f, indent=2)
    os.replace(tmp, path)
    return session
