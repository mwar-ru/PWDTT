# PWDTT Per-App Split Tunneling — Implementation Status

- **Fork Branch:** `feat/windows-split-tunnel`
- **Upstream Base:** `luminescq/PWDTT` (`v1.6.0`)
- **Platform Target:** Windows 11 x64 (Linux/macOS unaffected via build-tagged stubs)

## Task Progress (15 / 15 Completed)

| ID | Module | Status | Key Artifacts |
|---|---|---|---|
| T01 | Docs & ADRs | DONE | `AGENTS.md`, `docs/ARCHITECTURE.md`, `docs/DECISIONS/ADR-001..005` |
| T02 | Settings Store + OneDrive | DONE | `internal/splittunnel/settings/settings.go`, `*_test.go` |
| T03 | Path Normalization & Policy | DONE | `internal/splittunnel/appmgr/path_normalize.go`, `policy_eval.go` |
| T04 | Windows Process Tree | DONE | `internal/splittunnel/appmgr/process_tree_windows.go`, `process_types.go` |
| T05 | Network Interface Inspector | DONE | `internal/splittunnel/vpn/netif_windows.go`, `types.go` |
| T06 | PWDTT WireGuard Adapter | DONE | `internal/splittunnel/vpn/pwdtt_adapter.go`, minimal seam in `backend/wg.go` |
| T07 | WFP Kill Switch & IPv6 Guard | DONE | `internal/splittunnel/router/wfp_plan.go`, `wfp_guard_windows.go`, `wfp_cleanup_windows.go` |
| T08 | NAT Table & Staging Queue | DONE | `internal/splittunnel/router/interface.go`, `nat_table.go`, `staging_queue.go` |
| T09 | WinDivert 2.2 Loader & SOCKET | DONE | `internal/splittunnel/router/socket_table.go`, `windivert_loader_windows.go`, `windivert_socket_windows.go` |
| T10 | WinDivert NETWORK & Rewrite | DONE | `internal/splittunnel/router/packet_rewrite.go`, `windivert_network_windows.go`, `orchestrator_windows.go` |
| T11 | DNS Guard (`svchost` UDP/53) | DONE | `internal/splittunnel/router/dns_guard.go`, `dns_guard_windows.go` |
| T12 | Wails SplitTunnelService | DONE | `internal/splittunnel/service.go`, `service_windows.go`, `main_windows.go` |
| T13 | React UI + OneDrive + Sync | DONE | `frontend/src/lib/splitTunnelApi.ts`, `frontend/src/pages/SplitTunnel.tsx`, `App.tsx`, `Sidebar.tsx` |
| T14 | CI, VM E2E Script & Guide | DONE | `.github/workflows/split-tunnel-ci.yml`, `scripts/verify_split_tunnel_vm.ps1`, `docs/VM_TESTING_GUIDE.md` |
| T15 | Upstream Sync & AI Report | DONE | `internal/splittunnel/updater/*`, `internal/splittunnel/service_updater.go` |

## Minimal Upstream Seams

1. `backend/wg.go` — `vpn.ShouldSkipDefaultRoute()` + `vpn.NotifyTunnelState(...)`
2. `main_windows.go` — bind & lifecycle hooks for `splittunnel.Service`
3. `frontend/src/App.tsx` — `<Route path="/split-tunnel" element={<SplitTunnel />} />`
4. `frontend/src/components/Sidebar.tsx` — conditional Split Tunnel nav button (Windows only)

## Next Milestone

- Live E2E verification inside a Windows 11 VM following `docs/VM_TESTING_GUIDE.md` with official WHQL-signed WinDivert 2.2.2 binaries.
