import { useEffect, useState } from 'react';
import { X } from 'lucide-react';
import { profile as profileApi } from '../../lib/api';
import { TelegramPanel } from './TelegramPanel';

const inputClass =
  'w-full rounded-md border border-zinc-700 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-zinc-600 placeholder:text-zinc-600';

// SettingsModal is the global gear-icon panel: owner name + the Telegram bridge.
export function SettingsModal({ onClose }) {
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
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 backdrop-blur-sm"
      onClick={onClose}
    >
      <div
        className="max-h-[85vh] w-full max-w-md overflow-y-auto rounded-2xl border border-zinc-800 bg-zinc-850 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-zinc-800 px-4 py-3">
          <h2 className="text-sm font-semibold text-zinc-100">Settings</h2>
          <button onClick={onClose} className="text-zinc-500 hover:text-zinc-200" title="Close">
            <X size={18} />
          </button>
        </div>

        <div className="flex flex-col gap-6 p-4">
          <section>
            <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-zinc-500">Profile</h3>
            <label className="mb-1 block text-xs font-medium text-zinc-400">Your name</label>
            <div className="flex items-center gap-2">
              <input
                value={name}
                onChange={(e) => setName(e.target.value)}
                onBlur={saveName}
                placeholder="What should we call you?"
                className={inputClass}
              />
            </div>
          </section>

          <section>
            <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-zinc-500">Telegram</h3>
            <TelegramPanel />
          </section>
        </div>
      </div>
    </div>
  );
}
