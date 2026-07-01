import asyncio
import json
import os
from typing import AsyncIterator

import httpx

HARNESS_URL = os.getenv("HARNESS_URL", "http://localhost:8001/run")
SAFE_TOOLS = ["code_run", "web_search"]


async def stream_harness_turn(messages: list[dict], working_directory: str | None = None) -> AsyncIterator[dict]:
    payload = {
        "messages": messages,
        "allowed_tools": SAFE_TOOLS,
        "working_directory": working_directory or "",
    }
    async with httpx.AsyncClient(timeout=None) as client:
        async with client.stream("POST", HARNESS_URL, json=payload) as response:
            response.raise_for_status()
            async for line in response.aiter_lines():
                if not line.strip():
                    continue
                event = json.loads(line)
                yield event
