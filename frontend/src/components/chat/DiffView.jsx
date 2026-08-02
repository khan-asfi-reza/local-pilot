import { useEffect, useState } from 'react';
import { classHighlighter, highlightCode } from '@lezer/highlight';
import { cn } from '../../lib/utils';
import { languageOf, loadLanguage } from '../../lib/languages';

const MAX_HIGHLIGHT = 40000; // characters per hunk; past that, render plain

// highlightHunk parses a hunk as one block — so block comments and multi-line
// strings still resolve — and returns the token spans for each of its lines.
function highlightHunk(lines, language) {
  const text = lines.map((l) => l.text).join('\n');
  if (!language || text.length > MAX_HIGHLIGHT) return null;
  let tree;
  try {
    tree = language.parser.parse(text);
  } catch {
    return null;
  }
  const rows = lines.map(() => []);
  let row = 0;
  highlightCode(
    text,
    tree,
    classHighlighter,
    (code, classes) => rows[row]?.push([code, classes]),
    () => {
      row += 1;
    },
  );
  return rows;
}

// DiffView renders a unified diff (events.Diff: hunks of add/remove/context
// lines) the way an editor shows a change: old/new line numbers in the gutter,
// added and removed lines tinted green and red, and the code itself
// syntax-highlighted with the same parser the editor uses for that file type.
export function DiffView({ diff, className }) {
  const [language, setLanguage] = useState(null);
  const path = diff?.path;

  useEffect(() => {
    let cancelled = false;
    if (!path) {
      setLanguage(null);
      return undefined;
    }
    loadLanguage(path).then((support) => {
      if (!cancelled) setLanguage(languageOf(support));
    });
    return () => {
      cancelled = true;
    };
  }, [path]);

  if (!diff || !diff.hunks?.length) return null;
  return (
    <div
      className={cn(
        'diff-hl overflow-auto rounded-lg border border-zinc-800 bg-[#0c0c0e] font-mono text-[11px] leading-relaxed text-zinc-300',
        className,
      )}
    >
      {diff.hunks.map((h, hi) => {
        const rows = highlightHunk(h.lines, language);
        return (
          <div key={hi}>
            {hi > 0 && <div className="bg-zinc-900/60 px-2 py-0.5 text-zinc-600">⋯</div>}
            {h.lines.map((ln, li) => (
              <div
                key={li}
                className={cn(
                  'flex whitespace-pre',
                  ln.op === 'add' && 'bg-emerald-500/[0.12]',
                  ln.op === 'remove' && 'bg-red-500/[0.12]',
                )}
              >
                <span className="w-9 shrink-0 select-none pr-2 text-right text-zinc-700">{ln.old || ''}</span>
                <span className="w-9 shrink-0 select-none pr-2 text-right text-zinc-700">{ln.new || ''}</span>
                <span
                  className={cn(
                    'w-4 shrink-0 select-none',
                    ln.op === 'add' && 'text-emerald-400',
                    ln.op === 'remove' && 'text-red-400',
                  )}
                >
                  {ln.op === 'add' ? '+' : ln.op === 'remove' ? '-' : ''}
                </span>
                <span className="flex-1">
                  {rows
                    ? rows[li].map(([text, classes], ti) => (
                        <span key={ti} className={classes || undefined}>
                          {text}
                        </span>
                      ))
                    : ln.text}
                </span>
              </div>
            ))}
          </div>
        );
      })}
    </div>
  );
}
