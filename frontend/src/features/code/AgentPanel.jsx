import { useEffect, useMemo, useRef, useState } from 'react';
import { ArrowUp, AtSign, Check, ChevronDown, Eye, History, Paperclip, Plus, Sparkles, Square, Trash2, X } from 'lucide-react';
import { code, listModels } from '../../lib/api';
import { cn, humanizeModel } from '../../lib/utils';
import { Button } from '../../components/ui/button';
import { AgentContent } from '../../components/chat/AgentContent';
import { Reasoning } from '../../components/chat/Reasoning';
import { ToolCard } from '../../components/chat/ToolCard';
import { SettingsButton } from '../settings/SettingsButton';
import { Loader } from '../../components/chat/Loader';
import { DiffView } from '../../components/chat/DiffView';
import { FileIcon } from './fileIcons';
import { useCodeAgent } from './useCodeAgent';

let attSeq = 0;
const attId = () => `att${Date.now()}_${attSeq++}`;

// flattenFiles collapses the project tree into a flat file list for @-mention.
function flattenFiles(tree) {
  const out = [];
  const walk = (nodes) => {
    for (const n of nodes || []) {
      if (n.type === 'dir') walk(n.children);
      else out.push({ name: n.name, path: n.path });
    }
  };
  walk(tree);
  return out;
}

// detectMention finds an in-progress "@token" ending at the caret: an @ at the
// start or after whitespace, with no whitespace between it and the caret.
function detectMention(value, caret) {
  const upto = value.slice(0, caret);
  const at = upto.lastIndexOf('@');
  if (at < 0) return null;
  if (at > 0 && !/\s/.test(upto[at - 1])) return null;
  const query = upto.slice(at + 1);
  if (/\s/.test(query)) return null;
  return { start: at, query };
}

// rankMentions filters + orders files for the @-popup: name-prefix beats
// name-substring beats path-substring, shorter paths first.
function rankMentions(files, query) {
  const q = query.toLowerCase();
  return files
    .map((f) => {
      const name = f.name.toLowerCase();
      const path = f.path.toLowerCase();
      let score = 0;
      if (!q) score = 1;
      else if (name === q) score = 4;
      else if (name.startsWith(q)) score = 3;
      else if (name.includes(q)) score = 2;
      else if (path.includes(q)) score = 1;
      return { ...f, score };
    })
    .filter((f) => f.score > 0)
    .sort((a, b) => b.score - a.score || a.path.length - b.path.length)
    .slice(0, 50);
}

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

// SessionMenu is the history switcher: start a new chat, jump to a past one, or
// delete one. Sessions live in <root>/.pilot/sessions and are shared with the
// terminal, so this lists both.
function SessionMenu({ sessions, sessionId, onSelect, onDelete, onNew, disabled }) {
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
        title="Chat history"
      >
        <History size={14} />
        <ChevronDown size={13} className={cn('transition-transform', open && 'rotate-180')} />
      </button>
      {open && (
        <div className="absolute right-0 top-full z-30 mt-2 max-h-96 w-72 overflow-y-auto rounded-xl border border-zinc-800 bg-[#141416] p-1 shadow-2xl">
          <button
            type="button"
            disabled={disabled}
            onClick={() => {
              onNew();
              setOpen(false);
            }}
            className="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-[13px] text-zinc-200 hover:bg-zinc-800 disabled:opacity-40"
          >
            <Plus size={14} className="text-emerald-400" /> New chat
          </button>
          {sessions.length > 0 && <div className="my-1 border-t border-zinc-800" />}
          {sessions.map((s) => (
            <div
              key={s.id}
              className={cn(
                'group flex items-center gap-1 rounded-lg pr-1 hover:bg-zinc-800',
                s.id === sessionId ? 'text-zinc-100' : 'text-zinc-400',
              )}
            >
              <button
                type="button"
                disabled={disabled}
                onClick={() => {
                  onSelect(s.id);
                  setOpen(false);
                }}
                className="flex min-w-0 flex-1 items-center gap-2 px-3 py-2 text-left text-[13px] disabled:opacity-40"
              >
                {s.id === sessionId ? (
                  <Check size={13} className="shrink-0 text-emerald-400" />
                ) : (
                  <span className="w-[13px] shrink-0" />
                )}
                <span className="truncate">{s.title || 'Untitled chat'}</span>
              </button>
              <button
                type="button"
                onClick={() => onDelete(s.id)}
                className="shrink-0 rounded p-1 text-zinc-600 opacity-0 transition-opacity hover:text-red-400 group-hover:opacity-100"
                title="Delete chat"
                aria-label="Delete chat"
              >
                <Trash2 size={13} />
              </button>
            </div>
          ))}
          {sessions.length === 0 && (
            <div className="px-3 py-2 text-[12px] text-zinc-600">No saved chats yet</div>
          )}
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

export function AgentPanel({
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
  const {
    messages,
    busy,
    send,
    stop,
    pendingConfirm,
    respondConfirm,
    resume,
    newSession,
    sessions,
    sessionId,
    switchSession,
    removeSession,
  } = useCodeAgent(onFileChange, onActivity);
  const [models, setModels] = useState([]);
  const [defaultModel, setDefaultModel] = useState(null);
  const [currentModel, setCurrentModel] = useState(null);
  const [draft, setDraft] = useState('');
  // 'ask' pauses the agent for approval before each file edit or command;
  // 'auto' applies changes without asking.
  const [mode, setMode] = useState('ask');
  const [resumed, setResumed] = useState(false);
  // Attached context sent with the next message: uploaded docs (kind 'doc', with
  // extracted text) and @-mentioned project files (kind 'file', resolved on send).
  const [attachments, setAttachments] = useState([]);
  const [uploading, setUploading] = useState(false);
  const [dragging, setDragging] = useState(false);
  // The in-progress @-mention ({start, query}) and the highlighted match.
  const [mention, setMention] = useState(null);
  const [mentionIndex, setMentionIndex] = useState(0);
  const endRef = useRef(null);
  const scrollRef = useRef(null);
  const atBottomRef = useRef(true);
  const sentRef = useRef(false); // the initial prompt is sent at most once
  const textareaRef = useRef(null);
  const fileInputRef = useRef(null);
  const activeItemRef = useRef(null);
  const dismissedRef = useRef(-1); // @-start dismissed with Escape, so it stays closed

  const files = useMemo(() => flattenFiles(tree), [tree]);
  const matches = useMemo(
    () => (mention ? rankMentions(files, mention.query) : []),
    [mention, files],
  );
  useEffect(() => setMentionIndex(0), [mention?.query]);
  useEffect(() => {
    activeItemRef.current?.scrollIntoView({ block: 'nearest' });
  }, [mentionIndex]);

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
    setResumed(false);
    sentRef.current = false;
    if (!root) return;
    resume(root).finally(() => setResumed(true));
  }, [root, resume]);

  // A project created from the New project dialog carries a first instruction.
  // Send it once the project's history has loaded (resume clears the panel, so
  // sending earlier would wipe the prompt) and a model is known. The run goes in
  // auto mode — scaffolding a fresh project should not stop at every write — and
  // the panel's toggle shows that.
  useEffect(() => {
    if (!initialPrompt || sentRef.current || !root || !resumed) return;
    const model = currentModel || defaultModel;
    if (!model) return;
    sentRef.current = true;
    setMode('auto');
    onInitialPromptSent?.();
    // initialPrompt is { text, attachments }: the spec/PRD rides along as an
    // attached document (analyze-as-context), never as a file written into the project.
    send(initialPrompt.text, root, model, 'auto', () => onDone?.(), initialPrompt.attachments || []);
  }, [initialPrompt, resumed, root, currentModel, defaultModel, send, onDone, onInitialPromptSent]);

  // Bubble the pending confirm (and its responder) up to CodePage, so the full
  // diff + Approve/Reject can render in the center pane, not just this column.
  useEffect(() => {
    onConfirmChange?.(pendingConfirm ? { confirm: pendingConfirm, respond: respondConfirm } : null);
  }, [pendingConfirm, respondConfirm, onConfirmChange]);

  const onScroll = () => {
    const el = scrollRef.current;
    if (el) atBottomRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 120;
  };

  useEffect(() => {
    if (atBottomRef.current) endRef.current?.scrollIntoView({ behavior: 'auto' });
  }, [messages.length, busy]);

  const removeAttachment = (id) => setAttachments((prev) => prev.filter((a) => a.id !== id));

  // uploadDoc extracts a dropped/picked document server-side and attaches its text.
  const uploadDoc = async (file) => {
    setUploading(true);
    try {
      const { filename, text } = await code.extractDocument(file);
      setAttachments((prev) => [...prev, { id: attId(), kind: 'doc', name: filename, text }]);
    } catch (e) {
      setAttachments((prev) => [
        ...prev,
        { id: attId(), kind: 'doc', name: file.name, error: String(e.message || e) },
      ]);
    } finally {
      setUploading(false);
    }
  };

  const onPickFiles = (e) => {
    Array.from(e.target.files || []).forEach(uploadDoc);
    e.target.value = ''; // let the same file be picked again
  };

  const onDrop = (e) => {
    e.preventDefault();
    setDragging(false);
    Array.from(e.dataTransfer?.files || []).forEach(uploadDoc);
  };

  // recompute updates the @-mention state as the caret/text changes, honouring a
  // token the user dismissed with Escape (so it does not immediately reopen).
  const recompute = (value, caret) => {
    const m = detectMention(value, caret);
    if (!m) dismissedRef.current = -1;
    setMention(m && m.start === dismissedRef.current ? null : m);
  };

  const onDraftChange = (e) => {
    setDraft(e.target.value);
    recompute(e.target.value, e.target.selectionStart);
  };

  // chooseFile replaces the @token with @path, attaches the file (deduped), and
  // drops the caret just past the inserted reference.
  const chooseFile = (file) => {
    if (!mention) return;
    const el = textareaRef.current;
    const caret = el ? el.selectionStart : draft.length;
    const before = draft.slice(0, mention.start);
    const insert = `@${file.path} `;
    const next = before + insert + draft.slice(caret);
    setDraft(next);
    setAttachments((prev) =>
      prev.some((a) => a.kind === 'file' && a.path === file.path)
        ? prev
        : [...prev, { id: attId(), kind: 'file', name: file.name, path: file.path }],
    );
    setMention(null);
    dismissedRef.current = -1;
    requestAnimationFrame(() => {
      const pos = before.length + insert.length;
      el?.focus();
      el?.setSelectionRange(pos, pos);
    });
  };

  const onDraftKeyDown = (e) => {
    if (mention && matches.length) {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setMentionIndex((i) => (i + 1) % matches.length);
        return;
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault();
        setMentionIndex((i) => (i - 1 + matches.length) % matches.length);
        return;
      }
      if (e.key === 'Enter' || e.key === 'Tab') {
        e.preventDefault();
        chooseFile(matches[mentionIndex]);
        return;
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        dismissedRef.current = mention.start;
        setMention(null);
        return;
      }
    }
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend(e);
    }
  };

  const handleSend = async (e) => {
    e?.preventDefault?.();
    if (busy || !root) return;
    const text = draft.trim();
    const atts = attachments.filter((a) => !a.error);
    if (!text && atts.length === 0) return;
    // @-mentioned files carry only a path; fetch their content now (docs already
    // hold their extracted text).
    const resolved = await Promise.all(
      atts.map(async (a) => {
        if (a.kind === 'file' && a.text == null) {
          try {
            const { content } = await code.readFile(root, a.path);
            return { ...a, text: content };
          } catch {
            return { ...a, text: '' };
          }
        }
        return a;
      }),
    );
    setDraft('');
    setAttachments([]);
    setMention(null);
    dismissedRef.current = -1;
    atBottomRef.current = true;
    await send(text, root, currentModel || defaultModel, mode, () => onDone?.(), resolved);
  };

  const approve = () => respondConfirm('approve');
  const reject = () => respondConfirm('decline');

  const last = messages[messages.length - 1];
  const waiting = busy && !(last && last.role === 'assistant' && (last.content || last.reasoning));

  return (
    <div className="flex w-[360px] shrink-0 flex-col border-l border-zinc-800 bg-[#101012]">
      <header className="flex h-12 shrink-0 items-center gap-2 border-b border-zinc-800 px-3">
        <Sparkles size={15} className="text-emerald-400" />
        <span className="text-sm font-medium text-zinc-300">Agent</span>
        <div className="ml-auto flex items-center gap-1">
          <SessionMenu
            sessions={sessions}
            sessionId={sessionId}
            onSelect={(id) => switchSession(root, id)}
            onDelete={(id) => removeSession(root, id)}
            onNew={newSession}
            disabled={busy}
          />
          <button
            type="button"
            onClick={newSession}
            disabled={busy}
            className="rounded-lg px-2 py-1 text-[13px] text-zinc-400 transition-colors hover:bg-zinc-800 hover:text-zinc-200 disabled:opacity-40"
            title="New chat"
          >
            New
          </button>
        </div>
        <SettingsButton />
      </header>

      <div ref={scrollRef} onScroll={onScroll} className="min-h-0 flex-1 overflow-y-auto px-3 py-4">
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
                  <>
                    {message.content && <span className="whitespace-pre-wrap break-words [overflow-wrap:anywhere]">{message.content}</span>}
                    {message.attachments?.length > 0 && (
                      <div className={cn('flex flex-wrap gap-1', message.content && 'mt-1.5')}>
                        {message.attachments.map((a, i) => (
                          <span
                            key={i}
                            className="flex items-center gap-1 rounded-md bg-zinc-900/70 px-1.5 py-0.5 text-[10px] text-zinc-400"
                            title={a.path || a.name}
                          >
                            {a.kind === 'file' ? (
                              <AtSign size={10} className="text-fuchsia-400" />
                            ) : (
                              <Paperclip size={10} className="text-indigo-400" />
                            )}
                            <span className="max-w-[180px] truncate">{a.path || a.name}</span>
                          </span>
                        ))}
                      </div>
                    )}
                  </>
                ) : (
                  <>
                    <Reasoning>{message.reasoning}</Reasoning>
                    <AgentContent>{message.content}</AgentContent>
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
            {pendingConfirm.summary ? `: ${pendingConfirm.summary}` : ''}
          </p>
          {pendingConfirm.diff && (
            <div className="mt-1.5 flex items-center gap-2">
              <p className="min-w-0 flex-1 truncate font-mono text-[11px] text-zinc-500">
                {pendingConfirm.diff.path}{' '}
                <span className="text-emerald-400">+{pendingConfirm.diff.added}</span>{' '}
                <span className="text-red-400">-{pendingConfirm.diff.removed}</span>
              </p>
              <button
                type="button"
                onClick={onViewDiff}
                className="flex shrink-0 items-center gap-1 rounded-lg border border-amber-500/40 px-2 py-1 text-[11px] text-amber-200 transition-colors hover:bg-amber-500/10"
              >
                <Eye size={12} /> View diff
              </button>
            </div>
          )}
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
          <div
            onDragOver={(e) => {
              if (!root) return;
              e.preventDefault();
              if (!dragging) setDragging(true);
            }}
            onDragLeave={(e) => {
              if (!e.currentTarget.contains(e.relatedTarget)) setDragging(false);
            }}
            onDrop={onDrop}
            className={cn(
              'relative rounded-2xl border border-zinc-800 bg-[#141416] px-2 pb-2 pt-2 shadow-lg focus-within:border-zinc-600',
              dragging && 'border-fuchsia-500/60 ring-1 ring-fuchsia-500/40',
            )}
          >
            {mention && matches.length > 0 && (
              <div className="absolute bottom-full left-0 z-30 mb-2 max-h-64 w-full overflow-y-auto rounded-xl border border-zinc-800 bg-[#141416] p-1 shadow-2xl">
                {matches.map((f, i) => (
                  <button
                    key={f.path}
                    ref={i === mentionIndex ? activeItemRef : null}
                    type="button"
                    onMouseDown={(e) => {
                      e.preventDefault();
                      chooseFile(f);
                    }}
                    onMouseEnter={() => setMentionIndex(i)}
                    className={cn(
                      'flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-[13px]',
                      i === mentionIndex ? 'bg-zinc-800 text-zinc-100' : 'text-zinc-300 hover:bg-zinc-800/60',
                    )}
                  >
                    <FileIcon name={f.name} size={14} />
                    <span className="shrink-0 truncate">{f.name}</span>
                    <span className="ml-auto min-w-0 truncate text-[11px] text-zinc-600">{f.path}</span>
                  </button>
                ))}
              </div>
            )}

            {attachments.length > 0 && (
              <div className="mb-1.5 flex flex-wrap gap-1.5 px-1">
                {attachments.map((a) => (
                  <span
                    key={a.id}
                    title={a.error || a.path || a.name}
                    className={cn(
                      'flex items-center gap-1 rounded-md border px-1.5 py-0.5 text-[11px]',
                      a.error
                        ? 'border-red-500/40 bg-red-500/10 text-red-300'
                        : 'border-zinc-700 bg-zinc-800/60 text-zinc-300',
                    )}
                  >
                    {a.kind === 'file' ? (
                      <AtSign size={11} className="shrink-0 text-fuchsia-400" />
                    ) : (
                      <Paperclip size={11} className="shrink-0 text-indigo-400" />
                    )}
                    <span className="max-w-[140px] truncate">{a.name}</span>
                    <button
                      type="button"
                      onClick={() => removeAttachment(a.id)}
                      className="shrink-0 text-zinc-500 hover:text-zinc-200"
                      aria-label="Remove attachment"
                    >
                      <X size={11} />
                    </button>
                  </span>
                ))}
              </div>
            )}

            <textarea
              ref={textareaRef}
              value={draft}
              onChange={onDraftChange}
              onKeyDown={onDraftKeyDown}
              rows={2}
              placeholder={root ? 'Describe what to do... (@ to link a file)' : 'Open a folder first...'}
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
                <button
                  type="button"
                  onClick={() => fileInputRef.current?.click()}
                  disabled={!root || uploading}
                  title="Attach a document (PRD, spec, .docx/.pdf/.pptx/.xlsx/.txt/.md)"
                  aria-label="Attach a document"
                  className="rounded-lg px-2 py-1 text-zinc-400 transition-colors hover:bg-zinc-800 hover:text-zinc-200 disabled:opacity-40"
                >
                  <Paperclip size={15} className={cn(uploading && 'animate-pulse text-indigo-400')} />
                </button>
                <input
                  ref={fileInputRef}
                  type="file"
                  multiple
                  accept=".docx,.pdf,.pptx,.xlsx,.txt,.md"
                  className="hidden"
                  onChange={onPickFiles}
                />
              </div>
              {busy ? (
                <Button type="button" size="icon" onClick={stop} aria-label="Stop" title="Stop">
                  <Square size={15} strokeWidth={2.4} fill="currentColor" />
                </Button>
              ) : (
                <Button
                  type="submit"
                  size="icon"
                  disabled={(!draft.trim() && attachments.length === 0) || !root}
                  aria-label="Send"
                >
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
