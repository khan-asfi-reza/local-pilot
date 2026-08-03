import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { ArrowLeft } from 'lucide-react';
import { profile as profileApi } from '../../lib/api';
import { TelegramPanel } from './TelegramPanel';
import { ModelsPanel } from './ModelsPanel';

const inputClass =
  'w-full rounded-md border border-zinc-700 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-zinc-600 placeholder:text-zinc-600';

function Section({ title, subtitle, children }) {
  return (
    <section className="rounded-2xl border border-zinc-800 bg-zinc-850 p-5">
      <h2 className="text-sm font-semibold text-zinc-100">{title}</h2>
      {subtitle && <p className="mb-4 mt-0.5 text-[13px] text-zinc-500">{subtitle}</p>}
      {!subtitle && <div className="mb-4" />}
      {children}
    </section>
  );
}

// SettingsPage is the dedicated /settings route: model management, profile, and
// the Telegram bridge, replacing the old gear-icon modal.
export function SettingsPage() {
  const navigate = useNavigate();
  const [name, setName] = useState('');
  const [savedName, setSavedName] = useState('');

  useEffect(() => {
    profileApi.get().then((p) => {
      setName(p?.name || '');
      setSavedName(p?.name || '');
    });
  }, []);

  async function saveName() {
    const trimmed = name.trim();
    if (!trimmed || trimmed === savedName) return;
    await profileApi.save(trimmed);
    setSavedName(trimmed);
  }

  return (
    <div className="hero-wash min-h-full overflow-y-auto">
      <div className="mx-auto flex max-w-2xl flex-col gap-6 px-6 py-10">
        <div className="flex items-center gap-3">
          <button
            onClick={() => navigate(-1)}
            title="Back"
            className="flex h-9 w-9 items-center justify-center rounded-lg text-zinc-400 transition-colors hover:bg-zinc-800 hover:text-zinc-100"
          >
            <ArrowLeft size={18} />
          </button>
          <div>
            <p className="eyebrow">Settings</p>
            <h1 className="bg-gradient-to-br from-zinc-50 to-zinc-400 bg-clip-text text-2xl font-semibold tracking-tight text-transparent">
              Preferences
            </h1>
          </div>
        </div>

        <Section title="Models" subtitle="Add, switch, or remove the models Pilot runs against.">
          <ModelsPanel />
        </Section>

        <Section title="Profile">
          <label className="mb-1 block text-xs font-medium text-zinc-400">Your name</label>
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            onBlur={saveName}
            placeholder="What should we call you?"
            className={inputClass}
          />
        </Section>

        <Section title="Telegram" subtitle="Drive Pilot from a Telegram chat.">
          <TelegramPanel />
        </Section>
      </div>
    </div>
  );
}
