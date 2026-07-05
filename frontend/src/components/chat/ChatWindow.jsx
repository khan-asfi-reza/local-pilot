import { useState } from 'react';
import { useConversations } from '../../hooks/useConversations';

export function ChatWindow() {
  const { threads, activeThread, activeThreadId, loading, createNewThread, selectThread, send } = useConversations();
  const [draft, setDraft] = useState('');
  const [busy, setBusy] = useState(false);

  const handleSend = async (event) => {
    event.preventDefault();
    if (!draft.trim()) return;
    setBusy(true);
    await send(draft.trim());
    setDraft('');
    setBusy(false);
  };

  if (loading) {
    return <div className="p-6 text-sm text-slate-500">Loading conversations...</div>;
  }

  return (
    <div className="flex h-full">
      <aside className="w-72 border-r border-slate-200 bg-slate-50 p-4">
        <button className="mb-4 w-full rounded bg-slate-900 px-3 py-2 text-sm font-medium text-white" onClick={createNewThread}>
          New chat
        </button>
        <div className="space-y-2">
          {threads.map((thread) => (
            <button key={thread.id} className={`w-full rounded px-3 py-2 text-left text-sm ${activeThreadId === thread.id ? 'bg-white shadow' : 'hover:bg-slate-100'}`} onClick={() => selectThread(thread.id)}>
              {thread.title}
            </button>
          ))}
        </div>
      </aside>
      <main className="flex-1 p-6">
        <div className="mx-auto flex h-full max-w-3xl flex-col">
          <div className="flex-1 overflow-y-auto rounded border border-slate-200 bg-white p-4">
            {activeThread?.messages?.length ? (
              activeThread.messages.map((message) => (
                <div key={message.id} className={`mb-3 rounded px-3 py-2 ${message.role === 'assistant' ? 'bg-slate-100' : 'bg-slate-900 text-white'}`}>
                  {message.content}
                </div>
              ))
            ) : (
              <div className="text-sm text-slate-500">No messages yet.</div>
            )}
          </div>
          <form className="mt-4 flex gap-2" onSubmit={handleSend}>
            <input value={draft} onChange={(event) => setDraft(event.target.value)} className="flex-1 rounded border border-slate-300 px-3 py-2" placeholder="Type a message" />
            <button disabled={busy} className="rounded bg-slate-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-60">
              Send
            </button>
          </form>
        </div>
      </main>
    </div>
  );
}
