import { useEffect, useRef, useState } from 'react';
import { ArrowUp, Check, ChevronDown, Sparkles, Square } from 'lucide-react';
import { listModels } from '../../lib/api';
import { cn, humanizeModel } from '../../lib/utils';
import { Button } from '../../components/ui/button';
import { Markdown } from '../../components/chat/Markdown';
import { Reasoning } from '../../components/chat/Reasoning';
import { ToolCard } from '../../components/chat/ToolCard';
import { Loader } from '../../components/chat/Loader';
import { DiffView } from '../../components/chat/DiffView';
import { useCodeAgent } from './useCodeAgent';

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
        <div className="absolute bottom-full left-0 z-20 mb-2 max-h-72 w-60 overflow-y-auto rounded-xl border border-zinc-800 bg-[#141416] p-1 shadow-2xl">
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

export function AgentPanel({ root, activePath, onDone, onFileChange }) {
  const { messages, busy, send, stop, pendingConfirm, respondConfirm, resume, newSession } = useCodeAgent(onFileChange);
  const [models, setModels] = useState([]);
  const [defaultModel, setDefaultModel] = useState(null);
  const [currentModel, setCurrentModel] = useState(null);
  const [draft, setDraft] = useState('');
  // 'ask' pauses the agent for approval before each file edit or command;
  // 'auto' applies changes without asking.
  const [mode, setMode] = useState('ask');
  const [feedback, setFeedback] = useState('');
  const endRef = useRef(null);
  const scrollRef = useRef(null);
  const atBottomRef = useRef(true);

  useEffect(() => {
    listModels()
      .then((d) => {
        setModels(d.models || []);
        setDefaultModel(d.default || null);
        setCurrentModel(d.default || null);
      })
      .catch(() => {});
  }, []);

  // Resume the project's latest session on open/reload, so the conversation
  // persists (and matches what the terminal shows for the same folder).
  useEffect(() => {
    if (root) resume(root);
  }, [root, resume]);

  const onScroll = () => {
    const el = scrollRef.current;
    if (el) atBottomRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 120;
  };

  useEffect(() => {
    if (atBottomRef.current) endRef.current?.scrollIntoView({ behavior: 'auto' });
  }, [messages.length, busy]);

  const handleSend = async (e) => {
    e.preventDefault();
    if (!draft.trim() || busy || !root) return;
    const text = draft.trim();
    setDraft('');
    atBottomRef.current = true;
    await send(text, root, currentModel || defaultModel, mode, () => onDone?.());
  };

  const approve = () => respondConfirm('approve');
  const reject = () => {
    respondConfirm('decline', feedback.trim());
    setFeedback('');
  };

  const last = messages[messages.length - 1];
  const waiting = busy && !(last && last.role === 'assistant' && (last.content || last.reasoning));

  return (
    <div className="flex w-[360px] shrink-0 flex-col border-l border-zinc-800 bg-[#101012]">
      <header className="flex h-12 shrink-0 items-center gap-2 border-b border-zinc-800 px-3">
        <Sparkles size={15} className="text-emerald-400" />
        <span className="text-sm font-medium text-zinc-300">Agent</span>
        <button
          type="button"
          onClick={newSession}
          disabled={busy}
          className="ml-auto rounded-lg px-2 py-1 text-[13px] text-zinc-400 transition-colors hover:bg-zinc-800 hover:text-zinc-200 disabled:opacity-40"
          title="New chat"
        >
          New
        </button>
      </header>

      <div ref={scrollRef} onScroll={onScroll} className="flex-1 overflow-y-auto px-3 py-4">
        {messages.length === 0 && !busy && (
          <div className="mt-8 flex flex-col items-center text-center">
            <span className="mb-3 flex h-10 w-10 items-center justify-center rounded-2xl bg-gradient-to-br from-emerald-500 to-teal-600 text-white shadow-lg">
              <Sparkles size={18} />
            </span>
            <p className="text-sm text-zinc-500">Ask the agent to edit files, run commands, or explain code.</p>
          </div>
        )}

        {messages.map((message) => {
          if (message.role === 'tool') {
            const step = normalizeTool(message);
            if (!step) return null;
            return (
              <div key={message.id}>
                <ToolCard {...step} />
                {message.diff && <DiffView diff={message.diff} className="mb-3 mt-1 max-h-72" />}
              </div>
            );
          }
          if (message.role === 'error') {
            return (
              <div key={message.id} className="mb-4 ml-10 rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-2 text-[13px] text-red-400">
                {message.content}
              </div>
            );
          }
          const isUser = message.role === 'user';
          return (
            <div key={message.id} className={cn('mb-4 flex gap-3', isUser && 'justify-end')}>
              {!isUser && (
                <span className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-gradient-to-br from-emerald-500 to-teal-600 text-white">
                  <Sparkles size={14} />
                </span>
              )}
              <div
                className={cn(
                  'min-w-0',
                  isUser
                    ? 'max-w-[85%] rounded-2xl bg-zinc-800 px-3 py-2 text-[0.85rem] text-zinc-100'
                    : 'flex-1 pt-0.5',
                )}
              >
                {isUser ? (
                  <span className="whitespace-pre-wrap">{message.content}</span>
                ) : (
                  <>
                    <Reasoning>{message.reasoning}</Reasoning>
                    <Markdown>{message.content}</Markdown>
                  </>
                )}
              </div>
            </div>
          );
        })}
        {waiting && (
          <div className="mb-4 flex gap-3">
            <span className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-gradient-to-br from-emerald-500 to-teal-600 text-white">
              <Sparkles size={14} />
            </span>
            <div className="flex-1 pt-0.5">
              <Loader />
            </div>
          </div>
        )}
        <div ref={endRef} />
      </div>

      {pendingConfirm && (
        <div className="mx-3 mb-2 shrink-0 rounded-xl border border-amber-500/40 bg-amber-500/10 p-3">
          <div className="flex items-center gap-2 text-[13px] font-medium text-amber-300">
            <Check size={14} />
            Approve this action?
          </div>
          <p className="mt-1 break-words text-[12px] text-zinc-300">
            <span className="font-mono text-amber-200/90">{pendingConfirm.tool}</span>
            {pendingConfirm.summary ? ` — ${pendingConfirm.summary}` : ''}
          </p>
          {pendingConfirm.diff && (
            <>
              <p className="mt-1 font-mono text-[11px] text-zinc-500">
                {pendingConfirm.diff.path}
                {' '}
                <span className="text-emerald-400">+{pendingConfirm.diff.added}</span>{' '}
                <span className="text-red-400">-{pendingConfirm.diff.removed}</span>
              </p>
              <DiffView diff={pendingConfirm.diff} className="mt-2 max-h-64" />
            </>
          )}
          <input
            value={feedback}
            onChange={(e) => setFeedback(e.target.value)}
            placeholder="Optional: tell the agent what to do instead"
            className="mt-2 w-full rounded-lg border border-zinc-800 bg-[#101012] px-2 py-1.5 text-[12px] text-zinc-200 outline-none placeholder:text-zinc-600 focus:border-zinc-600"
          />
          <div className="mt-2 flex gap-2">
            <Button type="button" size="sm" onClick={approve} className="flex-1">
              Approve
            </Button>
            <Button type="button" size="sm" variant="secondary" onClick={reject} className="flex-1">
              Reject
            </Button>
          </div>
        </div>
      )}

      <div className="shrink-0 px-3 pb-3 pt-2">
        <form onSubmit={handleSend}>
          <div className="rounded-2xl border border-zinc-800 bg-[#141416] px-2 pb-2 pt-2 shadow-lg focus-within:border-zinc-600">
            <textarea
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && !e.shiftKey) {
                  e.preventDefault();
                  handleSend(e);
                }
              }}
              rows={2}
              placeholder={root ? 'Describe what to do...' : 'Open a folder first...'}
              disabled={!root}
              className="max-h-32 w-full resize-none bg-transparent px-2 py-1.5 text-[0.85rem] text-zinc-100 outline-none placeholder:text-zinc-600 disabled:opacity-40"
            />
            <div className="flex items-center justify-between gap-2 pl-1">
              <div className="flex items-center gap-1">
                <ModelPicker
                  models={models}
                  currentModel={currentModel}
                  defaultModel={defaultModel}
                  onSelect={setCurrentModel}
                />
                <button
                  type="button"
                  onClick={() => setMode((m) => (m === 'ask' ? 'auto' : 'ask'))}
                  title={mode === 'ask' ? 'Ask before each change' : 'Apply changes automatically'}
                  className={cn(
                    'rounded-lg px-2 py-1 text-[13px] transition-colors hover:bg-zinc-800',
                    mode === 'ask' ? 'text-amber-400' : 'text-zinc-400',
                  )}
                >
                  {mode === 'ask' ? 'Ask' : 'Auto'}
                </button>
              </div>
              {busy ? (
                <Button type="button" size="icon" onClick={stop} aria-label="Stop" title="Stop">
                  <Square size={15} strokeWidth={2.4} fill="currentColor" />
                </Button>
              ) : (
                <Button type="submit" size="icon" disabled={!draft.trim() || !root} aria-label="Send">
                  <ArrowUp size={18} strokeWidth={2.4} />
                </Button>
              )}
            </div>
          </div>
        </form>
      </div>
    </div>
  );
}
