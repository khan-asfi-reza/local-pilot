// App Builder API client (Replit-style multi-project). Each project is an
// isolated Vite app previewed through the gateway on :6969/<id>/.
const BASE =
  import.meta.env.VITE_API_URL || `${window.location.protocol}//${window.location.hostname}:8182`;

async function json(res) {
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json();
}

export async function listProjects() {
  return json(await fetch(`${BASE}/builder/projects`)); // { projects: [{id,name,prompt,created}] }
}

export async function createProject(name, prompt) {
  const res = await fetch(`${BASE}/builder/projects`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, prompt }),
  });
  return json(res); // { id, name }
}

export async function getProject(id) {
  return json(await fetch(`${BASE}/builder/projects/${id}`)); // { id,name,url,running,files }
}

export async function runProject(id) {
  return json(await fetch(`${BASE}/builder/projects/${id}/run`, { method: 'POST' })); // { url, running }
}

export async function renameProject(id, name) {
  return json(
    await fetch(`${BASE}/builder/projects/${id}/rename`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name }),
    }),
  ); // { id, name }
}

export async function deleteProject(id) {
  await fetch(`${BASE}/builder/projects/${id}`, { method: 'DELETE' });
}

export async function readFile(id, path) {
  return json(await fetch(`${BASE}/builder/projects/${id}/file?path=${encodeURIComponent(path)}`));
}

export async function writeFile(id, path, content) {
  return json(
    await fetch(`${BASE}/builder/projects/${id}/file`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path, content }),
    }),
  );
}

// exportUrl is a direct link that downloads the project as a zip.
export function exportUrl(id) {
  return `${BASE}/builder/projects/${id}/export`;
}

// generate streams the build for a project, calling onEvent per parsed SSE frame.
export async function generate(id, prompt, onEvent, signal, history, model) {
  let res;
  try {
    res = await fetch(`${BASE}/builder/projects/${id}/generate`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ prompt, history: history || [], model }),
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
