import { useCallback, useEffect, useRef, useState } from 'react';
import * as api from '../lib/api';

let seq = 0;
const uid = (p) => `${p}${Date.now()}_${seq++}`;

// useConversations is the frontend's data layer: threads, the active thread's
// messages (including live tool steps), the model list, and the per-session model.
export function useConversations() {
  const [threads, setThreads] = useState([]);
  const [activeThreadId, setActiveThreadId] = useState(null);
  const [activeThread, setActiveThread] = useState(null); // { thread, messages }
  const [models, setModels] = useState([]);
  const [defaultModel, setDefaultModel] = useState(null);
  const [pendingModel, setPendingModel] = useState(null); // model chosen before a thread exists
  const [loading, setLoading] = useState(true);
  const abortRef = useRef(null); // AbortController for the in-flight stream (pause)

  // refreshThreads reloads the sidebar list and keeps the open thread's title and
  // model in sync (the backend titles threads in the background).
  const refreshThreads = useCallback(async () => {
    const list = await api.listThreads();
    setThreads(list);
    setActiveThread((prev) => {
      if (!prev?.thread) return prev;
      const match = list.find((t) => t.id === prev.thread.id);
      return match ? { ...prev, thread: { ...prev.thread, title: match.title, model: match.model } } : prev;
    });
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
    setPendingModel(null);
    setActiveThread(null);
    setActiveThreadId(null);
  }, []);

  const selectThread = useCallback((id) => loadThread(id), [loadThread]);

  const removeThread = useCallback(
    async (id) => {
      await api.deleteThread(id);
      const list = await refreshThreads();
      if (activeThreadId === id) {
        if (list.length) {
          await loadThread(list[0].id);
        } else {
          setActiveThread(null);
          setActiveThreadId(null);
        }
      }
    },
    [activeThreadId, refreshThreads, loadThread],
  );

  const setModel = useCallback(
    async (model) => {
      if (!activeThreadId) {
        setPendingModel(model); // remember it for the thread this chat will create
        return;
      }
      const thread = await api.setThreadModel(activeThreadId, model);
      setActiveThread((prev) => (prev ? { ...prev, thread } : prev));
      await refreshThreads();
    },
    [activeThreadId, refreshThreads],
  );

  // appendText adds streamed text to the trailing assistant message, or starts a
  // new one if the last message is a tool step (so text before and after a tool
  // call stay in natural order).
  const appendText = (text) =>
    setActiveThread((prev) => {
      if (!prev) return prev;
      const messages = [...prev.messages];
      const last = messages[messages.length - 1];
      if (last && last.role === 'assistant') {
        messages[messages.length - 1] = { ...last, content: last.content + text };
      } else {
        messages.push({ id: uid('a'), role: 'assistant', content: text });
      }
      return { ...prev, messages };
    });

  // startTool appends a tool step (with the input it was called with) in order.
  const startTool = (ev) =>
    setActiveThread((prev) => {
      if (!prev) return prev;
      const step = { id: uid('t'), role: 'tool', tool: ev.tool, info: ev.info, input: ev.data, output: null, running: true };
      return { ...prev, messages: [...prev.messages, step] };
    });

  const finishTool = (ev) =>
    setActiveThread((prev) => {
      if (!prev) return prev;
      const messages = [...prev.messages];
      for (let i = messages.length - 1; i >= 0; i--) {
        if (messages[i].role === 'tool' && messages[i].running) {
          messages[i] = { ...messages[i], output: ev.data, running: false };
          break;
        }
      }
      return { ...prev, messages };
    });

  const send = useCallback(
    async (content) => {
      let id = activeThreadId;
      const model = pendingModel || defaultModel;
      if (!id) {
        const thread = await api.createThread(model);
        id = thread.id;
        setActiveThreadId(id);
        setActiveThread({ thread, messages: [] });
        setPendingModel(null);
        refreshThreads();
      }
      setActiveThread((prev) => {
        const base = prev || { thread: { id, model }, messages: [] };
        return { ...base, messages: [...base.messages, { id: uid('u'), role: 'user', content }] };
      });
      const controller = new AbortController();
      abortRef.current = controller;
      await api.sendMessage(
        id,
        content,
        (ev) => {
          if (ev.type === 'text') appendText(ev.content || '');
          else if (ev.type === 'tool_call') startTool(ev);
          else if (ev.type === 'tool_result') finishTool(ev);
          else if (ev.type === 'error') appendText(`\n\n[error] ${ev.message || ''}`);
        },
        controller.signal,
      );
      abortRef.current = null;
      await refreshThreads();
      // The backend titles the thread in the background; poll a few times to catch it.
      [4000, 10000, 18000].forEach((ms) => setTimeout(() => refreshThreads(), ms));
    },
    [activeThreadId, defaultModel, pendingModel, refreshThreads],
  );

  // stop aborts the in-flight stream (pause button). The partial reply is kept
  // and a "paused" marker is appended so the user sees it was stopped.
  const stop = useCallback(() => {
    if (!abortRef.current) return;
    abortRef.current.abort();
    abortRef.current = null;
    setActiveThread((prev) =>
      prev ? { ...prev, messages: [...prev.messages, { id: uid('n'), role: 'note', kind: 'paused' }] } : prev,
    );
  }, []);

  const currentModel = activeThread?.thread?.model || pendingModel || defaultModel;

  return {
    stop,
    threads,
    activeThread,
    activeThreadId,
    loading,
    models,
    defaultModel,
    currentModel,
    createNewThread,
    selectThread,
    removeThread,
    send,
    setModel,
  };
}
