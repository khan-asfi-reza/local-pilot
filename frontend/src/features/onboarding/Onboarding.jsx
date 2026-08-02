import { useState } from 'react';
import { profile as profileApi } from '../../lib/api';
import { TelegramPanel } from '../settings/TelegramPanel';

const inputClass =
  'w-full rounded-md border border-zinc-700 bg-zinc-900 px-3 py-2.5 text-sm text-zinc-100 outline-none focus:border-zinc-600 placeholder:text-zinc-600';
const primaryBtn =
  'inline-flex items-center justify-center gap-2 rounded-xl bg-gradient-to-br from-emerald-500 to-teal-600 px-4 py-2.5 text-sm font-medium text-white shadow-glow transition-transform hover:-translate-y-0.5 disabled:opacity-50 disabled:hover:translate-y-0';

// Onboarding is the first-run gate: ask a name, then (optionally) set up Telegram.
// Saving the name marks the profile onboarded, so it never shows again.
export function Onboarding({ onDone }) {
  const [step, setStep] = useState(1);
  const [name, setName] = useState('');
  const [saving, setSaving] = useState(false);

  async function saveName() {
    const trimmed = name.trim();
    if (!trimmed) return;
    setSaving(true);
    try {
      await profileApi.save(trimmed);
      setStep(2);
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm">
      <div className="w-full max-w-md overflow-hidden rounded-2xl border border-zinc-800 bg-zinc-850 shadow-2xl">
        {step === 1 ? (
          <div className="flex flex-col gap-4 p-6">
            <div>
              <p className="eyebrow mb-2">Welcome</p>
              <h2 className="bg-gradient-to-br from-zinc-50 to-zinc-400 bg-clip-text text-2xl font-semibold text-transparent">
                Let's set up Pilot
              </h2>
              <p className="mt-1.5 text-sm text-zinc-400">First, what should we call you?</p>
            </div>
            <input
              autoFocus
              value={name}
              onChange={(e) => setName(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && saveName()}
              placeholder="Your name"
              className={inputClass}
            />
            <button onClick={saveName} disabled={saving || !name.trim()} className={primaryBtn}>
              {saving ? 'Saving…' : 'Continue'}
            </button>
          </div>
        ) : (
          <div className="flex flex-col gap-4 p-6">
            <div>
              <p className="eyebrow mb-2">Optional</p>
              <h2 className="text-xl font-semibold text-zinc-100">Control Pilot from Telegram</h2>
              <p className="mt-1.5 text-sm text-zinc-400">
                Add a bot token to drive your projects from your phone, or skip and do it later in Settings.
              </p>
            </div>
            <TelegramPanel />
            <div className="flex items-center justify-end gap-2 pt-1">
              <button onClick={onDone} className="rounded-lg px-3 py-2 text-sm text-zinc-400 hover:text-zinc-100">
                Skip
              </button>
              <button onClick={onDone} className={primaryBtn}>
                Done
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
