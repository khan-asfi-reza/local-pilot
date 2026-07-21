from datetime import datetime, timezone

from sqlmodel import Field, SQLModel


class Project(SQLModel, table=True):
    id: str = Field(primary_key=True)
    path: str = Field(unique=True, index=True)
    name: str
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
