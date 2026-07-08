import { useState } from 'react';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import { oneDark } from 'react-syntax-highlighter/dist/esm/styles/prism';
import { Check, Copy } from 'lucide-react';

export function CodeBlock({ language, value }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      /* clipboard unavailable */
    }
  };
  return (
    <div className="mb-3 overflow-hidden rounded-xl border border-zinc-800">
      <div className="flex items-center justify-between bg-[#0b0d11] px-3 py-1.5 text-xs text-zinc-500">
        <span className="font-mono">{language || 'text'}</span>
        <button
          type="button"
          onClick={copy}
          className="flex items-center gap-1 rounded px-1.5 py-0.5 transition-colors hover:bg-zinc-800 hover:text-zinc-200"
        >
          {copied ? <Check size={13} className="text-emerald-400" /> : <Copy size={13} />}
          {copied ? 'Copied' : 'Copy'}
        </button>
      </div>
      <SyntaxHighlighter
        language={language || 'text'}
        style={oneDark}
        customStyle={{ margin: 0, background: '#0d0f13', padding: '0.9rem 1rem', fontSize: '0.85rem' }}
        codeTagProps={{ style: { fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' } }}
      >
        {value}
      </SyntaxHighlighter>
    </div>
  );
}
