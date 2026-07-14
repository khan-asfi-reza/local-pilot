import { useNavigate } from 'react-router-dom';
import { Code, Compass, MessageSquare, Wand2 } from 'lucide-react';
import { cn } from '../../lib/utils';

const CARDS = [
  {
    key: 'code',
    title: 'Code',
    description: 'Write, edit, and run code in your project.',
    icon: Code,
    to: '/code',
  },
  {
    key: 'chat',
    title: 'Chat',
    description: 'Ask questions and brainstorm with your local model.',
    icon: MessageSquare,
    to: '/chat',
  },
  {
    key: 'builder',
    title: 'App Builder',
    description: 'Describe an app and watch it build live.',
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
        'group flex w-64 flex-col items-center gap-4 rounded-2xl border border-zinc-800 bg-[#15181d] p-6 text-center',
        'transition-all duration-200 hover:-translate-y-1 hover:border-zinc-600 hover:shadow-lg hover:shadow-black/30',
      )}
    >
      <span className="flex h-14 w-14 items-center justify-center rounded-2xl bg-gradient-to-br from-emerald-500 to-teal-600 text-white shadow-md transition-transform group-hover:scale-110">
        <Icon size={26} strokeWidth={2} />
      </span>
      <div>
        <h3 className="text-base font-semibold text-zinc-100">{title}</h3>
        <p className="mt-1 text-sm text-zinc-500">{description}</p>
      </div>
    </button>
  );
}

export function Home() {
  const navigate = useNavigate();

  return (
    <div className="flex h-full flex-col items-center justify-center gap-10">
      <div className="flex flex-col items-center gap-3">
        <span className="flex h-16 w-16 items-center justify-center rounded-2xl bg-gradient-to-br from-emerald-500 to-teal-600 text-white shadow-lg">
          <Compass size={32} strokeWidth={2.2} />
        </span>
        <h1 className="text-3xl font-bold tracking-tight text-zinc-100">Pilot</h1>
        <p className="text-sm text-zinc-500">Your local AI coding assistant</p>
      </div>
      <div className="flex flex-wrap items-center justify-center gap-5">
        {CARDS.map((c) => (
          <Card key={c.key} {...c} navigate={navigate} />
        ))}
      </div>
    </div>
  );
}
