import { useCallback, useEffect, useRef, useState } from 'react';
import * as api from './api';

let seq = 0;
const uid = (p) => `${p}${Date.now()}_${seq++}`;

// useBuilder manages the project list and the currently open project: its build
// log (streamed), live preview URL, and source files.
export function useBuilder() {
  const [projects, setProjects] = useState([]);
  const [activeId, setActiveId] = useState(null);
  const [activeName, setActiveName] = useState('');
  const [previewUrl, setPreviewUrl] = useState(null);
  const [files, setFiles] = useState([]);
  const [messages, setMessages] = useState([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const [models, setModels] = useState([]);
  const [defaultModel, setDefaultModel] = useState(null);
  const [currentModel, setCurrentModel] = useState(null);
  // Runtime console errors forwarded from the preview iframe (postMessage bridge).
  const [consoleErrors, setConsoleErrors] = useState([]);
  const abortRef = useRef(null);

  useEffect(() => {
    const onMsg = (e) => {
      const d = e.data;
      if (!d || d.source !== 'builder-preview') return;
      setConsoleErrors((prev) => [...prev.slice(-49), { id: uid('e'), level: d.level, text: d.text }]);
    };
    window.addEventListener('message', onMsg);
    return () => window.removeEventListener('message', onMsg);
  }, []);
  const clearConsole = useCallback(() => setConsoleErrors([]), []);

  const loadProjects = useCallback(async () => {
    try {
      const { projects: list } = await api.listProjects();
      setProjects(list || []);
    } catch {
      /* backend down */
    }
  }, []);

  useEffect(() => {
    loadProjects();
    (async () => {
      try {
        const base =
          import.meta.env.VITE_API_URL || `${window.location.protocol}//${window.location.hostname}:8182`;
        const res = await fetch(`${base}/models`);
        if (res.ok) {
          const d = await res.json();
          setModels(d.models || []);
          setDefaultModel(d.default || null);
          setCurrentModel(d.default || null);
        }
      } catch {
        /* ignore */
      }
    })();
  }, [loadProjects]);

  // --- streaming reducer ---------------------------------------------------
  const appendText = useCallback((text) => {
    setMessages((prev) => {
      const msgs = [...prev];
      const last = msgs[msgs.length - 1];
      if (last && last.role === 'assistant') msgs[msgs.length - 1] = { ...last, content: last.content + text };
      else msgs.push({ id: uid('a'), role: 'assistant', content: text, reasoning: '' });
      return msgs;
    });
  }, []);
  const appendReasoning = useCallback((text) => {
    setMessages((prev) => {
      const msgs = [...prev];
      const last = msgs[msgs.length - 1];
      if (last && last.role === 'assistant') msgs[msgs.length - 1] = { ...last, reasoning: (last.reasoning || '') + text };
      else msgs.push({ id: uid('a'), role: 'assistant', content: '', reasoning: text });
      return msgs;
    });
  }, []);
  const startTool = useCallback((ev) => {
    setMessages((prev) => [...prev, { id: uid('t'), role: 'tool', tool: ev.tool, info: ev.info, input: ev.data, output: null, running: true }]);
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
  const pushError = useCallback((text) => {
    setMessages((prev) => [...prev, { id: uid('e'), role: 'error', content: String(text) }]);
  }, []);
  const handleEvent = useCallback(
    (ev) => {
      if (ev.type === 'text') appendText(ev.content || '');
      else if (ev.type === 'reasoning') appendReasoning(ev.content || '');
      else if (ev.type === 'tool_call') startTool(ev);
      else if (ev.type === 'tool_result') finishTool(ev);
      else if (ev.type === 'files') setFiles(ev.files || []);
      else if (ev.type === 'error') pushError(ev.message || 'Something went wrong');
    },
    [appendText, appendReasoning, startTool, finishTool, pushError],
  );

  // --- project navigation --------------------------------------------------
  const rename = useCallback(
    async (name) => {
      const n = (name || '').trim();
      if (!n || !activeId) return;
      setActiveName(n);
      try {
        await api.renameProject(activeId, n);
        loadProjects();
      } catch {
        /* ignore */
      }
    },
    [activeId, loadProjects],
  );

  const openProject = useCallback(async (id) => {
    setActiveId(id);
    setMessages([]);
    setConsoleErrors([]);
    setError(null);
    try {
      const p = await api.getProject(id);
      setActiveName(p.name);
      setPreviewUrl(p.url);
      setFiles(p.files || []);
      // Restore the persisted build log so past messages show after reload.
      setMessages((p.messages || []).map((m) => ({ id: uid(m.role[0] || 'm'), role: m.role, content: m.content || '' })));
    } catch (e) {
      setError(String(e));
    }
  }, []);

  // createBlank makes an empty project and returns its id; the caller navigates
  // to /builder/<id> and the URL drives openProject.
  const createBlank = useCallback(async () => {
    const { id } = await api.createProject('', '');
    await loadProjects();
    return id;
  }, [loadProjects]);

  const closeProject = useCallback(() => {
    if (abortRef.current) abortRef.current.abort();
    abortRef.current = null;
    setActiveId(null);
    setActiveName('');
    setPreviewUrl(null);
    setFiles([]);
    setMessages([]);
    setBusy(false);
    loadProjects();
  }, [loadProjects]);

  const removeProject = useCallback(
    async (id) => {
      await api.deleteProject(id);
      await loadProjects();
    },
    [loadProjects],
  );

  // --- build + edit --------------------------------------------------------
  const send = useCallback(
    async (prompt) => {
      if (!activeId) return;
      // Name the app from the first prompt if it is still the default.
      if (!activeName || activeName === 'Untitled app') {
        const nm = prompt.trim().replace(/\s+/g, ' ').slice(0, 40) || 'Untitled app';
        setActiveName(nm);
        api.renameProject(activeId, nm).then(loadProjects).catch(() => {});
      }
      setMessages((prev) => [...prev, { id: uid('u'), role: 'user', content: prompt }]);
      setConsoleErrors([]);
      setBusy(true);
      setError(null);
      const controller = new AbortController();
      abortRef.current = controller;
      const history = messages
        .filter((m) => m.role === 'user' || (m.role === 'assistant' && m.content))
        .map((m) => ({ role: m.role, content: m.content }));
      try {
        await api.generate(
          activeId,
          prompt,
          (ev) => {
            handleEvent(ev);
            if (ev.type === 'done') {
              setBusy(false);
              abortRef.current = null;
            }
          },
          controller.signal,
          history,
          currentModel || defaultModel,
        );
      } catch (e) {
        if (e?.name !== 'AbortError') setError(String(e));
      }
      setBusy(false);
      abortRef.current = null;
      // Refresh the file list after a build.
      api.getProject(activeId).then((p) => setFiles(p.files || [])).catch(() => {});
    },
    [activeId, activeName, loadProjects, messages, handleEvent, currentModel, defaultModel],
  );

  const stop = useCallback(() => {
    if (!abortRef.current) return;
    abortRef.current.abort();
    abortRef.current = null;
    setBusy(false);
  }, []);

  const readSource = useCallback(async (path) => (await api.readFile(activeId, path)).content, [activeId]);
  const saveSource = useCallback(async (path, content) => { await api.writeFile(activeId, path, content); }, [activeId]);

  const run = useCallback(async () => {
    if (!activeId) return;
    setConsoleErrors([]);
    try {
      const { url } = await api.runProject(activeId);
      // Force the iframe to reload by re-setting the URL with a cache-key.
      setPreviewUrl(`${url}?t=${Date.now()}`);
    } catch (e) {
      setError(String(e));
    }
  }, [activeId]);

  return {
    projects, activeId, activeName, previewUrl, files, messages, busy, error,
    models, defaultModel, currentModel, setCurrentModel, consoleErrors, clearConsole,
    loadProjects, openProject, createBlank, closeProject, removeProject, rename,
    send, stop, readSource, saveSource, run, exportUrl: api.exportUrl,
  };
}
