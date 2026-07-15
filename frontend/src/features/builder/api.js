const BASE =
  import.meta.env.VITE_API_URL || `${window.location.protocol}//${window.location.hostname}:8182`;

async function json(res) {
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json();
}

// createSession creates a new builder session with an initial prompt.
export async function createSession(prompt, model) {
  const res = await fetch(`${BASE}/builder/sessions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ prompt, model }),
  });
  return (await json(res)); // { id }
}

// generate streams a build for the given session, calling onEvent for each
// parsed SSE frame. history is the prior message list for context on follow-ups.
export async function generate(id, prompt, onEvent, signal, history) {
  let res;
  try {
    res = await fetch(`${BASE}/builder/sessions/${id}/generate`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ prompt, history: history || [] }),
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

// previewUrl returns the URL for the live preview iframe.
export function previewUrl(id) {
  return `${BASE}/builder/sessions/${id}/preview`;
}

// getFiles returns the generated files for a session.
export async function getFiles(id) {
  return json(await fetch(`${BASE}/builder/sessions/${id}/files`));
}
