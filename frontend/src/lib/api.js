// Thin client for the local-pilot backend (FastAPI on :6000). CORS is configured
// there for the Vite dev origin, so absolute URLs work in development.
const BASE = import.meta.env.VITE_API_URL || 'http://localhost:8000';

async function json(res) {
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json();
}

export async function listModels() {
  return json(await fetch(`${BASE}/models`)); // { models:[{name,ready,url,active}], default }
}

export async function listThreads() {
  try {
    return await json(await fetch(`${BASE}/threads`));
  } catch {
    return [];
  }
}

export async function createThread(model) {
  const res = await fetch(`${BASE}/threads`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ model }),
  });
  return (await json(res)).thread;
}

export async function getThread(id) {
  try {
    return await json(await fetch(`${BASE}/threads/${id}`)); // { thread, messages }
  } catch {
    return null;
  }
}

export async function setThreadModel(id, model) {
  const res = await fetch(`${BASE}/threads/${id}/model`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ model }),
  });
  return (await json(res)).thread;
}

export async function deleteThread(id) {
  await fetch(`${BASE}/threads/${id}`, { method: 'DELETE' });
}

// sendMessage POSTs the message and streams the server-sent events back, calling
// onEvent for each parsed event ({type:'text'|'tool_call'|'tool_result'|'error'|'done', ...}).
export async function sendMessage(id, content, onEvent) {
  const res = await fetch(`${BASE}/threads/${id}/messages`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content }),
  });
  if (!res.ok || !res.body) {
    onEvent({ type: 'error', message: `request failed (${res.status})` });
    return;
  }
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    let sep;
    while ((sep = buffer.indexOf('\n\n')) >= 0) {
      const frame = buffer.slice(0, sep);
      buffer = buffer.slice(sep + 2);
      const dataLine = frame.split('\n').find((l) => l.startsWith('data:'));
      if (dataLine) {
        try {
          onEvent(JSON.parse(dataLine.slice(5).trim()));
        } catch {
          /* ignore malformed frame */
        }
      }
    }
  }
}
