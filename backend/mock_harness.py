from fastapi import FastAPI, Request
from fastapi.responses import StreamingResponse
import asyncio
import json

app = FastAPI()

@app.post("/run")
async def run(request: Request):
    body = await request.json() # accept anything, ignore it for the mock

    async def gen():
        for piece in ["Hello", " there.", "This", " is", " a", " test", " answer."]:
            yield json.dumps({"type": "text", "content": piece}) + "\n"
            await asyncio.sleep(0.1)
        yield json.dumps({"type": "done"}) + "\n"

    return StreamingResponse(gen(), media_type="application/x-ndjson")