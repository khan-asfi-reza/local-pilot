import { useNavigate } from 'react-router-dom';
import { Settings as SettingsIcon } from 'lucide-react';

// SettingsButton is the gear dropped into each page header. It navigates to the
// dedicated /settings page (models, profile, Telegram). `className` positions it
// for the host.
export function SettingsButton({ className = '' }) {
  const navigate = useNavigate();
  return (
    <button
      type="button"
      onClick={() => navigate('/settings')}
      title="Settings"
      className={`flex h-8 w-8 items-center justify-center rounded-lg text-zinc-400 transition-colors hover:bg-zinc-800 hover:text-zinc-100 ${className}`}
    >
      <SettingsIcon size={17} />
    </button>
  );
}
