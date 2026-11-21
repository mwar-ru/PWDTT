import { describe, it, expect, beforeEach, vi } from 'vitest';
import {
  addOrUpdateApp,
  extractExeName,
  hasSplitTunnelBinding,
  normalizeAppKey,
  splitTunnelApi,
} from '../lib/splitTunnelApi';

describe('splitTunnelApi helpers & Wails binding bridge', () => {
  beforeEach(() => {
    delete window.go;
  });

  it('gracefully reports unsupported when Wails binding is absent (Linux/macOS)', async () => {
    expect(hasSplitTunnelBinding()).toBe(false);
    await expect(splitTunnelApi.isSupported()).resolves.toBe(false);
    expect(() => splitTunnelApi.getConfig()).toThrow();
  });

  it('normalizes Windows exe paths and deduplicates apps case-insensitively', () => {
    expect(normalizeAppKey('C:/Program Files/Firefox/firefox.exe ')).toBe(
      'c:\\program files\\firefox\\firefox.exe'
    );
    expect(extractExeName('C:\\Program Files\\Mozilla Firefox\\firefox.exe')).toBe('firefox');

    const list1 = addOrUpdateApp([], {
      name: '',
      exe_path: 'C:\\Program Files\\Mozilla Firefox\\firefox.exe',
      match_children: true,
    });
    expect(list1).toHaveLength(1);
    expect(list1[0].name).toBe('firefox');

    const list2 = addOrUpdateApp(list1, {
      name: 'Firefox Browser',
      exe_path: 'c:/program files/mozilla firefox/FIREFOX.EXE',
      match_children: false,
    });
    expect(list2).toHaveLength(1);
    expect(list2[0].name).toBe('Firefox Browser');
    expect(list2[0].match_children).toBe(false);
  });

  it('delegates calls to window.go.splittunnel.Service when available', async () => {
    const setConfigPath = vi.fn().mockResolvedValue({
      schema_version: 1,
      enabled: true,
      mode: 'include',
      kill_switch: 'fail_closed',
      wfp_crash_mode: 'dynamic',
      system_dns: 'tunnel_dns',
      portable_env_paths: true,
      apps: [],
    });
    const isSupported = vi.fn().mockResolvedValue(true);

    window.go = {
      splittunnel: {
        Service: {
          IsSupported: isSupported,
          GetConfig: vi.fn(),
          SaveConfig: vi.fn(),
          GetConfigPath: vi.fn(),
          SetConfigPath: setConfigPath,
          ListRunningApps: vi.fn(),
          GetStatus: vi.fn(),
          ResetWfpRules: vi.fn(),
          PickExecutableDialog: vi.fn(),
          PickConfigFileDialog: vi.fn(),
          CheckUpstreamUpdate: vi.fn(),
          AttemptUpstreamSync: vi.fn(),
          OpenUpstreamReport: vi.fn(),
        },
      },
    };

    expect(hasSplitTunnelBinding()).toBe(true);
    await expect(splitTunnelApi.isSupported()).resolves.toBe(true);
    await splitTunnelApi.setConfigPath('C:\\Users\\Me\\OneDrive\\split_tunnel.json', false);
    expect(setConfigPath).toHaveBeenCalledWith('C:\\Users\\Me\\OneDrive\\split_tunnel.json', false);
  });
});
