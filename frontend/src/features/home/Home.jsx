import { useNavigate } from 'react-router-dom';
import { Code, MessageSquare, Wand2, ArrowUpRight } from 'lucide-react';
import { cn } from '../../lib/utils';
import { SettingsButton } from '../settings/SettingsButton';

const CARDS = [
  {
    key: 'code',
    title: 'Code',
    description: 'Open a project and let the agent edit real files with you.',
    icon: Code,
    to: '/code',
  },
  {
    key: 'chat',
    title: 'Chat',
    description: 'Think out loud with a model running on your own machine.',
    icon: MessageSquare,
    to: '/chat',
  },
  {
    key: 'builder',
    title: 'App Builder',
    description: 'Describe an app and watch it build, live, in the preview.',
    icon: Wand2,
    to: '/builder',
  },
];

function Card({ title, description, icon: Icon, to, navigate }) {
  return (
    <button
      type="button"
      onClick={() => navigate(to)}
      className={cn(
        'group relative flex w-72 flex-col gap-5 overflow-hidden rounded-2xl border border-zinc-800 bg-zinc-850 p-6 text-left',
        'transition-all duration-200 hover:-translate-y-1 hover:border-zinc-700',
      )}
    >
      {/* gradient wash that blooms on hover */}
      <span className="pointer-events-none absolute inset-0 bg-gradient-to-br from-emerald-500/0 to-teal-600/0 opacity-0 transition-opacity duration-300 group-hover:from-emerald-500/[0.07] group-hover:to-teal-600/[0.07] group-hover:opacity-100" />
      <span className="flex h-11 w-11 items-center justify-center rounded-xl bg-gradient-to-br from-emerald-500 to-teal-600 text-white shadow-glow">
        <Icon size={20} strokeWidth={2} />
      </span>
      <div className="relative">
        <div className="flex items-center gap-1.5">
          <h3 className="text-[15px] font-semibold text-zinc-100">{title}</h3>
          <ArrowUpRight
            size={15}
            className="text-zinc-600 opacity-0 transition-all duration-200 group-hover:translate-x-0.5 group-hover:-translate-y-0.5 group-hover:text-zinc-300 group-hover:opacity-100"
          />
        </div>
        <p className="mt-1.5 text-[13px] leading-relaxed text-zinc-400">{description}</p>
      </div>
    </button>
  );
}

export function Home() {
  const navigate = useNavigate();

  return (
    <div className="hero-wash relative flex h-full flex-col items-center justify-center gap-12 px-6">
      <SettingsButton className="absolute right-4 top-4" />
      <div className="flex flex-col items-center text-center">
        <p className="eyebrow mb-4">Local AI · runs on your machine</p>
        <h1 className="bg-gradient-to-br from-zinc-50 to-zinc-400 bg-clip-text text-5xl font-semibold tracking-tight text-transparent">
          Pilot
        </h1>
        <p className="mt-3 max-w-sm text-[15px] leading-relaxed text-zinc-400">
          Your coding agent, chat, and app builder, all running against a model on this machine.
        </p>
      </div>

      <div className="flex flex-wrap items-stretch justify-center gap-4">
        {CARDS.map((c) => (
          <Card key={c.key} {...c} navigate={navigate} />
        ))}
      </div>
    </div>
  );
}
