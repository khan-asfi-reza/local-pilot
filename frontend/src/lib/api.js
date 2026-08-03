// Thin client for the local-pilot backend (FastAPI on :8182). The base is
// derived from the page's own host so the app works both on localhost and when
// opened from another machine on the LAN (http://<mac-ip>:5173 → :8182).
const BASE =
  import.meta.env.VITE_API_URL || `${window.location.protocol}//${window.location.hostname}:8182`;

// Websocket base for the terminal (http -> ws, https -> wss).
const WS_BASE = BASE.replace(/^http/, 'ws');

async function json(res) {
  if (!res.ok) {
    // FastAPI puts the human-readable reason in `detail`; surface it so the UI can
    // say "already exists" instead of "409 Conflict".
    let detail = '';
    try {
      detail = (await res.json())?.detail || '';
    } catch {
      /* not a JSON error body */
    }
    throw new Error(typeof detail === 'string' && detail ? detail : `${res.status} ${res.statusText}`);
  }
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

// code is the API group for the Code IDE: server-side folder browsing, project
// registration, the file tree, reading/writing files, and the streaming agent
// that edits real project files. The backend serves these on :8182 under /code.
export const code = {
  async browseDir(path = '') {
    return json(await fetch(`${BASE}/code/browse?path=${encodeURIComponent(path)}`));
  },
  async listProjects() {
    return json(await fetch(`${BASE}/code/projects`)); // { projects: [...] }
  },
  async openProject(path) {
    const res = await fetch(`${BASE}/code/projects`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path }),
    });
    return (await json(res)).project;
  },
  // forgetProject / clearProjects only drop registry entries — the folders and
  // their files stay on disk, so re-opening a path brings the project back.
  async forgetProject(id) {
    return json(await fetch(`${BASE}/code/projects?id=${encodeURIComponent(id)}`, { method: 'DELETE' }));
  },
  async clearProjects() {
    return json(await fetch(`${BASE}/code/projects`, { method: 'DELETE' }));
  },
  // createProject makes <location>/<name> on disk and registers the folder as a
  // project. The brief/PRD is not written as a file — it goes to the agent as an
  // attachment, and any uploaded source doc is kept via saveProjectDoc.
  async createProject({ name, location }) {
    const res = await fetch(`${BASE}/code/projects/new`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, location }),
    });
    return json(res); // { project }
  },
  // saveProjectDoc keeps an uploaded source doc with the project by writing the
  // raw bytes (base64) into <root>/.pilot/<filename>.
  async saveProjectDoc(root, filename, contentB64) {
    const res = await fetch(`${BASE}/code/projects/doc`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ root, filename, content_b64: contentB64 }),
    });
    return json(res); // { ok, path }
  },
  async createEntry(root, path, type = 'file') {
    const res = await fetch(`${BASE}/code/fs/entry`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ root, path, type }),
    });
    return json(res); // { ok, path }
  },
  async renameEntry(root, path, newPath) {
    const res = await fetch(`${BASE}/code/fs/entry`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ root, path, new_path: newPath }),
    });
    return json(res); // { ok, path }
  },
  async deleteEntry(root, path) {
    const res = await fetch(
      `${BASE}/code/fs/entry?root=${encodeURIComponent(root)}&path=${encodeURIComponent(path)}`,
      { method: 'DELETE' },
    );
    return json(res); // { ok }
  },
  // listTerminals reports the shells still alive for a project, so a reloaded
  // page reattaches to them instead of spawning duplicates.
  async listTerminals(root) {
    return json(await fetch(`${BASE}/code/terminals?root=${encodeURIComponent(root)}`)); // { supported, ids }
  },
  async killTerminal(id) {
    await fetch(`${BASE}/code/terminal?id=${encodeURIComponent(id)}`, { method: 'DELETE' });
  },
  // terminalUrl is the websocket for one shell: it streams pty output down as
  // binary frames and takes {type:'input'|'resize'|'kill'} JSON frames up.
  terminalUrl(root, id, cols, rows) {
    const q = new URLSearchParams({ root, id, cols: String(cols), rows: String(rows) });
    return `${WS_BASE}/code/terminal?${q}`;
  },
  async readTree(root) {
    return json(await fetch(`${BASE}/code/tree?root=${encodeURIComponent(root)}`)); // { root, tree }
  },
  async readFile(root, path) {
    return json(
      await fetch(
        `${BASE}/code/file?root=${encodeURIComponent(root)}&path=${encodeURIComponent(path)}`,
      ),
    ); // { path, content }
  },
  async writeFile(root, path, content) {
    const res = await fetch(`${BASE}/code/file`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ root, path, content }),
    });
    return json(res); // { ok: true }
  },
  // extractDocument uploads a doc (docx/pdf/pptx/xlsx/txt/md) and gets its plain
  // text back, so it can be attached to an agent message as context.
  async extractDocument(file) {
    const form = new FormData();
    form.append('file', file);
    const res = await fetch(`${BASE}/code/extract`, { method: 'POST', body: form });
    return json(res); // { filename, text }
  },
  // confirmAgent answers an ask-mode confirmation for a paused run. decision is
  // 'approve' | 'decline' | 'approve_always'; feedback is an optional note handed
  // back to the model when redirecting instead of a plain reject.
  async confirmAgent(id, decision, feedback = '') {
    const res = await fetch(`${BASE}/code/agent/confirm`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id, decision, feedback }),
    });
    return json(res); // { ok: true }
  },
  // listSessions / loadSession back the resume-on-reload behaviour; sessions live
  // in <root>/.pilot/sessions and are shared with the terminal.
  async listSessions(root) {
    return json(await fetch(`${BASE}/code/sessions?root=${encodeURIComponent(root)}`)); // { sessions }
  },
  async loadSession(root, id) {
    return json(
      await fetch(`${BASE}/code/session?root=${encodeURIComponent(root)}&id=${encodeURIComponent(id)}`),
    ); // { id, messages, ... }
  },
  async deleteSession(root, id) {
    await fetch(
      `${BASE}/code/session?root=${encodeURIComponent(root)}&id=${encodeURIComponent(id)}`,
      { method: 'DELETE' },
    );
  },
  // streamCodeAgent POSTs { root, model, messages, mode } and streams the
  // server-sent events back, calling onEvent for each parsed event
  // ({type:'text'|'reasoning'|'tool_call'|'tool_result'|'confirm'|'usage'|'done'|'error', ...}).
  // mode 'ask' pauses on mutating ops (a 'confirm' event) until confirmAgent is
  // called; default 'auto' never pauses. Pass an AbortSignal to stop the stream.
  // sessionId resumes/continues a stored session; the backend also emits a
  // {type:'session', id} event so the caller can persist a new one.
  async streamCodeAgent(root, model, messages, onEvent, signal, mode, sessionId) {
    let res;
    try {
      res = await fetch(`${BASE}/code/agent`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ root, model, messages, mode, session_id: sessionId }),
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
  },
};

// profile is the owner profile used by onboarding + Settings. get() returns null
// on failure so the onboarding gate never pops when the backend is unreachable.
export const profile = {
  async get() {
    try {
      return await json(await fetch(`${BASE}/profile`)); // { name, onboarded, telegram:{...} }
    } catch {
      return null;
    }
  },
  async save(name) {
    const res = await fetch(`${BASE}/profile`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name }),
    });
    return json(res);
  },
};

// telegram is the Settings/Connect API group: the bot token lives in the DB, and
// linking a chat is a one-time code the bot redeems.
export const telegram = {
  async getSettings() {
    return json(await fetch(`${BASE}/telegram/settings`)); // { enabled, configured, bot_username, bot_token }
  },
  async saveSettings({ bot_token, enabled }) {
    const res = await fetch(`${BASE}/telegram/settings`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ bot_token, enabled }),
    });
    return json(res); // { enabled, configured, bot_username }
  },
  async linkStart() {
    const res = await fetch(`${BASE}/telegram/link/start`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: '{}',
    });
    return json(res); // { code, deep_link, expires_in }
  },
  async revokeLink(chatId) {
    await fetch(`${BASE}/telegram/link/${chatId}`, { method: 'DELETE' });
  },
};

// models is the Settings model-management API group. list() is the registered
// models + default; available() is ollama-installed tags not yet registered (for
// the add autocomplete); add/remove/activate mutate the registry; pull() streams
// a download's NDJSON progress. All but list/available are admin (localhost + same-site).
export const models = {
  async list() {
    return json(await fetch(`${BASE}/models`)); // { models:[{name,ready,url,active}], default }
  },
  async available() {
    return json(await fetch(`${BASE}/models/available`)); // { available:[name] }
  },
  // add registers an installed ollama tag `model`. opts.host targets a remote
  // server (blank = local); opts.name is an optional display label (a unique one
  // is derived when blank, so the same tag on two servers never collides).
  async add(model, { host = '', name = '' } = {}) {
    const res = await fetch(`${BASE}/models`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ model, host, name }),
    });
    return json(res);
  },
  async remove(name) {
    const res = await fetch(`${BASE}/models/remove`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name }),
    });
    return json(res);
  },
  async activate(name) {
    const res = await fetch(`${BASE}/models/activate`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name }),
    });
    return json(res);
  },
  // pull downloads a NEW model, calling onProgress({status,completed,total}) per
  // NDJSON line, and resolves with the final { done, models } event.
  async pull(name, onProgress) {
    const res = await fetch(`${BASE}/models/pull`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name }),
    });
    if (!res.ok || !res.body) throw new Error(`pull failed (${res.status})`);
    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    let final = null;
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      let nl;
      while ((nl = buffer.indexOf('\n')) >= 0) {
        const line = buffer.slice(0, nl).trim();
        buffer = buffer.slice(nl + 1);
        if (!line) continue;
        let ev;
        try {
          ev = JSON.parse(line);
        } catch {
          continue;
        }
        if (ev.error) throw new Error(ev.error);
        if (ev.done) final = ev;
        else if (onProgress) onProgress(ev);
      }
    }
    return final;
  },
};
