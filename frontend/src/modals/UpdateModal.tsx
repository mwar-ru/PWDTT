import { useState } from 'react';
import { IconX, IconDownload, IconRocket } from '@tabler/icons-react';
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime';
import { renderMarkdown } from '../lib/utils/markdown';

interface Props {
  version: string;
  body: string;
  url: string;
  onClose: () => void;
}

const RELEASES_FALLBACK = 'https://github.com/mwar-ru/mwPWDTT_windows/releases';

export default function UpdateModal({ version, body, url, onClose }: Props) {
  const sanitizedHtml = renderMarkdown(body);
  const [updating, setUpdating] = useState(false);
  const [error, setError] = useState('');

  const handleInstall = async () => {
    setUpdating(true);
    setError('');
    try {
      const fn = (window as unknown as {
        go?: { backend?: { App?: { InstallUpdate?: () => Promise<void> } } };
      })?.go?.backend?.App?.InstallUpdate;
      if (!fn) {
        BrowserOpenURL(url || RELEASES_FALLBACK);
        onClose();
        return;
      }
      await fn();
    } catch (e) {
      setUpdating(false);
      setError(`Ошибка автообновления: ${String(e)}`);
    }
  };

  return (
    <>
      <style>{`
        .upd-overlay { position: fixed; inset: 0; background: var(--overlay-bg); backdrop-filter: blur(6px); display: flex; align-items: center; justify-content: center; z-index: 200; animation: overlay-in 0.25s ease-out; }
        .upd-modal { background: var(--surface); border-radius: 16px; padding: 24px; width: 420px; max-width: 92vw; box-shadow: var(--shadow); animation: modal-in 0.3s ease-out; border: 1px solid var(--border); position: relative; overflow: hidden; }
        .upd-modal::before { content: ''; position: absolute; top: 0; left: 0; right: 0; height: 3px; background: linear-gradient(90deg, var(--accent), #22c55e); }
        .upd-header { display: flex; align-items: center; gap: 10px; margin-bottom: 16px; }
        .upd-icon { width: 40px; height: 40px; border-radius: 12px; background: rgba(34, 197, 94, 0.12); display: flex; align-items: center; justify-content: center; color: #22c55e; flex-shrink: 0; }
        .upd-title { font-size: 16px; font-weight: 700; color: var(--text); }
        .upd-version { font-size: 13px; color: var(--text-3); margin-top: 2px; }
        .upd-close { position: absolute; top: 16px; right: 16px; background: none; border: none; cursor: pointer; color: var(--text-3); padding: 4px; border-radius: 6px; transition: color 0.15s, background 0.15s; }
        .upd-close:hover { color: var(--text); background: var(--bg-3); }
        .upd-body { font-size: 13px; color: var(--text-2); line-height: 1.6; max-height: 200px; overflow-y: auto; padding: 12px 14px; background: var(--bg-2); border-radius: 10px; margin-bottom: 16px; word-break: break-word; }
        .upd-body h3 { font-size: 14px; font-weight: 700; color: var(--text); margin: 12px 0 6px; }
        .upd-body h3:first-child { margin-top: 0; }
        .upd-body ul { margin: 4px 0; padding-left: 18px; }
        .upd-body li { margin: 2px 0; }
        .upd-body code { font-size: 12px; background: var(--bg-3); padding: 1px 5px; border-radius: 4px; font-family: monospace; }
        .upd-body a { color: var(--accent); text-decoration: none; }
        .upd-body a:hover { text-decoration: underline; }
        .upd-error { font-size: 12px; color: #ef4444; background: rgba(239, 68, 68, 0.1); border: 1px solid rgba(239, 68, 68, 0.25); border-radius: 8px; padding: 8px 10px; margin-bottom: 12px; word-break: break-word; }
        .upd-actions { display: flex; gap: 10px; }
        .upd-btn { flex: 1; display: flex; align-items: center; justify-content: center; gap: 6px; padding: 11px; border-radius: 10px; font-size: 13px; font-weight: 600; cursor: pointer; transition: all 0.15s; font-family: 'Geist', sans-serif; }
        .upd-btn:disabled { opacity: 0.6; cursor: wait; }
        .upd-btn--primary { background: var(--accent); color: var(--accent-fg); border: none; }
        .upd-btn--primary:hover:not(:disabled) { opacity: 0.9; box-shadow: 0 4px 12px rgba(0,0,0,0.2); }
        .upd-btn--secondary { background: transparent; color: var(--text-3); border: 1px solid var(--border); }
        .upd-btn--secondary:hover:not(:disabled) { border-color: var(--text-3); color: var(--text); }
      `}</style>
      <div className="upd-overlay" onClick={updating ? undefined : onClose}>
        <div className="upd-modal" onClick={e => e.stopPropagation()}>
          <button type="button" className="upd-close" onClick={onClose} disabled={updating} aria-label="Закрыть"><IconX size={18} /></button>

          <div className="upd-header">
            <div className="upd-icon"><IconRocket size={20} /></div>
            <div>
              <div className="upd-title">Доступно обновление</div>
              <div className="upd-version">v{version}</div>
            </div>
          </div>

          {body && <div className="upd-body" dangerouslySetInnerHTML={{ __html: sanitizedHtml }} />}
          {error && <div className="upd-error">{error}</div>}

          <div className="upd-actions">
            {error ? (
              <button type="button" className="upd-btn upd-btn--secondary" onClick={() => { BrowserOpenURL(url || RELEASES_FALLBACK); onClose(); }}>
                Скачать вручную
              </button>
            ) : (
              <button type="button" className="upd-btn upd-btn--secondary" onClick={onClose} disabled={updating}>
                Позже
              </button>
            )}
            <button type="button" className="upd-btn upd-btn--primary" onClick={() => void handleInstall()} disabled={updating}>
              <IconDownload size={16} />
              {updating ? 'Обновление...' : 'Обновить'}
            </button>
          </div>
        </div>
      </div>
    </>
  );
}
