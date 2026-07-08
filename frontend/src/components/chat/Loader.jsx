import { useEffect, useState } from 'react';

const WORDS = [
  'Thinking',
  'Reasoning',
  'Cooking',
  'Crunching',
  'Pondering',
  'Brewing',
  'Conjuring',
  'Untangling',
  'Computing',
];

export function Loader() {
  const [i, setI] = useState(0);
  useEffect(() => {
    const t = setInterval(() => setI((n) => (n + 1) % WORDS.length), 1600);
    return () => clearInterval(t);
  }, []);
  return (
    <div className="flex items-center gap-2 text-sm text-zinc-400">
      <span className="flex items-end gap-1">
        <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-emerald-400 [animation-delay:-0.3s]" />
        <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-emerald-400 [animation-delay:-0.15s]" />
        <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-emerald-400" />
      </span>
      <span className="animate-pulse font-medium">{WORDS[i]}…</span>
    </div>
  );
}
