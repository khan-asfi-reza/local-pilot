import { useCallback, useEffect, useState } from 'react';
import CodeMirror from '@uiw/react-codemirror';
import { LanguageDescription } from '@codemirror/language';
import { languages } from '@codemirror/language-data';
import { vscodeDark } from '@uiw/codemirror-theme-vscode';
import { Save, X } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { cn } from '../../lib/utils';
import { FileIcon } from './fileIcons';

const AUTOSAVE_KEY = 'code:autoSave';

// language-data ships ~140 languages (C, C++, Java, C#, Rust, Go, TS, JSX, Lua,
// PHP, Ruby, Kotlin, Swift, SQL, YAML, ...), each behind a lazy loader so only
// the ones actually opened are fetched. Cache loaded supports by language name.
const langCache = new Map();

function basename(path) {
  return path.split('/').pop() || path;
}

// loadLanguage resolves the CodeMirror language extension for a filename, or []
// (plain text) when the extension is unknown. Async because each parser is a
// separate lazily-loaded chunk.
async function loadLanguage(path) {
  const desc = LanguageDescription.matchFilename(languages, basename(path));
  if (!desc) return [];
  if (langCache.has(desc.name)) return langCache.get(desc.name);
  try {
    const support = await desc.load();
    langCache.set(desc.name, support);
    return support;
  } catch {
    return [];
  }
}

export function Editor({ openFiles, activePath, onTabClick, onChange, onSave, onClose }) {
  const active = openFiles.find((f) => f.path === activePath);
  const [langExt, setLangExt] = useState([]);
  const [autoSave, setAutoSave] = useState(() => localStorage.getItem(AUTOSAVE_KEY) !== 'false');

  useEffect(() => {
    localStorage.setItem(AUTOSAVE_KEY, String(autoSave));
  }, [autoSave]);

  // Load the language extension for the active file (async, cached). Ignore a
  // stale result if the user switches files before it resolves.
  useEffect(() => {
    let cancelled = false;
    if (!activePath) {
      setLangExt([]);
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
    <div className="flex min-w-0 flex-1 flex-col">
      {openFiles.length > 0 && (
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
      )}

      <div className="flex-1 overflow-hidden bg-[#0c0c0e]">
        {active ? (
          <CodeMirror
            key={active.path}
            value={active.content}
            theme={vscodeDark}
            className="h-full"
            height="100%"
            extensions={[langExt]}
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
