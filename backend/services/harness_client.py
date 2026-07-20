import asyncio
import json
import os
from typing import AsyncIterator

import httpx

HARNESS_URL = os.getenv("HARNESS_URL", "http://localhost:9000/run")
SAFE_TOOLS = ["code_run", "web_search"]


def harness_base() -> str:
    """Base URL of the harness server, e.g. http://localhost:9000."""
    base = HARNESS_URL.rsplit("/run", 1)[0]
    return base or "http://localhost:9000"


async def list_models() -> dict:
    """Fetch the models the harness offers (name, ready, url, active) + default."""
    async with httpx.AsyncClient(timeout=5) as client:
        response = await client.get(harness_base() + "/models")
        response.raise_for_status()
        return response.json()


async def stream_harness_turn(messages: list[dict], working_directory: str | None = None, model: str | None = None) -> AsyncIterator[dict]:
    payload = {
        "messages": messages,
        "allowed_tools": SAFE_TOOLS,
        "working_directory": working_directory or "",
    }
    if model:
        payload["model"] = model
    async with httpx.AsyncClient(timeout=None) as client:
        async with client.stream("POST", HARNESS_URL, json=payload) as response:
            response.raise_for_status()
            async for line in response.aiter_lines():
                if not line.strip():
                    continue
                event = json.loads(line)
                yield event
