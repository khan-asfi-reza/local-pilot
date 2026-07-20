"""Locate the shared local-pilot data directory (same rule as the Go appdir), and
give each web thread its own sandbox working directory under it."""

import os
import platform


def data_dir() -> str:
    home = os.path.expanduser("~")
    system = platform.system()
    if system == "Windows":
        base = os.getenv("LOCALAPPDATA") or os.path.join(home, "AppData", "Local")
        return os.path.join(base, "localpilot")
    if system == "Darwin":
        return os.path.join(home, ".localpilot")
    xdg = os.getenv("XDG_DATA_HOME")
    if xdg:
        return os.path.join(xdg, "localpilot")
    return os.path.join(home, ".local", "share", "localpilot")


def sandbox_for(thread_id: str) -> str:
    """Return (creating if needed) the sandbox dir a web thread runs in, so any
    files or code it produces stay under .localpilot/sandbox/<thread_id>."""
    path = os.path.join(data_dir(), "sandbox", thread_id)
    os.makedirs(path, exist_ok=True)
    return path
