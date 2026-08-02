import { useEffect, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import {
  ArrowLeft, ArrowUp, Check, ChevronDown, Compass, Download, Eye, FileCode,
  Loader2, Play, Plus, Sparkles, Square, Trash2, TriangleAlert, X,
} from 'lucide-react';
import CodeMirror from '@uiw/react-codemirror';
import { javascript } from '@codemirror/lang-javascript';
import { vscodeDark } from '@uiw/codemirror-theme-vscode';
import { Button } from '../../components/ui/button';
import { ToolCard } from '../../components/chat/ToolCard';
import { Loader } from '../../components/chat/Loader';
import { Markdown } from '../../components/chat/Markdown';
import { Reasoning } from '../../components/chat/Reasoning';
import { FileIcon } from '../code/fileIcons';
import { cn, humanizeModel } from '../../lib/utils';
import { useBuilder } from './useBuilder';
import { SettingsButton } from '../settings/SettingsButton';

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
        <div className="absolute bottom-full left-0 z-20 mb-2 max-h-72 w-60 overflow-y-auto rounded-xl border border-zinc-800 bg-zinc-850 p-1 shadow-2xl">
          {models.map((m) => (
            <button
              key={m.name}
              type="button"
              onClick={() => { onSelect(m.name); setOpen(false); }}
              className="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-[13px] text-zinc-200 hover:bg-zinc-800"
            >
              <span className="flex-1 truncate">
                {humanizeModel(m.name)}
                {m.name === defaultModel && <span className="ml-1 text-[11px] text-zinc-500">· default</span>}
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
  return { tool: m.tool, info: m.info, input: m.input, output: m.output, running: m.running };
}

// --- Project list --------------------------------------------------------
function ProjectList({ projects, onOpen, onNew, onDelete, navigate }) {
  return (
    <div className="hero-wash h-full overflow-y-auto">
      <header className="flex h-14 items-center justify-between border-b border-zinc-800 px-6">
        <button type="button" onClick={() => navigate('/')} className="flex items-center gap-2 text-zinc-300 hover:text-zinc-100">
          <span className="flex h-7 w-7 items-center justify-center rounded-lg bg-gradient-to-br from-emerald-500 to-teal-600 text-white">
            <Compass size={15} strokeWidth={2.2} />
          </span>
          <span className="text-sm font-medium">App Builder</span>
        </button>
        <div className="flex items-center gap-2">
          <Button size="sm" onClick={onNew} className="gap-1.5">
            <Plus size={15} /> New app
          </Button>
          <SettingsButton />
        </div>
      </header>
      <div className="mx-auto max-w-4xl px-6 py-10">
        <p className="eyebrow mb-2">Your apps</p>
        <h1 className="text-2xl font-semibold tracking-tight text-zinc-100">Build something</h1>

        {projects.length === 0 ? (
          <button
            type="button"
            onClick={onNew}
            className="mt-8 flex w-full flex-col items-center justify-center gap-3 rounded-2xl border border-dashed border-zinc-800 bg-zinc-900/40 py-16 text-zinc-500 transition-colors hover:border-zinc-700 hover:text-zinc-300"
          >
            <Plus size={24} />
            <span className="text-sm">Create your first app</span>
          </button>
        ) : (
          <div className="mt-8 grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {projects.map((p) => (
              <div
                key={p.id}
                className="group relative flex cursor-pointer flex-col gap-2 rounded-2xl border border-zinc-800 bg-zinc-850 p-5 transition-all hover:-translate-y-0.5 hover:border-zinc-700"
                onClick={() => onOpen(p.id)}
              >
                <span className="flex h-9 w-9 items-center justify-center rounded-xl bg-gradient-to-br from-emerald-500 to-teal-600 text-white shadow-glow">
                  <Sparkles size={16} />
                </span>
                <h3 className="truncate text-[15px] font-semibold text-zinc-100">{p.name}</h3>
                <p className="line-clamp-2 text-[12px] leading-relaxed text-zinc-500">{p.prompt || 'Empty project'}</p>
                <button
                  type="button"
                  onClick={(e) => { e.stopPropagation(); onDelete(p.id); }}
                  className="absolute right-3 top-3 rounded-lg p-1.5 text-zinc-600 opacity-0 transition-all hover:bg-zinc-800 hover:text-red-400 group-hover:opacity-100"
                  title="Delete"
                >
                  <Trash2 size={14} />
                </button>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

// --- Source tab ----------------------------------------------------------
function basename(p) { return p.split('/').pop() || p; }

function SourcePanel({ files, readSource, saveSource, consoleErrors, clearConsole }) {
  const [active, setActive] = useState('src/App.jsx');
  const [content, setContent] = useState('');
  const [dirty, setDirty] = useState(false);
  const [saving, setSaving] = useState(false);
  const [autoSave, setAutoSave] = useState(() => localStorage.getItem('builder:autoSave') !== 'false');
  const saveTimer = useRef(null);
  useEffect(() => { localStorage.setItem('builder:autoSave', String(autoSave)); }, [autoSave]);

  useEffect(() => {
    if (!files.length) return;
    if (!files.includes(active)) setActive(files.find((f) => f.endsWith('App.jsx')) || files[0]);
  }, [files]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (!active || dirty) return;
    let cancelled = false;
    readSource(active).then((c) => !cancelled && setContent(c)).catch(() => {});
    return () => { cancelled = true; };
  }, [active, readSource, dirty]);

  const save = async (text = content) => {
    setSaving(true);
    try { await saveSource(active, text); setDirty(false); } finally { setSaving(false); }
  };

  // Auto-save: debounce edits and write them (Vite hot-reloads the preview).
  const onChange = (v) => {
    setContent(v);
    setDirty(true);
    if (!autoSave) return;
    clearTimeout(saveTimer.current);
    saveTimer.current = setTimeout(() => save(v), 700);
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex min-h-0 flex-1">
        <div className="w-56 shrink-0 overflow-y-auto border-r border-zinc-800 bg-[#0c0c0e] py-2">
          {files.map((f) => (
            <button
              key={f}
              type="button"
              onClick={() => { setDirty(false); setActive(f); }}
              className={cn(
                'flex w-full items-center gap-2 px-3 py-1.5 text-left text-[12px] transition-colors',
                f === active ? 'bg-zinc-800 text-zinc-100' : 'text-zinc-400 hover:bg-zinc-800/40 hover:text-zinc-200',
              )}
              title={f}
            >
              <FileIcon name={basename(f)} size={15} />
              <span className="truncate">{f}</span>
            </button>
          ))}
        </div>
        <div className="flex min-h-0 min-w-0 flex-1 flex-col">
          <div className="flex shrink-0 items-center gap-2 border-b border-zinc-800 px-3 py-2">
            <FileIcon name={basename(active)} size={14} />
            <span className="text-[12px] text-zinc-400">{active}</span>
            {dirty && <span className="h-1.5 w-1.5 rounded-full bg-amber-400" title="Unsaved" />}
            <button
              type="button"
              role="switch"
              aria-checked={autoSave}
              onClick={() => setAutoSave((v) => !v)}
              title={autoSave ? 'Auto-save on' : 'Auto-save off'}
              className="ml-auto flex items-center gap-2 rounded-md px-2 py-1 text-[12px] text-zinc-400 transition-colors hover:text-zinc-200"
            >
              <span
                className={cn(
                  'flex h-4 w-7 shrink-0 items-center rounded-full px-0.5 transition-colors',
                  autoSave ? 'bg-emerald-500/80' : 'bg-zinc-600',
                )}
              >
                <span
                  className={cn(
                    'h-3 w-3 rounded-full bg-white shadow-sm transition-transform',
                    autoSave ? 'translate-x-3' : 'translate-x-0',
                  )}
                />
              </span>
              Auto-save
            </button>
            {!autoSave && (
              <Button size="sm" className="gap-1.5" disabled={!dirty || saving} onClick={() => save()}>
                {saving ? 'Saving…' : 'Save'}
              </Button>
            )}
          </div>
          <div className="min-h-0 flex-1 overflow-hidden">
            <CodeMirror
              value={content}
              theme={vscodeDark}
              height="100%"
              extensions={[javascript({ jsx: true })]}
              onChange={onChange}
              basicSetup={{ lineNumbers: true, foldGutter: true }}
            />
          </div>
        </div>
      </div>

      {/* Console: runtime errors forwarded from the live preview. */}
      {consoleErrors.length > 0 && (
        <div className="flex max-h-48 shrink-0 flex-col border-t border-zinc-800 bg-[#0c0c0e]">
          <div className="flex items-center gap-2 border-b border-zinc-800 px-3 py-1.5">
            <TriangleAlert size={13} className="text-red-400" />
            <span className="text-[12px] font-medium text-zinc-300">Console</span>
            <span className="rounded-full bg-red-500/20 px-1.5 text-[11px] text-red-400">{consoleErrors.length}</span>
            <button type="button" onClick={clearConsole} className="ml-auto rounded p-1 text-zinc-500 hover:bg-zinc-800 hover:text-zinc-300" title="Clear">
              <X size={13} />
            </button>
          </div>
          <div className="overflow-y-auto px-3 py-2">
            {consoleErrors.map((e) => (
              <pre
                key={e.id}
                className={cn('whitespace-pre-wrap break-words py-0.5 font-mono text-[11px]', e.level === 'warn' ? 'text-amber-400/90' : 'text-red-400/90')}
              >
                {e.text}
              </pre>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

// --- Project workspace ---------------------------------------------------
function ProjectView(b) {
  const {
    activeName, previewUrl, files, messages, busy, error,
    models, defaultModel, currentModel, setCurrentModel,
    send, stop, readSource, saveSource, run, closeProject, exportUrl, activeId,
    rename, consoleErrors, clearConsole,
  } = b;
  const [draft, setDraft] = useState('');
  const [tab, setTab] = useState('preview');
  const [nameDraft, setNameDraft] = useState(activeName);
  const [editingName, setEditingName] = useState(false);
  useEffect(() => setNameDraft(activeName), [activeName]);

  const commitName = () => {
    setEditingName(false);
    if (nameDraft.trim() && nameDraft.trim() !== activeName) rename(nameDraft.trim());
  };
  const endRef = useRef(null);
  const scrollRef = useRef(null);
  const atBottom = useRef(true);

  const onScroll = () => {
    const el = scrollRef.current;
    if (el) atBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 120;
  };
  useEffect(() => {
    if (atBottom.current) endRef.current?.scrollIntoView({ behavior: 'auto' });
  }, [messages.length, busy]);

  const submit = (e) => {
    e.preventDefault();
    if (!draft.trim() || busy) return;
    const text = draft.trim();
    setDraft('');
    atBottom.current = true;
    send(text);
  };

  const last = messages[messages.length - 1];
  const waiting = busy && !(last && last.role === 'assistant' && (last.content || last.reasoning));

  return (
    <div className="flex h-full">
      {/* Left: build log */}
      <div className="flex w-[420px] shrink-0 flex-col border-r border-zinc-800">
        <header className="flex h-14 shrink-0 items-center gap-2 border-b border-zinc-800 px-4">
          <button type="button" onClick={closeProject} className="rounded-lg p-1.5 text-zinc-400 hover:bg-zinc-800 hover:text-zinc-200" title="All apps">
            <ArrowLeft size={16} />
          </button>
          {editingName ? (
            <input
              autoFocus
              value={nameDraft}
              onChange={(e) => setNameDraft(e.target.value)}
              onBlur={commitName}
              onKeyDown={(e) => { if (e.key === 'Enter') commitName(); if (e.key === 'Escape') { setNameDraft(activeName); setEditingName(false); } }}
              className="min-w-0 flex-1 rounded-md border border-zinc-700 bg-zinc-850 px-2 py-0.5 text-sm text-zinc-100 outline-none"
            />
          ) : (
            <button
              type="button"
              onClick={() => setEditingName(true)}
              className="truncate rounded-md px-1 text-sm font-medium text-zinc-200 hover:bg-zinc-800"
              title="Rename"
            >
              {activeName || 'Untitled app'}
            </button>
          )}
          <a
            href={exportUrl(activeId)}
            className="ml-auto flex items-center gap-1.5 rounded-lg border border-zinc-800 px-2.5 py-1 text-[13px] text-zinc-300 hover:bg-zinc-800"
            title="Download .zip"
          >
            <Download size={14} /> Export
          </a>
        </header>

        <div ref={scrollRef} onScroll={onScroll} className="flex-1 overflow-y-auto px-4 py-4">
          {messages.length === 0 && !busy && (
            <div className="mt-10 flex flex-col items-center text-center">
              <span className="mb-3 flex h-11 w-11 items-center justify-center rounded-2xl bg-gradient-to-br from-emerald-500 to-teal-600 text-white shadow-glow">
                <Sparkles size={20} />
              </span>
              <p className="text-sm text-zinc-500">Describe what to build. The preview updates live.</p>
            </div>
          )}
          {messages.map((m) => {
            if (m.role === 'tool') return <ToolCard key={m.id} {...normalizeTool(m)} />;
            if (m.role === 'error') {
              return (
                <div key={m.id} className="mb-4 ml-10 rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-2 text-[13px] text-red-400">
                  {m.content}
                </div>
              );
            }
            const isUser = m.role === 'user';
            return (
              <div key={m.id} className={cn('mb-4 flex gap-3', isUser && 'justify-end')}>
                {!isUser && (
                  <span className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-gradient-to-br from-emerald-500 to-teal-600 text-white">
                    <Sparkles size={14} />
                  </span>
                )}
                <div className={cn('min-w-0', isUser ? 'max-w-[80%] rounded-2xl bg-zinc-800 px-3 py-2 text-[0.9rem] text-zinc-100' : 'flex-1 pt-0.5')}>
                  {isUser ? <span className="whitespace-pre-wrap">{m.content}</span> : (
                    <>
                      <Reasoning>{m.reasoning}</Reasoning>
                      <Markdown>{m.content}</Markdown>
                    </>
                  )}
                </div>
              </div>
            );
          })}
          {waiting && (
            <div className="mb-4 flex gap-3">
              <span className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-gradient-to-br from-emerald-500 to-teal-600 text-white"><Sparkles size={14} /></span>
              <div className="flex-1 pt-0.5"><Loader /></div>
            </div>
          )}
          {error && <div className="mb-4 rounded-xl border border-red-800/50 bg-red-900/20 px-3 py-2 text-sm text-red-400">{error}</div>}
          <div ref={endRef} />
        </div>

        <div className="shrink-0 px-4 pb-4 pt-2">
          <form onSubmit={submit}>
            <div className="rounded-2xl border border-zinc-800 bg-zinc-850 px-2 pb-2 pt-2 shadow-lg focus-within:border-zinc-600">
              <textarea
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
                onKeyDown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); submit(e); } }}
                rows={2}
                placeholder="Describe the app, or a change…"
                className="max-h-40 w-full resize-none bg-transparent px-2 py-1.5 text-[0.95rem] text-zinc-100 outline-none placeholder:text-zinc-600"
              />
              <div className="flex items-center justify-between gap-2 pl-1">
                <ModelPicker models={models} currentModel={currentModel} defaultModel={defaultModel} onSelect={setCurrentModel} />
                {busy ? (
                  <Button type="button" size="icon" onClick={stop} title="Stop"><Square size={15} strokeWidth={2.4} fill="currentColor" /></Button>
                ) : (
                  <Button type="submit" size="icon" disabled={!draft.trim()}><ArrowUp size={18} strokeWidth={2.4} /></Button>
                )}
              </div>
            </div>
          </form>
        </div>
      </div>

      {/* Right: preview / source */}
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 shrink-0 items-center justify-between gap-3 border-b border-zinc-800 px-4">
          <div className="flex items-center gap-1 rounded-lg bg-zinc-850 p-0.5">
            <button type="button" onClick={() => setTab('preview')} className={cn('flex items-center gap-1.5 rounded-md px-3 py-1 text-[13px] transition-colors', tab === 'preview' ? 'bg-zinc-800 text-zinc-100' : 'text-zinc-500 hover:text-zinc-300')}>
              <Eye size={14} /> Preview
            </button>
            <button type="button" onClick={() => setTab('source')} className={cn('flex items-center gap-1.5 rounded-md px-3 py-1 text-[13px] transition-colors', tab === 'source' ? 'bg-zinc-800 text-zinc-100' : 'text-zinc-500 hover:text-zinc-300')}>
              <FileCode size={14} /> Source
              {consoleErrors.length > 0 && (
                <span className="ml-1 rounded-full bg-red-500/20 px-1.5 text-[10px] font-semibold text-red-400">
                  {consoleErrors.length}
                </span>
              )}
            </button>
          </div>
          <div className="flex items-center gap-2">
            {busy && <span className="flex items-center gap-1 text-xs text-zinc-500"><Loader2 size={12} className="animate-spin" /> Building…</span>}
            <Button size="sm" variant="outline" className="gap-1.5" onClick={run} title="Restart preview"><Play size={14} /> Run</Button>
            <SettingsButton />
          </div>
        </header>

        <div className={cn('relative flex-1 bg-white', tab !== 'preview' && 'hidden')}>
          {previewUrl ? (
            <iframe key={previewUrl} src={previewUrl} className="h-full w-full border-0" title="Live preview" />
          ) : (
            <div className="flex h-full items-center justify-center bg-[#08080a]"><p className="text-sm text-zinc-600">Starting preview…</p></div>
          )}
        </div>
        {tab === 'source' && (
          <SourcePanel
            files={files}
            readSource={readSource}
            saveSource={saveSource}
            consoleErrors={consoleErrors}
            clearConsole={clearConsole}
          />
        )}
      </div>
    </div>
  );
}

export function Builder() {
  const navigate = useNavigate();
  const { projectId } = useParams();
  const b = useBuilder();

  // The URL is the source of truth: opening/closing a project navigates, and
  // this effect syncs the hook to whatever project id is in the URL, so reload
  // and the back button restore the right view instead of dropping to home.
  const { openProject, closeProject, activeId } = b;
  useEffect(() => {
    if (projectId) {
      if (activeId !== projectId) openProject(projectId);
    } else if (activeId) {
      closeProject();
    }
  }, [projectId, activeId, openProject, closeProject]);

  if (!projectId) {
    return (
      <ProjectList
        projects={b.projects}
        onOpen={(id) => navigate(`/builder/${id}`)}
        onNew={async () => navigate(`/builder/${await b.createBlank()}`)}
        onDelete={b.removeProject}
        navigate={navigate}
      />
    );
  }
  return <ProjectView {...b} activeId={projectId} closeProject={() => navigate('/builder')} />;
}
