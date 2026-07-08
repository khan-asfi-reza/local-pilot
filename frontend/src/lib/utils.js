import { clsx } from 'clsx';
import { twMerge } from 'tailwind-merge';

export function cn(...inputs) {
  return twMerge(clsx(inputs));
}

// humanizeModel turns an ollama tag into a friendly label, e.g.
// "qwen3.5:4b-tools" -> "Qwen3.5:4Billion". The -tools suffix is dropped.
export function humanizeModel(name) {
  if (!name) return '';
  const base = name.replace(/-tools$/i, '');
  const [family, size] = base.split(':');
  const fam = family ? family.charAt(0).toUpperCase() + family.slice(1) : base;
  if (!size) return fam;
  const human = size.replace(/b$/i, 'Billion').replace(/m$/i, 'Million');
  return `${fam}:${human}`;
}
