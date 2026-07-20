import os

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from routes.chat import router as chat_router


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
    app.include_router(chat_router)
    return app


app = create_app()