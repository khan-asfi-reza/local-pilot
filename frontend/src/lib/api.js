// Thin client for the local-pilot backend (FastAPI on :8182). The base is
// derived from the page's own host so the app works both on localhost and when
// opened from another machine on the LAN (http://<mac-ip>:5173 → :8182).
const BASE =
  import.meta.env.VITE_API_URL || `${window.location.protocol}//${window.location.hostname}:8182`;

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
// Pass an AbortSignal to stop the stream (the pause button).
export async function sendMessage(id, content, onEvent, signal) {
  let res;
  try {
    res = await fetch(`${BASE}/threads/${id}/messages`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ content }),
      signal,
    });
  } catch (e) {
    if (e?.name !== 'AbortError') onEvent({ type: 'error', message: String(e) });
    return;
  }
  if (!res.ok || !res.body) {
    onEvent({ type: 'error', message: `request failed (${res.status})` });
    return;
  }
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  try {
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
  } catch (e) {
    if (e?.name !== 'AbortError') onEvent({ type: 'error', message: String(e) });
  }
}
