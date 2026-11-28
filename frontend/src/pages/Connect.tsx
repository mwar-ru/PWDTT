import { useState, useEffect, useRef, useCallback } from 'react';
import type React from 'react';
import {
  IconCloverFilled, IconPlus, IconChevronUp, IconPencil,
  IconFlameFilled, IconShieldFilled, IconLayoutGridFilled, IconCloudFilled, IconBrandSpeedtest,
  IconStarFilled, IconHeartFilled, IconBoltFilled, IconRocket,
  IconCrownFilled, IconDiamondFilled, IconLeafFilled, IconSnowflake,
  IconServer, IconGlobe, IconLockFilled, IconWifi,
} from '@tabler/icons-react';
import AddServer from '../modals/Add-server';
import EditServer from '../modals/Edit-server';
import { serverStore, settingsStore } from '../lib/store';
import { tunnelStore } from '../lib/stores/tunnelStore';
import { themeStore } from '../lib/stores/themeStore';
import { toastStore } from '../lib/stores/toastStore';
import { logStore } from '../lib/stores/logStore';
import { wdttLinkStore } from '../lib/utils/wdttLink';
import { SaveProfile } from '../../wailsjs/go/backend/App';
import type { Server, TunnelState } from '../lib/types';
import { Connect as WailsConnect, Disconnect as WailsDisconnect, ListProfiles } from '../../wailsjs/go/backend/App';
import shapeLight from '../assets/shape-light.png';
import shapeDark from '../assets/shape-dark.png';
import powerIcon from '../assets/power-icon.png';
import { splitTunnelApi } from '../lib/splitTunnelApi';
import { enterProxyMode, leaveProxyMode, onProxyReconnect } from '../lib/utils/proxyModeGuard';

import './Connect.css';

const SERVER_ICONS: { key: string; render: (size: number) => React.ReactNode }[] = [
  { key: 'clover',     render: s => <IconCloverFilled size={s} /> },
  { key: 'flame',      render: s => <IconFlameFilled size={s} /> },
  { key: 'shield',     render: s => <IconShieldFilled size={s} /> },
  { key: 'grid',       render: s => <IconLayoutGridFilled size={s} /> },
  { key: 'cloud',      render: s => <IconCloudFilled size={s} /> },
  { key: 'speed',      render: s => <IconBrandSpeedtest size={s} stroke={2} /> },
  { key: 'star',       render: s => <IconStarFilled size={s} /> },
  { key: 'heart',      render: s => <IconHeartFilled size={s} /> },
  { key: 'bolt',       render: s => <IconBoltFilled size={s} /> },
  { key: 'rocket',     render: s => <IconRocket size={s} stroke={2} /> },
  { key: 'crown',      render: s => <IconCrownFilled size={s} /> },
  { key: 'diamond',    render: s => <IconDiamondFilled size={s} /> },
  { key: 'leaf',       render: s => <IconLeafFilled size={s} /> },
  { key: 'snowflake',  render: s => <IconSnowflake size={s} stroke={2} /> },
  { key: 'server',     render: s => <IconServer size={s} stroke={2} /> },
  { key: 'globe',      render: s => <IconGlobe size={s} stroke={2} /> },
  { key: 'lock',       render: s => <IconLockFilled size={s} /> },
  { key: 'wifi',       render: s => <IconWifi size={s} stroke={2} /> },
  { key: 'flag-ru',    render: s => <img src="/flags/ru.svg" alt="" width={s} height={s * 0.67} style={{ objectFit: 'cover', borderRadius: 2 }} /> },
  { key: 'flag-us',    render: s => <img src="/flags/us.svg" alt="" width={s} height={s * 0.67} style={{ objectFit: 'cover', borderRadius: 2 }} /> },
  { key: 'flag-de',    render: s => <img src="/flags/de.svg" alt="" width={s} height={s * 0.67} style={{ objectFit: 'cover', borderRadius: 2 }} /> },
  { key: 'flag-nl',    render: s => <img src="/flags/nl.svg" alt="" width={s} height={s * 0.67} style={{ objectFit: 'cover', borderRadius: 2 }} /> },
  { key: 'flag-fi',    render: s => <img src="/flags/fi.svg" alt="" width={s} height={s * 0.67} style={{ objectFit: 'cover', borderRadius: 2 }} /> },
  { key: 'flag-fr',    render: s => <img src="/flags/fr.svg" alt="" width={s} height={s * 0.67} style={{ objectFit: 'cover', borderRadius: 2 }} /> },
  { key: 'flag-gb',    render: s => <img src="/flags/gb.svg" alt="" width={s} height={s * 0.67} style={{ objectFit: 'cover', borderRadius: 2 }} /> },
  { key: 'flag-jp',    render: s => <img src="/flags/jp.svg" alt="" width={s} height={s * 0.67} style={{ objectFit: 'cover', borderRadius: 2 }} /> },
  { key: 'flag-pl',    render: s => <img src="/flags/pl.svg" alt="" width={s} height={s * 0.67} style={{ objectFit: 'cover', borderRadius: 2 }} /> },
  { key: 'flag-se',    render: s => <img src="/flags/se.svg" alt="" width={s} height={s * 0.67} style={{ objectFit: 'cover', borderRadius: 2 }} /> },
  { key: 'flag-ch',    render: s => <img src="/flags/ch.svg" alt="" width={s} height={s * 0.67} style={{ objectFit: 'cover', borderRadius: 2 }} /> },
  { key: 'flag-lt',    render: s => <img src="/flags/lt.svg" alt="" width={s} height={s * 0.67} style={{ objectFit: 'cover', borderRadius: 2 }} /> },
  { key: 'flag-lv',    render: s => <img src="/flags/lv.svg" alt="" width={s} height={s * 0.67} style={{ objectFit: 'cover', borderRadius: 2 }} /> },
  { key: 'flag-ee',    render: s => <img src="/flags/ee.svg" alt="" width={s} height={s * 0.67} style={{ objectFit: 'cover', borderRadius: 2 }} /> },
  { key: 'flag-cz',    render: s => <img src="/flags/cz.svg" alt="" width={s} height={s * 0.67} style={{ objectFit: 'cover', borderRadius: 2 }} /> },
  { key: 'flag-at',    render: s => <img src="/flags/at.svg" alt="" width={s} height={s * 0.67} style={{ objectFit: 'cover', borderRadius: 2 }} /> },
  { key: 'flag-ca',    render: s => <img src="/flags/ca.svg" alt="" width={s} height={s * 0.67} style={{ objectFit: 'cover', borderRadius: 2 }} /> },
  { key: 'flag-au',    render: s => <img src="/flags/au.svg" alt="" width={s} height={s * 0.67} style={{ objectFit: 'cover', borderRadius: 2 }} /> },
  { key: 'flag-sg',    render: s => <img src="/flags/sg.svg" alt="" width={s} height={s * 0.67} style={{ objectFit: 'cover', borderRadius: 2 }} /> },
  { key: 'flag-hk',    render: s => <img src="/flags/hk.svg" alt="" width={s} height={s * 0.67} style={{ objectFit: 'cover', borderRadius: 2 }} /> },
  { key: 'flag-tr',    render: s => <img src="/flags/tr.svg" alt="" width={s} height={s * 0.67} style={{ objectFit: 'cover', borderRadius: 2 }} /> },
  { key: 'flag-kz',    render: s => <img src="/flags/kz.svg" alt="" width={s} height={s * 0.67} style={{ objectFit: 'cover', borderRadius: 2 }} /> },
];

function ServerIcon({ iconKey, size }: { iconKey?: string; size: number }) {
  const entry = SERVER_ICONS.find(i => i.key === (iconKey ?? 'clover')) ?? SERVER_ICONS[0];
  return <>{entry.render(size)}</>;
}

const PING_COLORS: Record<string, string> = {
  good: '#22c55e',
  mid: '#f59e0b',
  bad: '#ef4444',
  none: 'var(--border)',
};

function pingColor(ping?: number) {
  if (!ping) return PING_COLORS.none;
  if (ping < 100) return PING_COLORS.good;
  if (ping < 200) return PING_COLORS.mid;
  return PING_COLORS.bad;
}

const TUNNEL_LABEL: Record<TunnelState, string> = {
  idle: 'Подключить',
  connecting: 'Подключение...',
  connected: 'Отключить',
  disconnecting: 'Отключение...',
};

function PowerButton({ theme, tunnelState, isActive, isSpinning, linkFlash, selected, proxyMode, splitActive, onTunnel }: {
  theme: string; tunnelState: TunnelState; isActive: boolean; isSpinning: boolean;
  linkFlash: boolean; selected: Server | null; proxyMode: boolean; splitActive: boolean; onTunnel: () => void;
}) {
  const modeClass = proxyMode
    ? (isActive ? ' orb--proxy-active' : ' orb--proxy')
    : (splitActive && isActive ? ' orb--split-active' : '');
  return (
    <button
      type="button"
      className={`power-btn${proxyMode ? ' power-btn--proxy' : ''}`}
      onClick={onTunnel}
      disabled={!selected}
      title={selected ? TUNNEL_LABEL[tunnelState] : 'Добавьте сервер'}
      aria-label={selected ? TUNNEL_LABEL[tunnelState] : 'Добавьте сервер'}
    >
      <div className={`orb${isSpinning ? ' orb--spinning' : isActive ? ' orb--active' : ''}${linkFlash ? ' orb--flash' : ''}${modeClass}`}>
        <img src={theme === 'dark' ? shapeLight : shapeDark} alt="" draggable={false} />
      </div>
      <div className="power-icon">
        <img src={powerIcon} alt="" draggable={false} style={{ width: 28, height: 35 }} />
      </div>
    </button>
  );
}

function ServerSelector({ servers, selected, listOpen, onToggleList, onSelect, onIconClick, onEdit }: {
  servers: Server[]; selected: Server | null; listOpen: boolean;
  onToggleList: () => void; onSelect: (s: Server) => void;
  onIconClick: (e: React.MouseEvent, s: Server) => void; onEdit: (s: Server) => void;
}) {
  return (
    <div className="status-bar">
      {listOpen && servers.length > 0 && (
        <div className="server-list">
          {servers.map(s => (
            <div
              key={s.id}
              className={`server-item${s.id === selected?.id ? ' server-item--active' : ''}`}
              style={{ cursor: 'pointer' }}
              role="button"
              tabIndex={0}
              onClick={() => onSelect(s)}
              onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') onSelect(s); }}
            >
              <button type="button" className="server-icon-btn" onClick={(e) => { e.stopPropagation(); onIconClick(e, s); }} aria-label="Выбрать иконку">
                <ServerIcon iconKey={s.icon} size={20} />
              </button>
              <span className="status-name">
                {s.name}
              </span>
              {s.ping != null && (
                <span className="status-ping">
                  <span className="ping-dot" style={{ background: pingColor(s.ping) }} />
                  {s.ping}
                </span>
              )}
              <button type="button" className="server-edit-btn" onClick={(e) => { e.stopPropagation(); onEdit(s); }} aria-label="Редактировать">
                <IconPencil size={15} stroke={2} />
              </button>
            </div>
          ))}
        </div>
      )}

      <button type="button" className={`status-server${!selected ? ' status-server--empty' : ''}`} onClick={onToggleList}>
        <ServerIcon iconKey={selected?.icon} size={20} />
        <span className="status-name">{selected ? selected.name : 'Нет серверов'}</span>
        {selected?.ping != null && (
          <span className="status-ping">
            <span className="ping-dot" style={{ background: pingColor(selected.ping) }} />
            {selected.ping}
          </span>
        )}
        <IconChevronUp
          size={16}
          style={{ transform: listOpen ? 'rotate(0deg)' : 'rotate(180deg)', transition: 'transform 0.2s' }}
        />
      </button>
    </div>
  );
}

function IconPicker({ iconMenu, onPick, onClose }: {
  iconMenu: { server: Server; x: number; y: number };
  onPick: (key: string) => void; onClose: () => void;
}) {
  return (
    <>
      <button type="button" aria-label="Закрыть" className="icon-picker-backdrop" onClick={onClose} />
      <div
        className="icon-picker"
        style={{
          left: Math.min(iconMenu.x, window.innerWidth - 256),
          top: iconMenu.y - 4 - (Math.ceil(SERVER_ICONS.length / 6) * 40 + 20),
        }}
      >
        {SERVER_ICONS.map(ic => (
          <button
            type="button"
            key={ic.key}
            className={`icon-picker-btn${(iconMenu.server.icon ?? 'clover') === ic.key ? ' icon-picker-btn--active' : ''}`}
            onClick={() => onPick(ic.key)}
            title={ic.key}
          >
            {ic.render(18)}
          </button>
        ))}
      </div>
    </>
  );
}

export default function Connect() {
  const [servers, setServers] = useState<Server[]>(() => serverStore.getAll());
  const [selected, setSelected] = useState<Server | null>(() => {
    const all = serverStore.getAll();
    if (all.length === 0) return null;
    const lastId = serverStore.getLastSelectedId();
    return all.find(s => s.id === lastId) ?? all[0];
  });
  const [listOpen, setListOpen] = useState(false);

  useEffect(() => {
    ListProfiles().then(profiles => {
      if (!profiles) return;
      const existing = serverStore.getAll();
      let changed = false;
      for (const [name, p] of Object.entries(profiles)) {
        const host = p.peer || '';
        if (!host) continue;
        const stored = existing.find(s => s.name === name || s.host === host);
        if (stored) {
          if (p.workers && stored.power !== p.workers) {
            serverStore.update({ ...stored, power: p.workers });
            changed = true;
          }
          continue;
        }
        const h4: [string,string,string,string] = [p.hashes?.[0]??'', p.hashes?.[1]??'', p.hashes?.[2]??'', p.hashes?.[3]??''];
        serverStore.add({ name, host, password: p.password ?? '', hashes: h4, power: p.workers });
        changed = true;
      }
      if (changed) {
        setServers(serverStore.getAll());
        const all = serverStore.getAll();
        if (all.length > 0) setSelected(prev => prev ?? all[0]);
      }

    }).catch(() => {});
  }, []);

  // tunnelState из глобального store — переживает смену роута
  const [tunnelState, setTunnelState] = useState<TunnelState>(() => tunnelStore.get());
  useEffect(() => tunnelStore.subscribe(setTunnelState), []);
  const [theme, setTheme] = useState(() => themeStore.get());
  useEffect(() => themeStore.subscribe(setTheme), []);

  const selectedRef = useRef(selected);
  const tunnelStateRef = useRef(tunnelState);

  useEffect(() => {
    selectedRef.current = selected;
    tunnelStateRef.current = tunnelState;
  });

  useEffect(() => {
    serverStore.setLastSelectedId(selected?.id ?? null);
  }, [selected?.id]);

  const [addServerOpen, setAddServerOpen] = useState(false);
  const [editServer, setEditServer] = useState<Server | null>(null);

  const [linkFlash, setLinkFlash] = useState(false);
  const linkFlashTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => { if (linkFlashTimerRef.current) clearTimeout(linkFlashTimerRef.current); };
  }, []);

  useEffect(() => {
    return wdttLinkStore.subscribe((link) => {
      if (!link) return;
      const consumed = wdttLinkStore.consume();
      if (!consumed) return;

      const applyLink = async () => {
        const h4 = consumed.hashes.slice(0, 4);
        const padded: [string,string,string,string] = [h4[0]??'', h4[1]??'', h4[2]??'', h4[3]??''];

        // Генерируем уникальное имя: "Сервер 1", "Сервер 2", ...
        const existingNames = serverStore.getAll().map(s => s.name);
        let autoName = consumed.name || 'Сервер';
        if (autoName === 'Server') autoName = 'Сервер';
        let counter = 1;
        while (existingNames.includes(`${autoName} ${counter}`)) counter++;
        const name = `${autoName} ${counter}`;

        await SaveProfile(name, {
          peer: consumed.host,
          password: consumed.password,
          hashes: h4 as unknown as string[],
          workers: consumed.workers,
          turn: '', port: consumed.port || '', device_id: '', listen: '', turn_tcp: false,
        });
        const s = serverStore.add({
          name,
          host: consumed.host,
          password: consumed.password,
          hashes: padded,
          power: consumed.workers,
        });
        setServers(serverStore.getAll());
        setSelected({ ...s });
        setLinkFlash(true);
        if (linkFlashTimerRef.current) clearTimeout(linkFlashTimerRef.current);
        linkFlashTimerRef.current = setTimeout(() => setLinkFlash(false), 800);
        toastStore.show(`Профиль добавлен: ${name}`, 3000);
      };
      applyLink().catch(e => {
        console.warn('applyLink failed:', e);
        toastStore.show('Не удалось добавить сервер по ссылке', 3000);
      });
    });
  }, []);

  const connectingRef = useRef(false);

  const doConnect = async () => {
    const cur = selectedRef.current;
    if (!cur) return;
    if (tunnelState !== 'idle') return;
    if (connectingRef.current) return; // guard от двойного клика
    connectingRef.current = true;
    const hashes = (cur.hashes ?? []).filter(h => h.trim());
    if (hashes.length === 0) {
      toastStore.show('Добавьте хеши в профиле сервера');
      connectingRef.current = false;
      return;
    }
    tunnelStore.set('connecting');
    logStore.clear();
    try {
      const workers = cur.power || Math.max(9, hashes.length * 9);
      await WailsConnect({
        peerAddr: cur.host,
        password: cur.password,
        hashes,
        deviceId: cur.deviceId,
        workers,
        captchaMode: 'auto',
        obfsMode: settingsStore.get().obfsMode || 'audio',
        turnTcp: settingsStore.get().turnTcp || false,
        proxyMode: !!settingsStore.get().proxyMode,
        proxyPort: settingsStore.get().proxyPort || 10808,
      } as never);
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      logStore.push('ERROR', msg);
      toastStore.show(msg, 6000);
      tunnelStore.set('idle');
    } finally {
      connectingRef.current = false;
    }
  };

  const reconnectAtRef = useRef(0);

  const handleTunnel = async () => {
    if (!selectedRef.current) return;
    if (tunnelState === 'idle') {
      if (Date.now() < reconnectAtRef.current) {
        const secs = Math.ceil((reconnectAtRef.current - Date.now()) / 1000);
        toastStore.show(`Подождите ${secs} сек.`, 2000);
        return;
      }
      toastStore.show('Убедитесь что другие VPN отключены', 4000);
      await doConnect();
    } else if (tunnelState === 'connected' || tunnelState === 'connecting') {
      tunnelStore.set('disconnecting');
      try {
        await WailsDisconnect();
      } catch {
        // игнорируем ошибку disconnect
      } finally {
        tunnelStore.set('idle');
        reconnectAtRef.current = Date.now() + 4000;
      }
    }
  };

  const handleAdd = (data: Omit<Server, 'id'>) => {
    const s = serverStore.add(data);
    setServers(serverStore.getAll());
    setSelected(s);
  };

  const handleSave = (server: Server) => {
    serverStore.update(server);
    const all = serverStore.getAll();
    setServers(all);
    if (selected?.id === server.id) setSelected(server);
  };

  const handleDelete = (id: string) => {
    serverStore.remove(id);
    const all = serverStore.getAll();
    setServers(all);
    if (selected?.id === id) setSelected(all[0] ?? null);
  };

  const [iconMenu, setIconMenu] = useState<{ server: Server; x: number; y: number } | null>(null);

  const handleIconClick = (e: React.MouseEvent, server: Server) => {
    e.stopPropagation();
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    setIconMenu({ server, x: rect.left, y: rect.top });
  };

  const handlePickIcon = (key: string) => {
    if (!iconMenu) return;
    const updated = { ...iconMenu.server, icon: key };
    serverStore.update(updated);
    const all = serverStore.getAll();
    setServers(all);
    if (selected?.id === iconMenu.server.id) setSelected(updated);
    setIconMenu(null);
  };

  const [proxyMode, setProxyMode] = useState<boolean>(() => !!settingsStore.get().proxyMode);
  const [proxyPort, setProxyPort] = useState<number>(() => settingsStore.get().proxyPort || 10808);
  const [splitSupported, setSplitSupported] = useState<boolean>(false);
  const [splitEnabled, setSplitEnabled] = useState<boolean>(false);
  const [modeBusy, setModeBusy] = useState<boolean>(false);
  const pendingReconnectRef = useRef<boolean>(false);
  const doConnectRef = useRef<() => Promise<void>>(doConnect);
  doConnectRef.current = doConnect;

  const refreshSplitState = useCallback(async () => {
    try {
      const supported = await splitTunnelApi.isSupported();
      setSplitSupported(supported);
      if (supported) {
        const cfg = await splitTunnelApi.getConfig();
        setSplitEnabled(!!cfg.enabled && cfg.mode !== 'global');
      }
    } catch {
      setSplitSupported(false);
    }
  }, []);

  useEffect(() => {
    void refreshSplitState();
    const unsubSettings = settingsStore.subscribe(s => {
      setProxyMode(!!s.proxyMode);
      setProxyPort(s.proxyPort || 10808);
      void refreshSplitState();
    });
    const unsubReconnect = onProxyReconnect(() => {
      const curState = tunnelStore.get();
      if (curState === 'connected' || curState === 'connecting') {
        pendingReconnectRef.current = true;
        tunnelStore.set('disconnecting');
        void WailsDisconnect().catch(() => {}).finally(() => {
          tunnelStore.set('idle');
        });
      }
    });
    const onSplitChanged = () => { void refreshSplitState(); };
    window.addEventListener('mwpwdtt:split-changed', onSplitChanged);
    return () => {
      unsubSettings();
      unsubReconnect();
      window.removeEventListener('mwpwdtt:split-changed', onSplitChanged);
    };
  }, [refreshSplitState]);

  useEffect(() => {
    if (tunnelState !== 'idle' || !pendingReconnectRef.current) return;
    pendingReconnectRef.current = false;
    const timer = setTimeout(() => {
      if (tunnelStore.get() === 'idle' && selectedRef.current) {
        void doConnectRef.current();
      }
    }, 400);
    return () => clearTimeout(timer);
  }, [tunnelState]);

  const handleToggleProxyMode = async (next: boolean) => {
    if (modeBusy) return;
    setModeBusy(true);
    try {
      const curSettings = settingsStore.get();
      const port = curSettings.proxyPort || 10808;
      settingsStore.save({ ...curSettings, proxyMode: next, proxyPort: port });
      setProxyMode(next);
      setProxyPort(port);

      const wasActive = tunnelStore.get() === 'connected' || tunnelStore.get() === 'connecting';

      if (next) {
        const guarded = await enterProxyMode(splitTunnelApi, port);
        await refreshSplitState();
        window.dispatchEvent(new Event('mwpwdtt:split-changed'));
        if (wasActive) {
          pendingReconnectRef.current = true;
          tunnelStore.set('disconnecting');
          toastStore.show(`Режим SOCKS5 (127.0.0.1:${port}) — переподключение...`, 3500);
          try {
            await WailsDisconnect();
          } finally {
            tunnelStore.set('idle');
          }
        } else {
          toastStore.show(
            guarded
              ? `Режим SOCKS5 (127.0.0.1:${port}) — Split Tunneling скрыто приостановлен`
              : `Режим SOCKS5 (127.0.0.1:${port}) включён`,
            3000,
          );
        }
      } else {
        const restored = await leaveProxyMode(splitTunnelApi);
        await refreshSplitState();
        window.dispatchEvent(new Event('mwpwdtt:split-changed'));
        if (wasActive) {
          pendingReconnectRef.current = true;
          tunnelStore.set('disconnecting');
          toastStore.show(
            restored
              ? 'Обычный режим TUN (Split Tunneling восстановлен) — переподключение...'
              : 'Обычный режим TUN — переподключение...',
            3500,
          );
          try {
            await WailsDisconnect();
          } finally {
            tunnelStore.set('idle');
          }
        } else {
          toastStore.show(
            restored
              ? 'Обычный режим TUN — прежние настройки Split Tunneling восстановлены'
              : 'Обычный режим TUN',
            3000,
          );
        }
      }
    } finally {
      setModeBusy(false);
    }
  };

  const handleToggleSplitTunnel = async (next: boolean) => {
    if (modeBusy || proxyMode || !splitSupported) return;
    setModeBusy(true);
    try {
      const cfg = await splitTunnelApi.getConfig();
      const nextMode = next && cfg.mode === 'global' ? 'include' : cfg.mode;
      const saved = await splitTunnelApi.saveConfig({ ...cfg, enabled: next, mode: nextMode });
      setSplitEnabled(!!saved.enabled && saved.mode !== 'global');
      window.dispatchEvent(new Event('mwpwdtt:split-changed'));
      toastStore.show(
        next ? 'Split Tunneling включён' : 'Split Tunneling выключен',
        2500,
      );
    } catch (e) {
      toastStore.show(`Ошибка Split Tunneling: ${String(e)}`, 4000);
    } finally {
      setModeBusy(false);
    }
  };

  const isActive = tunnelState === 'connected';
  const isSpinning = tunnelState === 'connecting';
  const splitActive = splitSupported && splitEnabled && !proxyMode;

  return (
    <>
      <main className={`main${proxyMode ? ' main--proxy' : ''}`}>
        <div className="quick-modes">
          <label className={`mode-check${proxyMode ? ' mode-check--proxy' : ''}`}>
            <input
              type="checkbox"
              checked={proxyMode}
              disabled={modeBusy}
              onChange={e => void handleToggleProxyMode(e.target.checked)}
            />
            <span>Прокси mihomo</span>
            {proxyMode && <span className="mode-check-port">:{proxyPort}</span>}
          </label>

          {splitSupported && (
            <label
              className={`mode-check${splitActive ? ' mode-check--split' : ''}${proxyMode ? ' mode-check--paused' : ''}`}
              title={proxyMode ? 'Приостановлено на время работы режима прокси для mihomo' : 'Маршрутизация отдельных приложений (Split Tunneling)'}
            >
              <input
                type="checkbox"
                checked={splitActive}
                disabled={modeBusy || proxyMode}
                onChange={e => void handleToggleSplitTunnel(e.target.checked)}
              />
              <span>{proxyMode ? 'Split Tunnel (пауза)' : 'Split Tunnel'}</span>
            </label>
          )}
        </div>

        <button type="button" className="btn-add" onClick={() => setAddServerOpen(true)} aria-label="Добавить сервер">
          <IconPlus stroke={2} size={22} />
        </button>

        <PowerButton
          theme={theme}
          tunnelState={tunnelState}
          isActive={isActive}
          isSpinning={isSpinning}
          linkFlash={linkFlash}
          selected={selected}
          proxyMode={proxyMode}
          splitActive={splitActive}
          onTunnel={handleTunnel}
        />

        <div className="tunnel-status-wrap">
          <span className={`tunnel-label-text${proxyMode && isActive ? ' tunnel-label-text--proxy' : splitActive && isActive ? ' tunnel-label-text--split' : ''}`}>
            {selected ? TUNNEL_LABEL[tunnelState] : 'Нет серверов'}
          </span>
          {proxyMode && (
            <span className={`mode-indicator mode-indicator--proxy${isActive ? ' mode-indicator--live' : ''}`}>
              <span className="mode-dot" />
              SOCKS5 · 127.0.0.1:{proxyPort} · {isActive ? 'VPN' : 'DIRECT'}
            </span>
          )}
          {!proxyMode && splitActive && (
            <span className={`mode-indicator mode-indicator--split${isActive ? ' mode-indicator--live' : ''}`}>
              <span className="mode-dot" />
              Split Tunneling
            </span>
          )}
        </div>

        <ServerSelector
          servers={servers}
          selected={selected}
          listOpen={listOpen}
          onToggleList={() => setListOpen(o => !o)}
          onSelect={(s) => { setSelected({ ...s }); setListOpen(false); }}
          onIconClick={handleIconClick}
          onEdit={(s) => setEditServer(s)}
        />

        {addServerOpen && <AddServer onClose={() => setAddServerOpen(false)} onAdd={handleAdd} />}
        {editServer && (
          <EditServer
            server={editServer}
            onClose={() => setEditServer(null)}
            onSave={handleSave}
            onDelete={handleDelete}
          />
        )}

        {iconMenu && (
          <IconPicker
            iconMenu={iconMenu}
            onPick={handlePickIcon}
            onClose={() => setIconMenu(null)}
          />
        )}
      </main>

    </>
  );
}
