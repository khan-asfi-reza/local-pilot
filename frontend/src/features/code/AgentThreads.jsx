import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Plus, X } from 'lucide-react';
import { code } from '../../lib/api';
import { cn } from '../../lib/utils';
import { AgentPanel } from './AgentPanel';

const POLL_MS = 4000;

// A 6-byte hex id, matching the backend session id shape (^[a-f0-9]{6,64}$), so a
// brand-new thread persists under an id the backend accepts on first run.
function newSid() {
  const bytes = crypto.getRandomValues(new Uint8Array(6));
  return Array.from(bytes)
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('');
}

// AgentThreads is the multi-thread shell for the Code IDE: a tab bar plus one
// mounted AgentPanel per open thread. Every open thread stays mounted, so a run
// in one keeps streaming while you read or type in another — and a Telegram
// thread mirrors its messages live even while it is off-screen. Only the active
// thread is visible; the rest are hidden (display:none) but alive.
export function AgentThreads({
  root,
  activePath,
  tree,
  initialPrompt,
  onInitialPromptSent,
  onDone,
  onFileChange,
  onActivity,
  onConfirmChange,
  onViewDiff,
}) {
  const [threads, setThreads] = useState([]); // [{id, title, source, updated_at}]
  const [activeSid, setActiveSid] = useState(null);
  const [openSids, setOpenSids] = useState([]); // mounted threads
  const [busySids, setBusySids] = useState({});
  const [unreadSids, setUnreadSids] = useState({});
  const [promptSid, setPromptSid] = useState(null); // the thread that gets initialPrompt
  const initialPromptRef = useRef(initialPrompt);
  initialPromptRef.current = initialPrompt;

  const refresh = useCallback(async () => {
    if (!root) return [];
    try {
      const { sessions } = await code.listSessions(root);
      const list = sessions || [];
      setThreads(list);
      return list;
    } catch {
      return [];
    }
  }, [root]);

  // (Re)initialise when the project changes: load its threads, pick an active
  // one, and auto-open any Telegram threads so they mirror in the background. A
  // freshly created project (initialPrompt present) opens a new thread for it.
  useEffect(() => {
    let cancelled = false;
    setThreads([]);
    setActiveSid(null);
    setOpenSids([]);
    setBusySids({});
    setUnreadSids({});
    setPromptSid(null);
    if (!root) return undefined;
    (async () => {
      const list = await refresh();
      if (cancelled) return;
      const tg = list.filter((t) => t.source === 'telegram').map((t) => t.id);
      if (initialPromptRef.current) {
        const nsid = newSid();
        setThreads((prev) => [{ id: nsid, title: 'New chat', source: 'web', updated_at: '' }, ...prev]);
        setOpenSids([nsid, ...tg]);
        setActiveSid(nsid);
        setPromptSid(nsid);
      } else if (list.length) {
        setOpenSids(Array.from(new Set([list[0].id, ...tg])));
        setActiveSid(list[0].id);
      } else {
        const nsid = newSid();
        setThreads([{ id: nsid, title: 'New chat', source: 'web', updated_at: '' }]);
        setOpenSids([nsid]);
        setActiveSid(nsid);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [root, refresh]);

  // Poll the thread list so new Telegram threads (and titles) show up, and
  // auto-open any newly-linked Telegram thread so it starts mirroring.
  useEffect(() => {
    if (!root) return undefined;
    const timer = setInterval(async () => {
      if (document.hidden) return;
      const list = await refresh();
      setOpenSids((prev) => {
        const fresh = list
          .filter((t) => t.source === 'telegram' && !prev.includes(t.id))
          .map((t) => t.id);
        return fresh.length ? [...prev, ...fresh] : prev;
      });
    }, POLL_MS);
    return () => clearInterval(timer);
  }, [root, refresh]);

  const selectThread = useCallback((sid) => {
    setActiveSid(sid);
    setOpenSids((prev) => (prev.includes(sid) ? prev : [...prev, sid]));
    setUnreadSids((prev) => {
      if (!prev[sid]) return prev;
      const next = { ...prev };
      delete next[sid];
      return next;
    });
  }, []);

  const newThread = useCallback(() => {
    const sid = newSid();
    setThreads((prev) => [{ id: sid, title: 'New chat', source: 'web', updated_at: '' }, ...prev]);
    setOpenSids((prev) => [...prev, sid]);
    setActiveSid(sid);
  }, []);

  const deleteThread = useCallback(
    async (sid) => {
      try {
        await code.deleteSession(root, sid);
      } catch {
        /* already gone */
      }
      setThreads((prev) => prev.filter((t) => t.id !== sid));
      setOpenSids((prev) => {
        const next = prev.filter((s) => s !== sid);
        setActiveSid((cur) => (cur === sid ? next[next.length - 1] ?? null : cur));
        return next;
      });
    },
    [root],
  );

  const onBusyChange = useCallback((sid, b) => {
    setBusySids((prev) => (prev[sid] === b ? prev : { ...prev, [sid]: b }));
  }, []);

  const onUnread = useCallback(
    (sid) => {
      setUnreadSids((prev) => (sid === activeSid || prev[sid] ? prev : { ...prev, [sid]: true }));
    },
    [activeSid],
  );

  const handleInitialSent = useCallback(() => {
    setPromptSid(null);
    onInitialPromptSent?.();
  }, [onInitialPromptSent]);

  // Order the tab bar by the thread list (newest first), keeping any open-but-
  // unsaved threads that are not on disk yet.
  const tabs = useMemo(() => {
    const byId = new Map(threads.map((t) => [t.id, t]));
    const extra = openSids
      .filter((s) => !byId.has(s))
      .map((s) => ({ id: s, title: 'New chat', source: 'web', updated_at: '' }));
    return [...extra, ...threads];
  }, [threads, openSids]);

  if (!root) return null;

  return (
    <div className="flex w-[380px] shrink-0 flex-col border-l border-zinc-800 bg-[#101012]">
      <div className="flex items-center gap-1 overflow-x-auto border-b border-zinc-800 px-2 py-1.5">
        {tabs.map((t) => {
          const isActive = t.id === activeSid;
          return (
            <button
              key={t.id}
              type="button"
              onClick={() => selectThread(t.id)}
              title={t.title || 'New chat'}
              className={cn(
                'group flex shrink-0 items-center gap-1.5 rounded-lg px-2.5 py-1 text-[12px] transition-colors',
                isActive ? 'bg-zinc-800 text-zinc-100' : 'text-zinc-400 hover:bg-zinc-800/60 hover:text-zinc-200',
              )}
            >
              {busySids[t.id] && (
                <span className="h-1.5 w-1.5 shrink-0 animate-pulse rounded-full bg-emerald-400" />
              )}
              {!busySids[t.id] && unreadSids[t.id] && (
                <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-sky-400" />
              )}
              <span className="max-w-[120px] truncate">{t.title || 'New chat'}</span>
              {t.source === 'telegram' && (
                <span className="rounded bg-sky-500/15 px-1 text-[9px] font-medium text-sky-300">tg</span>
              )}
              <span
                role="button"
                tabIndex={-1}
                onClick={(e) => {
                  e.stopPropagation();
                  deleteThread(t.id);
                }}
                className="ml-0.5 rounded p-0.5 text-zinc-600 opacity-0 transition-opacity hover:text-red-400 group-hover:opacity-100"
                aria-label="Close thread"
              >
                <X size={12} />
              </span>
            </button>
          );
        })}
        <button
          type="button"
          onClick={newThread}
          className="ml-auto flex shrink-0 items-center gap-1 rounded-lg px-2 py-1 text-[12px] text-zinc-400 transition-colors hover:bg-zinc-800 hover:text-zinc-200"
          title="New thread"
        >
          <Plus size={14} className="text-emerald-400" />
        </button>
      </div>

      <div className="relative min-h-0 flex-1">
        {openSids.map((sid) => {
          const meta = tabs.find((t) => t.id === sid) || { source: 'web' };
          const isActive = sid === activeSid;
          return (
            <div key={sid} className={cn('absolute inset-0', isActive ? 'block' : 'hidden')}>
              <AgentPanel
                root={root}
                sid={sid}
                active={isActive}
                source={meta.source}
                activePath={activePath}
                tree={tree}
                initialPrompt={sid === promptSid ? initialPrompt : null}
                onInitialPromptSent={handleInitialSent}
                onDone={onDone}
                onFileChange={onFileChange}
                onActivity={() => {
                  onActivity?.();
                  refresh();
                }}
                onConfirmChange={onConfirmChange}
                onViewDiff={onViewDiff}
                onBusyChange={onBusyChange}
                onUnread={onUnread}
              />
            </div>
          );
        })}
      </div>
    </div>
  );
}
