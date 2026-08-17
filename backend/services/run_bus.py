"""A tiny in-process pub/sub for agent run events, keyed by project root.

Every full-access run (web Code IDE or Telegram) publishes its harness events
here as they stream. Passive watchers — the web Code IDE showing a project that a
Telegram run is editing — subscribe to the same root and render the run live.

The initiator of a run does NOT subscribe: it already gets the events from the
stream it opened. The bus is only for everyone else watching the same project."""

from __future__ import annotations

import asyncio
import os

# root (realpath) -> set of subscriber queues
_subscribers: dict[str, set[asyncio.Queue]] = {}


def _key(root: str) -> str:
    return os.path.realpath(root) if root else ""


def publish(root: str, event: dict) -> None:
    """Fan an event out to everyone watching this root. Never blocks the run:
    a full subscriber queue just drops the event for that watcher."""
    subs = _subscribers.get(_key(root))
    if not subs:
        return
    for q in list(subs):
        try:
            q.put_nowait(event)
        except asyncio.QueueFull:
            pass


def subscribe(root: str) -> asyncio.Queue:
    q: asyncio.Queue = asyncio.Queue(maxsize=1000)
    _subscribers.setdefault(_key(root), set()).add(q)
    return q


def unsubscribe(root: str, q: asyncio.Queue) -> None:
    subs = _subscribers.get(_key(root))
    if not subs:
        return
    subs.discard(q)
    if not subs:
        _subscribers.pop(_key(root), None)
