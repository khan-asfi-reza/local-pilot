import React from 'react';
import ReactDOM from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import '@fontsource-variable/inter';
import App from './App';
import './index.css';

// Self-heal a stale dynamic import: if a lazily-loaded chunk fails to fetch
// (e.g. Vite re-optimized its deps and the old chunk 404/504s, or CodeMirror's
// on-demand language loader misses), reload the page ONCE instead of leaving a
// blank screen. The sessionStorage flag prevents a reload loop and is cleared
// after a successful load so future misses can still self-heal.
function reloadOnceOnStaleChunk() {
  if (sessionStorage.getItem('chunkReloaded')) return;
  sessionStorage.setItem('chunkReloaded', '1');
  window.location.reload();
}
window.addEventListener('vite:preloadError', reloadOnceOnStaleChunk);
window.addEventListener('unhandledrejection', (e) => {
  const msg = String((e && e.reason && e.reason.message) || (e && e.reason) || '');
  if (msg.includes('dynamically imported module') || msg.includes('Failed to fetch')) {
    reloadOnceOnStaleChunk();
  }
});
window.addEventListener('load', () => {
  setTimeout(() => sessionStorage.removeItem('chunkReloaded'), 5000);
});

ReactDOM.createRoot(document.getElementById('root')).render(
  <React.StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </React.StrictMode>
);
