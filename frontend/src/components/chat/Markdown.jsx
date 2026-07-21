import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { CodeBlock } from './CodeBlock';

const components = {
  p: (p) => <p className="mb-2 leading-relaxed last:mb-0" {...p} />,
  h1: (p) => <h1 className="mb-2 mt-4 text-xl font-semibold first:mt-0" {...p} />,
  h2: (p) => <h2 className="mb-2 mt-4 text-lg font-semibold first:mt-0" {...p} />,
  h3: (p) => <h3 className="mb-2 mt-3 text-base font-semibold first:mt-0" {...p} />,
  ul: (p) => <ul className="mb-3 list-disc space-y-1 pl-6" {...p} />,
  ol: (p) => <ol className="mb-3 list-decimal space-y-1 pl-6" {...p} />,
  li: (p) => <li className="leading-relaxed" {...p} />,
  strong: (p) => <strong className="font-semibold text-white" {...p} />,
  a: (p) => <a className="text-sky-400 underline underline-offset-2 hover:text-sky-300" target="_blank" rel="noreferrer" {...p} />,
  blockquote: (p) => <blockquote className="mb-3 border-l-2 border-zinc-700 pl-3 text-zinc-400" {...p} />,
  hr: () => <hr className="my-4 border-zinc-800" />,
  pre: ({ children }) => <>{children}</>,
  code: ({ className, children, ...props }) => {
    const text = String(children ?? '').replace(/\n$/, '');
    const lang = /language-(\w+)/.exec(className || '');
    const block = !!lang || text.includes('\n');
    return block ? (
      <CodeBlock language={lang ? lang[1] : ''} value={text} />
    ) : (
      <code className="rounded bg-zinc-800/80 px-1.5 py-0.5 font-mono text-[0.85em] text-zinc-200" {...props}>
        {children}
      </code>
    );
  },
  table: (p) => (
    <div className="mb-3 overflow-x-auto">
      <table className="w-full border-collapse text-sm" {...p} />
    </div>
  ),
  th: (p) => <th className="border border-zinc-800 px-3 py-1.5 text-left font-semibold" {...p} />,
  td: (p) => <td className="border border-zinc-800 px-3 py-1.5" {...p} />,
};

// balanceBold drops a trailing unmatched ** marker (small models often open bold
// and never close it), so it does not render as literal asterisks.
function balanceBold(md) {
  const parts = md.split('**');
  if (parts.length % 2 === 0) {
    return parts.slice(0, -1).join('**') + parts[parts.length - 1];
  }
  return md;
}

export function Markdown({ children }) {
  return (
    <div className="text-[0.875rem] text-zinc-100">
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>
        {balanceBold(children || '')}
      </ReactMarkdown>
    </div>
  );
}
