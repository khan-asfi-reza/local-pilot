import { useCallback, useRef, useState } from 'react';
import * as api from './api';

let seq = 0;
const uid = (p) => `${p}${Date.now()}_${seq++}`;

// useBuilder holds builder session state and the streaming reducer, mirroring
// the pattern from useConversations.js.
export function useBuilder() {
  const [sessionId, setSessionId] = useState(null);
  const [messages, setMessages] = useState([]); // streaming log entries
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);
  const [error, setError] = useState(null);
  const [writtenFiles, setWrittenFiles] = useState([]);
  const [previewVersion, setPreviewVersion] = useState(0);
  const [models, setModels] = useState([]);
  const [defaultModel, setDefaultModel] = useState(null);
  const [currentModel, setCurrentModel] = useState(null);
  const [tokens, setTokens] = useState(0);
  const abortRef = useRef(null);

  const appendText = useCallback((text) => {
    setMessages((prev) => {
      const msgs = [...prev];
      const last = msgs[msgs.length - 1];
      if (last && last.role === 'assistant') {
        msgs[msgs.length - 1] = { ...last, content: last.content + text };
      } else {
        msgs.push({ id: uid('a'), role: 'assistant', content: text });
      }
      return msgs;
    });
  }, []);

  const appendReasoning = useCallback((text) => {
    setMessages((prev) => {
      const msgs = [...prev];
      const last = msgs[msgs.length - 1];
      if (last && last.role === 'assistant') {
        msgs[msgs.length - 1] = { ...last, reasoning: (last.reasoning || '') + text };
      } else {
        msgs.push({ id: uid('a'), role: 'assistant', content: '', reasoning: text });
      }
      return msgs;
    });
  }, []);

  const startTool = useCallback((ev) => {
    setMessages((prev) => {
      const step = { id: uid('t'), role: 'tool', tool: ev.tool, info: ev.info, input: ev.data, output: null, running: true };
      return [...prev, step];
    });
  }, []);

  const finishTool = useCallback((ev) => {
    setMessages((prev) => {
      const msgs = [...prev];
      for (let i = msgs.length - 1; i >= 0; i--) {
        if (msgs[i].role === 'tool' && msgs[i].running) {
          msgs[i] = { ...msgs[i], output: ev.data, running: false };
          break;
        }
      }
      return msgs;
    });
  }, []);

  const handleEvent = useCallback((ev) => {
    if (ev.type === 'text') appendText(ev.content || '');
    else if (ev.type === 'reasoning') appendReasoning(ev.content || '');
    else if (ev.type === 'tool_call') startTool(ev);
    else if (ev.type === 'tool_result') finishTool(ev);
    else if (ev.type === 'error') appendText(`\n\n[error] ${ev.message || ''}`);
    else if (ev.type === 'done') setDone(true);
    else if (ev.type === 'usage') setTokens(ev.tokens || 0);
    else if (ev.type === 'files') {
      setWrittenFiles(ev.files || []);
      setPreviewVersion((v) => v + 1);
    }
  }, [appendText, appendReasoning, startTool, finishTool]);

  // create starts a new builder session and streams the initial build.
  const create = useCallback(
    async (prompt) => {
      setMessages([]);
      setDone(false);
      setError(null);
      setWrittenFiles([]);
      setBusy(true);
      const model = currentModel || defaultModel;
      try {
        const { id } = await api.createSession(prompt, model);
        setSessionId(id);
        const controller = new AbortController();
        abortRef.current = controller;
        await api.generate(id, prompt, handleEvent, controller.signal);
        abortRef.current = null;
      } catch (e) {
        const msg = String(e);
        if (msg.includes('Failed to fetch') || msg.includes('NetworkError') || msg.includes('TypeError')) {
          setError('Cannot reach the backend. Make sure the server is running on port 8182.');
        } else {
          setError(msg);
        }
      }
      setBusy(false);
    },
    [currentModel, defaultModel, handleEvent],
  );

  // generate sends a follow-up prompt to an existing session with prior context.
  const generate = useCallback(
    async (prompt) => {
      if (!sessionId) return;
      setDone(false);
      setBusy(true);
      // Build compact history: user prompts + assistant content only (no tool noise).
      const history = messages
        .filter((m) => m.role === 'user' || (m.role === 'assistant' && m.content))
        .map((m) => ({ role: m.role, content: m.content }));
      const controller = new AbortController();
      abortRef.current = controller;
      try {
        await api.generate(sessionId, prompt, handleEvent, controller.signal, history);
      } catch (e) {
        const msg = String(e);
        if (msg.includes('Failed to fetch') || msg.includes('NetworkError') || msg.includes('TypeError')) {
          setError('Cannot reach the backend. Make sure the server is running on port 8182.');
        } else {
          setError(msg);
        }
      }
      abortRef.current = null;
      setBusy(false);
    },
    [sessionId, messages, handleEvent],
  );

  // preview returns the iframe URL, cache-busted after each file write.
  const preview = useCallback(() => {
    if (!sessionId) return null;
    return `${api.previewUrl(sessionId)}?v=${previewVersion}`;
  }, [sessionId, previewVersion]);

  const stop = useCallback(() => {
    if (!abortRef.current) return;
    abortRef.current.abort();
    abortRef.current = null;
    setBusy(false);
  }, []);

  const reset = useCallback(() => {
    if (abortRef.current) {
      abortRef.current.abort();
      abortRef.current = null;
    }
    setSessionId(null);
    setMessages([]);
    setBusy(false);
    setDone(false);
    setError(null);
    setWrittenFiles([]);
    setTokens(0);
    setPreviewVersion((v) => v + 1);
  }, []);

  // exportToFolder uses the File System Access API, or falls back to a download.
  const exportToFolder = useCallback(async () => {
    if (!sessionId) return;
    let files;
    try {
      const data = await api.getFiles(sessionId);
      files = data.files || [];
    } catch (e) {
      throw new Error(`Failed to fetch files: ${e.message || e}`);
    }
    if (!files.length) return;

    if (window.showDirectoryPicker) {
      try {
        const dirHandle = await window.showDirectoryPicker();
        for (const f of files) {
          const fileHandle = await dirHandle.getFileHandle(f.path, { create: true });
          const writable = await fileHandle.createWritable();
          await writable.write(f.content);
          await writable.close();
        }
      } catch (e) {
        if (e?.name !== 'AbortError') {
          throw new Error(`Export failed: ${e.message || e}`);
        }
      }
    } else {
      // Fallback: bundle everything into a single self-contained HTML file.
      const htmlFile = files.find((f) => f.path.endsWith('.html'));
      if (!htmlFile) return;
      let html = htmlFile.content;
      for (const f of files) {
        if (f.path.endsWith('.css')) {
          html = html.replace(
            new RegExp(`<link[^>]*href=["']${f.path.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}["'][^>]*>`, 'i'),
            `<style>\n${f.content}\n</style>`,
          );
        }
      }
      for (const f of files) {
        if (f.path.endsWith('.js')) {
          html = html.replace(
            new RegExp(`<script[^>]*src=["']${f.path.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}["'][^>]*>\\s*</script>`, 'i'),
            `<script>\n${f.content}\n</script>`,
          );
        }
      }
      const blob = new Blob([html], { type: 'text/html' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = 'index.html';
      a.click();
      URL.revokeObjectURL(url);
    }
  }, [sessionId]);

  return {
    sessionId,
    messages,
    busy,
    done,
    error,
    writtenFiles,
    tokens,
    models,
    defaultModel,
    currentModel,
    setCurrentModel,
    setModels,
    setDefaultModel,
    create,
    generate,
    preview,
    stop,
    reset,
    exportToFolder,
  };
}
