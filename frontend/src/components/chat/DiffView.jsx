import { cn } from '../../lib/utils';

// DiffView renders a unified diff (events.Diff: hunks of add/remove/context lines)
// the way an editor shows a change: green additions, red removals.
export function DiffView({ diff, className }) {
  if (!diff || !diff.hunks?.length) return null;
  return (
    <div className={cn('overflow-auto rounded-lg border border-zinc-800 bg-[#0c0c0e] font-mono text-[11px] leading-relaxed', className)}>
      {diff.hunks.map((h, hi) => (
        <div key={hi}>
          {hi > 0 && <div className="bg-zinc-900/60 px-2 py-0.5 text-zinc-600">⋯</div>}
          {h.lines.map((ln, li) => (
            <div
              key={li}
              className={cn(
                'flex whitespace-pre px-2',
                ln.op === 'add' && 'bg-emerald-500/10 text-emerald-300',
                ln.op === 'remove' && 'bg-red-500/10 text-red-300',
                ln.op === 'context' && 'text-zinc-500',
              )}
            >
              <span className="w-4 shrink-0 select-none text-zinc-600">
                {ln.op === 'add' ? '+' : ln.op === 'remove' ? '-' : ''}
              </span>
              <span className="flex-1">{ln.text}</span>
            </div>
          ))}
        </div>
      ))}
    </div>
  );
}
