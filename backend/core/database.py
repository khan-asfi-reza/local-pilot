import os
from typing import Generator

from sqlmodel import Session, SQLModel, create_engine


def get_database_url() -> str:
    return os.getenv("DATABASE_URL", "sqlite:///./local-pilot.db")


def get_engine():
    return create_engine(get_database_url(), connect_args={"check_same_thread": False})


def init_db() -> None:
    SQLModel.metadata.create_all(get_engine())


def get_session() -> Generator[Session, None, None]:
    engine = get_engine()
    with Session(engine) as session:
        yield session
