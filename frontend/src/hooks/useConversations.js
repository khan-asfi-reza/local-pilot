import { useCallback, useEffect, useState } from 'react';
import * as api from '../lib/api';

// useConversations is the frontend's data layer: threads, the active thread's
// messages, the model list, and the per-session model. Sending streams the
// assistant reply token by token.
export function useConversations() {
  const [threads, setThreads] = useState([]);
  const [activeThreadId, setActiveThreadId] = useState(null);
  const [activeThread, setActiveThread] = useState(null); // { thread, messages }
  const [models, setModels] = useState([]);
  const [defaultModel, setDefaultModel] = useState(null);
  const [loading, setLoading] = useState(true);

  const refreshThreads = useCallback(async () => {
    const list = await api.listThreads();
    setThreads(list);
    return list;
  }, []);

  const loadThread = useCallback(async (id) => {
    const data = await api.getThread(id);
    if (data) {
      setActiveThread(data);
      setActiveThreadId(id);
    }
  }, []);

  useEffect(() => {
    (async () => {
      try {
        const m = await api.listModels();
        setModels(m.models || []);
        setDefaultModel(m.default || null);
      } catch {
        /* backend/harness may be down; the UI still renders */
      }
      const list = await refreshThreads();
      if (list.length) await loadThread(list[0].id);
      setLoading(false);
    })();
  }, [refreshThreads, loadThread]);

  const createNewThread = useCallback(async () => {
    const thread = await api.createThread(defaultModel);
    await refreshThreads();
    await loadThread(thread.id);
  }, [defaultModel, refreshThreads, loadThread]);

  const selectThread = useCallback((id) => loadThread(id), [loadThread]);

  const setModel = useCallback(
    async (model) => {
      if (!activeThreadId) return;
      const thread = await api.setThreadModel(activeThreadId, model);
      setActiveThread((prev) => (prev ? { ...prev, thread } : prev));
      await refreshThreads();
    },
    [activeThreadId, refreshThreads],
  );

  const appendToAssistant = (text) =>
    setActiveThread((prev) => {
      if (!prev) return prev;
      const messages = [...prev.messages];
      for (let i = messages.length - 1; i >= 0; i--) {
        if (messages[i].role === 'assistant') {
          messages[i] = { ...messages[i], content: messages[i].content + text };
          break;
        }
      }
      return { ...prev, messages };
    });

  const send = useCallback(
    async (content) => {
      let id = activeThreadId;
      if (!id) {
        const thread = await api.createThread(defaultModel);
        id = thread.id;
        await refreshThreads();
        await loadThread(id);
      }
      const now = Date.now();
      setActiveThread((prev) => {
        const base = prev || { thread: { id, model: defaultModel }, messages: [] };
        return {
          ...base,
          messages: [
            ...base.messages,
            { id: `u${now}`, role: 'user', content },
            { id: `a${now}`, role: 'assistant', content: '' },
          ],
        };
      });
      await api.sendMessage(id, content, (ev) => {
        if (ev.type === 'text') appendToAssistant(ev.content || '');
        else if (ev.type === 'error') appendToAssistant(`\n[error] ${ev.message || ''}`);
      });
      await refreshThreads();
    },
    [activeThreadId, defaultModel, refreshThreads, loadThread],
  );

  const currentModel = activeThread?.thread?.model || defaultModel;

  return {
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
  };
}
