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


async def available_models() -> dict:
    """Ollama-installed model tags not yet registered, for the add autocomplete."""
    async with httpx.AsyncClient(timeout=15) as client:
        response = await client.get(harness_base() + "/models/available")
        response.raise_for_status()
        return response.json()


async def add_model(model: str, host: str = "", name: str = "") -> dict:
    """Register an already-installed model (tag `model` on `host`, labeled `name`);
    returns the updated model list."""
    async with httpx.AsyncClient(timeout=30) as client:
        response = await client.post(
            harness_base() + "/models", json={"model": model, "host": host, "name": name}
        )
        response.raise_for_status()
        return response.json()


async def remove_model(name: str) -> dict:
    """Remove a model from the registry and ollama; returns the updated list."""
    async with httpx.AsyncClient(timeout=60) as client:
        response = await client.post(harness_base() + "/models/remove", json={"name": name})
        response.raise_for_status()
        return response.json()


async def activate_model(name: str) -> dict:
    """Set the persistent default model; returns the updated list."""
    async with httpx.AsyncClient(timeout=10) as client:
        response = await client.post(harness_base() + "/models/activate", json={"name": name})
        response.raise_for_status()
        return response.json()


async def pull_model(name: str) -> AsyncIterator[dict]:
    """Pull a new model, yielding NDJSON progress events (status/completed/total)."""
    async with httpx.AsyncClient(timeout=None) as client:
        async with client.stream("POST", harness_base() + "/models/pull", json={"name": name}) as response:
            response.raise_for_status()
            async for line in response.aiter_lines():
                if not line.strip():
                    continue
                yield json.loads(line)


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
