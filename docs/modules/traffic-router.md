# Паспорт модуля: Traffic Router (M4)

- **Каталог:** `internal/splittunnel/router/`
- **Зависимости:** `M1 Settings`, `M2 Application Manager`, `M3 VPN Integration`.

## Внутренняя структура
1. `interface.go` — абстрактный `TrafficRouter` (для будущей замены на WFP Callout в v2).
2. `nat_table.go` + `staging_queue.go` — платформонезависимые, тестируемые без прав админа.
3. `windivert_socket_windows.go` — захват `ProcessId` для TCP и UDP/QUIC.
4. `windivert_network_windows.go` — SNAT/DNAT, контрольные суммы, реинжекция по `IfIdx`.
5. `wfp_guard_windows.go` — Fail-Closed Kill Switch (`ALE_AUTH_CONNECT_V4/V6`) + очистка по Provider GUID.
6. `dns_guard_windows.go` — защита системного DNS (порт 53).

