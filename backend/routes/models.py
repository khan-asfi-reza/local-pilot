"""Admin routes for managing the model registry from the web Settings page.

These change server config (add / pull / remove / activate), so — like the /code
routes — they are restricted to a local, same-site caller. The read-only model
list stays in chat.py (unguarded) so the LAN chat picker keeps working. Each route
proxies the harness-server, which enforces its own loopback gate as a second layer.
"""

import json
from typing import Any

import httpx
from fastapi import APIRouter, Body, Depends, HTTPException
from fastapi.responses import StreamingResponse
from starlette.requests import HTTPConnection

from services import harness_client

_LOCAL_HOSTS = {"127.0.0.1", "::1", "localhost"}
_SAFE_FETCH_SITE = {"same-origin", "same-site", "none"}


def admin_only(conn: HTTPConnection) -> None:
    host = conn.client.host if conn.client else ""
    if host not in _LOCAL_HOSTS:
        raise HTTPException(status_code=403, detail="localhost only")
    sfs = conn.headers.get("sec-fetch-site")
    if sfs and sfs not in _SAFE_FETCH_SITE:
        raise HTTPException(status_code=403, detail="cross-site request rejected")


router = APIRouter(prefix="/models", dependencies=[Depends(admin_only)])


async def _proxy(coro):
    """Await a harness proxy call, surfacing its error message and status."""
    try:
        return await coro
    except httpx.HTTPStatusError as exc:
        detail = exc.response.text.strip() or str(exc)
        raise HTTPException(status_code=exc.response.status_code, detail=detail)
    except httpx.HTTPError as exc:
        raise HTTPException(status_code=502, detail=f"harness unreachable: {exc}")


def _require_name(body: dict) -> str:
    name = (body.get("name") or "").strip()
    if not name:
        raise HTTPException(status_code=400, detail="name is required")
    return name


@router.get("/available")
async def available() -> dict[str, Any]:
    return await _proxy(harness_client.available_models())


@router.post("")
async def add(body: dict = Body(...)) -> dict[str, Any]:
    tag = (body.get("model") or "").strip()
    if not tag:
        raise HTTPException(status_code=400, detail="model is required")
    return await _proxy(harness_client.add_model(tag, body.get("host", ""), body.get("name", "")))


@router.post("/remove")
async def remove(body: dict = Body(...)) -> dict[str, Any]:
    return await _proxy(harness_client.remove_model(_require_name(body)))


@router.post("/activate")
async def activate(body: dict = Body(...)) -> dict[str, Any]:
    return await _proxy(harness_client.activate_model(_require_name(body)))


@router.post("/pull")
async def pull(body: dict = Body(...)) -> StreamingResponse:
    name = _require_name(body)

    async def gen():
        try:
            async for event in harness_client.pull_model(name):
                yield json.dumps(event) + "\n"
        except httpx.HTTPError as exc:
            yield json.dumps({"error": f"harness unreachable: {exc}"}) + "\n"

    return StreamingResponse(gen(), media_type="application/x-ndjson")
