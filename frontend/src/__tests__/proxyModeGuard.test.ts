import { describe, it, expect, beforeEach, vi } from 'vitest';
import {
  enterProxyMode,
  leaveProxyMode,
  getSavedSplitBackup,
  type ProxyGuardApi,
} from '../lib/utils/proxyModeGuard';
import type { SplitTunnelConfig } from '../lib/splitTunnelApi';

function makeMockApi(initial: SplitTunnelConfig) {
  let current: SplitTunnelConfig = { ...initial, apps: [...initial.apps] };
  const api: ProxyGuardApi = {
    isSupported: vi.fn(async () => true),
    getConfig: vi.fn(async () => ({ ...current, apps: [...current.apps] })),
    saveConfig: vi.fn(async (cfg: SplitTunnelConfig) => {
      current = { ...cfg, apps: [...cfg.apps] };
      return current;
    }),
    resetWfpRules: vi.fn(async () => {}),
    setProxySuspended: vi.fn(async () => {}),
  };
  return { api, getCurrent: () => current };
}

describe('proxyModeGuard', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('snapshots settings, disables conflicting options, and restores everything cleanly', async () => {
    const { api, getCurrent } = makeMockApi({
      schema_version: 1,
      enabled: true,
      mode: 'include',
      kill_switch: 'fail_closed',
      ignore_kill_switch_on_disconnect: false,
      wfp_crash_mode: 'persistent_guard',
      system_dns: 'tunnel_dns',
      portable_env_paths: true,
      apps: [{ name: 'chrome', exe_path: 'C:\\chrome.exe', match_children: true }],
    });

    const entered = await enterProxyMode(api);
    expect(entered).toBe(true);
    expect(getSavedSplitBackup()).toEqual({
      enabled: true,
      kill_switch: 'fail_closed',
      wfp_crash_mode: 'persistent_guard',
      system_dns: 'tunnel_dns',
    });
    expect(getCurrent().enabled).toBe(false);
    expect(getCurrent().wfp_crash_mode).toBe('dynamic');
    expect(getCurrent().kill_switch).toBe('fail_open');
    expect(getCurrent().apps).toHaveLength(1);
    expect(api.setProxySuspended).toHaveBeenCalledWith(true);
    expect(api.resetWfpRules).toHaveBeenCalled();

    await enterProxyMode(api);
    expect(getSavedSplitBackup()?.enabled).toBe(true);
    expect(getSavedSplitBackup()?.wfp_crash_mode).toBe('persistent_guard');

    const restored = await leaveProxyMode(api);
    expect(restored).toBe(true);
    expect(api.setProxySuspended).toHaveBeenCalledWith(false);
    expect(getCurrent().enabled).toBe(true);
    expect(getCurrent().kill_switch).toBe('fail_closed');
    expect(getCurrent().wfp_crash_mode).toBe('persistent_guard');
    expect(getCurrent().system_dns).toBe('tunnel_dns');
    expect(getCurrent().apps).toHaveLength(1);
    expect(getSavedSplitBackup()).toBeNull();
  });

  it('keeps split tunneling disabled on restore if it was disabled before', async () => {
    const { api, getCurrent } = makeMockApi({
      schema_version: 1,
      enabled: false,
      mode: 'exclude',
      kill_switch: 'fail_open',
      ignore_kill_switch_on_disconnect: true,
      wfp_crash_mode: 'dynamic',
      system_dns: 'system_dns',
      portable_env_paths: true,
      apps: [],
    });

    await enterProxyMode(api);
    await leaveProxyMode(api);
    expect(getCurrent().enabled).toBe(false);
    expect(getCurrent().system_dns).toBe('system_dns');
    expect(getSavedSplitBackup()).toBeNull();
  });
});