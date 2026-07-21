import { useEffect, useRef, useState } from 'react';
import { ArrowUp, Check, ChevronDown, Sparkles, Square } from 'lucide-react';
import { listModels } from '../../lib/api';
import { cn, humanizeModel } from '../../lib/utils';
import { Button } from '../../components/ui/button';
import { Markdown } from '../../components/chat/Markdown';
import { ToolCard } from '../../components/chat/ToolCard';
import { Loader } from '../../components/chat/Loader';
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

export function AgentPanel({ root, activePath, onDone }) {
  const { messages, busy, send, stop } = useCodeAgent();
  const [models, setModels] = useState([]);
  const [defaultModel, setDefaultModel] = useState(null);
  const [currentModel, setCurrentModel] = useState(null);
  const [draft, setDraft] = useState('');
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
    await send(text, root, currentModel || defaultModel, () => onDone?.());
  };

  const last = messages[messages.length - 1];
  const waiting = busy && !(last && last.role === 'assistant' && (last.content || last.reasoning));

  return (
    <div className="flex w-[360px] shrink-0 flex-col border-l border-zinc-800 bg-[#0e1014]">
      <header className="flex h-12 shrink-0 items-center gap-2 border-b border-zinc-800 px-3">
        <Sparkles size={15} className="text-emerald-400" />
        <span className="text-sm font-medium text-zinc-300">Agent</span>
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
            return step ? <ToolCard key={message.id} {...step} /> : null;
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
                    {message.reasoning && (
                      <div className="mb-2 whitespace-pre-wrap border-l-2 border-zinc-800 pl-3 text-[12px] italic text-zinc-500">
                        {message.reasoning}
                      </div>
                    )}
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

      <div className="shrink-0 px-3 pb-3 pt-2">
        <form onSubmit={handleSend}>
          <div className="rounded-2xl border border-zinc-800 bg-[#15181d] px-2 pb-2 pt-2 shadow-lg focus-within:border-zinc-600">
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
              <ModelPicker
                models={models}
                currentModel={currentModel}
                defaultModel={defaultModel}
                onSelect={setCurrentModel}
              />
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
