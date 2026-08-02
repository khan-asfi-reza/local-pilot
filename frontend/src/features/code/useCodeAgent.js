import { useCallback, useRef, useState } from 'react';
import { code } from '../../lib/api';

let seq = 0;
const uid = (p) => `${p}${Date.now()}_${seq++}`;

export function useCodeAgent(onFileChange, onActivity) {
  const [messages, setMessages] = useState([]);
  const [busy, setBusy] = useState(false);
  // In ask mode the run pauses on a mutating action; pendingConfirm holds the
  // details ({id, tool, summary, diff}) until the user approves or rejects.
  const [pendingConfirm, setPendingConfirm] = useState(null);
  // The project's saved sessions (newest first) and the active one, for the
  // history switcher. sessionIdRef mirrors sessionId so the streaming callbacks
  // read the current id without going stale.
  const [sessions, setSessions] = useState([]);
  const [sessionId, setSessionId] = useState(null);
  const abortRef = useRef(null);
  const messagesRef = useRef([]);
  const sessionIdRef = useRef(null); // current .pilot session id (shared w/ terminal)
  const onFileChangeRef = useRef(onFileChange);
  onFileChangeRef.current = onFileChange; // keep the callback fresh without re-subscribing
  const onActivityRef = useRef(onActivity);
  onActivityRef.current = onActivity;

  const syncRef = useCallback((msgs) => {
    messagesRef.current = msgs;
  }, []);

  // setSid keeps the ref (read by streaming callbacks) and the state (read by
  // the UI) in lockstep.
  const setSid = useCallback((id) => {
    sessionIdRef.current = id;
    setSessionId(id);
  }, []);

  const refreshSessions = useCallback(async (root) => {
    if (!root) return;
    try {
      const { sessions: list } = await code.listSessions(root);
      setSessions(list || []);
    } catch {
      setSessions([]);
    }
  }, []);

  const appendText = useCallback((text) => {
    setMessages((prev) => {
      const msgs = [...prev];
      const last = msgs[msgs.length - 1];
      if (last && last.role === 'assistant') {
        msgs[msgs.length - 1] = { ...last, content: last.content + text };
      } else {
        msgs.push({ id: uid('a'), role: 'assistant', content: text, reasoning: '' });
      }
      syncRef(msgs);
      return msgs;
    });
  }, [syncRef]);

  const appendReasoning = useCallback((text) => {
    setMessages((prev) => {
      const msgs = [...prev];
      const last = msgs[msgs.length - 1];
      if (last && last.role === 'assistant') {
        msgs[msgs.length - 1] = { ...last, reasoning: (last.reasoning || '') + text };
      } else {
        msgs.push({ id: uid('a'), role: 'assistant', content: '', reasoning: text });
      }
      syncRef(msgs);
      return msgs;
    });
  }, [syncRef]);

  const startTool = useCallback((ev) => {
    setMessages((prev) => {
      const step = { id: uid('t'), role: 'tool', tool: ev.tool, info: ev.info, input: ev.data, output: null, running: true };
      const msgs = [...prev, step];
      syncRef(msgs);
      return msgs;
    });
  }, [syncRef]);

  const finishTool = useCallback((ev) => {
    setMessages((prev) => {
      const msgs = [...prev];
      for (let i = msgs.length - 1; i >= 0; i--) {
        if (msgs[i].role === 'tool' && msgs[i].running) {
          msgs[i] = { ...msgs[i], output: ev.data, diff: ev.diff || null, running: false };
          break;
        }
      }
      syncRef(msgs);
      return msgs;
    });
    // A completed write/edit carries a diff whose path is the changed file; tell
    // the editor to reload it (live, like VS Code).
    if (ev.diff?.path) onFileChangeRef.current?.(ev.diff.path);
    // Any finished tool may have added or removed files (write, mkdir, a shell
    // command), so refresh the tree now instead of waiting for the run to end.
    onActivityRef.current?.();
  }, [syncRef]);

  const pushError = useCallback((text) => {
    setMessages((prev) => {
      const msgs = [...prev, { id: uid('e'), role: 'error', content: String(text) }];
      syncRef(msgs);
      return msgs;
    });
  }, [syncRef]);

  const handleEvent = useCallback(
    (ev) => {
      if (ev.type === 'session') setSid(ev.id);
      else if (ev.type === 'text') appendText(ev.content || '');
      else if (ev.type === 'reasoning') appendReasoning(ev.content || '');
      else if (ev.type === 'tool_call') startTool(ev);
      else if (ev.type === 'tool_result') finishTool(ev);
      else if (ev.type === 'confirm') {
        setPendingConfirm({ id: ev.id, tool: ev.tool, summary: ev.summary, diff: ev.diff });
      } else if (ev.type === 'error') pushError(ev.message || 'Something went wrong');
    },
    [appendText, appendReasoning, startTool, finishTool, pushError, setSid],
  );

  // respondConfirm answers the current ask-mode prompt. The paused run then
  // continues streaming on the still-open connection. decision is
  // 'approve' | 'decline'; feedback redirects the model instead of a plain reject.
  const respondConfirm = useCallback(async (decision, feedback = '') => {
    setPendingConfirm((cur) => {
      if (cur) code.confirmAgent(cur.id, decision, feedback).catch(() => {});
      return null;
    });
  }, []);

  const send = useCallback(
    async (prompt, root, model, mode, onDone) => {
      const userMsg = { id: uid('u'), role: 'user', content: prompt };

      // Build the outgoing history from the prior turns (already in the ref)
      // plus this prompt, explicitly. Reading messagesRef right after
      // setMessages would miss the new prompt — React hasn't run the updater
      // yet — which drops the latest turn from both the model input and the
      // saved session (empty title, "dumb" replies). Only user/assistant text
      // goes to the model, not the transient tool cards.
      const outgoing = messagesRef.current
        .filter((m) => m.role === 'user' || m.role === 'assistant')
        .map((m) => ({ role: m.role, content: m.content || '' }));
      outgoing.push({ role: 'user', content: prompt });

      setMessages((prev) => {
        const msgs = [...prev, userMsg];
        syncRef(msgs);
        return msgs;
      });
      setBusy(true);
      const controller = new AbortController();
      abortRef.current = controller;

      try {
        await code.streamCodeAgent(root, model, outgoing, (ev) => {
          handleEvent(ev);
          if (ev.type === 'done') {
            setBusy(false);
            setPendingConfirm(null);
            abortRef.current = null;
            refreshSessions(root); // the run persisted a session; refresh the list/titles
            onDone?.();
          }
        }, controller.signal, mode, sessionIdRef.current);
      } catch (e) {
        if (e?.name !== 'AbortError') {
          pushError(String(e));
        }
        setBusy(false);
        setPendingConfirm(null);
        abortRef.current = null;
        onDone?.();
      }
    },
    [handleEvent, pushError, syncRef, refreshSessions],
  );

  const stop = useCallback(() => {
    if (!abortRef.current) return;
    abortRef.current.abort();
    abortRef.current = null;
    setBusy(false);
    setPendingConfirm(null);
  }, []);

  // load pulls a stored session into the panel (used by resume + switch).
  const load = useCallback(
    async (root, id) => {
      const s = await code.loadSession(root, id);
      const restored = (s.messages || [])
        .filter((m) => m.role === 'user' || m.role === 'assistant')
        .map((m) => ({ id: uid(m.role[0]), role: m.role, content: m.content || '', reasoning: '' }));
      setSid(s.id);
      setMessages(restored);
      syncRef(restored);
      setPendingConfirm(null);
    },
    [syncRef, setSid],
  );

  // resume restores the most recent session for a project (on open/reload), so
  // the conversation survives and matches what the terminal would show. It also
  // seeds the history list.
  const resume = useCallback(
    async (root) => {
      if (!root) return;
      try {
        const { sessions: list } = await code.listSessions(root);
        setSessions(list || []);
        if (!list?.length) {
          setSid(null);
          setMessages([]);
          syncRef([]);
          return;
        }
        await load(root, list[0].id);
      } catch {
        /* no sessions yet */
      }
    },
    [syncRef, setSid, load],
  );

  // switchSession loads a specific past session from the history menu.
  const switchSession = useCallback(
    async (root, id) => {
      if (!root || !id || id === sessionIdRef.current) return;
      try {
        await load(root, id);
      } catch {
        /* session vanished; leave the panel as-is */
      }
    },
    [load],
  );

  // removeSession deletes a stored session; if it was the active one, the panel
  // drops to a fresh chat.
  const removeSession = useCallback(
    async (root, id) => {
      if (!root || !id) return;
      try {
        await code.deleteSession(root, id);
      } catch {
        /* ignore */
      }
      setSessions((prev) => prev.filter((s) => s.id !== id));
      if (sessionIdRef.current === id) {
        setSid(null);
        setMessages([]);
        syncRef([]);
        setPendingConfirm(null);
      }
    },
    [syncRef, setSid],
  );

  // newSession clears the panel and starts a fresh session id.
  const newSession = useCallback(() => {
    setSid(null);
    setMessages([]);
    syncRef([]);
    setPendingConfirm(null);
  }, [syncRef, setSid]);

  return {
    messages,
    busy,
    send,
    stop,
    pendingConfirm,
    respondConfirm,
    resume,
    newSession,
    sessions,
    sessionId,
    switchSession,
    removeSession,
  };
}
