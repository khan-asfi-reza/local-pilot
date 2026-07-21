import { useCallback, useRef, useState } from 'react';
import { code } from '../../lib/api';

let seq = 0;
const uid = (p) => `${p}${Date.now()}_${seq++}`;

export function useCodeAgent(onFileChange) {
  const [messages, setMessages] = useState([]);
  const [busy, setBusy] = useState(false);
  // In ask mode the run pauses on a mutating action; pendingConfirm holds the
  // details ({id, tool, summary, diff}) until the user approves or rejects.
  const [pendingConfirm, setPendingConfirm] = useState(null);
  const abortRef = useRef(null);
  const messagesRef = useRef([]);
  const sessionIdRef = useRef(null); // current .pilot session id (shared w/ terminal)
  const onFileChangeRef = useRef(onFileChange);
  onFileChangeRef.current = onFileChange; // keep the callback fresh without re-subscribing

  const syncRef = useCallback((msgs) => {
    messagesRef.current = msgs;
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
      if (ev.type === 'session') sessionIdRef.current = ev.id;
      else if (ev.type === 'text') appendText(ev.content || '');
      else if (ev.type === 'reasoning') appendReasoning(ev.content || '');
      else if (ev.type === 'tool_call') startTool(ev);
      else if (ev.type === 'tool_result') finishTool(ev);
      else if (ev.type === 'confirm') {
        setPendingConfirm({ id: ev.id, tool: ev.tool, summary: ev.summary, diff: ev.diff });
      } else if (ev.type === 'error') pushError(ev.message || 'Something went wrong');
    },
    [appendText, appendReasoning, startTool, finishTool, pushError],
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
      let msgs;
      setMessages((prev) => {
        msgs = [...prev, userMsg];
        syncRef(msgs);
        return msgs;
      });
      setBusy(true);
      const controller = new AbortController();
      abortRef.current = controller;

      // Only the real conversation (user/assistant text) goes to the model and
      // the saved session — not the transient tool cards.
      const outgoing = messagesRef.current
        .filter((m) => m.role === 'user' || m.role === 'assistant')
        .map((m) => ({ role: m.role, content: m.content || '' }));

      try {
        await code.streamCodeAgent(root, model, outgoing, (ev) => {
          handleEvent(ev);
          if (ev.type === 'done') {
            setBusy(false);
            setPendingConfirm(null);
            abortRef.current = null;
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
    [handleEvent, pushError, syncRef],
  );

  const stop = useCallback(() => {
    if (!abortRef.current) return;
    abortRef.current.abort();
    abortRef.current = null;
    setBusy(false);
    setPendingConfirm(null);
  }, []);

  // resume restores the most recent session for a project (on open/reload), so
  // the conversation survives and matches what the terminal would show.
  const resume = useCallback(
    async (root) => {
      if (!root) return;
      try {
        const { sessions: list } = await code.listSessions(root);
        if (!list?.length) {
          sessionIdRef.current = null;
          setMessages([]);
          syncRef([]);
          return;
        }
        const s = await code.loadSession(root, list[0].id);
        const restored = (s.messages || [])
          .filter((m) => m.role === 'user' || m.role === 'assistant')
          .map((m) => ({ id: uid(m.role[0]), role: m.role, content: m.content || '', reasoning: '' }));
        sessionIdRef.current = s.id;
        setMessages(restored);
        syncRef(restored);
      } catch {
        /* no sessions yet */
      }
    },
    [syncRef],
  );

  // newSession clears the panel and starts a fresh session id.
  const newSession = useCallback(() => {
    sessionIdRef.current = null;
    setMessages([]);
    syncRef([]);
    setPendingConfirm(null);
  }, [syncRef]);

  return { messages, busy, send, stop, pendingConfirm, respondConfirm, resume, newSession };
}
