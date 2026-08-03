import { AlertTriangle, Box, CircleCheck, CircleDot, Database, Layers, Server } from 'lucide-react';
import { Markdown } from './Markdown';
import { cn } from '../../lib/utils';

// The orchestrator streams progress as bracketed lines ("[provisioned postgres →
// …]", "[decomposed into 7 sub-tasks]", "▸ t1: …", "Build finished: …"). Rendered
// as raw text they read as ugly [] noise, so classify each into a styled pill and
// leave real prose to Markdown.
const STATUS_RE = /^\s*\[(.+?)\]\s*$/;
const STEP_RE = /^\s*▸\s+(.+?)\s*$/;

function pill(text) {
  const t = text.toLowerCase();
  if (t.startsWith('provisioned'))
    return { Icon: Database, cls: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-300' };
  if (t.startsWith('initializing project'))
    return { Icon: Box, cls: 'border-violet-500/30 bg-violet-500/10 text-violet-300' };
  if (t.startsWith('decomposed into'))
    return { Icon: Layers, cls: 'border-sky-500/30 bg-sky-500/10 text-sky-300' };
  if (t.startsWith('build finished'))
    return { Icon: CircleCheck, cls: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-300' };
  if (
    t.includes('could not') || t.includes('failed') || t.includes('not available') ||
    t.includes('problem') || t.includes('no tasks') || t.includes('building from spec')
  )
    return { Icon: AlertTriangle, cls: 'border-amber-500/30 bg-amber-500/10 text-amber-300' };
  return { Icon: Server, cls: 'border-zinc-700 bg-zinc-800/70 text-zinc-300' };
}

function StatusPill({ text }) {
  const { Icon, cls } = pill(text);
  return (
    <span
      className={cn(
        'my-1 inline-flex max-w-full items-center gap-2 rounded-lg border px-2.5 py-1 text-[12.5px] font-medium leading-snug',
        cls,
      )}
    >
      <Icon size={13} className="mt-px shrink-0" />
      <span className="[overflow-wrap:anywhere]">{text}</span>
    </span>
  );
}

function StepPill({ text }) {
  return (
    <span className="my-0.5 inline-flex max-w-full items-center gap-2 rounded-lg border border-zinc-800 bg-zinc-900/60 px-2.5 py-1 text-[12.5px] text-zinc-400">
      <CircleDot size={12} className="mt-px shrink-0 text-fuchsia-400/80" />
      <span className="[overflow-wrap:anywhere]">{text}</span>
    </span>
  );
}

// AgentContent renders one assistant turn: bracketed/▸ progress lines become pills,
// and every other run of lines is handed to Markdown untouched (so prose, lists and
// code fences still render normally).
export function AgentContent({ children }) {
  const content = children || '';
  if (!content) return null;
  const blocks = [];
  let buf = [];
  const flushMd = () => {
    if (buf.join('').trim()) blocks.push({ type: 'md', text: buf.join('\n') });
    buf = [];
  };
  for (const line of content.split('\n')) {
    const status = STATUS_RE.exec(line);
    const step = !status && STEP_RE.exec(line);
    const finished = !status && !step && /^\s*Build finished:/.test(line);
    if (status) {
      flushMd();
      blocks.push({ type: 'status', text: status[1].trim() });
    } else if (step) {
      flushMd();
      blocks.push({ type: 'step', text: step[1].trim() });
    } else if (finished) {
      flushMd();
      blocks.push({ type: 'status', text: line.trim() });
    } else {
      buf.push(line);
    }
  }
  flushMd();

  return (
    <div className="flex flex-col gap-0.5">
      {blocks.map((b, i) =>
        b.type === 'md' ? (
          <Markdown key={i}>{b.text}</Markdown>
        ) : b.type === 'step' ? (
          <div key={i}>
            <StepPill text={b.text} />
          </div>
        ) : (
          <div key={i}>
            <StatusPill text={b.text} />
          </div>
        ),
      )}
    </div>
  );
}
