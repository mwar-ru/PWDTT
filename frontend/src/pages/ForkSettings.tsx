import { useState, useEffect, useCallback } from 'react';
import {
  IconSettings2,
  IconRocket,
  IconNetwork,
  IconRefresh,
  IconCopy,
  IconCheck,
  IconFileText,
} from '@tabler/icons-react';
import appIcon from '../assets/app-icon.png';
import { settingsStore } from '../lib/store';
import type { AppSettings } from '../lib/types';
import { GetVersion } from '../../wailsjs/go/backend/App';
import { toastStore } from '../lib/stores/toastStore';
import { tunnelStore } from '../lib/stores/tunnelStore';
import { splitTunnelApi } from '../lib/splitTunnelApi';
import {
  enterProxyMode,
  leaveProxyMode,
  requestProxyReconnect,
  syncProxyStandbyBackend,
} from '../lib/utils/proxyModeGuard';
import { getUpdatePauseState, setPauseNextUpdate } from '../lib/utils/updateSkip';
import './ForkSettings.css';

type WailsAppExt = {
  GetStartMinimized?: () => Promise<boolean>;
  SetStartMinimized?: (v: boolean) => Promise<void>;
  OpenUpdateLog?: () => Promise<void>;
};

function getAppExt(): WailsAppExt | undefined {
  return (window as unknown as { go?: { backend?: { App?: WailsAppExt } } })?.go?.backend?.App;
}

export default function ForkSettings() {
  const [settings, setSettings] = useState<AppSettings>(() => settingsStore.get());
  const [version, setVersion] = useState('...');
  const [updatePause, setUpdatePause] = useState(() => getUpdatePauseState());
  const [copiedMihomo, setCopiedMihomo] = useState(false);

  const proxyEnabled = !!settings.proxyMode;
  const proxyPort = settings.proxyPort || 10808;
  const isUpdatePaused = updatePause.pausedNext || Boolean(updatePause.skippedVersion);

  const update = useCallback(<K extends keyof AppSettings>(key: K, value: AppSettings[K]) => {
    setSettings(s => {
      const next = { ...s, [key]: value };
      settingsStore.save(next);
      return next;
    });
  }, []);

  useEffect(() => {
    const unsub = settingsStore.subscribe(s => setSettings(s));
    GetVersion().then(setVersion).catch(() => {});
    const ext = getAppExt();
    if (ext?.GetStartMinimized) {
      ext.GetStartMinimized().then(v => {
        if (v !== !!settingsStore.get().startMinimized) {
          update('startMinimized', v);
        }
      }).catch(() => {});
    }
    return unsub;
  }, [update]);

  const handleToggleStartMinimized = async () => {
    const next = !settings.startMinimized;
    update('startMinimized', next);
    try {
      const ext = getAppExt();
      if (ext?.SetStartMinimized) {
        await ext.SetStartMinimized(next);
      }
      toastStore.show(
        next ? 'При следующем запуске окно откроется сразу в трее' : 'Обычный запуск с открытым окном',
        3000,
      );
    } catch (e) {
      toastStore.show(`Не удалось сохранить настройку: ${String(e)}`, 3500);
    }
  };

  const handleToggleProxy = () => {
    const next = !proxyEnabled;
    update('proxyMode', next);
    if (next && !settings.proxyPort) update('proxyPort', 10808);
    const wasActive = tunnelStore.get() === 'connected' || tunnelStore.get() === 'connecting';
    if (next) {
      void enterProxyMode(splitTunnelApi, proxyPort).then(guarded => {
        window.dispatchEvent(new Event('mwpwdtt:split-changed'));
        if (wasActive) {
          requestProxyReconnect();
          toastStore.show(`Режим SOCKS5 (127.0.0.1:${proxyPort}) — переподключение...`, 3500);
        } else {
          toastStore.show(
            guarded
              ? `Режим SOCKS5 (127.0.0.1:${proxyPort}) — Split Tunneling скрыто приостановлен`
              : `Режим SOCKS5 (127.0.0.1:${proxyPort}) включён`,
            3500,
          );
        }
      });
    } else {
      void leaveProxyMode(splitTunnelApi).then(restored => {
        window.dispatchEvent(new Event('mwpwdtt:split-changed'));
        if (wasActive) {
          requestProxyReconnect();
          toastStore.show(
            restored
              ? 'Обычный режим TUN (Split Tunneling восстановлен) — переподключение...'
              : 'Обычный режим TUN — переподключение...',
            3500,
          );
        } else {
          toastStore.show(
            restored
              ? 'Обычный режим TUN — прежние настройки Split Tunneling восстановлены'
              : 'Обычный режим TUN',
            3500,
          );
        }
      });
    }
  };

  const handleCopyMihomo = async () => {
    const yaml = [
      'proxies:',
      '  - name: "PWDTT"',
      '    type: socks5',
      '    server: 127.0.0.1',
      `    port: ${proxyPort}`,
      '    udp: true',
      '',
      'rules:',
      '  - PROCESS-NAME,mwPWDTT_windows.exe,DIRECT',
    ].join('\n');
    try {
      await navigator.clipboard.writeText(yaml);
      setCopiedMihomo(true);
      toastStore.show(`Конфиг mihomo (127.0.0.1:${proxyPort}) скопирован`, 3000);
      setTimeout(() => setCopiedMihomo(false), 2000);
    } catch {
      toastStore.show('Не удалось скопировать в буфер', 3000);
    }
  };

  const handleOpenUpdateLog = async () => {
    try {
      const fn = getAppExt()?.OpenUpdateLog;
      if (!fn) {
        toastStore.show('Доступно только в Windows-сборке', 3000);
        return;
      }
      await fn();
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      toastStore.show(msg || 'Не удалось открыть лог обновления', 3500);
    }
  };

  return (
    <div className="fs-page">
      <div className="fs-header">
        <div className="fs-title-group">
          <div className="fs-app-badge">
            <img src={appIcon} alt="" draggable={false} />
            <span className="fs-app-badge-gear">
              <IconSettings2 size={11} stroke={2.4} />
            </span>
          </div>
          <div>
            <h2 className="fs-title">Настройки mwPWDTT</h2>
            <span className="fs-subtitle">Дополнительные функции Windows-форка: трей, автоподключение, mihomo и обновления</span>
          </div>
        </div>
        <span className="fs-ver-pill">v{version}</span>
      </div>

      {/* Запуск и трей */}
      <div className="fs-card">
        <div className="fs-card-head">
          <IconRocket size={16} />
          <span>Запуск и системный трей</span>
        </div>

        <div className="fs-row">
          <div className="fs-row-text">
            <span className="fs-row-label">Автоподключение при запуске</span>
            <span className="fs-row-desc">
              Сразу подключаться к последнему выбранному серверу с текущим режимом (mihomo SOCKS5 или TUN).
            </span>
          </div>
          <button
            type="button"
            className={`fs-toggle fs-toggle--${settings.autoConnect ? 'on' : 'off'}`}
            aria-label={settings.autoConnect ? 'Отключить автоподключение' : 'Включить автоподключение'}
            onClick={() => {
              const next = !settings.autoConnect;
              update('autoConnect', next);
              toastStore.show(
                next ? 'Автоподключение при запуске включено' : 'Автоподключение при запуске выключено',
                2500,
              );
            }}
          />
        </div>

        <div className="fs-row">
          <div className="fs-row-text">
            <span className="fs-row-label">Запускать свёрнутым в трей</span>
            <span className="fs-row-desc">
              Открывать программу сразу в системном трее без показа главного окна (удобно вместе с автозапуском и автоподключением).
            </span>
          </div>
          <button
            type="button"
            className={`fs-toggle fs-toggle--${settings.startMinimized ? 'on' : 'off'}`}
            aria-label={settings.startMinimized ? 'Отключить запуск в трее' : 'Включить запуск в трее'}
            onClick={() => void handleToggleStartMinimized()}
          />
        </div>
      </div>

      {/* Прокси для mihomo */}
      <div className="fs-card">
        <div className="fs-card-head">
          <IconNetwork size={16} />
          <span>Прокси для mihomo (SOCKS5)</span>
        </div>

        <div className="fs-row">
          <div className="fs-row-text">
            <span className="fs-row-label">
              Режим SOCKS5-прокси
              <span className="fs-badge">TCP + UDP</span>
            </span>
            <span className="fs-row-desc">
              Чистый userspace-туннель без Wintun и системных маршрутов. Split Tunneling и WFP Kill Switch скрыто приостанавливаются и восстанавливаются при выключении. Когда VPN отключён, прокси работает в режиме DIRECT.
            </span>
          </div>
          <button
            type="button"
            className={`fs-toggle fs-toggle--proxy fs-toggle--${proxyEnabled ? 'on' : 'off'}`}
            aria-label={proxyEnabled ? 'Отключить режим SOCKS5-прокси' : 'Включить режим SOCKS5-прокси'}
            onClick={handleToggleProxy}
          />
        </div>

        <div className="fs-proxy-box">
          <div className="fs-proxy-addr">
            <span>socks5://127.0.0.1 :</span>
            <input
              type="number"
              className="fs-port-input"
              min={1024}
              max={65535}
              value={proxyPort}
              onChange={e => {
                const v = parseInt(e.target.value, 10);
                if (!Number.isNaN(v) && v >= 1 && v <= 65535) {
                  update('proxyPort', v);
                  if (proxyEnabled && v >= 1024) {
                    void syncProxyStandbyBackend(true, v);
                  }
                }
              }}
            />
          </div>
          <button type="button" className="fs-btn" onClick={() => void handleCopyMihomo()}>
            {copiedMihomo ? <IconCheck size={14} /> : <IconCopy size={14} />}
            {copiedMihomo ? 'Скопировано!' : 'Копировать YAML для mihomo'}
          </button>
        </div>
      </div>

      {/* Обновления и диагностика */}
      <div className="fs-card">
        <div className="fs-card-head">
          <IconRefresh size={16} />
          <span>Обновления и диагностика</span>
        </div>

        <div className="fs-row">
          <div className="fs-row-text">
            <span className="fs-row-label">
              Приостановить обновления
              {isUpdatePaused && (
                <span className="fs-badge">
                  {updatePause.skippedVersion ? `пропуск v${updatePause.skippedVersion}` : 'след. версия'}
                </span>
              )}
            </span>
            <span className="fs-row-desc">
              Пропустить следующее обновление — уведомление появится снова только при выходе ещё более новой версии.
            </span>
          </div>
          <button
            type="button"
            className={`fs-toggle fs-toggle--${isUpdatePaused ? 'on' : 'off'}`}
            aria-label={isUpdatePaused ? 'Возобновить уведомления об обновлениях' : 'Приостановить обновления'}
            onClick={() => {
              const next = !isUpdatePaused;
              setPauseNextUpdate(next);
              setUpdatePause(getUpdatePauseState());
              toastStore.show(
                next
                  ? 'Следующее обновление будет пропущено (напомним через одно)'
                  : 'Уведомления об обновлениях включены',
                3500,
              );
            }}
          />
        </div>

        <div className="fs-row">
          <div className="fs-row-text">
            <span className="fs-row-label">Лог последнего автообновления</span>
            <span className="fs-row-desc">Открыть файл %TEMP%\mwpwdtt-last-update.log в системном редакторе.</span>
          </div>
          <button type="button" className="fs-btn" onClick={() => void handleOpenUpdateLog()}>
            <IconFileText size={14} />
            Открыть лог
          </button>
        </div>
      </div>
    </div>
  );
}