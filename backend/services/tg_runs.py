"""Interactive, progress-reporting Telegram agent runs.

The web Code IDE streams harness events straight to the browser, but the Telegram
bridge is request/reply. To show live progress (not a frozen "Working...") and to
support ask-mode approvals that arrive as a later button tap, each run is driven
by a background task that accumulates a snapshot the bot polls:

  start()    -> launch the run, return immediately
  progress() -> latest snapshot: status, activity lines, an approval prompt, or
                the final reply
  resume()   -> deliver an ask-mode decision (Approve/Decline); the task carries on

The run's events are also mirrored to run_bus, so the web Code IDE shows the same
run live when the project is open there."""

from __future__ import annotations

import asyncio
from dataclasses import dataclass, field

import httpx

from services.agent_runner import run_full_access
from services.harness_client import harness_base

# One in-flight run per chat.
_runs: dict[int, "TgRun"] = {}

MAX_DIFF_LINES = 24
MAX_ACTIVITY = 12  # cap the activity list carried in a snapshot


def _ascii(s: str) -> str:
    """Normalise punctuation the harness/model may emit to plain ASCII, so no
    em/en-dashes or ellipses reach Telegram."""
    return (s or "").replace("—", "-").replace("–", "-").replace("−", "-").replace("…", "...")


@dataclass
class TgRun:
    chat_id: int
    root: str
    sid: str = ""
    status: str = "running"  # running | paused | done
    text: list[str] = field(default_factory=list)
    activity: list[str] = field(default_factory=list)
    buf: str = ""  # partial text line not yet terminated by a newline
    current: str = ""  # the action in flight right now, for a live status line
    pending_id: str | None = None
    pending_summary: str = ""
    pending_diff: dict | None = None
    pending_tool: str = ""
    error: str | None = None
    task: asyncio.Task | None = None


def _fmt_diff(diff: dict | None) -> str:
    if not diff:
        return ""
    lines: list[str] = []
    for hunk in diff.get("hunks", []):
        for ln in hunk.get("lines", []):
            prefix = {"add": "+", "remove": "-"}.get(ln.get("op"), " ")
            lines.append(prefix + ln.get("text", ""))
            if len(lines) >= MAX_DIFF_LINES:
                lines.append("... (diff truncated)")
                return "\n".join(lines)
    return "\n".join(lines)


def _fmt_activity(event: dict) -> str:
    tool = event.get("tool", "")
    info = event.get("info", "")
    diff = event.get("diff")
    if diff and diff.get("path"):
        return f"✏️ {diff['path']} (+{diff.get('added', 0)} -{diff.get('removed', 0)})"
    label = (tool + (" " + info if info else "")).strip()
    return _ascii(f"✓ {label}") if label else ""


def _fmt_call(event: dict) -> str:
    """A short 'doing X now' line from a tool_call event."""
    tool = event.get("tool", "")
    info = event.get("info", "")
    return _ascii((tool + (" " + info if info else "")).strip())


def _step_line(line: str) -> str:
    """Turn a status-marker text line into a progress step, or '' to skip prose.

    The harness narrates progress as bracketed markers, e.g.
    '[scaffolding backend/ - django]' or '▸ t2: build the API', which the web
    Code IDE renders as cards. We surface those (not ordinary assistant prose)
    as live steps in Telegram."""
    s = line.strip()
    if not s:
        return ""
    if s.startswith("[") and s.endswith("]"):
        return _ascii("• " + s[1:-1].strip())
    if s[0] in "▸✓•":
        return _ascii(s)
    return ""


async def _run_task(run: TgRun, messages: list[dict], model: str | None, mode: str) -> None:
    try:
        async for event in run_full_access(
            run.root, messages, model=model, mode=mode, session_id=run.sid or None
        ):
            etype = event.get("type")
            if etype == "session":
                run.sid = event.get("id", run.sid)
            elif etype == "text":
                chunk = event.get("content", "")
                run.text.append(chunk)
                # Pull status-marker lines out of the stream so they show as live
                # steps (the same info the web Code IDE renders as cards).
                run.buf += chunk
                while "\n" in run.buf:
                    line, run.buf = run.buf.split("\n", 1)
                    step = _step_line(line)
                    if step:
                        run.activity.append(step)
            elif etype == "reasoning":
                pass
            elif etype == "tool_call":
                label = _fmt_call(event)
                run.current = label
                run.status = "running"
            elif etype == "tool_result":
                note = _fmt_activity(event)
                if note:
                    run.activity.append(note)
                run.current = ""
            elif etype == "confirm":
                run.pending_id = event.get("id")
                run.pending_summary = event.get("summary", "")
                run.pending_diff = event.get("diff")
                run.pending_tool = event.get("tool", "")
                run.status = "paused"
            elif etype == "error":
                run.error = event.get("message", "")
    except asyncio.CancelledError:
        raise
    except Exception as exc:
        run.error = str(exc)
    finally:
        run.status = "done"


def _final_reply(run: TgRun) -> str:
    raw = "".join(run.text)
    # Keep the model's prose; drop the status-marker lines (shown as steps below).
    body = "\n".join(ln for ln in raw.splitlines() if not _step_line(ln)).strip()
    if run.error:
        body = (body + f"\n\n[error: {run.error}]").strip()
    if run.activity:
        steps = "\n".join(list(dict.fromkeys(run.activity))[-20:])  # de-dupe, keep order
        body = (body + "\n\nsteps:\n" + steps).strip() if body else steps
    return _ascii(body) or "(done, no output)"


def _snapshot(run: TgRun) -> dict:
    snap: dict = {
        "status": run.status,
        "pending": run.status == "paused",
        "activity": run.activity[-MAX_ACTIVITY:],
        "current": run.current,
        "session_id": run.sid,
    }
    if run.status == "paused":
        prompt = _ascii(f"🔧 {run.pending_tool or 'action'}: {run.pending_summary}".strip())
        diff_text = _fmt_diff(run.pending_diff)
        snap["confirm"] = {"prompt": prompt, "diff": diff_text}
    if run.status == "done":
        snap["reply"] = _final_reply(run)
    return snap


async def _post_confirm(pending_id: str, decision: str, feedback: str = "") -> None:
    url = harness_base() + "/confirm"
    async with httpx.AsyncClient(timeout=30) as client:
        resp = await client.post(url, json={"id": pending_id, "decision": decision, "feedback": feedback})
        resp.raise_for_status()


async def start(chat_id: int, root: str, messages: list[dict], model: str | None,
                mode: str, sid: str | None) -> dict:
    """Launch a run for a chat in the background and return immediately."""
    await cancel(chat_id)
    run = TgRun(chat_id=chat_id, root=root, sid=sid or "")
    run.task = asyncio.create_task(_run_task(run, messages, model, mode))
    _runs[chat_id] = run
    # Give the run a beat to emit its first events, so the first snapshot the bot
    # sees already has something to show rather than an empty "Working...".
    await asyncio.sleep(0.1)
    return _snapshot(run)


def progress(chat_id: int) -> dict:
    run = _runs.get(chat_id)
    if run is None:
        return {"status": "idle", "pending": False}
    return _snapshot(run)


async def resume(chat_id: int, decision: str, feedback: str = "") -> dict:
    """Deliver an ask-mode decision; the background task then carries on."""
    run = _runs.get(chat_id)
    if run is None or run.pending_id is None:
        return {"status": run.status if run else "idle", "pending": False}
    pending_id = run.pending_id
    run.pending_id = None
    run.status = "running"
    run.pending_summary = ""
    run.pending_diff = None
    run.pending_tool = ""
    try:
        await _post_confirm(pending_id, decision, feedback)
    except Exception as exc:
        run.error = str(exc)
        run.status = "done"
    await asyncio.sleep(0.1)
    return _snapshot(run)


async def cancel(chat_id: int) -> None:
    """Drop a chat's run, unblocking the harness if it was paused."""
    run = _runs.pop(chat_id, None)
    if run is None:
        return
    if run.pending_id:
        try:
            await _post_confirm(run.pending_id, "decline", "superseded by a new message")
        except Exception:
            pass
    if run.task and not run.task.done():
        run.task.cancel()
        try:
            await run.task
        except (asyncio.CancelledError, Exception):
            pass
