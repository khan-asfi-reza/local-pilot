import { useEffect, useState } from 'react';
import { Check, Trash2, Download, Loader2, Server } from 'lucide-react';
import { models as modelsApi } from '../../lib/api';
import { humanizeModel } from '../../lib/utils';
import { Button } from '../../components/ui/button';
import { Combobox } from '../../components/ui/Combobox';

const inputClass =
  'w-full rounded-md border border-zinc-700 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-zinc-600 placeholder:text-zinc-600';

// hostOf extracts a display host from a backend URL, or '' when it's local.
function hostOf(url) {
  if (!url || /(localhost|127\.0\.0\.1|\[::1\])/.test(url)) return '';
  return url.replace(/^https?:\/\//, '');
}

// ModelsPanel manages the model registry: list registered models (use/remove),
// register an installed local model, add one on a remote ollama server, or pull a
// new model with a live progress bar. The registry label is decoupled from the
// ollama tag, so the same tag can live on two servers under distinct names.
export function ModelsPanel() {
  const [list, setList] = useState([]);
  const [def, setDef] = useState(null);
  const [available, setAvailable] = useState([]);
  const [tag, setTag] = useState('');
  const [host, setHost] = useState('');
  const [name, setName] = useState('');
  const [busy, setBusy] = useState(false);
  const [pull, setPull] = useState(null); // { name, pct, status }
  const [error, setError] = useState('');

  async function refresh() {
    try {
      const [ml, av] = await Promise.all([
        modelsApi.list(),
        modelsApi.available().catch(() => ({ available: [] })),
      ]);
      setList(ml.models || []);
      setDef(ml.default || null);
      setAvailable(av.available || []);
    } catch (e) {
      setError(String(e.message || e));
    }
  }

  useEffect(() => {
    refresh();
  }, []);

  const registered = new Set(list.map((m) => m.name));
  const trimmedTag = tag.trim();
  const trimmedHost = host.trim();
  const installedLocal = available.includes(trimmedTag);

  async function run(fn) {
    setBusy(true);
    setError('');
    try {
      await fn();
      await refresh();
    } catch (e) {
      setError(String(e.message || e));
    } finally {
      setBusy(false);
    }
  }

  function resetForm() {
    setTag('');
    setHost('');
    setName('');
  }

  async function onAdd() {
    if (!trimmedTag) return;
    await run(async () => {
      await modelsApi.add(trimmedTag, { host: trimmedHost, name: name.trim() });
      resetForm();
    });
  }

  async function onPull() {
    if (!trimmedTag) return;
    setError('');
    setPull({ name: trimmedTag, pct: 0, status: 'starting' });
    try {
      await modelsApi.pull(trimmedTag, (ev) => {
        const pct = ev.total > 0 ? Math.round((ev.completed / ev.total) * 100) : 0;
        setPull({ name: trimmedTag, pct, status: ev.status || '' });
      });
      resetForm();
      await refresh();
    } catch (e) {
      setError(String(e.message || e));
    } finally {
      setPull(null);
    }
  }

  function onRemove(m) {
    const msg = hostOf(m.url)
      ? `Remove ${humanizeModel(m.name)} from the registry?`
      : `Remove ${humanizeModel(m.name)} and delete it from disk?`;
    if (!window.confirm(msg)) return;
    run(() => modelsApi.remove(m.name));
  }

  // Remote adds can't be pulled from here; local installed tags register directly;
  // anything else local is pulled.
  const canPull = !trimmedHost && trimmedTag && !installedLocal;
  const addOptions = available.filter((n) => !registered.has(n));

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-2">
        {list.length === 0 && <p className="text-sm text-zinc-500">No models yet — add one below.</p>}
        {list.map((m) => {
          const remoteHost = hostOf(m.url);
          return (
            <div
              key={m.name}
              className="flex items-center justify-between gap-3 rounded-xl border border-zinc-800 bg-zinc-900 px-4 py-3"
            >
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <span className="truncate text-sm font-medium text-zinc-100">{humanizeModel(m.name)}</span>
                  {m.name === def && (
                    <span className="rounded bg-emerald-500/15 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-emerald-300">
                      default
                    </span>
                  )}
                  {remoteHost && (
                    <span className="flex items-center gap-1 rounded bg-zinc-700/40 px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-zinc-300">
                      <Server size={9} /> remote
                    </span>
                  )}
                  {!m.ready && (
                    <span className="rounded bg-zinc-700/50 px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-zinc-400">
                      offline
                    </span>
                  )}
                </div>
                <div className="mt-0.5 truncate font-mono text-[11px] text-zinc-500">
                  {remoteHost ? `${remoteHost}` : m.name}
                </div>
              </div>
              <div className="flex shrink-0 items-center gap-2">
                {m.name === def ? (
                  <span className="flex items-center gap-1 text-xs text-emerald-400">
                    <Check size={14} /> In use
                  </span>
                ) : (
                  <Button size="sm" onClick={() => run(() => modelsApi.activate(m.name))} disabled={busy}>
                    Use
                  </Button>
                )}
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => onRemove(m)}
                  disabled={busy}
                  title="Remove"
                >
                  <Trash2 size={14} />
                </Button>
              </div>
            </div>
          );
        })}
      </div>

      <div className="flex flex-col gap-2 rounded-xl border border-zinc-800 bg-zinc-900/50 p-3">
        <label className="text-xs font-medium text-zinc-400">Add a model</label>
        <Combobox
          value={tag}
          onChange={setTag}
          options={addOptions}
          placeholder="Model tag — search installed, or type a name to pull…"
        />
        <div className="grid grid-cols-2 gap-2">
          <input
            value={host}
            onChange={(e) => setHost(e.target.value)}
            placeholder="Remote host (optional)"
            className={inputClass}
          />
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Display name (optional)"
            className={inputClass}
          />
        </div>
        <p className="text-[11px] text-zinc-600">
          Leave the host blank for a local model. Set it (e.g. http://192.168.10.99:11434) to use the same
          model on another machine — it gets its own name so both can be used.
        </p>

        {pull ? (
          <div className="mt-1">
            <div className="mb-1 flex items-center justify-between text-xs text-zinc-400">
              <span className="flex items-center gap-1.5">
                <Loader2 size={12} className="animate-spin" /> Pulling {pull.name}
                {pull.status ? ` — ${pull.status}` : ''}
              </span>
              <span>{pull.pct}%</span>
            </div>
            <div className="h-1.5 w-full overflow-hidden rounded-full bg-zinc-800">
              <div
                className="h-full rounded-full bg-gradient-to-r from-emerald-500 to-teal-600 transition-all"
                style={{ width: `${pull.pct}%` }}
              />
            </div>
          </div>
        ) : (
          <div className="flex items-center gap-2">
            {canPull ? (
              <Button size="sm" onClick={onPull} disabled={busy}>
                <Download size={14} /> Pull &amp; add
              </Button>
            ) : (
              <Button size="sm" onClick={onAdd} disabled={busy || !trimmedTag}>
                Add
              </Button>
            )}
          </div>
        )}
      </div>

      {error && <p className="text-xs text-red-400">{error}</p>}
    </div>
  );
}
