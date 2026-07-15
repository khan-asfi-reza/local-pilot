import { useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { ArrowUp, Check, ChevronDown, Compass, Download, FileCode, Loader2, Plus, Square, Sparkles, Terminal } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { ToolCard } from '../../components/chat/ToolCard';
import { Loader } from '../../components/chat/Loader';
import { Markdown } from '../../components/chat/Markdown';
import { cn, humanizeModel } from '../../lib/utils';
import { useBuilder } from './useBuilder';

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

export function Builder() {
  const navigate = useNavigate();
  const {
    messages,
    busy,
    done,
    error,
    writtenFiles,
    tokens,
    models,
    defaultModel,
    currentModel,
    sessionId,
    setCurrentModel,
    setModels,
    setDefaultModel,
    create,
    generate,
    preview,
    stop,
    reset,
    exportToFolder,
  } = useBuilder();

  const [draft, setDraft] = useState('');
  const [exporting, setExporting] = useState(false);
  const endRef = useRef(null);
  const scrollRef = useRef(null);
  const atBottomRef = useRef(true);

  useEffect(() => {
    (async () => {
      try {
        const base =
          import.meta.env.VITE_API_URL || `${window.location.protocol}//${window.location.hostname}:8182`;
        const res = await fetch(`${base}/models`);
        if (res.ok) {
          const data = await res.json();
          setModels(data.models || []);
          setDefaultModel(data.default || null);
        }
      } catch {
        /* backend may be down */
      }
    })();
  }, [setModels, setDefaultModel]);

  const onScroll = () => {
    const el = scrollRef.current;
    if (el) atBottomRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 120;
  };

  useEffect(() => {
    if (atBottomRef.current) endRef.current?.scrollIntoView({ behavior: 'auto' });
  }, [messages.length, busy, writtenFiles.length]);

  const handleSend = async (e) => {
    e.preventDefault();
    if (!draft.trim() || busy) return;
    const text = draft.trim();
    setDraft('');
    atBottomRef.current = true;
    if (!sessionId) {
      await create(text);
    } else {
      await generate(text);
    }
  };

  const handleExport = async () => {
    setExporting(true);
    try {
      await exportToFolder();
    } catch (e) {
      setError(String(e.message || e));
    } finally {
      setExporting(false);
    }
  };

  const previewUrl = preview();
  const waiting = busy && !(messages.length > 0 && messages[messages.length - 1]?.role === 'assistant' && (messages[messages.length - 1].content || messages[messages.length - 1].reasoning));

  return (
    <div className="flex h-full">
      {/* Left panel: build log */}
      <div className="flex w-[420px] shrink-0 flex-col border-r border-zinc-800">
        <header className="flex h-12 shrink-0 items-center justify-between border-b border-zinc-800 px-4">
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => navigate('/')}
              className="flex items-center gap-1.5 rounded-lg p-1 transition-colors hover:bg-zinc-800"
              title="Home"
            >
              <span className="flex h-6 w-6 items-center justify-center rounded-md bg-gradient-to-br from-emerald-500 to-teal-600 text-white">
                <Compass size={13} strokeWidth={2.2} />
              </span>
            </button>
            <Terminal size={16} className="text-emerald-400" />
            <span className="text-sm font-medium text-zinc-300">Build Log</span>
          </div>
          {sessionId && (
            <div className="flex items-center gap-2">
              {tokens > 0 && (
                <span className="text-[11px] text-zinc-600">{tokens.toLocaleString()} tokens</span>
              )}
              <Button
                variant="outline"
                size="sm"
                onClick={reset}
                disabled={busy}
              >
                <Plus size={14} />
                New
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={handleExport}
                disabled={exporting || !done}
              >
                <Download size={14} />
                {exporting ? 'Exporting...' : 'Export'}
              </Button>
            </div>
          )}
        </header>

        <div ref={scrollRef} onScroll={onScroll} className="flex-1 overflow-y-auto px-4 py-4">
          {messages.length === 0 && !busy && (
            <div className="mt-12 flex flex-col items-center text-center">
              <span className="mb-3 flex h-12 w-12 items-center justify-center rounded-2xl bg-gradient-to-br from-emerald-500 to-teal-600 text-white shadow-lg">
                <Sparkles size={22} />
              </span>
              <h2 className="text-lg font-semibold text-zinc-100">What do you want to build?</h2>
              <p className="mt-1 text-sm text-zinc-500">Describe your app and Pilot will generate it live.</p>
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
                      ? 'max-w-[80%] rounded-2xl bg-zinc-800 px-3 py-2 text-[0.9rem] text-zinc-100'
                      : 'flex-1 pt-0.5',
                  )}
                >
                  {isUser ? (
                    <span className="whitespace-pre-wrap">{message.content}</span>
                  ) : (
                    <>
                      {message.reasoning && (
                        <div className="mb-2 whitespace-pre-wrap border-l-2 border-zinc-800 pl-3 text-[13px] italic text-zinc-500">
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
          {writtenFiles.length > 0 && (
            <div className="mb-4 ml-11 rounded-xl border border-zinc-800 bg-[#0e1116] p-3">
              <div className="mb-2 text-xs font-medium text-zinc-500">Files written</div>
              <div className="flex flex-wrap gap-1.5">
                {writtenFiles.map((name) => (
                  <span key={name} className="flex items-center gap-1 rounded-md bg-zinc-800/60 px-2 py-1 text-xs text-zinc-300">
                    <FileCode size={12} className="text-emerald-400" />
                    {name}
                  </span>
                ))}
              </div>
            </div>
          )}
          {error && (
            <div className="mb-4 rounded-xl border border-red-800/50 bg-red-900/20 px-3 py-2 text-sm text-red-400">
              {error}
            </div>
          )}
          <div ref={endRef} />
        </div>

        {/* Composer */}
        <div className="shrink-0 px-4 pb-4 pt-2">
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
                placeholder={sessionId ? 'Describe changes...' : 'Describe the app to build...'}
                className="max-h-40 w-full resize-none bg-transparent px-2 py-1.5 text-[0.95rem] text-zinc-100 outline-none placeholder:text-zinc-600"
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
                  <Button type="submit" size="icon" disabled={!draft.trim()} aria-label="Send">
                    <ArrowUp size={18} strokeWidth={2.4} />
                  </Button>
                )}
              </div>
            </div>
          </form>
        </div>
      </div>

      {/* Right panel: live preview */}
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-12 shrink-0 items-center gap-3 border-b border-zinc-800 px-4">
          <span className="text-sm font-medium text-zinc-300">Preview</span>
          {done && writtenFiles.length > 0 && (
            <span className="rounded-full bg-emerald-500/20 px-2 py-0.5 text-xs text-emerald-400">Ready</span>
          )}
          {busy && <span className="flex items-center gap-1 text-xs text-zinc-500"><Loader2 size={12} className="animate-spin" /> Building...</span>}
        </header>
        <div className="relative flex-1 bg-[#0b0d11]">
          {previewUrl && writtenFiles.length > 0 ? (
            <iframe
              key={previewUrl}
              src={previewUrl}
              className="h-full w-full border-0"
              title="Live preview"
              sandbox="allow-scripts allow-same-origin"
            />
          ) : (
            <div className="flex h-full flex-col items-center justify-center text-center">
              <div className="rounded-2xl border border-zinc-800 bg-[#15181d] p-8">
                {busy ? (
                  <>
                    <Loader />
                    <p className="mt-3 text-sm text-zinc-500">Generating preview...</p>
                  </>
                ) : (
                  <>
                    <p className="text-sm text-zinc-500">Your preview will appear here</p>
                    <p className="mt-1 text-xs text-zinc-600">Enter a prompt to start building</p>
                  </>
                )}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
