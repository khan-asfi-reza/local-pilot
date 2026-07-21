import { useCallback, useRef, useState } from 'react';
import { code } from '../../lib/api';

let seq = 0;
const uid = (p) => `${p}${Date.now()}_${seq++}`;

export function useCodeAgent() {
  const [messages, setMessages] = useState([]);
  const [busy, setBusy] = useState(false);
  const abortRef = useRef(null);
  const messagesRef = useRef([]);

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
          msgs[i] = { ...msgs[i], output: ev.data, running: false };
          break;
        }
      }
      syncRef(msgs);
      return msgs;
    });
  }, [syncRef]);

  const handleEvent = useCallback(
    (ev) => {
      if (ev.type === 'text') appendText(ev.content || '');
      else if (ev.type === 'reasoning') appendReasoning(ev.content || '');
      else if (ev.type === 'tool_call') startTool(ev);
      else if (ev.type === 'tool_result') finishTool(ev);
      else if (ev.type === 'error') appendText(`\n\n[error] ${ev.message || ''}`);
    },
    [appendText, appendReasoning, startTool, finishTool],
  );

  const send = useCallback(
    async (prompt, root, model, onDone) => {
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

      const outgoing = messagesRef.current.map((m) => ({ role: m.role, content: m.content }));

      try {
        await code.streamCodeAgent(root, model, outgoing, (ev) => {
          handleEvent(ev);
          if (ev.type === 'done') {
            setBusy(false);
            abortRef.current = null;
            onDone?.();
          }
        }, controller.signal);
      } catch (e) {
        if (e?.name !== 'AbortError') {
          appendText(`\n\n[error] ${String(e)}`);
        }
        setBusy(false);
        abortRef.current = null;
        onDone?.();
      }
    },
    [handleEvent, appendText, syncRef],
  );

  const stop = useCallback(() => {
    if (!abortRef.current) return;
    abortRef.current.abort();
    abortRef.current = null;
    setBusy(false);
  }, []);

  return { messages, busy, send, stop };
}
