import { useState } from 'react';
import { ChevronRight, Terminal } from 'lucide-react';
import { CodeBlock } from './CodeBlock';
import { cn } from '../../lib/utils';

// summarize pulls a short human output from a tool result payload.
function summarize(data) {
  if (!data) return '';
  let o = data;
  if (typeof data === 'string') {
    try {
      o = JSON.parse(data);
    } catch {
      return data;
    }
  }
  if (o.stdout) return o.stdout;
  if (o.error) return `error: ${o.error}`;
  if (o.stderr) return o.stderr;
  if (Array.isArray(o.results)) return o.results.map((r) => `• ${r.title}\n  ${r.url}`).join('\n');
  return '';
}

// extractCode pulls the code/command the tool was called with, plus its language.
function extractCode(input) {
  if (!input) return { code: '', lang: 'text' };
  try {
    const a = JSON.parse(input);
    if (a.code) return { code: a.code, lang: a.language || 'python' };
    if (a.command) return { code: a.command, lang: 'bash' };
    if (a.content) return { code: a.content, lang: 'text' };
  } catch {
    return { code: '', lang: 'text' };
  }
  return { code: '', lang: 'text' };
}

export function ToolCard({ tool, info, input, output, running }) {
  const [open, setOpen] = useState(false);
  const { code, lang } = extractCode(input);
  const out = summarize(output).trim();
  return (
    <div className="mb-4 ml-11 overflow-hidden rounded-xl border border-zinc-800 bg-[#0e1116] text-sm">
      <button
        type="button"
        onClick={() => code && setOpen((v) => !v)}
        className={cn('flex w-full items-center gap-2 px-3 py-2 text-left', code && 'hover:bg-zinc-800/40')}
      >
        <Terminal size={14} className="text-emerald-400" />
        <span className="font-medium text-zinc-300">{tool}</span>
        {info ? <span className="truncate text-xs text-zinc-500">{info}</span> : null}
        {running ? (
          <span className="ml-auto text-xs text-zinc-500">running…</span>
        ) : code ? (
          <ChevronRight size={15} className={cn('ml-auto text-zinc-500 transition-transform', open && 'rotate-90')} />
        ) : null}
      </button>
      {open && code ? (
        <div className="border-t border-zinc-800 p-2">
          <CodeBlock language={lang} value={code} />
        </div>
      ) : null}
      {out ? (
        <pre className="max-h-64 overflow-auto border-t border-zinc-800 px-3 py-2 font-mono text-[0.8rem] leading-5 text-zinc-300">
          {out}
        </pre>
      ) : null}
    </div>
  );
}
