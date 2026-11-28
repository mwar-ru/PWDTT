import { useEffect, useState, useCallback } from 'react';
import {
  IconRoute,
  IconShieldLock,
  IconApps,
  IconFolder,
  IconRefresh,
  IconPlus,
  IconTrash,
  IconAlertTriangle,
  IconGitMerge,
  IconFileText,
  IconExternalLink,
} from '@tabler/icons-react';
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime';
import { toastStore } from '../lib/stores/toastStore';
import { settingsStore } from '../lib/store';
import {
  addOrUpdateApp,
  defaultSplitTunnelConfig,
  extractExeName,
  normalizeAppKey,
  splitTunnelApi,
  type AppConfig,
  type KillSwitchMode,
  type RunningApp,
  type SplitMode,
  type SplitTunnelConfig,
  type StatusSnapshot,
  type SyncAttemptResult,
  type SystemDNSPolicy,
  type UpdateCheckResult,
  type WFPCrashMode,
} from '../lib/splitTunnelApi';
import './SplitTunnel.css';

const safeCfg = (c: SplitTunnelConfig): SplitTunnelConfig => ({ ...c, apps: Array.isArray(c?.apps) ? c.apps : [] });

const UPSTREAM_URL = 'https://github.com/luminescq/PWDTT';

export default function SplitTunnel() {
  const [supported, setSupported] = useState<boolean | null>(null);
  const [cfg, setCfg] = useState<SplitTunnelConfig>(defaultSplitTunnelConfig);
  const [configPath, setConfigPath] = useState('');
  const [status, setStatus] = useState<StatusSnapshot | null>(null);
  const [saving, setSaving] = useState(false);

  const [showRunning, setShowRunning] = useState(false);
  const [runningApps, setRunningApps] = useState<RunningApp[]>([]);
  const [runningFilter, setRunningFilter] = useState('');
  const [loadingApps, setLoadingApps] = useState(false);

  const [updateInfo, setUpdateInfo] = useState<UpdateCheckResult | null>(null);
  const [checkingUpdate, setCheckingUpdate] = useState(false);
  const [syncResult, setSyncResult] = useState<SyncAttemptResult | null>(null);
  const [syncing, setSyncing] = useState(false);

  const refreshStatus = useCallback(async () => {
    try {
      const st = await splitTunnelApi.getStatus();
      setStatus(st);
      if (st.config_path) setConfigPath(st.config_path);
    } catch {
      // ignore transient polling error
    }
  }, []);

  useEffect(() => {
    let active = true;
    splitTunnelApi.isSupported().then(async ok => {
      if (!active) return;
      setSupported(ok);
      if (!ok) return;
      try {
        const [loaded, path, st] = await Promise.all([
          splitTunnelApi.getConfig(),
          splitTunnelApi.getConfigPath(),
          splitTunnelApi.getStatus(),
        ]);
        if (!active) return;
        setCfg(safeCfg(loaded));
        setConfigPath(path || st.config_path || '');
        setStatus(st);
      } catch (err) {
        toastStore.show(`Ошибка загрузки Split Tunnel: ${String(err)}`, 4000);
      }
    });
    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
    if (!supported) return;
    const id = setInterval(refreshStatus, 2500);
    return () => clearInterval(id);
  }, [supported, refreshStatus]);

  const persistConfig = async (next: SplitTunnelConfig) => {
    setCfg(safeCfg(next));
    setSaving(true);
    try {
      const saved = await splitTunnelApi.saveConfig(next);
      setCfg(safeCfg(saved));
      await refreshStatus();
    } catch (err) {
      toastStore.show(`Не удалось сохранить настройки: ${String(err)}`, 4500);
    } finally {
      setSaving(false);
    }
  };

  const handleModeChange = (mode: SplitMode) => persistConfig({ ...cfg, mode });
  const handleKillSwitchChange = (kill_switch: KillSwitchMode) =>
    persistConfig({ ...cfg, kill_switch });
  const handleCrashModeChange = (wfp_crash_mode: WFPCrashMode) =>
    persistConfig({ ...cfg, wfp_crash_mode });
  const handleDNSChange = (system_dns: SystemDNSPolicy) =>
    persistConfig({ ...cfg, system_dns });

  const handlePickExe = async () => {
    try {
      const selected = await splitTunnelApi.pickExecutableDialog();
      if (!selected) return;
      const nextApps = addOrUpdateApp(cfg.apps, {
        name: extractExeName(selected),
        exe_path: selected,
        match_children: true,
      });
      await persistConfig({ ...cfg, apps: nextApps });
      toastStore.show(`Добавлено: ${extractExeName(selected)}`);
    } catch (err) {
      toastStore.show(`Ошибка выбора файла: ${String(err)}`, 4000);
    }
  };

  const handleScanRunningApps = async () => {
    setShowRunning(true);
    setLoadingApps(true);
    try {
      const apps = await splitTunnelApi.listRunningApps();
      setRunningApps(apps);
    } catch (err) {
      toastStore.show(`Ошибка сканирования процессов: ${String(err)}`, 4000);
    } finally {
      setLoadingApps(false);
    }
  };

  const handleAddRunningApp = async (app: RunningApp) => {
    const nextApps = addOrUpdateApp(cfg.apps, {
      name: app.name || extractExeName(app.exe_path),
      exe_path: app.exe_path,
      match_children: true,
    });
    await persistConfig({ ...cfg, apps: nextApps });
    toastStore.show(`Добавлено: ${app.name}`);
  };

  const handleToggleChildren = (target: AppConfig) => {
    const key = normalizeAppKey(target.exe_path);
    const nextApps = cfg.apps.map(a =>
      normalizeAppKey(a.exe_path) === key ? { ...a, match_children: !a.match_children } : a
    );
    void persistConfig({ ...cfg, apps: nextApps });
  };

  const handleToggleAppEnabled = (target: AppConfig) => {
    const key = normalizeAppKey(target.exe_path);
    const nextApps = cfg.apps.map(a =>
      normalizeAppKey(a.exe_path) === key ? { ...a, disabled: !a.disabled } : a
    );
    void persistConfig({ ...cfg, apps: nextApps });
  };

  const handleRemoveApp = (target: AppConfig) => {
    const key = normalizeAppKey(target.exe_path);
    const nextApps = cfg.apps.filter(a => normalizeAppKey(a.exe_path) !== key);
    void persistConfig({ ...cfg, apps: nextApps });
  };

  const handleChangeConfigPath = async (existingOnly: boolean, moveExisting: boolean) => {
    try {
      const picked = await splitTunnelApi.pickConfigFileDialog(existingOnly);
      if (!picked) return;
      const loaded = await splitTunnelApi.setConfigPath(picked, moveExisting);
      setCfg(safeCfg(loaded));
      setConfigPath(picked);
      await refreshStatus();
      toastStore.show(
        existingOnly
          ? 'Подключён существующий файл настроек'
          : 'Файл настроек перенесён по новому пути'
      );
    } catch (err) {
      toastStore.show(`Не удалось сменить путь конфига: ${String(err)}`, 4500);
    }
  };

  const handleResetWFP = async () => {
    try {
      await splitTunnelApi.resetWfpRules();
      await refreshStatus();
      toastStore.show('Все WFP-правила PWDTT сброшены');
    } catch (err) {
      toastStore.show(`Ошибка сброса WFP: ${String(err)}`, 4500);
    }
  };

  const handleCheckUpdate = async () => {
    setCheckingUpdate(true);
    try {
      const res = await splitTunnelApi.checkUpstreamUpdate();
      setUpdateInfo(res);
      if (res.error) {
        toastStore.show(`Ошибка проверки GitHub: ${res.error}`, 4500);
      } else if (res.update_available) {
        toastStore.show(`Доступен новый upstream релиз: ${res.latest_tag}`);
      } else {
        toastStore.show(`Актуальная база upstream (${res.base_tag})`);
      }
    } catch (err) {
      toastStore.show(`Ошибка проверки upstream: ${String(err)}`, 4500);
    } finally {
      setCheckingUpdate(false);
    }
  };

  const handleAttemptSync = async () => {
    const target = updateInfo?.latest_tag || 'v1.6.0';
    setSyncing(true);
    try {
      const res = await splitTunnelApi.attemptUpstreamSync(target);
      setSyncResult(res);
      toastStore.show(res.summary, 4000);
    } catch (err) {
      toastStore.show(`Ошибка пробного слияния: ${String(err)}`, 5000);
    } finally {
      setSyncing(false);
    }
  };

  const handleOpenReport = async () => {
    if (!syncResult?.report_path) return;
    try {
      await splitTunnelApi.openUpstreamReport(syncResult.report_path);
    } catch (err) {
      toastStore.show(`Не удалось открыть отчёт: ${String(err)}`, 4000);
    }
  };

  const openExternal = (url: string) => {
    try {
      BrowserOpenURL(url);
    } catch {
      // ignore outside Wails
    }
  };

  if (supported === false) {
    return (
      <div className="st-page">
        <div className="st-card">
          <div className="st-card-title">
            <IconRoute size={18} />
            Split Tunneling
          </div>
          <div className="st-hint">
            Раздельное туннелирование по приложениям (WinDivert + WFP) доступно только в Windows 11.
          </div>
        </div>
      </div>
    );
  }

  const [proxyModeActive, setProxyModeActive] = useState<boolean>(() => !!settingsStore.get().proxyMode);
  useEffect(() => {
    const unsub = settingsStore.subscribe(s => setProxyModeActive(!!s.proxyMode));
    const onSplitChanged = () => {
      void splitTunnelApi.getConfig().then(setCfg).catch(() => {});
    };
    window.addEventListener('mwpwdtt:split-changed', onSplitChanged);
    return () => {
      unsub();
      window.removeEventListener('mwpwdtt:split-changed', onSplitChanged);
    };
  }, []);

  const competingVPNs = status?.competing_vpns ?? [];
  const filteredRunning = runningApps.filter(a => {
    const q = runningFilter.trim().toLowerCase();
    if (!q) return true;
    return a.name.toLowerCase().includes(q) || a.exe_path.toLowerCase().includes(q);
  });

  return (
    <div className="st-page">
      <div className="st-header">
        <div className="st-title-wrap">
          <h2 className="st-title">Split Tunneling</h2>
          <span className="st-subtitle">
            Маршрутизация отдельных приложений через VPN (WinDivert 2.2 + WFP Kill Switch)
          </span>
        </div>
        <label className="st-switch">
          <input
            type="checkbox"
            checked={cfg.enabled && !proxyModeActive}
            disabled={saving || proxyModeActive}
            onChange={e => void persistConfig({ ...cfg, enabled: e.target.checked })}
          />
          <span>{proxyModeActive ? 'Пауза (mihomo)' : cfg.enabled ? 'Включено' : 'Выключено'}</span>
        </label>
      </div>

      {proxyModeActive && (
        <div className="st-banner st-banner--warn">
          <IconAlertTriangle size={17} style={{ flexShrink: 0, marginTop: 1 }} />
          <div>
            Активен режим <strong>Прокси для mihomo (SOCKS5)</strong> — Split Tunneling и WFP Kill Switch
            временно приостановлены, чтобы не конфликтовать с mihomo. Прежние настройки восстановятся
            автоматически после выключения прокси в Настройках.
          </div>
        </div>
      )}

      {competingVPNs.length > 0 && (
        <div className="st-banner st-banner--warn">
          <IconAlertTriangle size={17} style={{ flexShrink: 0, marginTop: 1 }} />
          <div>
            Обнаружен другой активный VPN-адаптер:{' '}
            <strong>{competingVPNs.map(v => v.name).join(', ')}</strong>. Одновременная работа двух
            VPN может вызывать конфликты маршрутизации.
          </div>
        </div>
      )}

      {status?.warning && (
        <div className="st-banner st-banner--err">
          <IconAlertTriangle size={17} style={{ flexShrink: 0, marginTop: 1 }} />
          <div>{status.warning}</div>
        </div>
      )}

      {/* Mode & Live Counters */}
      <div className="st-card">
        <div className="st-card-head">
          <span className="st-card-title">
            <IconRoute size={17} />
            Режим маршрутизации
          </span>
          <span className="st-hint">
            Туннель: <strong>{status?.tunnel_state ?? 'down'}</strong> · Роутер:{' '}
            <strong>{status?.router?.state ?? 'stopped'}</strong>
          </span>
        </div>

        <div className="st-row">
          <div className="st-segmented" role="group" aria-label="Split mode">
            {(
              [
                ['include', 'Только выбранные'],
                ['exclude', 'Кроме выбранных'],
                ['global', 'Весь трафик (VPN)'],
              ] as [SplitMode, string][]
            ).map(([val, label]) => (
              <button
                key={val}
                type="button"
                className={`st-seg-btn${cfg.mode === val ? ' st-seg-btn--active' : ''}`}
                disabled={saving}
                onClick={() => void handleModeChange(val)}
              >
                {label}
              </button>
            ))}
          </div>
          <span className="st-hint">
            {cfg.mode === 'include' && 'Остальные приложения идут напрямую без падения скорости'}
            {cfg.mode === 'exclude' && 'Всё через VPN, выбранные приложения — напрямую'}
            {cfg.mode === 'global' && 'Штатный режим PWDTT без перехвата WinDivert'}
          </span>
        </div>

        <div className="st-stats-grid">
          <div className="st-stat">
            <span className="st-stat-label">Пакеты в VPN</span>
            <span className="st-stat-value">{status?.router?.packets_to_vpn ?? 0}</span>
          </div>
          <div className="st-stat">
            <span className="st-stat-label">Напрямую</span>
            <span className="st-stat-value">{status?.router?.packets_direct ?? 0}</span>
          </div>
          <div className="st-stat">
            <span className="st-stat-label">DNS перехвачено</span>
            <span className="st-stat-value">{status?.router?.dns_redirected ?? 0}</span>
          </div>
          <div className="st-stat">
            <span className="st-stat-label">NAT / WFP</span>
            <span className="st-stat-value">
              {status?.router?.active_nat_sessions ?? 0} / {status?.router?.active_wfp_filters ?? 0}
            </span>
          </div>
        </div>
      </div>

      {/* Apps List */}
      <div className="st-card">
        <div className="st-card-head">
          <span className="st-card-title">
            <IconApps size={17} />
            Приложения ({cfg.apps.length})
          </span>
          <div className="st-app-actions">
            <button type="button" className="st-btn" onClick={() => void handleScanRunningApps()}>
              <IconRefresh size={14} />
              Запущенные
            </button>
            <button type="button" className="st-btn st-btn--primary" onClick={() => void handlePickExe()}>
              <IconPlus size={14} />
              Выбрать .exe
            </button>
          </div>
        </div>

        {cfg.apps.length === 0 ? (
          <div className="st-hint">
            Список пуст. Добавьте Firefox, Edge или другие программы через «Запущенные» или «Выбрать .exe».
          </div>
        ) : (
          <div className="st-app-list">
            {cfg.apps.map(app => (
              <div key={normalizeAppKey(app.exe_path)} className="st-app-item">
                <div className="st-app-meta">
                  <span className="st-app-name">{app.name || extractExeName(app.exe_path)}</span>
                  <span className="st-app-path" title={app.exe_path}>
                    {app.exe_path}
                  </span>
                </div>
                <div className="st-app-actions">
                  <label
                    className="st-switch"
                    title={app.disabled ? 'Включить правило для приложения' : 'Временно выключить правило для приложения'}
                  >
                    <input
                      type="checkbox"
                      checked={!app.disabled}
                      onChange={() => handleToggleAppEnabled(app)}
                    />
                    <span>{app.disabled ? 'Выкл' : 'Вкл'}</span>
                  </label>
                  <label className="st-switch" title="Применять правило к дочерним процессам">
                    <input
                      type="checkbox"
                      checked={app.match_children}
                      onChange={() => handleToggleChildren(app)}
                    />
                    <span>Дочерние</span>
                  </label>
                  <button
                    type="button"
                    className="st-btn st-btn--danger"
                    aria-label={`Удалить ${app.name}`}
                    onClick={() => handleRemoveApp(app)}
                  >
                    <IconTrash size={14} />
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}

        {showRunning && (
          <div className="st-picker-list">
            <div className="st-row">
              <input
                type="text"
                className="st-input"
                placeholder="Поиск по запущенным приложениям..."
                value={runningFilter}
                onChange={e => setRunningFilter(e.target.value)}
              />
              <button type="button" className="st-btn" onClick={() => setShowRunning(false)}>
                Скрыть
              </button>
            </div>
            {loadingApps ? (
              <span className="st-hint">Сканирование процессов Windows...</span>
            ) : filteredRunning.length === 0 ? (
              <span className="st-hint">Ничего не найдено.</span>
            ) : (
              filteredRunning.slice(0, 40).map(app => (
                <div key={app.normalized_path || app.exe_path} className="st-app-item">
                  <div className="st-app-meta">
                    <span className="st-app-name">
                      {app.name} <span className="st-hint">({app.process_count} проц.)</span>
                    </span>
                    <span className="st-app-path">{app.exe_path}</span>
                  </div>
                  <button
                    type="button"
                    className="st-btn"
                    onClick={() => void handleAddRunningApp(app)}
                  >
                    <IconPlus size={14} />
                    Добавить
                  </button>
                </div>
              ))
            )}
          </div>
        )}
      </div>

      {/* Security: Kill Switch & DNS */}
      <div className="st-card">
        <div className="st-card-head">
          <span className="st-card-title">
            <IconShieldLock size={17} />
            Защита от утечек (WFP Kill Switch &amp; DNS Guard)
          </span>
          <button type="button" className="st-btn st-btn--danger" onClick={() => void handleResetWFP()}>
            Сбросить WFP-правила
          </button>
        </div>

        <div className="st-row">
          <div>
            <div className="st-label">Kill Switch при обрыве туннеля</div>
            <div className="st-hint">Блокировать выбранные приложения и IPv6-утечки через WFP</div>
          </div>
          <div className="st-segmented">
            <button
              type="button"
              className={`st-seg-btn${cfg.kill_switch === 'fail_closed' ? ' st-seg-btn--active' : ''}`}
              onClick={() => void handleKillSwitchChange('fail_closed')}
            >
              Fail-Closed
            </button>
            <button
              type="button"
              className={`st-seg-btn${cfg.kill_switch === 'fail_open' ? ' st-seg-btn--active' : ''}`}
              onClick={() => void handleKillSwitchChange('fail_open')}
            >
              Fail-Open
            </button>
          </div>
        </div>

        <div className="st-row">
          <div>
            <div className="st-label">Игнорировать Kill Switch при ручном отключении</div>
            <div className="st-hint">
              {cfg.ignore_kill_switch_on_disconnect !== false
                ? 'При статусе «Отключён» выбранные приложения работают напрямую (Direct); при обрыве Kill Switch сработает'
                : 'Строгий режим: приложения заблокированы даже при ручном отключении VPN'}
            </div>
          </div>
          <label style={{ display: 'inline-flex', alignItems: 'center', gap: 8, cursor: 'pointer' }}>
            <input
              type="checkbox"
              checked={cfg.ignore_kill_switch_on_disconnect !== false}
              onChange={(e) =>
                void persistConfig({
                  ...cfg,
                  ignore_kill_switch_on_disconnect: e.target.checked,
                })
              }
            />
            <span>Direct при «Отключён»</span>
          </label>
        </div>

        <div className="st-row">
          <div>
            <div className="st-label">Режим WFP-фильтров</div>
            <div className="st-hint">Dynamic снимается автоматически при закрытии PWDTT</div>
          </div>
          <div className="st-segmented">
            <button
              type="button"
              className={`st-seg-btn${cfg.wfp_crash_mode === 'dynamic' ? ' st-seg-btn--active' : ''}`}
              onClick={() => void handleCrashModeChange('dynamic')}
            >
              Dynamic
            </button>
            <button
              type="button"
              className={`st-seg-btn${cfg.wfp_crash_mode === 'persistent_guard' ? ' st-seg-btn--active' : ''}`}
              onClick={() => void handleCrashModeChange('persistent_guard')}
            >
              Persistent Guard
            </button>
          </div>
        </div>

        <div className="st-row">
          <div>
            <div className="st-label">Системный DNS (svchost UDP/53)</div>
            <div className="st-hint">DoH браузеров (порт 443) не затрагивается</div>
          </div>
          <div className="st-segmented">
            <button
              type="button"
              className={`st-seg-btn${cfg.system_dns === 'tunnel_dns' ? ' st-seg-btn--active' : ''}`}
              onClick={() => void handleDNSChange('tunnel_dns')}
            >
              DNS туннеля
            </button>
            <button
              type="button"
              className={`st-seg-btn${cfg.system_dns === 'system_dns' ? ' st-seg-btn--active' : ''}`}
              onClick={() => void handleDNSChange('system_dns')}
            >
              Системный DNS
            </button>
          </div>
        </div>
      </div>

      {/* Custom / Portable Config Path */}
      <div className="st-card">
        <div className="st-card-head">
          <span className="st-card-title">
            <IconFolder size={17} />
            Файл настроек
          </span>
          <label className="st-switch">
            <input
              type="checkbox"
              checked={cfg.portable_env_paths}
              onChange={e => void persistConfig({ ...cfg, portable_env_paths: e.target.checked })}
            />
            <span>Портативные %Env%-пути</span>
          </label>
        </div>

        <div className="st-row">
          <input type="text" className="st-input" readOnly value={configPath || 'По умолчанию (%AppData%\\PWDTT\\split_tunnel.json)'} />
          <button type="button" className="st-btn" onClick={() => void handleChangeConfigPath(true, false)}>
            Подключить существующий
          </button>
          <button type="button" className="st-btn" onClick={() => void handleChangeConfigPath(false, true)}>
            Перенести сюда...
          </button>
        </div>
      </div>

      {/* Upstream Sync & AI Report */}
      <div className="st-card">
        <div className="st-card-head">
          <span className="st-card-title">
            <IconGitMerge size={17} />
            Синхронизация с оригиналом (Upstream PWDTT)
          </span>
          <button
            type="button"
            className="st-btn"
            onClick={() => openExternal(updateInfo?.upstream_repo_url || UPSTREAM_URL)}
          >
            <IconExternalLink size={14} />
            luminescq/PWDTT ({updateInfo?.base_tag || 'v1.6.0'})
          </button>
        </div>

        <div className="st-row">
          <span className="st-hint">
            Безопасный dry-run merge в изолированном git worktree + генерация отчёта для ИИ-агента
          </span>
          <div className="st-app-actions">
            <button
              type="button"
              className="st-btn"
              disabled={checkingUpdate}
              onClick={() => void handleCheckUpdate()}
            >
              <IconRefresh size={14} />
              {checkingUpdate ? 'Проверка...' : 'Проверить обновления'}
            </button>
            <button
              type="button"
              className="st-btn st-btn--primary"
              disabled={syncing}
              onClick={() => void handleAttemptSync()}
            >
              <IconGitMerge size={14} />
              {syncing ? 'Проверка слияния...' : 'Пробное слияние (Dry-Run)'}
            </button>
          </div>
        </div>

        {updateInfo && !updateInfo.error && (
          <div className={`st-banner ${updateInfo.update_available ? 'st-banner--warn' : 'st-banner--ok'}`}>
            <div>
              База: <strong>{updateInfo.base_tag}</strong> · Последний релиз:{' '}
              <strong>{updateInfo.latest_tag}</strong>
              {updateInfo.compare && (
                <>
                  {' '}
                  · Изменено файлов: <strong>{updateInfo.compare.changed_files.length}</strong>
                  {updateInfo.compare.touched_seam_overlap.length > 0 && (
                    <>
                      {' '}
                      · Пересечения со швами:{' '}
                      <strong>{updateInfo.compare.touched_seam_overlap.join(', ')}</strong>
                    </>
                  )}
                </>
              )}
            </div>
          </div>
        )}

        {syncResult && (
          <>
            <div className={`st-banner ${syncResult.dry_run.clean ? 'st-banner--ok' : 'st-banner--warn'}`}>
              <div style={{ flex: 1 }}>
                <strong>{syncResult.summary}</strong>
                <div className="st-hint">{syncResult.report_path}</div>
              </div>
              <button type="button" className="st-btn" onClick={() => void handleOpenReport()}>
                <IconFileText size={14} />
                Открыть отчёт
              </button>
            </div>
            <pre className="st-code-preview">{syncResult.report_preview}</pre>
          </>
        )}
      </div>
    </div>
  );
}
