import { useState } from 'react';
import { Settings as SettingsIcon } from 'lucide-react';
import { SettingsModal } from './SettingsModal';

// SettingsButton is a self-contained gear + modal, dropped into each page's
// header (there is no shared app shell). Owning its own open state means a page
// only has to render <SettingsButton />, with no lifted state or floating overlay
// that collides with page controls. `className` positions it for the host.
export function SettingsButton({ className = '' }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        title="Settings"
        className={`flex h-8 w-8 items-center justify-center rounded-lg text-zinc-400 transition-colors hover:bg-zinc-800 hover:text-zinc-100 ${className}`}
      >
        <SettingsIcon size={17} />
      </button>
      {open && <SettingsModal onClose={() => setOpen(false)} />}
    </>
  );
}
