import { useEffect, useRef, useState } from 'react';
import { Copy, Check, Trash2 } from 'lucide-react';
import { profile as profileApi, telegram as telegramApi } from '../../lib/api';

const inputClass =
  'w-full rounded-md border border-zinc-700 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-zinc-600 placeholder:text-zinc-600';
const primaryBtn =
  'inline-flex items-center justify-center gap-2 rounded-xl bg-gradient-to-br from-emerald-500 to-teal-600 px-4 py-2 text-sm font-medium text-white shadow-glow transition-transform hover:-translate-y-0.5 disabled:opacity-50 disabled:hover:translate-y-0';

// TelegramPanel is the shared bot-token + Connect + linked-chats UI, used by both
// the onboarding flow and the Settings modal.
export function TelegramPanel() {
  const [token, setToken] = useState('');
  const [enabled, setEnabled] = useState(false);
  const [botUsername, setBotUsername] = useState('');
  const [configured, setConfigured] = useState(false);
  const [links, setLinks] = useState([]);
  const [connect, setConnect] = useState(null); // { code, deep_link }
  const [copied, setCopied] = useState(false);
  const [saving, setSaving] = useState(false);
  const pollRef = useRef(null);

  async function loadSettings() {
    const s = await telegramApi.getSettings();
    setToken(s.bot_token || '');
    setEnabled(s.enabled);
    setBotUsername(s.bot_username || '');
    setConfigured(s.configured);
  }

  async function loadLinks() {
    const p = await profileApi.get();
    setLinks(p?.telegram?.links || []);
  }

  useEffect(() => {
    loadSettings();
    loadLinks();
    return () => clearInterval(pollRef.current);
  }, []);

  // While the Connect panel is open, poll so a freshly linked chat appears.
  useEffect(() => {
    if (!connect) {
      clearInterval(pollRef.current);
      return;
    }
    pollRef.current = setInterval(loadLinks, 3000);
    return () => clearInterval(pollRef.current);
  }, [connect]);

  async function save() {
    setSaving(true);
    try {
      const r = await telegramApi.saveSettings({ bot_token: token.trim(), enabled: true });
      setBotUsername(r.bot_username || '');
      setEnabled(r.enabled);
      setConfigured(r.configured);
    } finally {
      setSaving(false);
    }
  }

  async function startConnect() {
    setConnect(await telegramApi.linkStart());
    setCopied(false);
  }

  function copyCode() {
    if (!connect) return;
    navigator.clipboard?.writeText(connect.code);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  }

  async function revoke(chatId) {
    await telegramApi.revokeLink(chatId);
    loadLinks();
  }

  return (
    <div className="flex flex-col gap-3">
      <div>
        <label className="mb-1 block text-xs font-medium text-zinc-400">Bot token</label>
        <input
          type="password"
          value={token}
          onChange={(e) => setToken(e.target.value)}
          placeholder="Paste your @BotFather token"
          className={inputClass}
        />
        <p className="mt-1 text-[11px] text-zinc-500">
          Create a bot with{' '}
          <a href="https://t.me/BotFather" target="_blank" rel="noreferrer" className="text-emerald-400 hover:underline">
            @BotFather
          </a>{' '}
          and paste the token. Stored on this machine only.
        </p>
      </div>

      <div className="flex items-center justify-between">
        <span className="text-xs text-zinc-500">
          {configured
            ? enabled
              ? `Connected${botUsername ? ` as @${botUsername}` : ''}`
              : 'Saved (disabled)'
            : 'Not configured'}
        </span>
        <button type="button" onClick={save} disabled={saving || !token.trim()} className={primaryBtn}>
          {saving ? 'Saving…' : 'Save token'}
        </button>
      </div>

      {configured && (
        <div className="rounded-xl border border-zinc-800 bg-zinc-900/50 p-3">
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium text-zinc-200">Connect a Telegram chat</span>
            <button
              type="button"
              onClick={startConnect}
              className="rounded-lg border border-zinc-700 px-3 py-1.5 text-xs text-zinc-200 hover:border-zinc-600"
            >
              {connect ? 'New code' : 'Connect'}
            </button>
          </div>

          {connect && (
            <div className="mt-3 flex flex-col gap-2">
              <p className="text-xs text-zinc-400">
                Open the bot and send this code, or tap the link (expires in 10 min):
              </p>
              <div className="flex items-center gap-2">
                <code className="flex-1 rounded-md bg-zinc-950 px-3 py-2 font-mono text-sm text-emerald-400">
                  /link {connect.code}
                </code>
                <button
                  type="button"
                  onClick={copyCode}
                  className="rounded-md border border-zinc-700 p-2 text-zinc-300 hover:border-zinc-600"
                  title="Copy code"
                >
                  {copied ? <Check size={15} /> : <Copy size={15} />}
                </button>
              </div>
              {connect.deep_link && (
                <a
                  href={connect.deep_link}
                  target="_blank"
                  rel="noreferrer"
                  className="text-xs text-emerald-400 hover:underline"
                >
                  Open in Telegram →
                </a>
              )}
            </div>
          )}

          {links.length > 0 && (
            <div className="mt-3 flex flex-col gap-1.5">
              <span className="text-[11px] uppercase tracking-wide text-zinc-500">Linked chats</span>
              {links.map((l) => (
                <div key={l.chat_id} className="flex items-center justify-between rounded-md bg-zinc-950/60 px-2.5 py-1.5">
                  <span className="text-sm text-zinc-200">
                    {l.display_name || l.tg_username || l.chat_id}
                    {!l.authorized && <span className="ml-1.5 text-[10px] text-amber-400">(pending)</span>}
                  </span>
                  <button
                    type="button"
                    onClick={() => revoke(l.chat_id)}
                    className="text-zinc-500 hover:text-red-400"
                    title="Revoke"
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
