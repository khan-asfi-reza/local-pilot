import os
from typing import Generator

from sqlmodel import Session, SQLModel, create_engine

from core.appdir import data_dir


def get_database_url() -> str:
    # Keep the SQLite DB in the global local-pilot config dir (not a cwd-relative
    # file), so every entry point resolves the same database.
    url = os.getenv("DATABASE_URL")
    if url:
        return url
    os.makedirs(data_dir(), exist_ok=True)
    return f"sqlite:///{os.path.join(data_dir(), 'localpilot.db')}"


def get_engine():
    return create_engine(get_database_url(), connect_args={"check_same_thread": False})


def init_db() -> None:
    SQLModel.metadata.create_all(get_engine())


def get_session() -> Generator[Session, None, None]:
    engine = get_engine()
    with Session(engine) as session:
        yield session
