import { useEffect, useRef, useState } from 'react';
import { useConversations } from '../../hooks/useConversations';

export function ChatWindow() {
  const {
    threads,
    activeThread,
    activeThreadId,
    loading,
    models,
    defaultModel,
    currentModel,
    createNewThread,
    selectThread,
    send,
    setModel,
  } = useConversations();
  const [draft, setDraft] = useState('');
  const [busy, setBusy] = useState(false);
  const endRef = useRef(null);

  const visible = (activeThread?.messages || []).filter(
    (m) => m.role === 'user' || m.role === 'assistant',
  );

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [visible.length, activeThread]);

  const handleSend = async (event) => {
    event.preventDefault();
    if (!draft.trim() || busy) return;
    const text = draft.trim();
    setDraft('');
    setBusy(true);
    await send(text);
    setBusy(false);
  };

  if (loading) {
    return <div className="p-6 text-sm text-slate-500">Loading…</div>;
  }

  return (
    <div className="flex h-full">
      <aside className="flex w-72 flex-col border-r border-slate-200 bg-slate-50 p-4">
        <button
          className="mb-4 w-full rounded bg-slate-900 px-3 py-2 text-sm font-medium text-white hover:bg-slate-700"
          onClick={createNewThread}
        >
          + New chat
        </button>
        <div className="flex-1 space-y-1 overflow-y-auto">
          {threads.length === 0 && (
            <div className="px-1 text-xs text-slate-400">No conversations yet.</div>
          )}
          {threads.map((thread) => (
            <button
              key={thread.id}
              className={`block w-full truncate rounded px-3 py-2 text-left text-sm ${
                activeThreadId === thread.id ? 'bg-white shadow' : 'hover:bg-slate-100'
              }`}
              onClick={() => selectThread(thread.id)}
              title={thread.title}
            >
              <div className="truncate">{thread.title || 'Untitled'}</div>
              {thread.model && (
                <div className="truncate text-[11px] text-slate-400">{thread.model}</div>
              )}
            </button>
          ))}
        </div>
      </aside>

      <main className="flex flex-1 flex-col">
        <div className="flex items-center justify-between border-b border-slate-200 bg-white px-6 py-3">
          <div className="text-sm font-medium text-slate-700">
            {activeThread?.thread?.title || 'New chat'}
          </div>
          <label className="flex items-center gap-2 text-xs text-slate-500">
            Model
            <select
              value={currentModel || ''}
              onChange={(e) => setModel(e.target.value)}
              disabled={!activeThreadId}
              className="rounded border border-slate-300 bg-white px-2 py-1 text-sm text-slate-800 disabled:opacity-50"
            >
              {models.length === 0 && <option value="">(no models)</option>}
              {models.map((m) => (
                <option key={m.name} value={m.name}>
                  {m.name}
                  {m.name === defaultModel ? ' · default' : ''}
                  {m.ready ? '' : ' · offline'}
                </option>
              ))}
            </select>
          </label>
        </div>

        <div className="flex-1 overflow-y-auto px-6 py-4">
          <div className="mx-auto max-w-3xl">
            {visible.length === 0 ? (
              <div className="mt-16 text-center text-sm text-slate-400">
                Ask anything to start the conversation.
              </div>
            ) : (
              visible.map((message) => (
                <div
                  key={message.id}
                  className={`mb-4 flex ${message.role === 'user' ? 'justify-end' : 'justify-start'}`}
                >
                  <div
                    className={`max-w-[80%] whitespace-pre-wrap rounded-2xl px-4 py-2 text-sm ${
                      message.role === 'user'
                        ? 'bg-slate-900 text-white'
                        : 'bg-slate-100 text-slate-900'
                    }`}
                  >
                    {message.content || (busy ? '…' : '')}
                  </div>
                </div>
              ))
            )}
            <div ref={endRef} />
          </div>
        </div>

        <form className="border-t border-slate-200 bg-white px-6 py-4" onSubmit={handleSend}>
          <div className="mx-auto flex max-w-3xl gap-2">
            <input
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
              className="flex-1 rounded-lg border border-slate-300 px-3 py-2 text-sm focus:border-slate-500 focus:outline-none"
              placeholder="Message local-pilot…"
            />
            <button
              disabled={busy || !draft.trim()}
              className="rounded-lg bg-slate-900 px-4 py-2 text-sm font-medium text-white hover:bg-slate-700 disabled:opacity-50"
            >
              {busy ? 'Sending…' : 'Send'}
            </button>
          </div>
        </form>
      </main>
    </div>
  );
}
