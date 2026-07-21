// App Builder gateway: one dedicated port (6969) that reverse-proxies each
// project's Vite dev server by uuid path (/<uuid>/...), including the HMR
// WebSocket. Routing comes from a registry JSON the backend maintains
// (uuid -> internal vite port).
import http from 'node:http';
import fs from 'node:fs';
import httpProxy from 'http-proxy';

const PORT = Number(process.env.GATEWAY_PORT || 6969);
const REGISTRY = process.env.REGISTRY || '';
const UUID_RE = /^\/([0-9a-fA-F-]{36})(?=\/|$)/;

const proxy = httpProxy.createProxyServer({ ws: true, changeOrigin: true });
proxy.on('error', (_e, _req, res) => {
  try { if (res && res.writeHead) { res.writeHead(502); res.end('builder: upstream not ready'); } } catch {}
});

function target(url) {
  const m = (url || '').match(UUID_RE);
  if (!m) return null;
  let reg = {};
  try { reg = JSON.parse(fs.readFileSync(REGISTRY, 'utf8')); } catch {}
  const port = reg[m[1]];
  return port ? `http://127.0.0.1:${port}` : null;
}

const server = http.createServer((req, res) => {
  const t = target(req.url);
  if (!t) { res.writeHead(404, { 'content-type': 'text/plain' }); res.end('unknown project'); return; }
  proxy.web(req, res, { target: t });
});
server.on('upgrade', (req, socket, head) => {
  const t = target(req.url);
  if (!t) { socket.destroy(); return; }
  proxy.ws(req, socket, head, { target: t });
});
server.listen(PORT, () => console.log(`builder gateway on :${PORT}`));
