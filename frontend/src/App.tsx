import { useEffect, useState, useRef } from 'react';
import { BrowserRouter, Routes, Route, useNavigate } from 'react-router-dom';
import Layout from './components/Layout';
import Connect from './pages/Connect';
import Logs from './pages/Logs';
import SplitTunnel from './pages/SplitTunnel';
import ForkSettings from './pages/ForkSettings';
import Toast from './components/Toast';
import UpdateModal from './modals/UpdateModal';
import { wdttLinkStore, parseWdttUrl } from './lib/utils/wdttLink';
import { toastStore } from './lib/stores/toastStore';
import { logStore } from './lib/stores/logStore';
import { tunnelStore } from './lib/stores/tunnelStore';
import type { LogLevel } from './lib/stores/logStore';
import { EventsOn } from '../wailsjs/runtime/runtime';
import { CheckUpdate, Connect as WailsConnect, Disconnect as WailsDisconnect } from '../wailsjs/go/backend/App';
import { shouldNotifyUpdate } from './lib/utils/updateSkip';
import { settingsStore, serverStore } from './lib/store';
import { splitTunnelApi } from './lib/splitTunnelApi';
import { syncProxyModeOnStartup, enterProxyMode, leaveProxyMode } from './lib/utils/proxyModeGuard';

function useWdttPaste() {
  useEffect(() => {
    const handler = (e: ClipboardEvent) => {
      const text = e.clipboardData?.getData('text') ?? '';
      const trimmed = text.trim();
      if (!trimmed.startsWith('qwdtt://') && !trimmed.startsWith('wdtt://')) return;
      const tag = (document.activeElement as HTMLElement)?.tagName;
      if (tag === 'INPUT' || tag === 'TEXTAREA') return;
      e.preventDefault();
      const link = parseWdttUrl(text.trim());
      if (!link) { toastStore.show('Неверный формат ссылки'); return; }
      wdttLinkStore.set(link);
    };
    document.addEventListener('paste', handler);
    document.body.tabIndex = 0;
    return () => document.removeEventListener('paste', handler);
  }, []);
}

function useWailsEvents() {
  useEffect(() => {
    const offs = [
      EventsOn('log', (level: unknown, msg: unknown) => {
        logStore.push((level as LogLevel) ?? 'INFO', String(msg ?? ''));
      }),
      EventsOn('error', (msg: unknown) => {
        const s = String(msg ?? '');
        logStore.push('ERROR', s);
        toastStore.show(s, 5000);
      }),
      EventsOn('state_changed', (status: unknown) => {
        const s = String(status ?? '');
        if (s === 'connected' || s === 'running') { tunnelStore.set('connected'); logStore.push('INFO', '✓ Туннель активен'); }
        else if (s === 'connecting') { tunnelStore.set('connecting'); logStore.clear(); logStore.push('INFO', '⟳ Подключение...'); }
        else if (s === 'stopped' || s === 'error' || s === 'disconnected') { tunnelStore.set('idle'); logStore.push('INFO', '— Отключено'); }
      }),
      EventsOn('event', (name: unknown) => {
        if (name === 'wg_config') tunnelStore.set('connected');
      }),
    ];
    return () => offs.forEach(off => off());
  }, []);
}

// Интервал повторной проверки: 5 минут
const RETRY_MS = 5 * 60 * 1000;

function useUpdateChecker() {
  const [updateInfo, setUpdateInfo] = useState<{ version: string; body: string; url: string } | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    const check = () => {
      CheckUpdate().then(info => {
        if (info.available && shouldNotifyUpdate(info.version)) {
          setUpdateInfo({ version: info.version, body: info.body, url: info.url });
        }
      }).catch(() => {
        timerRef.current = setTimeout(check, RETRY_MS);
      });
    };
    check();
    return () => { if (timerRef.current) clearTimeout(timerRef.current); };
  }, []);

  return { updateInfo, closeUpdate: () => setUpdateInfo(null) };
}

let startupAutoConnectDone = false;

async function connectSelectedServer(): Promise<void> {
  if (tunnelStore.get() !== 'idle') return;
  const all = serverStore.getAll();
  const lastId = serverStore.getLastSelectedId();
  const cur = all.find(s => s.id === lastId) ?? all[0] ?? null;
  if (!cur) {
    toastStore.show('Нет добавленных серверов', 3000);
    return;
  }
  const hashes = (cur.hashes ?? []).filter(h => h.trim());
  if (hashes.length === 0) {
    toastStore.show('Добавьте хеши в профиле сервера', 3000);
    return;
  }
  tunnelStore.set('connecting');
  logStore.clear();
  try {
    const s = settingsStore.get();
    const workers = cur.power || Math.max(9, hashes.length * 9);
    await WailsConnect({
      peerAddr: cur.host,
      password: cur.password,
      hashes,
      deviceId: cur.deviceId,
      workers,
      captchaMode: 'auto',
      obfsMode: s.obfsMode || 'audio',
      turnTcp: s.turnTcp || false,
      proxyMode: !!s.proxyMode,
      proxyPort: s.proxyPort || 10808,
    } as never);
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : String(e);
    logStore.push('ERROR', msg);
    toastStore.show(msg, 5000);
    tunnelStore.set('idle');
  }
}

async function disconnectTunnelSafe(): Promise<void> {
  tunnelStore.set('disconnecting');
  try {
    await WailsDisconnect();
  } catch {
    // ignore disconnect errors
  } finally {
    tunnelStore.set('idle');
  }
}

function TrayBridge() {
  const navigate = useNavigate();
  const busyRef = useRef(false);

  useEffect(() => {
    const pushTrayState = async () => {
      try {
        const fn = (window as unknown as {
          go?: { backend?: { App?: { UpdateTrayState?: (st: Record<string, unknown>) => Promise<void> } } };
        })?.go?.backend?.App?.UpdateTrayState;
        if (!fn) return;
        const s = settingsStore.get();
        let splitSupported = false;
        let splitEnabled = false;
        try {
          splitSupported = await splitTunnelApi.isSupported();
          if (splitSupported) {
            const cfg = await splitTunnelApi.getConfig();
            splitEnabled = !!cfg.enabled && cfg.mode !== 'global';
          }
        } catch {
          // ignore
        }
        await fn({
          tunnelState: tunnelStore.get(),
          proxyMode: !!s.proxyMode,
          proxyPort: s.proxyPort || 10808,
          splitEnabled,
          splitSupported,
        });
      } catch {
        // ignore outside Wails
      }
    };

    void pushTrayState();
    const unsubTunnel = tunnelStore.subscribe(() => { void pushTrayState(); });
    const unsubSettings = settingsStore.subscribe(() => { void pushTrayState(); });
    const onSplitChanged = () => { void pushTrayState(); };
    window.addEventListener('mwpwdtt:split-changed', onSplitChanged);

    // Startup auto-connect (works even if started minimized to tray)
    if (!startupAutoConnectDone) {
      startupAutoConnectDone = true;
      const s = settingsStore.get();
      if (s.autoConnect) {
        setTimeout(() => {
          if (tunnelStore.get() !== 'idle') return;
          void syncProxyModeOnStartup(!!s.proxyMode, splitTunnelApi, s.proxyPort || 10808)
            .catch(() => {})
            .finally(() => {
              if (tunnelStore.get() === 'idle') {
                void connectSelectedServer();
              }
            });
        }, 600);
      }
    }

    const offNav = EventsOn('tray:navigate', (path: string) => {
      window.dispatchEvent(new Event('mwpwdtt:close-modals'));
      if (path) navigate(path);
    });

    const offConn = EventsOn('tray:toggle_connect', () => {
      if (busyRef.current) return;
      busyRef.current = true;
      const cur = tunnelStore.get();
      const job = (cur === 'connected' || cur === 'connecting')
        ? disconnectTunnelSafe()
        : cur === 'idle'
          ? connectSelectedServer()
          : Promise.resolve();
      void job.finally(() => {
        busyRef.current = false;
        void pushTrayState();
      });
    });

    const offProxy = EventsOn('tray:toggle_proxy', () => {
      if (busyRef.current) return;
      busyRef.current = true;
      void (async () => {
        const s = settingsStore.get();
        const next = !s.proxyMode;
        const port = s.proxyPort || 10808;
        const wasActive = tunnelStore.get() === 'connected' || tunnelStore.get() === 'connecting';
        settingsStore.save({ ...s, proxyMode: next, proxyPort: port });
        if (next) {
          await enterProxyMode(splitTunnelApi, port);
        } else {
          await leaveProxyMode(splitTunnelApi);
        }
        window.dispatchEvent(new Event('mwpwdtt:split-changed'));
        if (wasActive) {
          await disconnectTunnelSafe();
          await new Promise(r => setTimeout(r, 400));
          await connectSelectedServer();
        }
      })().finally(() => {
        busyRef.current = false;
        void pushTrayState();
      });
    });

    const offSplit = EventsOn('tray:toggle_split', () => {
      if (busyRef.current) return;
      const s = settingsStore.get();
      if (s.proxyMode) return;
      busyRef.current = true;
      void (async () => {
        try {
          if (!(await splitTunnelApi.isSupported())) return;
          const cfg = await splitTunnelApi.getConfig();
          const next = !(cfg.enabled && cfg.mode !== 'global');
          const nextMode = next && cfg.mode === 'global' ? 'include' : cfg.mode;
          await splitTunnelApi.saveConfig({ ...cfg, enabled: next, mode: nextMode });
          window.dispatchEvent(new Event('mwpwdtt:split-changed'));
        } catch {
          // ignore
        }
      })().finally(() => {
        busyRef.current = false;
        void pushTrayState();
      });
    });

    return () => {
      unsubTunnel();
      unsubSettings();
      window.removeEventListener('mwpwdtt:split-changed', onSplitChanged);
      offNav();
      offConn();
      offProxy();
      offSplit();
    };
  }, [navigate]);

  return null;
}

export default function App() {
  useWailsEvents();
  useEffect(() => {
    void syncProxyModeOnStartup(!!settingsStore.get().proxyMode, splitTunnelApi, settingsStore.get().proxyPort || 10808);
  }, []);
  useWdttPaste();
  const { updateInfo, closeUpdate } = useUpdateChecker();

  return (
    <BrowserRouter>
      <TrayBridge />
      <Routes>
        <Route element={<Layout />}>
          <Route path="/" element={<Connect />} />
          <Route path="/split-tunnel" element={<SplitTunnel />} />
          <Route path="/logs" element={<Logs />} />
          <Route path="/mw-settings" element={<ForkSettings />} />
        </Route>
      </Routes>
      <Toast />
      {updateInfo && (
        <UpdateModal
          version={updateInfo.version}
          body={updateInfo.body}
          url={updateInfo.url}
          onClose={closeUpdate}
        />
      )}
    </BrowserRouter>
  );
}
