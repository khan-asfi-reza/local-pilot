import os

from fastapi import FastAPI, Request
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse

from routes.chat import router as chat_router
from routes.builder import router as builder_router

# Clients treated as local. The /code routes drive a full-access, unsandboxed
# agent over real files, so they must not be reachable from the LAN or driven by
# a malicious web page (CSRF / DNS-rebinding) even when the tab is on this host.
_LOCAL_HOSTS = {"127.0.0.1", "::1", "localhost"}
# Sec-Fetch-Site values a browser sends for trusted callers. The UI at :5173
# calls the API at :8182 — same registrable site, so "same-site". A malicious
# page is "cross-site". The header is set by the browser and cannot be forged by
# the page, so it blocks the text/plain no-preflight trick too. Absent (curl or
# older clients) is allowed; the localhost check below still bounds those.
_SAFE_FETCH_SITE = {"same-origin", "same-site", "none"}


def create_app() -> FastAPI:
    app = FastAPI()
    # Allow any origin: this is a local/LAN dev tool with no auth cookies, so the
    # UI may be opened from localhost or from another machine on the network.
    app.add_middleware(
        CORSMiddleware,
        allow_origins=["*"],
        allow_credentials=False,
        allow_methods=["*"],
        allow_headers=["*"],
    )

    # App-level guard for the Code IDE routes. Enforced here (a file we own) so it
    # holds regardless of how routes/code.py is implemented. CORS does not stop a
    # request from reaching a handler; this check does.
    @app.middleware("http")
    async def guard_agent_routes(request: Request, call_next):
        # /code and /builder drive a full-access agent (files, npm) over real
        # dirs, so they must not be reachable from the LAN or a malicious page.
        if request.url.path.startswith(("/code", "/builder")):
            host = request.client.host if request.client else ""
            if host not in _LOCAL_HOSTS:
                return JSONResponse({"detail": "localhost only"}, status_code=403)
            sfs = request.headers.get("sec-fetch-site")
            if sfs and sfs not in _SAFE_FETCH_SITE:
                return JSONResponse({"detail": "cross-site request rejected"}, status_code=403)
        return await call_next(request)

    app.include_router(chat_router)
    app.include_router(builder_router)
    # The Code IDE routes are optional at boot: tolerate a missing routes/code.py
    # so the app still starts if that part is not present yet.
    try:
        from routes.code import router as code_router

        app.include_router(code_router)
    except Exception as exc:
        print(f"code routes unavailable: {exc}")
    return app


app = create_app()