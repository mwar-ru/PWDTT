export type SplitMode = 'global' | 'include' | 'exclude';
export type KillSwitchMode = 'fail_closed' | 'fail_open';
export type WFPCrashMode = 'dynamic' | 'persistent_guard';
export type SystemDNSPolicy = 'tunnel_dns' | 'system_dns';

export interface AppConfig {
  name: string;
  exe_path: string;
  match_children: boolean;
}

export interface SplitTunnelConfig {
  schema_version: number;
  enabled: boolean;
  mode: SplitMode;
  kill_switch: KillSwitchMode;
  ignore_kill_switch_on_disconnect: boolean;
  wfp_crash_mode: WFPCrashMode;
  system_dns: SystemDNSPolicy;
  portable_env_paths: boolean;
  apps: AppConfig[];
}

export interface RunningApp {
  name: string;
  exe_path: string;
  normalized_path: string;
  process_count: number;
  pids: number[];
}

export interface InterfaceInfo {
  if_index: number;
  name: string;
  description: string;
  ipv4?: string;
  ipv6?: string;
  dns_servers?: string[];
  metric: number;
  is_up: boolean;
  is_competing_vpn: boolean;
}

export interface RouterStats {
  state: 'stopped' | 'running' | 'degraded' | 'error';
  mode: string;
  tunnel_up: boolean;
  packets_to_vpn: number;
  packets_direct: number;
  packets_dropped_fail_closed: number;
  dns_redirected: number;
  staging_queued: number;
  staging_expired: number;
  active_nat_sessions: number;
  active_wfp_filters: number;
  last_error?: string;
}

export interface StatusSnapshot {
  supported: boolean;
  running: boolean;
  config_path: string;
  tunnel_state: string;
  topology?: {
    tunnel?: InterfaceInfo;
    physical?: InterfaceInfo;
    competing_vpns?: InterfaceInfo[];
  };
  router: RouterStats;
  competing_vpns?: InterfaceInfo[];
  warning?: string;
}

export interface ReleaseInfo {
  tag_name: string;
  name: string;
  html_url: string;
  published_at: string;
  body: string;
}

export interface CompareSummary {
  base_tag: string;
  head_tag: string;
  ahead_by: number;
  total_commits: number;
  compare_url: string;
  changed_files: string[];
  touched_seam_overlap: string[];
}

export interface UpdateCheckResult {
  upstream_repo_url: string;
  base_tag: string;
  latest_tag: string;
  update_available: boolean;
  from_cache: boolean;
  checked_at: string;
  release?: ReleaseInfo;
  compare?: CompareSummary;
  error?: string;
}

export interface DryRunResult {
  target_ref: string;
  clean: boolean;
  merged_files?: string[];
  conflict_files?: string[];
  diff_stat?: string;
  raw_output?: string;
}

export interface SyncAttemptResult {
  target_ref: string;
  dry_run: DryRunResult;
  report_path: string;
  report_preview: string;
  summary: string;
}

interface WailsSplitTunnelBinding {
  IsSupported(): Promise<boolean>;
  GetConfig(): Promise<SplitTunnelConfig>;
  SaveConfig(cfg: SplitTunnelConfig): Promise<SplitTunnelConfig>;
  GetConfigPath(): Promise<string>;
  SetConfigPath(newPath: string, moveExisting: boolean): Promise<SplitTunnelConfig>;
  ListRunningApps(): Promise<RunningApp[]>;
  GetStatus(): Promise<StatusSnapshot>;
  ResetWfpRules(): Promise<void>;
  PickExecutableDialog(): Promise<string>;
  PickConfigFileDialog(existingOnly: boolean): Promise<string>;
  CheckUpstreamUpdate(): Promise<UpdateCheckResult>;
  AttemptUpstreamSync(targetRef: string): Promise<SyncAttemptResult>;
  OpenUpstreamReport(reportPath: string): Promise<void>;
}

declare global {
  interface Window {
    go?: {
      splittunnel?: {
        Service?: WailsSplitTunnelBinding;
      };
    };
  }
}

export function getSplitTunnelBinding(): WailsSplitTunnelBinding | null {
  return window.go?.splittunnel?.Service ?? null;
}

export function hasSplitTunnelBinding(): boolean {
  return getSplitTunnelBinding() !== null;
}

export function defaultSplitTunnelConfig(): SplitTunnelConfig {
  return {
    schema_version: 1,
    enabled: true,
    mode: 'include',
    kill_switch: 'fail_closed',
    ignore_kill_switch_on_disconnect: true,
    wfp_crash_mode: 'dynamic',
    system_dns: 'tunnel_dns',
    portable_env_paths: true,
    apps: [],
  };
}

export function normalizeAppKey(exePath: string): string {
  return exePath.trim().replace(/\//g, '\\').toLowerCase();
}

export function extractExeName(exePath: string): string {
  const clean = exePath.trim().replace(/\//g, '\\');
  const idx = clean.lastIndexOf('\\');
  const base = idx >= 0 ? clean.slice(idx + 1) : clean;
  return base.replace(/\.exe$/i, '') || base;
}

export function addOrUpdateApp(apps: AppConfig[], next: AppConfig): AppConfig[] {
  const key = normalizeAppKey(next.exe_path);
  if (!key) return apps;
  const exists = apps.some(a => normalizeAppKey(a.exe_path) === key);
  if (exists) {
    return apps.map(a =>
      normalizeAppKey(a.exe_path) === key
        ? { ...a, name: next.name || a.name, match_children: next.match_children }
        : a
    );
  }
  return [...apps, { ...next, name: next.name || extractExeName(next.exe_path) }];
}

function requireBinding(): WailsSplitTunnelBinding {
  const b = getSplitTunnelBinding();
  if (!b) {
    throw new Error('Split Tunneling поддерживается только в Windows-версии PWDTT');
  }
  return b;
}

export const splitTunnelApi = {
  async isSupported(): Promise<boolean> {
    const b = getSplitTunnelBinding();
    if (!b) return false;
    try {
      return await b.IsSupported();
    } catch {
      return false;
    }
  },
  getConfig: () => requireBinding().GetConfig(),
  saveConfig: (cfg: SplitTunnelConfig) => requireBinding().SaveConfig(cfg),
  getConfigPath: () => requireBinding().GetConfigPath(),
  setConfigPath: (newPath: string, moveExisting: boolean) =>
    requireBinding().SetConfigPath(newPath, moveExisting),
  listRunningApps: () => requireBinding().ListRunningApps(),
  getStatus: () => requireBinding().GetStatus(),
  resetWfpRules: () => requireBinding().ResetWfpRules(),
  pickExecutableDialog: () => requireBinding().PickExecutableDialog(),
  pickConfigFileDialog: (existingOnly: boolean) =>
    requireBinding().PickConfigFileDialog(existingOnly),
  checkUpstreamUpdate: () => requireBinding().CheckUpstreamUpdate(),
  attemptUpstreamSync: (targetRef: string) =>
    requireBinding().AttemptUpstreamSync(targetRef),
  openUpstreamReport: (reportPath: string) =>
    requireBinding().OpenUpstreamReport(reportPath),
};
