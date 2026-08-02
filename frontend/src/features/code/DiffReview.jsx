import { useState } from 'react';
import { Check, X } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { DiffView } from '../../components/chat/DiffView';

// DiffReview is the center-pane review of a pending ask-mode change: the full
// diff up top for readability, then a note field and Approve / Reject beneath.
// It replaces the editor while a change is under review so the change is easy to
// read at full width.
export function DiffReview({ confirm, onApprove, onReject, onClose }) {
  const [note, setNote] = useState('');
  const { tool, summary, diff } = confirm;

  return (
    <div className="flex h-full flex-col bg-[#0c0c0e]">
      <div className="flex shrink-0 items-center gap-2 border-b border-zinc-800 px-4 py-2.5">
        <span className="rounded-md bg-amber-500/15 px-2 py-0.5 text-[11px] font-medium text-amber-300">
          Review change
        </span>
        <span className="truncate font-mono text-[13px] text-zinc-300">{diff?.path || tool}</span>
        {diff && (
          <span className="shrink-0 font-mono text-[11px]">
            <span className="text-emerald-400">+{diff.added}</span>{' '}
            <span className="text-red-400">-{diff.removed}</span>
          </span>
        )}
        <button
          type="button"
          onClick={onClose}
          title="Back to editor"
          className="ml-auto shrink-0 rounded-lg p-1 text-zinc-500 transition-colors hover:bg-zinc-800 hover:text-zinc-200"
        >
          <X size={16} />
        </button>
      </div>

      <div className="flex-1 overflow-auto p-4">
        {summary && <p className="mb-3 text-[13px] text-zinc-400">{summary}</p>}
        {diff ? (
          <DiffView diff={diff} />
        ) : (
          <p className="font-mono text-[13px] text-zinc-400">{tool}</p>
        )}
      </div>

      <div className="shrink-0 border-t border-zinc-800 bg-[#101012] p-3">
        <input
          value={note}
          onChange={(e) => setNote(e.target.value)}
          placeholder="Optional note: tell the agent what to do instead"
          className="mb-2 w-full rounded-lg border border-zinc-800 bg-[#0c0c0e] px-3 py-2 text-[13px] text-zinc-200 outline-none placeholder:text-zinc-600 focus:border-zinc-600"
        />
        <div className="flex gap-2">
          <Button type="button" onClick={() => onApprove(note.trim())} className="flex-1 gap-1.5">
            <Check size={15} /> Approve
          </Button>
          <Button type="button" variant="secondary" onClick={() => onReject(note.trim())} className="flex-1">
            Reject
          </Button>
        </div>
      </div>
    </div>
  );
}
