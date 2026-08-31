import os
import threading
from typing import Generator

from sqlalchemy import event
from sqlalchemy.pool import NullPool
from sqlmodel import Session, SQLModel, create_engine

from core.appdir import data_dir

# One engine per database URL. Building a fresh engine per query (the old
# behaviour) opened a new connection pool every time, so SQLite handles piled up
# until the GC caught them and concurrent writers fought over the lock.
_engines: dict[str, object] = {}
_engines_lock = threading.Lock()


def get_database_url() -> str:
    # Keep the SQLite DB in the global local-pilot config dir (not a cwd-relative
    # file), so every entry point resolves the same database.
    url = os.getenv("DATABASE_URL")
    if url:
        return url
    os.makedirs(data_dir(), exist_ok=True)
    return f"sqlite:///{os.path.join(data_dir(), 'localpilot.db')}"


def _build_engine(url: str):
    """A SQLite engine tuned for the several processes that share this file:
    WAL so a reader never blocks the writer, a long busy timeout instead of an
    instant 'database is locked', and NullPool so no connection outlives the
    request that opened it (a pooled handle to a deleted file would be stale)."""
    engine = create_engine(
        url,
        connect_args={"check_same_thread": False, "timeout": 30},
        poolclass=NullPool,
    )
    if url.startswith("sqlite"):
        @event.listens_for(engine, "connect")
        def _pragmas(dbapi_connection, _record):  # noqa: ANN001
            cursor = dbapi_connection.cursor()
            cursor.execute("PRAGMA journal_mode=WAL")
            cursor.execute("PRAGMA busy_timeout=30000")
            cursor.execute("PRAGMA synchronous=NORMAL")
            cursor.close()
    return engine


def get_engine():
    url = get_database_url()
    engine = _engines.get(url)
    if engine is None:
        with _engines_lock:
            engine = _engines.get(url)
            if engine is None:
                engine = _build_engine(url)
                _engines[url] = engine
    return engine


def reset_engines() -> None:
    """Drop every cached engine. Tests that recreate the database file call this
    so the next query builds a fresh engine."""
    with _engines_lock:
        for engine in _engines.values():
            try:
                engine.dispose()
            except Exception:
                pass
        _engines.clear()


def init_db() -> None:
    SQLModel.metadata.create_all(get_engine())


def get_session() -> Generator[Session, None, None]:
    with Session(get_engine()) as session:
        yield session
