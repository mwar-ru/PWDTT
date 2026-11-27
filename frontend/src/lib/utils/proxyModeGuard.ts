import type {
  KillSwitchMode,
  SplitTunnelConfig,
  SystemDNSPolicy,
  WFPCrashMode,
} from '../splitTunnelApi';

const BACKUP_STORAGE_KEY = 'pwdtt_proxy_mode_split_backup_v1';

export interface SplitTunnelBackup {
  enabled: boolean;
  kill_switch: KillSwitchMode;
  wfp_crash_mode: WFPCrashMode;
  system_dns: SystemDNSPolicy;
}

export interface ProxyGuardApi {
  isSupported(): Promise<boolean>;
  getConfig(): Promise<SplitTunnelConfig>;
  saveConfig(cfg: SplitTunnelConfig): Promise<SplitTunnelConfig>;
  resetWfpRules(): Promise<void>;
  setProxySuspended(suspended: boolean): Promise<void>;
}

export function getSavedSplitBackup(): SplitTunnelBackup | null {
  try {
    const raw = localStorage.getItem(BACKUP_STORAGE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as Partial<SplitTunnelBackup>;
    if (typeof parsed.enabled !== 'boolean') return null;
    return {
      enabled: parsed.enabled,
      kill_switch: parsed.kill_switch || 'fail_closed',
      wfp_crash_mode: parsed.wfp_crash_mode || 'dynamic',
      system_dns: parsed.system_dns || 'tunnel_dns',
    };
  } catch {
    return null;
  }
}

function saveSplitBackup(backup: SplitTunnelBackup): void {
  try {
    localStorage.setItem(BACKUP_STORAGE_KEY, JSON.stringify(backup));
  } catch {
    // ignore storage errors
  }
}

function clearSplitBackup(): void {
  try {
    localStorage.removeItem(BACKUP_STORAGE_KEY);
  } catch {
    // ignore storage errors
  }
}

export async function syncProxyStandbyBackend(enabled: boolean, port = 10808): Promise<void> {
  try {
    const fn = (window as unknown as {
      go?: { backend?: { App?: { SyncProxyStandby?: (e: boolean, p: number) => Promise<void> } } };
    })?.go?.backend?.App?.SyncProxyStandby;
    if (fn) {
      await fn(enabled, port || 10808);
    }
  } catch {
    // ignore if port busy or running outside Wails
  }
}

export async function enterProxyMode(api: ProxyGuardApi, port = 10808): Promise<boolean> {
  await syncProxyStandbyBackend(true, port);
  try {
    const supported = await api.isSupported();
    if (!supported) return false;

    const cfg = await api.getConfig();
    if (!getSavedSplitBackup()) {
      saveSplitBackup({
        enabled: cfg.enabled,
        kill_switch: cfg.kill_switch,
        wfp_crash_mode: cfg.wfp_crash_mode,
        system_dns: cfg.system_dns,
      });
    }

    if (cfg.enabled || cfg.wfp_crash_mode !== 'dynamic' || cfg.kill_switch !== 'fail_open') {
      await api.saveConfig({
        ...cfg,
        enabled: false,
        wfp_crash_mode: 'dynamic',
        kill_switch: 'fail_open',
      });
    }

    await api.setProxySuspended(true);
    await api.resetWfpRules().catch(() => {});
    return true;
  } catch {
    return false;
  }
}

export async function leaveProxyMode(api: ProxyGuardApi): Promise<boolean> {
  await syncProxyStandbyBackend(false, 0);
  try {
    await api.setProxySuspended(false).catch(() => {});
    const backup = getSavedSplitBackup();
    if (!backup) return false;

    const supported = await api.isSupported();
    if (!supported) {
      clearSplitBackup();
      return false;
    }

    const cfg = await api.getConfig();
    await api.saveConfig({
      ...cfg,
      enabled: backup.enabled,
      kill_switch: backup.kill_switch,
      wfp_crash_mode: backup.wfp_crash_mode,
      system_dns: backup.system_dns,
    });
    clearSplitBackup();
    return true;
  } catch {
    return false;
  }
}

export async function syncProxyModeOnStartup(
  proxyEnabled: boolean,
  api: ProxyGuardApi,
  port = 10808,
): Promise<void> {
  if (!proxyEnabled) {
    await syncProxyStandbyBackend(false, 0);
    return;
  }
  await enterProxyMode(api, port);
}
const reconnectListeners = new Set<() => void>();

export function onProxyReconnect(fn: () => void): () => void {
  reconnectListeners.add(fn);
  return () => {
    reconnectListeners.delete(fn);
  };
}

export function requestProxyReconnect(): void {
  reconnectListeners.forEach(fn => fn());
}