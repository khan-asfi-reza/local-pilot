"""Replit-style multi-project App Builder.

Each project is an isolated, full Vite + React + Tailwind project in its own
directory under the data dir. Every project runs its own Vite dev server on an
internal port; a single gateway on port 6969 reverse-proxies each by uuid
(http://host:6969/<id>/), including the HMR WebSocket, so previews are isolated
and hot-reload. node_modules is shared (symlinked) from the frontend, so creating
a project needs no npm install."""

import json
import os
import shutil
import socket
import subprocess
import threading
import time
import uuid as uuidlib
import zipfile
from io import BytesIO

from core.appdir import data_dir

GATEWAY_PORT = 6969
_PORT_BASE = 7001  # internal vite ports; the gateway is the only public one

_REPO_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
_FRONTEND_NM = os.path.join(_REPO_ROOT, "frontend", "node_modules")
_GATEWAY = os.path.join(_REPO_ROOT, "builder_gateway", "server.mjs")

PROJECTS_DIR = os.path.join(data_dir(), "builder_projects")
_REGISTRY = os.path.join(PROJECTS_DIR, "registry.json")  # uuid -> vite port (for the gateway)
_META = os.path.join(PROJECTS_DIR, "projects.json")       # [{id,name,prompt,created,port}]
_SKIP = {"node_modules", "dist", ".vite", ".git"}

_procs: dict[str, subprocess.Popen] = {}   # uuid -> vite process
_gateway_proc: subprocess.Popen | None = None
_lock = threading.RLock()


# --- metadata + registry ---------------------------------------------------

def _read_json(path, default):
    try:
        with open(path) as f:
            return json.load(f)
    except Exception:
        return default


def _write_json(path, data):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w") as f:
        json.dump(data, f)


def _load_meta() -> list[dict]:
    return _read_json(_META, [])


def _save_meta(meta: list[dict]):
    _write_json(_META, meta)


def _sync_registry(meta: list[dict]):
    _write_json(_REGISTRY, {m["id"]: m["port"] for m in meta})


def _port_open(port: int) -> bool:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.settimeout(0.4)
        return s.connect_ex(("127.0.0.1", port)) == 0


def _next_port(meta: list[dict]) -> int:
    used = {m["port"] for m in meta}
    p = _PORT_BASE
    while p in used or _port_open(p):
        p += 1
    return p


# --- gateway ---------------------------------------------------------------

def ensure_gateway() -> bool:
    """Start the uuid-routing gateway on 6969 if it is not already running."""
    global _gateway_proc
    with _lock:
        os.makedirs(PROJECTS_DIR, exist_ok=True)
        if not os.path.exists(_REGISTRY):
            _sync_registry(_load_meta())
        if _port_open(GATEWAY_PORT):
            return True
        if not os.path.exists(_GATEWAY):
            return False
        env = {**os.environ, "REGISTRY": _REGISTRY, "GATEWAY_PORT": str(GATEWAY_PORT)}
        _gateway_proc = subprocess.Popen(
            ["node", _GATEWAY], cwd=os.path.dirname(_GATEWAY),
            env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
        )
        for _ in range(40):
            if _port_open(GATEWAY_PORT):
                return True
            time.sleep(0.2)
        return _port_open(GATEWAY_PORT)


# --- scaffolding -----------------------------------------------------------

_EMPTY_APP = """export default function App() {
  return <div className="min-h-screen bg-slate-50" />;
}
"""

# Injected into every project's index.html: forwards runtime errors and
# console.error/warn from the preview iframe to the parent (the App Builder UI),
# which shows them in the Source section. Marker lets us inject it idempotently.
_BRIDGE_MARKER = "builder-console-bridge"
_ERROR_BRIDGE = """    <script data-builder-console-bridge>
      (function () {
        function send(level, parts) {
          try {
            parent.postMessage({ source: 'builder-preview', level: level,
              text: parts.map(function (a) {
                if (a && a.stack) return a.stack;
                if (typeof a === 'object') { try { return JSON.stringify(a); } catch (e) { return String(a); } }
                return String(a);
              }).join(' ') }, '*');
          } catch (e) {}
        }
        window.addEventListener('error', function (e) {
          send('error', [e.message + (e.filename ? ' (' + e.filename + ':' + e.lineno + ')' : '')]);
        });
        window.addEventListener('unhandledrejection', function (e) {
          send('error', [String((e.reason && e.reason.stack) || e.reason)]);
        });
        ['error', 'warn'].forEach(function (lvl) {
          var orig = console[lvl];
          console[lvl] = function () { send(lvl, [].slice.call(arguments)); orig.apply(console, arguments); };
        });
      })();
    </script>
"""


def _index_html() -> str:
    return (
        "<!doctype html>\n<html lang=\"en\">\n  <head>\n    <meta charset=\"utf-8\" />\n"
        "    <meta name=\"viewport\" content=\"width=device-width, initial-scale=1\" />\n"
        "    <title>App</title>\n" + _ERROR_BRIDGE + "  </head>\n  <body>\n    <div id=\"root\"></div>\n"
        "    <script type=\"module\" src=\"/src/main.jsx\"></script>\n  </body>\n</html>\n"
    )


def _ensure_error_bridge(pid: str) -> None:
    """Add the console bridge to a project's index.html if it predates this change."""
    path = os.path.join(project_dir(pid), "index.html")
    try:
        with open(path) as f:
            html = f.read()
        if _BRIDGE_MARKER not in html and "</head>" in html:
            with open(path, "w") as f:
                f.write(html.replace("</head>", _ERROR_BRIDGE + "  </head>", 1))
    except OSError:
        pass


def _scaffold(project_dir: str, pid: str, port: int):
    os.makedirs(os.path.join(project_dir, "src"), exist_ok=True)
    # Reuse the frontend's installed deps — no per-project npm install.
    nm = os.path.join(project_dir, "node_modules")
    if not os.path.exists(nm):
        os.symlink(_FRONTEND_NM, nm)

    files = {
        "package.json": json.dumps({
            "name": f"builder-{pid[:8]}", "private": True, "type": "module",
            "scripts": {"dev": "vite", "build": "vite build"},
            "dependencies": {"react": "^18.3.1", "react-dom": "^18.3.1"},
            "devDependencies": {
                "@vitejs/plugin-react": "^4.3.3", "autoprefixer": "^10.4.20",
                "postcss": "^8.4.47", "tailwindcss": "^3.4.14", "vite": "^5.4.9",
            },
        }, indent=2),
        "vite.config.js": (
            "import { defineConfig } from 'vite';\n"
            "import react from '@vitejs/plugin-react';\n"
            "export default defineConfig({\n"
            f"  base: '/{pid}/',\n"
            "  plugins: [react()],\n"
            f"  server: {{ host: true, port: {port}, strictPort: true, "
            f"hmr: {{ clientPort: {GATEWAY_PORT}, path: '/{pid}/' }} }},\n"
            "});\n"
        ),
        "tailwind.config.js": (
            "/** @type {import('tailwindcss').Config} */\n"
            "export default { content: ['./index.html', './src/**/*.{js,jsx}'], "
            "theme: { extend: {} }, plugins: [] };\n"
        ),
        "postcss.config.js": "export default { plugins: { tailwindcss: {}, autoprefixer: {} } };\n",
        "index.html": _index_html(),
        "src/main.jsx": (
            "import { createRoot } from 'react-dom/client';\n"
            "import './index.css';\nimport App from './App.jsx';\n"
            "createRoot(document.getElementById('root')).render(<App />);\n"
        ),
        "src/index.css": "@tailwind base;\n@tailwind components;\n@tailwind utilities;\n",
        "src/App.jsx": _EMPTY_APP,
    }
    for rel, content in files.items():
        full = os.path.join(project_dir, rel)
        os.makedirs(os.path.dirname(full), exist_ok=True)
        with open(full, "w") as f:
            f.write(content)


# --- project lifecycle -----------------------------------------------------

def project_dir(pid: str) -> str:
    return os.path.join(PROJECTS_DIR, pid)


def src_dir(pid: str) -> str:
    return os.path.join(project_dir(pid), "src")


def list_projects() -> list[dict]:
    meta = _load_meta()
    return [{"id": m["id"], "name": m["name"], "prompt": m.get("prompt", ""),
             "created": m.get("created", "")} for m in reversed(meta)]


def get_meta(pid: str) -> dict | None:
    return next((m for m in _load_meta() if m["id"] == pid), None)


def create_project(name: str, prompt: str = "", created: str = "") -> dict:
    with _lock:
        ensure_gateway()
        meta = _load_meta()
        pid = str(uuidlib.uuid4())
        port = _next_port(meta)
        _scaffold(project_dir(pid), pid, port)
        entry = {"id": pid, "name": name or "Untitled app", "prompt": prompt,
                 "created": created, "port": port}
        meta.append(entry)
        _save_meta(meta)
        _sync_registry(meta)
        return entry


def preview_url(pid: str, host: str | None = None) -> str:
    return f"http://{host or '127.0.0.1'}:{GATEWAY_PORT}/{pid}/"


def ensure_server(pid: str) -> bool:
    """Start this project's Vite dev server if not already running."""
    with _lock:
        m = get_meta(pid)
        if not m:
            return False
        ensure_gateway()
        _ensure_error_bridge(pid)
        port = m["port"]
        if _port_open(port):
            return True
        vite = os.path.join(project_dir(pid), "node_modules", ".bin", "vite")
        if not os.path.exists(vite):
            return False
        _procs[pid] = subprocess.Popen(
            [vite], cwd=project_dir(pid),
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
        )
        for _ in range(60):
            if _port_open(port):
                return True
            time.sleep(0.25)
        return _port_open(port)


def stop_server(pid: str):
    with _lock:
        p = _procs.pop(pid, None)
        if p:
            p.terminate()


def stop_all():
    with _lock:
        for p in list(_procs.values()):
            try:
                p.terminate()
            except Exception:
                pass
        _procs.clear()
        global _gateway_proc
        if _gateway_proc:
            _gateway_proc.terminate()
            _gateway_proc = None


def _log_path(pid: str) -> str:
    return os.path.join(PROJECTS_DIR, "logs", pid + ".json")


def load_messages(pid: str) -> list[dict]:
    """The project's build-log conversation (user/assistant), for restore on reload."""
    try:
        with open(_log_path(pid)) as f:
            data = json.load(f)
            return data if isinstance(data, list) else []
    except Exception:
        return []


def save_messages(pid: str, messages: list[dict]) -> None:
    path = _log_path(pid)
    os.makedirs(os.path.dirname(path), exist_ok=True)
    tmp = path + ".tmp"
    with open(tmp, "w") as f:
        json.dump(messages, f)
    os.replace(tmp, path)


def rename_project(pid: str, name: str) -> dict | None:
    name = (name or "").strip()
    if not name:
        return get_meta(pid)
    with _lock:
        meta = _load_meta()
        for m in meta:
            if m["id"] == pid:
                m["name"] = name
                _save_meta(meta)
                return m
    return None


def delete_project(pid: str):
    with _lock:
        stop_server(pid)
        shutil.rmtree(project_dir(pid), ignore_errors=True)
        try:
            os.remove(_log_path(pid))
        except OSError:
            pass
        meta = [m for m in _load_meta() if m["id"] != pid]
        _save_meta(meta)
        _sync_registry(meta)


# --- files (Source tab) ----------------------------------------------------

def _safe(pid: str, rel: str) -> str:
    root = os.path.realpath(project_dir(pid))
    target = os.path.realpath(os.path.join(root, rel))
    if target != root and not target.startswith(root + os.sep):
        raise ValueError("path escapes the project")
    return target


def list_files(pid: str) -> list[str]:
    root = project_dir(pid)
    out: list[str] = []
    for dp, dirs, names in os.walk(root):
        dirs[:] = [d for d in dirs if d not in _SKIP]
        for n in names:
            rel = os.path.relpath(os.path.join(dp, n), root).replace(os.sep, "/")
            out.append(rel)
    return sorted(out)


def read_file(pid: str, rel: str) -> str:
    with open(_safe(pid, rel), "r", encoding="utf-8", errors="replace") as f:
        return f.read()


def write_file(pid: str, rel: str, content: str):
    target = _safe(pid, rel)
    os.makedirs(os.path.dirname(target), exist_ok=True)
    with open(target, "w", encoding="utf-8") as f:
        f.write(content)


def zip_project(pid: str) -> bytes:
    root = project_dir(pid)
    buf = BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as z:
        for dp, dirs, names in os.walk(root):
            dirs[:] = [d for d in dirs if d not in _SKIP]
            for n in names:
                full = os.path.join(dp, n)
                arc = os.path.relpath(full, root)
                z.write(full, arc)
    return buf.getvalue()
