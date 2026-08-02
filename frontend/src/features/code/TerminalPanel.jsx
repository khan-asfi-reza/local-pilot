import { useCallback, useEffect, useRef, useState } from 'react';
import { Terminal as XTerm } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';
import { ChevronDown, Plus, SquareTerminal, X } from 'lucide-react';
import { code } from '../../lib/api';
import { cn } from '../../lib/utils';

const MIN_HEIGHT = 120;
const XTERM_THEME = {
  background: '#0c0c0e',
  foreground: '#e4e4e7',
  cursor: '#a78bfa',
  cursorAccent: '#0c0c0e',
  selectionBackground: 'rgba(124, 58, 237, 0.35)',
  black: '#18181b',
  red: '#f87171',
  green: '#4ade80',
  yellow: '#fbbf24',
  blue: '#60a5fa',
  magenta: '#e879f9',
  cyan: '#22d3ee',
  white: '#e4e4e7',
  brightBlack: '#52525b',
  brightRed: '#fca5a5',
  brightGreen: '#86efac',
  brightYellow: '#fcd34d',
  brightBlue: '#93c5fd',
  brightMagenta: '#f0abfc',
  brightCyan: '#67e8f9',
  brightWhite: '#fafafa',
};

const newId = () =>
  crypto.randomUUID?.() || `t${Date.now()}${Math.random().toString(16).slice(2)}`;

// TerminalInstance is one shell: an xterm surface wired to the pty websocket.
// It stays mounted while another tab is in front (just hidden), so scrollback and
// anything running in it survive tab switches.
function TerminalInstance({ root, id, active, onExit }) {
  const hostRef = useRef(null);
  const termRef = useRef(null);
  const fitRef = useRef(null);
  const exitRef = useRef(onExit);
  exitRef.current = onExit;

  useEffect(() => {
    const host = hostRef.current;
    if (!host) return undefined;
    let disposed = false; // writing to a disposed xterm throws; guard every callback
    const term = new XTerm({
      cursorBlink: true,
      fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
      fontSize: 12.5,
      lineHeight: 1.25,
      scrollback: 8000,
      theme: XTERM_THEME,
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(host);
    termRef.current = term;
    fitRef.current = fit;
    try {
      fit.fit();
    } catch {
      /* not laid out yet */
    }

    const ws = new WebSocket(code.terminalUrl(root, id, term.cols, term.rows));
    ws.binaryType = 'arraybuffer';
    ws.onmessage = (ev) => {
      if (disposed) return;
      if (typeof ev.data === 'string') {
        try {
          const msg = JSON.parse(ev.data);
          if (msg.type === 'exit') exitRef.current?.(id);
          else if (msg.type === 'error') term.write(`\r\n\x1b[31m${msg.message}\x1b[0m\r\n`);
        } catch {
          /* ignore non-JSON control frame */
        }
        return;
      }
      term.write(new Uint8Array(ev.data));
    };
    ws.onclose = () => {
      if (!disposed) term.write('\r\n\x1b[2m[terminal disconnected]\x1b[0m\r\n');
    };
    const send = (payload) => ws.readyState === WebSocket.OPEN && ws.send(JSON.stringify(payload));
    const dataSub = term.onData((data) => send({ type: 'input', data }));
    const resizeSub = term.onResize(({ cols, rows }) => send({ type: 'resize', cols, rows }));

    // Refit on panel resize / sidebar changes, but only while this tab is visible
    // (a hidden element has no size, and fitting it would send rows=0).
    const observer = new ResizeObserver(() => {
      if (disposed || !host.offsetParent) return;
      try {
        fit.fit();
      } catch {
        /* mid-layout */
      }
    });
    observer.observe(host);

    return () => {
      disposed = true;
      observer.disconnect();
      dataSub.dispose();
      resizeSub.dispose();
      ws.onmessage = null;
      ws.onclose = null;
      ws.close();
      // Dispose after a beat: xterm queues frame callbacks when it opens and
      // refits, and tearing the renderer down underneath them throws. The element
      // is already detached by React, so nothing is visible in the meantime.
      setTimeout(() => term.dispose(), 300);
    };
  }, [root, id]);

  useEffect(() => {
    if (!active) return undefined;
    const frame = requestAnimationFrame(() => {
      try {
        fitRef.current?.fit();
        termRef.current?.focus();
      } catch {
        /* the instance went away mid-frame */
      }
    });
    return () => cancelAnimationFrame(frame);
  }, [active]);

  return <div ref={hostRef} className={cn('absolute inset-0 px-2 py-1', !active && 'hidden')} />;
}

// TerminalPanel is the bottom dock: VS Code-style tabs over real shells, all
// running in the project directory.
export function TerminalPanel({ root, height, onHeightChange, onClose }) {
  const [tabs, setTabs] = useState([]);
  const [activeId, setActiveId] = useState(null);
  const [supported, setSupported] = useState(true);
  const dragRef = useRef(null);

  // Reattach to shells this project already has (page reload), or open one.
  useEffect(() => {
    let cancelled = false;
    setTabs([]);
    setActiveId(null);
    code
      .listTerminals(root)
      .then(({ supported: ok, ids }) => {
        if (cancelled) return;
        setSupported(ok !== false);
        const live = ok === false ? [] : ids || [];
        const next = live.length ? live.map((id) => ({ id })) : [{ id: newId() }];
        setTabs(ok === false ? [] : next);
        setActiveId(ok === false ? null : next[0].id);
      })
      .catch(() => {
        if (cancelled) return;
        const first = { id: newId() };
        setTabs([first]);
        setActiveId(first.id);
      });
    return () => {
      cancelled = true;
    };
  }, [root]);

  const addTab = useCallback(() => {
    const tab = { id: newId() };
    setTabs((prev) => [...prev, tab]);
    setActiveId(tab.id);
  }, []);

  // closeTab kills the shell for good; dropping the last one closes the panel.
  const closeTab = useCallback(
    (id) => {
      code.killTerminal(id).catch(() => {});
      setTabs((prev) => {
        const next = prev.filter((t) => t.id !== id);
        if (!next.length) onClose?.();
        setActiveId((cur) => (cur === id ? next[next.length - 1]?.id ?? null : cur));
        return next;
      });
    },
    [onClose],
  );

  // A shell that exited on its own (Ctrl-D, `exit`) drops its tab.
  const handleExit = useCallback(
    (id) => {
      setTabs((prev) => {
        const next = prev.filter((t) => t.id !== id);
        if (!next.length) onClose?.();
        setActiveId((cur) => (cur === id ? next[next.length - 1]?.id ?? null : cur));
        return next;
      });
    },
    [onClose],
  );

  const startDrag = useCallback(
    (e) => {
      e.preventDefault();
      dragRef.current = { startY: e.clientY, startHeight: height };
      const onMove = (ev) => {
        const { startY, startHeight } = dragRef.current;
        const max = Math.max(MIN_HEIGHT, window.innerHeight - 160);
        onHeightChange(Math.min(max, Math.max(MIN_HEIGHT, startHeight + (startY - ev.clientY))));
      };
      const onUp = () => {
        window.removeEventListener('mousemove', onMove);
        window.removeEventListener('mouseup', onUp);
      };
      window.addEventListener('mousemove', onMove);
      window.addEventListener('mouseup', onUp);
    },
    [height, onHeightChange],
  );

  return (
    <div style={{ height }} className="flex shrink-0 flex-col border-t border-zinc-800 bg-[#0c0c0e]">
      <div
        onMouseDown={startDrag}
        title="Drag to resize"
        className="h-1 shrink-0 cursor-row-resize transition-colors hover:bg-violet-500/50"
      />
      <div className="flex shrink-0 items-center gap-1 border-b border-zinc-800 px-2 py-1">
        <span className="mr-1 flex items-center gap-1.5 text-[11px] font-medium uppercase tracking-wider text-zinc-500">
          <SquareTerminal size={13} /> Terminal
        </span>
        <div className="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto">
          {tabs.map((t, i) => (
            <div
              key={t.id}
              className={cn(
                'group flex shrink-0 items-center gap-1 rounded-md px-2 py-1 text-[12px]',
                t.id === activeId ? 'bg-zinc-800 text-zinc-100' : 'text-zinc-500 hover:bg-zinc-800/50',
              )}
            >
              <button type="button" onClick={() => setActiveId(t.id)}>
                {i + 1}: shell
              </button>
              <button
                type="button"
                onClick={() => closeTab(t.id)}
                title="Kill terminal"
                className="rounded p-0.5 text-zinc-600 opacity-0 transition-opacity hover:text-red-400 group-hover:opacity-100"
              >
                <X size={11} />
              </button>
            </div>
          ))}
        </div>
        <button
          type="button"
          onClick={addTab}
          title="New terminal"
          className="rounded-md p-1 text-zinc-400 transition-colors hover:bg-zinc-800 hover:text-zinc-200"
        >
          <Plus size={14} />
        </button>
        <button
          type="button"
          onClick={onClose}
          title="Hide panel"
          className="rounded-md p-1 text-zinc-400 transition-colors hover:bg-zinc-800 hover:text-zinc-200"
        >
          <ChevronDown size={14} />
        </button>
      </div>
      <div className="relative min-h-0 flex-1">
        {!supported && (
          <p className="p-3 text-[13px] text-zinc-500">Terminals are not supported on this platform.</p>
        )}
        {tabs.map((t) => (
          <TerminalInstance
            key={t.id}
            root={root}
            id={t.id}
            active={t.id === activeId}
            onExit={handleExit}
          />
        ))}
      </div>
    </div>
  );
}
