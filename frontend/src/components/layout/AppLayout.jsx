import { ChatWindow } from '../chat/ChatWindow';

export function AppLayout() {
  return (
    <div className="min-h-screen bg-slate-50 text-slate-900">
      <header className="border-b border-slate-200 bg-white px-6 py-4">
        <h1 className="text-lg font-semibold">local-pilot</h1>
      </header>
      <div className="h-[calc(100vh-73px)]">
        <ChatWindow />
      </div>
    </div>
  );
}
