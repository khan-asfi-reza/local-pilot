import { useCallback, useEffect, useMemo, useState } from 'react';
import CodeMirror from '@uiw/react-codemirror';
import { EditorView } from '@codemirror/view';
import { vscodeDark } from '@uiw/codemirror-theme-vscode';
import { Save, SquareTerminal, X } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { cn } from '../../lib/utils';
import { FileIcon } from './fileIcons';
import { loadLanguage } from '../../lib/languages';

const AUTOSAVE_KEY = 'code:autoSave';

// Code is not prose: kill the browser's red squiggles inside the editor.
const NO_SPELLCHECK = EditorView.contentAttributes.of({ spellcheck: 'false', autocorrect: 'off' });

function basename(path) {
  return path.split('/').pop() || path;
}

export function Editor({ openFiles, activePath, onTabClick, onChange, onSave, onClose, onToggleTerminal }) {
  const active = openFiles.find((f) => f.path === activePath);
  const [langExt, setLangExt] = useState(null);
  const [autoSave, setAutoSave] = useState(() => localStorage.getItem(AUTOSAVE_KEY) !== 'false');
  const extensions = useMemo(() => (langExt ? [NO_SPELLCHECK, langExt] : [NO_SPELLCHECK]), [langExt]);

  useEffect(() => {
    localStorage.setItem(AUTOSAVE_KEY, String(autoSave));
  }, [autoSave]);

  // Load the language extension for the active file (async, cached). Ignore a
  // stale result if the user switches files before it resolves.
  useEffect(() => {
    let cancelled = false;
    if (!activePath) {
      setLangExt(null);
      return undefined;
    }
    loadLanguage(activePath).then((ext) => {
      if (!cancelled) setLangExt(ext);
    });
    return () => {
      cancelled = true;
    };
  }, [activePath]);

  const handleSave = useCallback(() => {
    if (activePath) onSave(activePath);
  }, [activePath, onSave]);

  useEffect(() => {
    const onKey = (e) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 's') {
        e.preventDefault();
        handleSave();
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [handleSave]);

  // Auto-save: debounce writes while the active file stays dirty. Timer resets
  // on each keystroke (content dep) and clears once the save lands (dirty=false).
  useEffect(() => {
    if (!autoSave || !active?.dirty) return undefined;
    const path = active.path;
    const t = setTimeout(() => onSave(path), 700);
    return () => clearTimeout(t);
  }, [autoSave, active?.path, active?.content, active?.dirty, onSave]);

  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      <div className="flex shrink-0 items-center overflow-x-auto border-b border-zinc-800 bg-[#101012]">
        {openFiles.map((f) => {
            const isActive = f.path === activePath;
            return (
              <button
                key={f.path}
                type="button"
                onClick={() => onTabClick(f.path)}
                className={cn(
                  'group flex shrink-0 items-center gap-1.5 border-r border-zinc-800 px-3 py-2 text-[13px] transition-colors',
                  isActive ? 'bg-[#141416] text-zinc-100' : 'text-zinc-500 hover:bg-zinc-800/40 hover:text-zinc-300',
                )}
              >
                <FileIcon name={basename(f.path)} size={13} />
                <span className="truncate max-w-[120px]">{basename(f.path)}</span>
                {f.dirty && <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-amber-400" />}
                <span
                  role="button"
                  tabIndex={0}
                  onClick={(e) => {
                    e.stopPropagation();
                    onClose(f.path);
                  }}
                  className="ml-0.5 shrink-0 rounded p-0.5 text-zinc-600 opacity-0 transition-opacity hover:text-zinc-300 group-hover:opacity-100"
                >
                  <X size={12} />
                </span>
              </button>
            );
          })}
        <div className="ml-auto flex shrink-0 items-center gap-1 px-2">
            {onToggleTerminal && (
              <button
                type="button"
                onClick={onToggleTerminal}
                title="Toggle terminal (Ctrl+`)"
                className="rounded-md p-1.5 text-zinc-400 transition-colors hover:bg-zinc-800 hover:text-zinc-200"
              >
                <SquareTerminal size={15} />
              </button>
            )}
            <button
              type="button"
              role="switch"
              aria-checked={autoSave}
              onClick={() => setAutoSave((v) => !v)}
              title={autoSave ? 'Auto-save on' : 'Auto-save off'}
              className="flex items-center gap-2 rounded-md px-2 py-1 text-[12px] text-zinc-400 transition-colors hover:text-zinc-200"
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
            <Button
              variant="ghost"
              size="sm"
              disabled={!active || !active.dirty}
              onClick={handleSave}
              className="gap-1.5"
            >
              <Save size={14} />
              Save
            </Button>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-hidden bg-[#0c0c0e]">
        {active ? (
          <CodeMirror
            key={active.path}
            value={active.content}
            theme={vscodeDark}
            className="h-full"
            height="100%"
            extensions={extensions}
            onChange={(val) => onChange(active.path, val)}
            basicSetup={{ lineNumbers: true, foldGutter: true, highlightActiveLineGutter: true }}
          />
        ) : (
          <div className="flex h-full items-center justify-center">
            <div className="rounded-2xl border border-zinc-800 bg-[#141416] p-8 text-center">
              <p className="text-sm text-zinc-500">Open a file from the tree to start editing</p>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
