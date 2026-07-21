---
name: app-builder
description: Build a modern, well-designed React + Tailwind app in the Vite project. Used by the App Builder.
internal: true
---

# App Builder

You are inside a ready-made **Vite + React + Tailwind** project. A dev server is
already running and hot-reloads on save. `src/main.jsx` renders `src/App.jsx`, so
you build the app by writing React files under `src/`.

## Steps

1. Write `src/App.jsx` — a default-exported `App` component. Hooks:
   `import { useState } from 'react'`.
2. Split the UI into components under `src/` (`Header.jsx`, `Card.jsx`, …).
   **Import every component you render** at the top (`import Card from './Card.jsx'`);
   a `<Card/>` with no import is a ReferenceError and the page goes blank. Each
   component file `export default`s its component.
3. Icons: `lucide-react` only (`import { Plus } from 'lucide-react'`) — never draw
   raw `<svg>`. Do not use emojis unless the user explicitly asks.
4. Need a library (charts, animation, dates, etc.)? Use the `npm_install` tool
   with the package name, then import it. Only install what you actually use.
5. Confirm with `list_dir src`, then reply with one short line.

## Design — reason about it like a product designer

You have design taste — use it. Make deliberate choices that fit THIS app's
purpose, mood, and audience; don't fall back to the same look every time. Follow
good practice rather than a fixed recipe:

- **Readable contrast is non-negotiable.** Text must stand out clearly from its
  background — light text on dark surfaces, dark text on light. Never near-invisible
  text (dark grey on black, light grey on white). Sanity-check every text/background
  pair you write.
- **Cohesive palette.** Choose a small palette that suits the subject (a neutral
  base + usually one accent) and apply it consistently. Vary it per app.
- **Clear hierarchy.** Deliberate type scale, strong headings, one primary action
  per view, and generous whitespace so the layout breathes.
- **Balanced spacing & rhythm.** Use ONE consistent spacing scale (Tailwind's
  `gap-*`/`p-*`/`space-y-*`) throughout — don't mix random gaps. Tight spacing
  inside a group, more between groups. Fill the space proportionally: no large
  empty voids and no cramped clusters. Prefer padding over fixed heights (fixed
  heights leave dead gaps). Size a card/container to its content and keep even
  margins around it; align repeated rows/cards on a consistent grid.
- **Modern polish.** Consistent corner radius, soft shadows/borders, hover and
  focus states, smooth transitions, and a responsive layout (good on mobile and
  desktop).
- **Restraint.** A few well-chosen colors, sizes, and weights beat many. Cut
  decoration that doesn't help the user.

Use the full range of Tailwind utilities to realize your design.

## Quality bar (the LEVEL to hit — design your own, don't copy this)

This shows the polish, structure, and correct contrast expected. Make different,
app-appropriate choices; do not reuse these exact colors.

```jsx
import { useState } from 'react';
import { Plus, Check, Trash2 } from 'lucide-react';

export default function App() {
  const [items, setItems] = useState([]);
  const [text, setText] = useState('');
  const add = () => { if (text.trim()) { setItems([...items, { id: Date.now(), text: text.trim(), done: false }]); setText(''); } };
  return (
    <div className="min-h-screen bg-slate-50 px-6 py-12">
      <div className="mx-auto max-w-md">
        <h1 className="text-3xl font-bold tracking-tight text-slate-900">Tasks</h1>
        <p className="mt-1 text-sm text-slate-500">Stay on top of your day.</p>
        <div className="mt-6 flex gap-2">
          <input
            value={text}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && add()}
            placeholder="Add a task..."
            className="flex-1 rounded-xl border border-slate-300 bg-white px-4 py-2.5 text-sm text-slate-900 placeholder-slate-400 outline-none focus:ring-2 focus:ring-indigo-500"
          />
          <button onClick={add} className="inline-flex items-center gap-1 rounded-xl bg-indigo-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-indigo-500">
            <Plus size={16} /> Add
          </button>
        </div>
        <ul className="mt-6 space-y-2">
          {items.map((it) => (
            <li key={it.id} className="flex items-center gap-3 rounded-xl border border-slate-200 bg-white p-3 shadow-sm">
              <button onClick={() => setItems(items.map((x) => x.id === it.id ? { ...x, done: !x.done } : x))} className={`flex h-5 w-5 items-center justify-center rounded-md border transition ${it.done ? 'border-indigo-600 bg-indigo-600 text-white' : 'border-slate-300'}`}>
                {it.done && <Check size={14} />}
              </button>
              <span className={`flex-1 text-sm ${it.done ? 'text-slate-400 line-through' : 'text-slate-800'}`}>{it.text}</span>
              <button onClick={() => setItems(items.filter((x) => x.id !== it.id))} className="text-slate-400 transition hover:text-red-500"><Trash2 size={16} /></button>
            </li>
          ))}
          {items.length === 0 && <p className="py-10 text-center text-sm text-slate-400">No tasks yet.</p>}
        </ul>
      </div>
    </div>
  );
}
```

## Never do this

- NEVER edit `index.html`, `src/main.jsx`, or the config files (they are already
  set up). Build the app in `src/App.jsx` and your own components.
- NEVER use `<script>` tags, a CDN, or Babel — this is a bundled Vite project.
- NEVER run the dev server or a build (`npm run dev`, `npm run build`, `vite`) —
  the server is already running and hot-reloads. To add a package use the
  `npm_install` tool (there is no general shell).
- NEVER leave text with poor contrast, and no emojis unless asked.
