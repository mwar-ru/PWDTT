export namespace appmgr {
	
	export class RunningApp {
	    name: string;
	    exe_path: string;
	    normalized_path: string;
	    process_count: number;
	    pids: number[];
	
	    static createFrom(source: any = {}) {
	        return new RunningApp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.exe_path = source["exe_path"];
	        this.normalized_path = source["normalized_path"];
	        this.process_count = source["process_count"];
	        this.pids = source["pids"];
	    }
	}

}

export namespace backend {
	
	export class ConnectParams {
	    peerAddr: string;
	    password: string;
	    hashes: string[];
	    deviceId?: string;
	    workers?: number;
	    captchaMode?: string;
	    obfsMode?: string;
	    fingerprint?: string;
	    turnTcp?: boolean;
	    proxyMode?: boolean;
	    proxyPort?: number;
	
	    static createFrom(source: any = {}) {
	        return new ConnectParams(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.peerAddr = source["peerAddr"];
	        this.password = source["password"];
	        this.hashes = source["hashes"];
	        this.deviceId = source["deviceId"];
	        this.workers = source["workers"];
	        this.captchaMode = source["captchaMode"];
	        this.obfsMode = source["obfsMode"];
	        this.fingerprint = source["fingerprint"];
	        this.turnTcp = source["turnTcp"];
	        this.proxyMode = source["proxyMode"];
	        this.proxyPort = source["proxyPort"];
	    }
	}
	export class LogEntry {
	    level: string;
	    message: string;
	    time: string;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new LogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.level = source["level"];
	        this.message = source["message"];
	        this.time = source["time"];
	        this.count = source["count"];
	    }
	}
	export class ProfileData {
	    peer: string;
	    password: string;
	    hashes: string[];
	    workers?: number;
	    listen: string;
	    turn: string;
	    port: string;
	    device_id: string;
	    turn_tcp: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ProfileData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.peer = source["peer"];
	        this.password = source["password"];
	        this.hashes = source["hashes"];
	        this.workers = source["workers"];
	        this.listen = source["listen"];
	        this.turn = source["turn"];
	        this.port = source["port"];
	        this.device_id = source["device_id"];
	        this.turn_tcp = source["turn_tcp"];
	    }
	}
	export class UpdateInfo {
	    available: boolean;
	    version: string;
	    url: string;
	    body: string;
	    checksumsUrl: string;
	    assetName: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.version = source["version"];
	        this.url = source["url"];
	        this.body = source["body"];
	        this.checksumsUrl = source["checksumsUrl"];
	        this.assetName = source["assetName"];
	    }
	}

}

export namespace router {
	
	export class RouterStats {
	    state: string;
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
	
	    static createFrom(source: any = {}) {
	        return new RouterStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.mode = source["mode"];
	        this.tunnel_up = source["tunnel_up"];
	        this.packets_to_vpn = source["packets_to_vpn"];
	        this.packets_direct = source["packets_direct"];
	        this.packets_dropped_fail_closed = source["packets_dropped_fail_closed"];
	        this.dns_redirected = source["dns_redirected"];
	        this.staging_queued = source["staging_queued"];
	        this.staging_expired = source["staging_expired"];
	        this.active_nat_sessions = source["active_nat_sessions"];
	        this.active_wfp_filters = source["active_wfp_filters"];
	        this.last_error = source["last_error"];
	    }
	}

}

export namespace settings {
	
	export class AppConfig {
	    name: string;
	    exe_path: string;
	    match_children: boolean;
	    disabled?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.exe_path = source["exe_path"];
	        this.match_children = source["match_children"];
	        this.disabled = source["disabled"];
	    }
	}
	export class Config {
	    schema_version: number;
	    enabled: boolean;
	    mode: string;
	    kill_switch: string;
	    ignore_kill_switch_on_disconnect: boolean;
	    wfp_crash_mode: string;
	    system_dns: string;
	    portable_env_paths: boolean;
	    apps: AppConfig[];
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schema_version = source["schema_version"];
	        this.enabled = source["enabled"];
	        this.mode = source["mode"];
	        this.kill_switch = source["kill_switch"];
	        this.ignore_kill_switch_on_disconnect = source["ignore_kill_switch_on_disconnect"];
	        this.wfp_crash_mode = source["wfp_crash_mode"];
	        this.system_dns = source["system_dns"];
	        this.portable_env_paths = source["portable_env_paths"];
	        this.apps = this.convertValues(source["apps"], AppConfig);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace splittunnel {
	
	export class StatusSnapshot {
	    supported: boolean;
	    running: boolean;
	    config_path: string;
	    tunnel_state: number;
	    topology?: vpn.NetworkTopology;
	    router: router.RouterStats;
	    competing_vpns?: vpn.InterfaceInfo[];
	    warning?: string;
	
	    static createFrom(source: any = {}) {
	        return new StatusSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.supported = source["supported"];
	        this.running = source["running"];
	        this.config_path = source["config_path"];
	        this.tunnel_state = source["tunnel_state"];
	        this.topology = this.convertValues(source["topology"], vpn.NetworkTopology);
	        this.router = this.convertValues(source["router"], router.RouterStats);
	        this.competing_vpns = this.convertValues(source["competing_vpns"], vpn.InterfaceInfo);
	        this.warning = source["warning"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace updater {
	
	export class CompareSummary {
	    base_tag: string;
	    head_tag: string;
	    ahead_by: number;
	    total_commits: number;
	    compare_url: string;
	    changed_files: string[];
	    touched_seam_overlap: string[];
	
	    static createFrom(source: any = {}) {
	        return new CompareSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.base_tag = source["base_tag"];
	        this.head_tag = source["head_tag"];
	        this.ahead_by = source["ahead_by"];
	        this.total_commits = source["total_commits"];
	        this.compare_url = source["compare_url"];
	        this.changed_files = source["changed_files"];
	        this.touched_seam_overlap = source["touched_seam_overlap"];
	    }
	}
	export class DryRunResult {
	    target_ref: string;
	    clean: boolean;
	    merged_files?: string[];
	    conflict_files?: string[];
	    diff_stat?: string;
	    raw_output?: string;
	
	    static createFrom(source: any = {}) {
	        return new DryRunResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.target_ref = source["target_ref"];
	        this.clean = source["clean"];
	        this.merged_files = source["merged_files"];
	        this.conflict_files = source["conflict_files"];
	        this.diff_stat = source["diff_stat"];
	        this.raw_output = source["raw_output"];
	    }
	}
	export class ReleaseInfo {
	    tag_name: string;
	    name: string;
	    html_url: string;
	    published_at: string;
	    body: string;
	    prerelease: boolean;
	    draft: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ReleaseInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tag_name = source["tag_name"];
	        this.name = source["name"];
	        this.html_url = source["html_url"];
	        this.published_at = source["published_at"];
	        this.body = source["body"];
	        this.prerelease = source["prerelease"];
	        this.draft = source["draft"];
	    }
	}
	export class SyncAttemptResult {
	    target_ref: string;
	    dry_run: DryRunResult;
	    report_path: string;
	    report_preview: string;
	    summary: string;
	
	    static createFrom(source: any = {}) {
	        return new SyncAttemptResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.target_ref = source["target_ref"];
	        this.dry_run = this.convertValues(source["dry_run"], DryRunResult);
	        this.report_path = source["report_path"];
	        this.report_preview = source["report_preview"];
	        this.summary = source["summary"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class UpdateCheckResult {
	    upstream_repo_url: string;
	    base_tag: string;
	    latest_tag: string;
	    update_available: boolean;
	    from_cache: boolean;
	    // Go type: time
	    checked_at: any;
	    release?: ReleaseInfo;
	    compare?: CompareSummary;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateCheckResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.upstream_repo_url = source["upstream_repo_url"];
	        this.base_tag = source["base_tag"];
	        this.latest_tag = source["latest_tag"];
	        this.update_available = source["update_available"];
	        this.from_cache = source["from_cache"];
	        this.checked_at = this.convertValues(source["checked_at"], null);
	        this.release = this.convertValues(source["release"], ReleaseInfo);
	        this.compare = this.convertValues(source["compare"], CompareSummary);
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace vpn {
	
	export class InterfaceInfo {
	    if_index: number;
	    luid: number;
	    name: string;
	    description: string;
	    // Go type: netip
	    ipv4?: any;
	    // Go type: netip
	    ipv6?: any;
	    // Go type: netip
	    gateway_ipv4?: any;
	    // Go type: netip
	    gateway_ipv6?: any;
	    metric: number;
	    mtu: number;
	    is_up: boolean;
	    is_loopback: boolean;
	    is_virtual: boolean;
	    is_competing_vpn: boolean;
	
	    static createFrom(source: any = {}) {
	        return new InterfaceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.if_index = source["if_index"];
	        this.luid = source["luid"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.ipv4 = this.convertValues(source["ipv4"], null);
	        this.ipv6 = this.convertValues(source["ipv6"], null);
	        this.gateway_ipv4 = this.convertValues(source["gateway_ipv4"], null);
	        this.gateway_ipv6 = this.convertValues(source["gateway_ipv6"], null);
	        this.metric = source["metric"];
	        this.mtu = source["mtu"];
	        this.is_up = source["is_up"];
	        this.is_loopback = source["is_loopback"];
	        this.is_virtual = source["is_virtual"];
	        this.is_competing_vpn = source["is_competing_vpn"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class NetworkTopology {
	    tunnel?: InterfaceInfo;
	    physical?: InterfaceInfo;
	    competing_vpns?: InterfaceInfo[];
	    all?: InterfaceInfo[];
	
	    static createFrom(source: any = {}) {
	        return new NetworkTopology(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tunnel = this.convertValues(source["tunnel"], InterfaceInfo);
	        this.physical = this.convertValues(source["physical"], InterfaceInfo);
	        this.competing_vpns = this.convertValues(source["competing_vpns"], InterfaceInfo);
	        this.all = this.convertValues(source["all"], InterfaceInfo);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

