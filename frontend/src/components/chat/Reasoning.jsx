import { useState } from 'react';
import { ChevronRight } from 'lucide-react';
import { cn } from '../../lib/utils';

// Reasoning shows the model's thinking behind a collapsed "Thinking" toggle, so
// it stays out of the way by default and can be opened when wanted. Renders
// nothing when there is no reasoning text.
export function Reasoning({ children }) {
  const [open, setOpen] = useState(false);
  if (!children) return null;
  return (
    <div className="mb-2">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="flex items-center gap-1 text-[11px] font-medium text-zinc-600 transition-colors hover:text-zinc-400"
      >
        <ChevronRight size={11} className={cn('transition-transform', open && 'rotate-90')} />
        Thinking
      </button>
      {open && (
        <div className="mt-1 whitespace-pre-wrap border-l-2 border-zinc-800 pl-3 text-[12px] italic text-zinc-500">
          {children}
        </div>
      )}
    </div>
  );
}
