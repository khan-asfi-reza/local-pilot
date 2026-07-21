import { useCallback, useEffect } from 'react';
import CodeMirror from '@uiw/react-codemirror';
import { javascript } from '@codemirror/lang-javascript';
import { python } from '@codemirror/lang-python';
import { html } from '@codemirror/lang-html';
import { vscodeDark } from '@uiw/codemirror-theme-vscode';
import { Save, X } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { cn } from '../../lib/utils';

function langExt(path) {
  if (/\.(js|jsx|ts|tsx|mjs|cjs)$/.test(path)) return javascript({ jsx: true });
  if (/\.py$/.test(path)) return python();
  if (/\.html?$/i.test(path)) return html();
  return [];
}

function basename(path) {
  return path.split('/').pop() || path;
}

export function Editor({ openFiles, activePath, onTabClick, onChange, onSave, onClose }) {
  const active = openFiles.find((f) => f.path === activePath);

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

  return (
    <div className="flex min-w-0 flex-1 flex-col">
      {openFiles.length > 0 && (
        <div className="flex shrink-0 items-center overflow-x-auto border-b border-zinc-800 bg-[#0e1014]">
          {openFiles.map((f) => {
            const isActive = f.path === activePath;
            return (
              <button
                key={f.path}
                type="button"
                onClick={() => onTabClick(f.path)}
                className={cn(
                  'group flex shrink-0 items-center gap-1.5 border-r border-zinc-800 px-3 py-2 text-[13px] transition-colors',
                  isActive ? 'bg-[#15181d] text-zinc-100' : 'text-zinc-500 hover:bg-zinc-800/40 hover:text-zinc-300',
                )}
              >
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
          <div className="ml-auto flex shrink-0 items-center px-2">
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

      <div className="flex-1 overflow-hidden bg-[#0d0f13]">
        {active ? (
          <CodeMirror
            key={active.path}
            value={active.content}
            theme={vscodeDark}
            height="100%"
            extensions={[langExt(active.path)]}
            onChange={(val) => onChange(active.path, val)}
            basicSetup={{ lineNumbers: true, foldGutter: true, highlightActiveLineGutter: true }}
          />
        ) : (
          <div className="flex h-full items-center justify-center">
            <div className="rounded-2xl border border-zinc-800 bg-[#15181d] p-8 text-center">
              <p className="text-sm text-zinc-500">Open a file from the tree to start editing</p>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
