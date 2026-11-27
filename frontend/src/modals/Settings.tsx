import { useState, useEffect, useRef, useCallback } from 'react';
import { IconSettings2, IconX, IconHeartFilled, IconBug, IconAlertTriangle, IconFileText, IconCopy, IconCheck } from '@tabler/icons-react';
import { settingsStore } from '../lib/store';
import type { AppSettings } from '../lib/types';
import { SetAutoStart, GetAutoStart, GetVersion, GenerateReport, GetObfsAccepted, SetObfsAccepted, SetObfsMode } from '../../wailsjs/go/backend/App';
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime';
import { logStore } from '../lib/stores/logStore';
import { toastStore } from '../lib/stores/toastStore';
import { getUpdatePauseState, setPauseNextUpdate } from '../lib/utils/updateSkip';
import { splitTunnelApi } from '../lib/splitTunnelApi';
import { enterProxyMode, leaveProxyMode, requestProxyReconnect, syncProxyStandbyBackend } from '../lib/utils/proxyModeGuard';
import { tunnelStore } from '../lib/stores/tunnelStore';
import './Settings.css';

interface Props {
  onClose: () => void;
}

export default function Settings({ onClose }: Props) {
  const [settings, setSettings] = useState<AppSettings>(() => settingsStore.get());
  const [version, setVersion] = useState('...');
  const copiedTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [showObfsModal, setShowObfsModal] = useState(false);
  const [pendingObfsMode, setPendingObfsMode] = useState<'audio' | 'video'>('audio');
  const [updatePause, setUpdatePause] = useState(() => getUpdatePauseState());
  const [copiedMihomo, setCopiedMihomo] = useState(false);

  const proxyEnabled = !!settings.proxyMode;
  const proxyPort = settings.proxyPort || 10808;
  const isUpdatePaused = updatePause.pausedNext || Boolean(updatePause.skippedVersion);

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

  const update = useCallback(<K extends keyof AppSettings>(key: K, value: AppSettings[K]) => {
    setSettings(s => {
      const next = { ...s, [key]: value };
      settingsStore.save(next);
      return next;
    });
  }, []);

  useEffect(() => {
    return () => { if (copiedTimerRef.current) clearTimeout(copiedTimerRef.current); };
  }, []);

  useEffect(() => {
    GetAutoStart().then(v => {
      if (v !== settings.autoStart) update('autoStart', v);
    }).catch(() => { toastStore.show('Не удалось загрузить настройки', 3000); });
    GetObfsAccepted().then(v => {
      update('obfsAccepted', v);
    }).catch(() => {});
    GetVersion().then(setVersion).catch(() => {});
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const handleOpenUpdateLog = async () => {
    try {
      const fn = (window as unknown as { go?: { backend?: { App?: { OpenUpdateLog?: () => Promise<void> } } } })?.go?.backend?.App?.OpenUpdateLog;
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

  const handleReport = async () => {
    const logs = logStore.getAll();
    const report = await GenerateReport(logs.map(e => ({
      level: e.level,
      message: e.message,
      time: e.time,
      count: e.count,
    })));
    const mode = settings.obfsMode || 'audio';
    const accepted = settings.obfsAccepted ? 'да' : 'нет';
    const transport = settings.turnTcp ? 'TCP' : 'UDP';
    const full = report + `\n## Settings\n- Obfuscation: ${mode}\n- Obfs accepted: ${accepted}\n- Transport: ${transport}\n`;
    await navigator.clipboard.writeText(full);
    setCopiedReport(true);
    if (copiedTimerRef.current) clearTimeout(copiedTimerRef.current);
    copiedTimerRef.current = setTimeout(() => setCopiedReport(false), 2000);
  };

  const [copiedReport, setCopiedReport] = useState(false);

  const handleObfsClick = async (mode: 'audio' | 'video') => {
    if (settings.obfsAccepted) {
      update('obfsMode', mode);
      await SetObfsMode(mode);
    } else {
      setPendingObfsMode(mode);
      setShowObfsModal(true);
    }
  };

  const handleObfsAccept = async () => {
    try {
      await SetObfsAccepted(true);
      await SetObfsMode(pendingObfsMode);
      update('obfsAccepted', true);
      update('obfsMode', pendingObfsMode);
      setShowObfsModal(false);
    } catch (e) {
      toastStore.show('Не удалось сохранить настройки', 3000);
    }
  };

  return (
    <>
      <div className="st-overlay" onClick={onClose}>
        <div className="st-modal" onClick={e => e.stopPropagation()}>
          <div className="st-header">
            <IconSettings2 stroke={2} size={20} />
            <span className="st-title">Настройки</span>
            <button type="button" className="st-close" onClick={onClose} aria-label="Закрыть"><IconX size={18} /></button>
          </div>

          <div className="st-row">
            <span>Запускать при старте</span>
            <button type="button" className={`st-toggle st-toggle--${settings.autoStart ? 'on' : 'off'}`} aria-label={settings.autoStart ? 'Отключить автозапуск' : 'Включить автозапуск'} onClick={() => {
              const next = !settings.autoStart;
              update('autoStart', next);
              SetAutoStart(next);
            }} />
          </div>

          <div className="st-row">
            <span>Режим обфускации {!settings.obfsAccepted && <span className="st-badge">только latest</span>}</span>
            <div className="st-segment">
              <button type="button" className={`st-seg-btn${settings.obfsMode === 'audio' ? ' st-seg-btn--active' : ''}`} onClick={() => handleObfsClick('audio')}>Audio</button>
              <button type="button" className={`st-seg-btn${settings.obfsMode === 'video' ? ' st-seg-btn--active' : ''}`} onClick={() => handleObfsClick('video')}>Video</button>
            </div>
          </div>

          <div className="st-row">
            <span>Транспорт TURN</span>
            <div className="st-segment">
              <button type="button" className={`st-seg-btn${!settings.turnTcp ? ' st-seg-btn--active' : ''}`} onClick={() => update('turnTcp', false)}>UDP</button>
              <button type="button" className={`st-seg-btn${settings.turnTcp ? ' st-seg-btn--active' : ''}`} onClick={() => update('turnTcp', true)}>TCP</button>
            </div>
          </div>

          <div className="st-row">
            <span>Прокси для mihomo <span className="st-badge">SOCKS5</span></span>
            <button
              type="button"
              className={`st-toggle st-toggle--${proxyEnabled ? 'on' : 'off'}`}
              aria-label={proxyEnabled ? 'Отключить режим SOCKS5-прокси' : 'Включить режим SOCKS5-прокси'}
              onClick={() => {
                const next = !proxyEnabled;
                update('proxyMode', next);
                if (next && !settings.proxyPort) update('proxyPort', 10808);
                const wasActive = tunnelStore.get() === 'connected' || tunnelStore.get() === 'connecting';
                if (next) {
                  void enterProxyMode(splitTunnelApi, proxyPort).then(guarded => {
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
              }}
            />
          </div>

          {proxyEnabled && (
            <div className="st-subrow">
              <span className="st-subrow-label">127.0.0.1 :</span>
              <input
                type="number"
                className="st-port-input"
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
              <button type="button" className="st-mini-btn" onClick={() => void handleCopyMihomo()}>
                {copiedMihomo ? <IconCheck size={13} /> : <IconCopy size={13} />}
                {copiedMihomo ? 'Скопировано' : 'YAML'}
              </button>
            </div>
          )}

          <div className="st-row">
            <span>
              Приостановить обновления
              {isUpdatePaused && (
                <span className="st-badge">
                  {updatePause.skippedVersion ? `пропуск v${updatePause.skippedVersion}` : 'след. версия'}
                </span>
              )}
            </span>
            <button
              type="button"
              className={`st-toggle st-toggle--${isUpdatePaused ? 'on' : 'off'}`}
              aria-label={isUpdatePaused ? 'Возобновить уведомления об обновлениях' : 'Приостановить обновления до следующей версии'}
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

          <div className="st-info">
            <div className="st-info-name">mwPWDTT_windows</div>
            <div className="st-info-ver">v{version}</div>
          </div>

          <div className="st-actions">
            <button type="button" className={`st-action${copiedReport ? ' st-action--copied' : ''}`} onClick={handleReport}>
              <IconBug size={14} />
              {copiedReport ? 'Скопировано!' : 'Отчёт'}
            </button>
            <button type="button" className="st-action" onClick={handleOpenUpdateLog} title="Открыть лог последнего автообновления">
              <IconFileText size={14} />
              Лог обн.
            </button>
          </div>

          <button type="button" className="st-donate" onClick={() => BrowserOpenURL('https://pay.cloudtips.ru/p/1ea077ba')}>
            <IconHeartFilled size={16} />
            Поддержать проект
          </button>
        </div>
      </div>

      {showObfsModal && (
        <div className="st-overlay" onClick={() => setShowObfsModal(false)}>
          <div className="st-modal st-obfs-modal" onClick={e => e.stopPropagation()}>
            <div className="st-obfs-icon">
              <IconAlertTriangle size={32} />
            </div>
            <div className="st-obfs-title">Режим обфускации</div>
            <div className="st-obfs-text">
              Обфускация поддерживается только на последней версии сервера WDTT.
              При использовании на старых серверах соединение может не состояться.
              Если вы не уверены в совместимости, рекомендуется использовать режим "Audio".
            </div>
            <div className="st-obfs-text st-obfs-disclaimer">
              Разработчик не несёт ответственности за проблемы при использовании на несовместимых серверах.
            </div>
            <div className="st-obfs-actions">
              <button type="button" className="st-obfs-btn st-obfs-btn--cancel" onClick={() => setShowObfsModal(false)}>Отмена</button>
              <button type="button" className="st-obfs-btn st-obfs-btn--accept" onClick={handleObfsAccept}>Принимаю</button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
