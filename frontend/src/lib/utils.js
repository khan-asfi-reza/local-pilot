import { clsx } from 'clsx';
import { twMerge } from 'tailwind-merge';

export function cn(...inputs) {
  return twMerge(clsx(inputs));
}

// guessLang infers a syntax-highlighting language from code when the model
// emitted a fence without one (label shows "text" and nothing gets colored).
export function guessLang(code) {
  const c = code || '';
  if (/^\s*<[!?a-z]/i.test(c) && /<\/?[a-z][\s\S]*>/i.test(c)) return 'markup';
  if (/\b(def|elif|lambda)\b|^\s*from\s+\S+\s+import\b|print\(|self\.|import\s+\w+$/m.test(c)) return 'python';
  if (/\b(function|const|let|var)\b|=>|console\.\w+|require\(|export\s+(default|const|function)/.test(c)) return 'javascript';
  if (/\bpackage\s+\w+|\bfunc\s+\w*\s*\(|:=|fmt\.[A-Z]/.test(c)) return 'go';
  if (/\b(SELECT|INSERT\s+INTO|UPDATE|DELETE\s+FROM|CREATE\s+TABLE)\b/i.test(c)) return 'sql';
  if (/^\s*[{[][\s\S]*[}\]]\s*$/.test(c) && /"[^"]*"\s*:/.test(c)) return 'json';
  if (/^#!.*\b(ba)?sh\b|^\s*(sudo|apt|brew|npm|cd|ls|echo|curl|git|mkdir|export)\s/m.test(c)) return 'bash';
  if (/^\s*#include\b|\bstd::|\bint\s+main\s*\(/.test(c)) return 'cpp';
  return 'text';
}

// humanizeModel turns an ollama tag into a friendly label, e.g.
// "qwen3.5:4b-tools" -> "Qwen3.5:4Billion". The -tools suffix is dropped, and a
// disambiguating "(host)" label (for the same tag on two servers) is preserved.
export function humanizeModel(name) {
  if (!name) return '';
  const paren = name.indexOf(' (');
  const core = paren >= 0 ? name.slice(0, paren) : name;
  const suffix = paren >= 0 ? ' ' + name.slice(paren + 1) : ''; // "(192.168.10.99)"
  const base = core.replace(/-tools$/i, '');
  const [family, size] = base.split(':');
  const fam = family ? family.charAt(0).toUpperCase() + family.slice(1) : base;
  if (!size) return fam + suffix;
  const human = size.replace(/b$/i, 'Billion').replace(/m$/i, 'Million');
  return `${fam}:${human}${suffix}`;
}
