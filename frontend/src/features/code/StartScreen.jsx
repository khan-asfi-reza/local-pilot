import { useCallback, useEffect, useState } from 'react';
import { ArrowLeft, Clock, FolderOpen, FolderPlus, Search, SquareCode } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { code } from '../../lib/api';
import { cn } from '../../lib/utils';
import { DirBrowser } from './FileTree';
import { SettingsButton } from '../settings/SettingsButton';

// A prompt longer than one line reads as a document, so it is also written to
// PRD.md in the new project — short one-liners are just a first instruction.
const looksLikeDoc = (text) => text.includes('\n') || text.length > 280;

// shortPath keeps a project path readable in a narrow row: home becomes ~, and a
// long path keeps its head and its last two segments.
function shortPath(path) {
  const home = path.match(/^\/(Users|home)\/[^/]+/);
  let out = home ? `~${path.slice(home[0].length)}` : path;
  if (out.length <= 46) return out;
  const parts = out.split('/');
  if (parts.length <= 3) return out;
  return `${parts[0]}/…/${parts.slice(-2).join('/')}`;
}

function relTime(iso) {
  const then = Date.parse(iso || '');
  if (!then) return '';
  const mins = Math.round((Date.now() - then) / 60000);
  if (mins < 1) return 'just now';
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.round(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.round(hours / 24);
  if (days < 30) return `${days}d ago`;
  return new Date(then).toLocaleDateString();
}

function RecentRow({ project, onOpen }) {
  return (
    <button
      type="button"
      onClick={() => onOpen(project)}
      className="group flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left transition-colors hover:bg-zinc-800/60"
    >
      <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-zinc-800 bg-[#0c0c0e] text-[13px] font-semibold uppercase text-emerald-400/80">
        {project.name?.slice(0, 2) || '··'}
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[13.5px] font-medium text-zinc-200">{project.name}</span>
        <span className="block truncate text-[11.5px] text-zinc-600" title={project.path}>
          {shortPath(project.path)}
        </span>
      </span>
      <span className="shrink-0 text-[11px] text-zinc-600 group-hover:text-zinc-500">
        {relTime(project.last_opened)}
      </span>
    </button>
  );
}

// NewProjectPane is the create step: a name, a parent folder (the project lands
// in <location>/<name>), and one Prompt / PRD box the agent starts from.
function NewProjectPane({ onCreated }) {
  const [name, setName] = useState('');
  const [location, setLocation] = useState('');
  const [brief, setBrief] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);

  const trimmed = name.trim();
  const valid = trimmed && location && !trimmed.includes('/');

  const submit = useCallback(async () => {
    if (!valid || busy) return;
    setBusy(true);
    setError(null);
    const text = brief.trim();
    try {
      const { project } = await code.createProject({
        name: trimmed,
        location,
        prd: looksLikeDoc(text) ? text : '',
      });
      onCreated(project, text);
    } catch (e) {
      setError(String(e.message || e));
      setBusy(false);
    }
  }, [valid, busy, brief, trimmed, location, onCreated]);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex min-h-0 flex-1">
        <div className="flex min-h-0 w-[46%] shrink-0 flex-col gap-3 border-r border-zinc-800 p-5">
          <label className="block shrink-0">
            <span className="mb-1.5 block text-[12px] font-medium text-zinc-400">Project name</span>
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              autoFocus
              placeholder="my-app"
              className="w-full rounded-lg border border-zinc-800 bg-[#0c0c0e] px-3 py-2 text-[13px] text-zinc-100 outline-none placeholder:text-zinc-600 focus:border-violet-500/60"
            />
          </label>

          <span className="shrink-0 text-[12px] font-medium text-zinc-400">Location</span>
          <DirBrowser path={location} onPathChange={setLocation} fill />
          <p className="shrink-0 truncate font-mono text-[11px] text-zinc-500">
            {location ? `${location}/${trimmed || '<name>'}` : 'Pick a parent folder'}
          </p>
        </div>

        <div className="flex min-h-0 min-w-0 flex-1 flex-col gap-1.5 p-5">
          <span className="shrink-0 text-[12px] font-medium text-zinc-400">
            Prompt / PRD <span className="text-zinc-600">· optional</span>
          </span>
          <textarea
            value={brief}
            onChange={(e) => setBrief(e.target.value)}
            placeholder={'Build a CLI todo app in Go with add, list and done commands.\n\nOr paste a full PRD — anything longer than one line is also saved as PRD.md in the project.'}
            className="min-h-0 w-full flex-1 resize-none rounded-lg border border-zinc-800 bg-[#0c0c0e] px-3 py-2 text-[13px] leading-relaxed text-zinc-100 outline-none placeholder:text-zinc-600 focus:border-violet-500/60"
          />
          {error && (
            <p className="shrink-0 rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-2 text-[12px] text-red-300">
              {error}
            </p>
          )}
        </div>
      </div>

      <div className="flex shrink-0 items-center justify-between gap-3 border-t border-zinc-800 px-5 py-3">
        <p className="min-w-0 truncate text-[11.5px] text-zinc-600">
          {brief.trim()
            ? 'The agent starts on this as soon as the folder exists.'
            : 'You can prompt the agent later.'}
        </p>
        <Button size="sm" onClick={submit} disabled={!valid || busy} className="shrink-0">
          {busy ? 'Creating…' : brief.trim() ? 'Create & run' : 'Create project'}
        </Button>
      </div>
    </div>
  );
}

// OpenPane is the single "open a folder" surface in the app: browse the machine,
// open where you land.
function OpenPane({ onOpen }) {
  const [path, setPath] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);

  const open = useCallback(async () => {
    if (!path || busy) return;
    setBusy(true);
    setError(null);
    try {
      await onOpen(path);
    } catch (e) {
      setError(String(e.message || e));
      setBusy(false);
    }
  }, [path, busy, onOpen]);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex min-h-0 flex-1 flex-col p-5">
        <DirBrowser path={path} onPathChange={setPath} fill />
        {error && (
          <p className="mt-3 shrink-0 rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-2 text-[12px] text-red-300">
            {error}
          </p>
        )}
      </div>
      <div className="flex shrink-0 items-center justify-between gap-3 border-t border-zinc-800 px-5 py-3">
        <p className="min-w-0 truncate font-mono text-[11.5px] text-zinc-600">{path || '—'}</p>
        <Button size="sm" onClick={open} disabled={!path || busy} className="shrink-0">
          {busy ? 'Opening…' : 'Open'}
        </Button>
      </div>
    </div>
  );
}

// StartScreen is the workspace's welcome view: recent projects on the right, the
// two ways in on the left. Choosing one slides the whole card sideways to the
// matching pane, so there is exactly one place that opens or creates a project.
export function StartScreen({ onOpenProject, onOpenPath, onCreated }) {
  const [pane, setPane] = useState('home'); // 'home' | 'open' | 'new'
  const [projects, setProjects] = useState([]);
  const [query, setQuery] = useState('');

  useEffect(() => {
    code.listProjects().then((d) => setProjects(d.projects || [])).catch(() => {});
  }, []);

  useEffect(() => {
    if (pane === 'home') return undefined;
    const onKey = (e) => e.key === 'Escape' && setPane('home');
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [pane]);

  const filtered = query.trim()
    ? projects.filter((p) => `${p.name} ${p.path}`.toLowerCase().includes(query.trim().toLowerCase()))
    : projects;

  return (
    <div className="hero-wash relative flex h-full items-center justify-center p-6">
      <SettingsButton className="absolute right-4 top-4" />
      {/* Two panes, each exactly the card's size, slid in and out. Absolute
          positioning (rather than a double-width track) keeps the off-screen pane
          from being scrolled into view when something inside it takes focus. */}
      <div className="relative h-[540px] w-full max-w-4xl overflow-hidden rounded-3xl border border-zinc-800 bg-[#101012] shadow-2xl">
        <div>
          {/* Pane 1: recents + the two entry points */}
          <div
            aria-hidden={pane !== 'home'}
            className={cn(
              'absolute inset-0 flex transition-transform duration-300 ease-out',
              pane === 'home' ? 'translate-x-0' : '-translate-x-full',
            )}
          >
            <div className="flex w-[44%] shrink-0 flex-col border-r border-zinc-800 p-6">
              <span className="mb-5 flex h-12 w-12 items-center justify-center rounded-2xl bg-gradient-to-br from-emerald-500 to-teal-600 text-white shadow-glow">
                <SquareCode size={24} strokeWidth={2} />
              </span>
              <p className="eyebrow mb-2">Local workspace</p>
              <h1 className="text-2xl font-semibold tracking-tight text-zinc-100">Projects</h1>
              <p className="mt-2 text-[13px] leading-relaxed text-zinc-500">
                Everything runs on this machine — real files, a real shell, and an agent that edits them
                in place.
              </p>
              <div className="mt-6 space-y-2">
                <button
                  type="button"
                  onClick={() => setPane('new')}
                  className="flex w-full items-center gap-2.5 rounded-xl bg-gradient-to-br from-emerald-500 to-teal-600 px-4 py-2.5 text-[13.5px] font-medium text-white shadow-glow transition-transform hover:-translate-y-0.5"
                >
                  <FolderPlus size={16} strokeWidth={2} />
                  New project
                </button>
                <button
                  type="button"
                  onClick={() => setPane('open')}
                  className="flex w-full items-center gap-2.5 rounded-xl border border-zinc-700 px-4 py-2.5 text-[13.5px] font-medium text-zinc-200 transition-colors hover:border-zinc-600 hover:bg-zinc-800/60"
                >
                  <FolderOpen size={16} strokeWidth={2} />
                  Open folder
                </button>
              </div>
              <div className="mt-auto space-y-1.5 pt-6 text-[11.5px] leading-relaxed text-zinc-600">
                <p>A new project can start from a prompt or a full PRD.</p>
                <p>
                  Inside a project, <span className="font-mono text-zinc-500">Ctrl</span>
                  <span className="font-mono text-zinc-500"> + `</span> opens a terminal in its folder.
                </p>
              </div>
            </div>

            <div className="flex min-w-0 flex-1 flex-col">
              <div className="flex shrink-0 items-center gap-2 border-b border-zinc-800 px-4 py-3">
                <Clock size={14} className="shrink-0 text-zinc-600" />
                <span className="mr-auto text-[12px] font-medium uppercase tracking-wider text-zinc-500">
                  Recent
                </span>
                {projects.length > 4 && (
                  <div className="flex min-w-0 items-center gap-1.5 rounded-lg border border-zinc-800 bg-[#0c0c0e] px-2 py-1">
                    <Search size={12} className="shrink-0 text-zinc-600" />
                    <input
                      value={query}
                      onChange={(e) => setQuery(e.target.value)}
                      placeholder="Search"
                      className="w-24 bg-transparent text-[12px] text-zinc-200 outline-none placeholder:text-zinc-600"
                    />
                  </div>
                )}
              </div>
              <div className="min-h-0 flex-1 space-y-0.5 overflow-y-auto p-2">
                {filtered.map((p) => (
                  <RecentRow key={p.id} project={p} onOpen={onOpenProject} />
                ))}
                {projects.length === 0 && (
                  <div className="flex h-full flex-col items-center justify-center px-6 text-center">
                    <p className="text-[13px] text-zinc-500">No projects yet.</p>
                    <p className="mt-1 text-[12px] text-zinc-600">
                      Create one, or open a folder you already have.
                    </p>
                  </div>
                )}
                {projects.length > 0 && filtered.length === 0 && (
                  <p className="px-3 py-6 text-center text-[12px] text-zinc-600">No match.</p>
                )}
              </div>
            </div>
          </div>

          {/* Pane 2: whichever detail the user picked */}
          <div
            aria-hidden={pane === 'home'}
            className={cn(
              'absolute inset-0 flex flex-col transition-transform duration-300 ease-out',
              pane === 'home' ? 'translate-x-full' : 'translate-x-0',
            )}
          >
            <div className="flex shrink-0 items-center gap-2 border-b border-zinc-800 px-4 py-3">
              <button
                type="button"
                onClick={() => setPane('home')}
                className="flex items-center gap-1.5 rounded-lg px-2 py-1 text-[13px] text-zinc-400 transition-colors hover:bg-zinc-800 hover:text-zinc-200"
              >
                <ArrowLeft size={15} />
                Back
              </button>
              <span className="text-[13px] font-medium text-zinc-200">
                {pane === 'new' ? 'New project' : 'Open folder'}
              </span>
            </div>
            {pane === 'new' ? <NewProjectPane onCreated={onCreated} /> : <OpenPane onOpen={onOpenPath} />}
          </div>
        </div>
      </div>
    </div>
  );
}
