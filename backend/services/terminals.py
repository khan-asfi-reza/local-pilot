"""PTY-backed terminals for the Code IDE.

One real shell per terminal id, running in the project directory. Terminals are
kept in a process-wide registry and survive a websocket disconnect, so reloading
the page reattaches to the same shell (and to anything still running in it),
the way VS Code's integrated terminal does.
"""

import asyncio
import errno
import fcntl
import os
import shutil
import signal
import struct
import sys
import termios

SCROLLBACK = 128 * 1024  # bytes of output replayed to a (re)attaching client

_terms: dict[str, "Terminal"] = {}


def supported() -> bool:
    return sys.platform != "win32"


def default_shell() -> str:
    shell = os.environ.get("SHELL")
    if shell and os.path.exists(shell):
        return shell
    for candidate in ("zsh", "bash", "sh"):
        found = shutil.which(candidate)
        if found:
            return found
    return "/bin/sh"


def _set_size(fd: int, cols: int, rows: int) -> None:
    try:
        fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", rows, cols, 0, 0))
    except OSError:
        pass


class Terminal:
    """A shell attached to a pty master fd, streamed to at most one client."""

    def __init__(self, tid: str, cwd: str, cols: int = 80, rows: int = 24) -> None:
        import pty  # unix only; imported here so Windows can still load the module

        self.id = tid
        self.cwd = cwd
        self.buffer = bytearray()
        self.client = None  # the attached websocket, if any
        self.exit_code: int | None = None
        self._loop = asyncio.get_running_loop()
        self._reading = False

        shell = default_shell()
        pid, fd = pty.fork()
        if pid == 0:  # child: become the shell, never return
            try:
                os.chdir(cwd)
            except OSError:
                pass
            env = dict(os.environ, TERM="xterm-256color", COLORTERM="truecolor")
            env.pop("LINES", None)
            env.pop("COLUMNS", None)
            try:
                os.execvpe(shell, [shell, "-i"], env)
            except Exception:
                os._exit(1)
        self.pid = pid
        self.fd = fd
        os.set_blocking(fd, False)
        _set_size(fd, cols, rows)
        self._start_reading()

    def _start_reading(self) -> None:
        if self._reading:
            return
        self._loop.add_reader(self.fd, self._on_readable)
        self._reading = True

    def _stop_reading(self) -> None:
        if not self._reading:
            return
        try:
            self._loop.remove_reader(self.fd)
        except Exception:
            pass
        self._reading = False

    def _on_readable(self) -> None:
        try:
            data = os.read(self.fd, 64 * 1024)
        except OSError as exc:
            if exc.errno in (errno.EAGAIN, errno.EWOULDBLOCK):  # spurious wakeup
                return
            data = b""  # EIO: the shell exited and closed the slave side
        if not data:
            self._reap()
            return
        self.buffer.extend(data)
        if len(self.buffer) > SCROLLBACK:
            del self.buffer[: len(self.buffer) - SCROLLBACK]
        client = self.client
        if client is not None:
            self._loop.create_task(self._send(client, data))

    async def _send(self, client, data: bytes) -> None:
        try:
            await client.send_bytes(data)
        except Exception:
            if self.client is client:
                self.client = None

    def _reap(self) -> None:
        self._stop_reading()
        try:
            _, status = os.waitpid(self.pid, os.WNOHANG)
            self.exit_code = os.waitstatus_to_exitcode(status)
        except Exception:
            self.exit_code = self.exit_code if self.exit_code is not None else 0
        try:
            os.close(self.fd)
        except OSError:
            pass
        _terms.pop(self.id, None)
        client = self.client
        if client is not None:
            self._loop.create_task(self._notify_exit(client))

    async def _notify_exit(self, client) -> None:
        try:
            await client.send_text('{"type":"exit"}')
        except Exception:
            pass

    def write(self, data: str) -> None:
        if self.exit_code is not None:
            return
        try:
            os.write(self.fd, data.encode())
        except OSError:
            pass

    def resize(self, cols: int, rows: int) -> None:
        if self.exit_code is None:
            _set_size(self.fd, max(cols, 2), max(rows, 1))

    def kill(self) -> None:
        if self.exit_code is not None:
            return
        try:
            os.killpg(os.getpgid(self.pid), signal.SIGHUP)
        except OSError:
            try:
                os.kill(self.pid, signal.SIGKILL)
            except OSError:
                pass


def get(tid: str) -> Terminal | None:
    return _terms.get(tid)


def create(tid: str, cwd: str, cols: int, rows: int) -> Terminal:
    """Return the live terminal for tid, or start a fresh shell in cwd."""
    existing = _terms.get(tid)
    if existing is not None and existing.exit_code is None:
        existing.resize(cols, rows)
        return existing
    term = Terminal(tid, cwd, cols, rows)
    _terms[tid] = term
    return term


def kill(tid: str) -> bool:
    term = _terms.get(tid)
    if term is None:
        return False
    term.kill()
    return True


def list_ids(cwd: str) -> list[str]:
    return [t.id for t in _terms.values() if t.cwd == cwd and t.exit_code is None]
