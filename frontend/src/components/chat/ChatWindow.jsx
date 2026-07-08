import { useEffect, useRef, useState } from 'react';
import { ArrowUp, Check, ChevronDown, Compass, Menu, Plus, Sparkles, Square, Trash2, X } from 'lucide-react';
import { useConversations } from '../../hooks/useConversations';
import { Button } from '../ui/button';
import { Markdown } from './Markdown';
import { Loader } from './Loader';
import { ToolCard } from './ToolCard';
import { cn, humanizeModel } from '../../lib/utils';

function Brand() {
  return (
    <div className="flex items-center gap-2">
      <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br from-emerald-500 to-teal-600 text-white shadow-sm">
        <Compass size={18} strokeWidth={2.2} />
      </span>
      <span className="text-[15px] font-semibold tracking-tight">Pilot</span>
    </div>
  );
}

// normalizeTool turns a tool message (live or reloaded) into {tool, info, input, output}.
// Reloaded tool_call events keep their input; results carry the output.
function normalizeTool(m) {
  if (m.tool !== undefined) return { tool: m.tool, info: m.info, input: m.input, output: m.output, running: m.running };
  try {
    const e = JSON.parse(m.content);
    if (e.type === 'tool_call') return { tool: e.tool, info: e.info, input: e.data };
    if (e.type === 'tool_result') return { tool: e.tool, info: e.info, output: e.data };
    return null;
  } catch {
    return null;
  }
}

// ModelPicker is a compact dropdown that opens upward from the composer.
function ModelPicker({ models, currentModel, defaultModel, onSelect }) {
  const [open, setOpen] = useState(false);
  const ref = useRef(null);
  useEffect(() => {
    const onDoc = (e) => ref.current && !ref.current.contains(e.target) && setOpen(false);
    document.addEventListener('mousedown', onDoc);
    return () => document.removeEventListener('mousedown', onDoc);
  }, []);
  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="flex items-center gap-1 rounded-lg px-2 py-1 text-[13px] text-zinc-400 transition-colors hover:bg-zinc-800 hover:text-zinc-200"
      >
        {humanizeModel(currentModel) || 'Model'}
        <ChevronDown size={14} className={cn('transition-transform', open && 'rotate-180')} />
      </button>
      {open && (
        <div className="absolute bottom-full left-0 z-20 mb-2 max-h-72 w-60 overflow-y-auto rounded-xl border border-zinc-800 bg-[#15181d] p-1 shadow-2xl">
          {models.length === 0 && <div className="px-3 py-2 text-xs text-zinc-600">No models available</div>}
          {models.map((m) => (
            <button
              key={m.name}
              type="button"
              onClick={() => {
                onSelect(m.name);
                setOpen(false);
              }}
              className="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-[13px] text-zinc-200 hover:bg-zinc-800"
            >
              <span className="flex-1 truncate">
                {humanizeModel(m.name)}
                {m.name === defaultModel && <span className="ml-1 text-[11px] text-zinc-500">· default</span>}
                {m.ready === false && <span className="ml-1 text-[11px] text-amber-500/80">· offline</span>}
              </span>
              {m.name === currentModel && <Check size={15} className="shrink-0 text-emerald-400" />}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

// ConfirmDialog is a styled modal replacing the browser's confirm() box.
function ConfirmDialog({ open, title, body, confirmLabel, onConfirm, onCancel }) {
  if (!open) return null;
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/60 backdrop-blur-sm" onClick={onCancel} />
      <div className="relative w-full max-w-sm rounded-2xl border border-zinc-800 bg-[#15181d] p-5 shadow-2xl">
        <h3 className="text-base font-semibold text-zinc-100">{title}</h3>
        {body && <p className="mt-1.5 text-sm text-zinc-400">{body}</p>}
        <div className="mt-5 flex justify-end gap-2">
          <Button variant="outline" onClick={onCancel}>
            Cancel
          </Button>
          <Button className="bg-red-600 text-white hover:bg-red-500" onClick={onConfirm}>
            {confirmLabel || 'Delete'}
          </Button>
        </div>
      </div>
    </div>
  );
}

export function ChatWindow() {
  const {
    threads,
    activeThread,
    activeThreadId,
    loading,
    models,
    defaultModel,
    currentModel,
    createNewThread,
    selectThread,
    removeThread,
    send,
    setModel,
    stop,
  } = useConversations();
  const [draft, setDraft] = useState('');
  const [busy, setBusy] = useState(false);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [pendingDelete, setPendingDelete] = useState(null);
  const endRef = useRef(null);

  const messages = (activeThread?.messages || []).filter(
    (m) => m.role === 'user' || m.role === 'assistant' || m.role === 'tool' || m.role === 'note',
  );

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages.length, activeThread, busy]);

  const handleSend = async (event) => {
    event.preventDefault();
    if (!draft.trim() || busy) return;
    const text = draft.trim();
    setDraft('');
    setBusy(true);
    await send(text);
    setBusy(false);
  };

  const onKeyDown = (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend(e);
    }
  };

  const last = messages[messages.length - 1];
  const waiting = busy && !(last && last.role === 'assistant' && last.content);

  const sidebar = (
    <div className="flex h-full w-64 flex-col border-r border-zinc-800 bg-[#0e1014]">
      <div className="flex items-center justify-between px-4 py-4">
        <Brand />
        <button className="text-zinc-500 hover:text-zinc-200 md:hidden" onClick={() => setSidebarOpen(false)}>
          <X size={18} />
        </button>
      </div>
      <div className="px-3">
        <Button
          variant="outline"
          className="w-full justify-start"
          onClick={() => {
            createNewThread();
            setSidebarOpen(false);
          }}
        >
          <Plus size={16} /> New chat
        </Button>
      </div>
      <div className="mt-4 flex-1 space-y-0.5 overflow-y-auto px-3 pb-4">
        {threads.length === 0 && <div className="px-2 py-3 text-xs text-zinc-600">No conversations yet.</div>}
        {threads.map((thread) => (
          <div
            key={thread.id}
            className={cn(
              'group flex items-center rounded-lg pr-1 transition-colors',
              activeThreadId === thread.id ? 'bg-zinc-800/80' : 'hover:bg-zinc-800/40',
            )}
          >
            <button
              onClick={() => {
                selectThread(thread.id);
                setSidebarOpen(false);
              }}
              title={thread.title}
              className="min-w-0 flex-1 px-3 py-2 text-left"
            >
              <div className="truncate text-[13px] text-zinc-200">{thread.title || 'Untitled'}</div>
              {thread.model && (
                <div className="truncate text-[11px] text-zinc-500">{humanizeModel(thread.model)}</div>
              )}
            </button>
            <button
              onClick={(e) => {
                e.stopPropagation();
                setPendingDelete(thread.id);
              }}
              title="Delete chat"
              className="shrink-0 rounded p-1.5 text-zinc-600 opacity-0 transition-opacity hover:text-red-400 group-hover:opacity-100"
            >
              <Trash2 size={14} />
            </button>
          </div>
        ))}
      </div>
    </div>
  );

  return (
    <div className="flex h-full">
      <div className="hidden md:block">{sidebar}</div>

      {sidebarOpen && (
        <div className="fixed inset-0 z-40 md:hidden">
          <div className="absolute inset-0 bg-black/60" onClick={() => setSidebarOpen(false)} />
          <div className="absolute left-0 top-0 h-full">{sidebar}</div>
        </div>
      )}

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 shrink-0 items-center gap-3 border-b border-zinc-800 px-4">
          <button className="text-zinc-400 hover:text-zinc-100 md:hidden" onClick={() => setSidebarOpen(true)}>
            <Menu size={20} />
          </button>
          <div className="truncate text-sm font-medium text-zinc-300">
            {activeThread?.thread?.title || 'New chat'}
          </div>
        </header>

        <div className="flex-1 overflow-y-auto">
          <div className="mx-auto max-w-3xl px-4 py-6">
            {loading ? (
              <div className="mt-24 text-center text-sm text-zinc-500">Loading…</div>
            ) : messages.length === 0 ? (
              <div className="mt-24 flex flex-col items-center text-center">
                <span className="mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-gradient-to-br from-emerald-500 to-teal-600 text-white shadow-lg">
                  <Compass size={28} />
                </span>
                <h2 className="text-xl font-semibold text-zinc-100">How can I help?</h2>
                <p className="mt-1 text-sm text-zinc-500">Ask a question or describe a task to get started.</p>
              </div>
            ) : (
              messages.map((message) => {
                if (message.role === 'tool') {
                  const step = normalizeTool(message);
                  return step ? <ToolCard key={message.id} {...step} /> : null;
                }
                if (message.role === 'note') {
                  return (
                    <div key={message.id} className="mb-6 flex justify-center">
                      <span className="flex items-center gap-1.5 rounded-full border border-zinc-800 bg-zinc-900/80 px-3 py-1 text-xs text-zinc-500">
                        <Square size={10} fill="currentColor" /> Paused
                      </span>
                    </div>
                  );
                }
                const isUser = message.role === 'user';
                return (
                  <div key={message.id} className={cn('mb-6 flex gap-3', isUser && 'justify-end')}>
                    {!isUser && (
                      <span className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-gradient-to-br from-emerald-500 to-teal-600 text-white">
                        <Sparkles size={16} />
                      </span>
                    )}
                    <div
                      className={cn(
                        'min-w-0',
                        isUser
                          ? 'max-w-[80%] rounded-2xl bg-zinc-800 px-4 py-2.5 text-[0.95rem] text-zinc-100'
                          : 'flex-1 pt-1',
                      )}
                    >
                      {isUser ? (
                        <span className="whitespace-pre-wrap">{message.content}</span>
                      ) : (
                        <Markdown>{message.content}</Markdown>
                      )}
                    </div>
                  </div>
                );
              })
            )}
            {waiting && (
              <div className="mb-6 flex gap-3">
                <span className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-gradient-to-br from-emerald-500 to-teal-600 text-white">
                  <Sparkles size={16} />
                </span>
                <div className="flex-1 pt-1">
                  <Loader />
                </div>
              </div>
            )}
            <div ref={endRef} />
          </div>
        </div>

        <div className="shrink-0 px-4 pb-5 pt-2">
          <form onSubmit={handleSend} className="mx-auto max-w-3xl">
            <div className="rounded-2xl border border-zinc-800 bg-[#15181d] px-2 pb-2 pt-2 shadow-lg focus-within:border-zinc-600">
              <textarea
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
                onKeyDown={onKeyDown}
                rows={1}
                placeholder="Message Pilot…"
                className="max-h-40 w-full resize-none bg-transparent px-2 py-1.5 text-[0.95rem] text-zinc-100 outline-none placeholder:text-zinc-600"
              />
              <div className="flex items-center justify-between gap-2 pl-1">
                <ModelPicker
                  models={models}
                  currentModel={currentModel}
                  defaultModel={defaultModel}
                  onSelect={setModel}
                />
                {busy ? (
                  <Button type="button" size="icon" onClick={stop} aria-label="Stop" title="Pause">
                    <Square size={15} strokeWidth={2.4} fill="currentColor" />
                  </Button>
                ) : (
                  <Button type="submit" size="icon" disabled={!draft.trim()} aria-label="Send">
                    <ArrowUp size={18} strokeWidth={2.4} />
                  </Button>
                )}
              </div>
            </div>
            <p className="mt-2 text-center text-[11px] text-zinc-600">
              Pilot runs a local model · responses may be imperfect
            </p>
          </form>
        </div>
      </div>

      <ConfirmDialog
        open={!!pendingDelete}
        title="Delete this chat?"
        body="This conversation will be permanently removed."
        confirmLabel="Delete"
        onCancel={() => setPendingDelete(null)}
        onConfirm={() => {
          removeThread(pendingDelete);
          setPendingDelete(null);
        }}
      />
    </div>
  );
}
