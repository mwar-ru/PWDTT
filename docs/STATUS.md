# STATUS — PWDTT Per-App Split Tunneling (Windows 11)

**Последнее обновление:** Реализована подзадача T08 (TrafficRouter Contract, NAT Table & Staging Queue)
**Активная ветка:** `feat/windows-split-tunnel` (форк `https://github.com/mwar-ru/PWDTT`)
**Синхронизация с upstream:** Базовый релиз `luminescq/PWDTT` (`v1.6.0`)

## Что уже реализовано
- **Фаза 0 & Фаза 1 завершены (`T01`–`T04`):** документация, Settings Store (OneDrive + `%Env%`), Policy Engine + Anti-Loop, Windows Process Tree Resolver + Scanner.
- **Фаза 2 завершена (`T05`–`T07`):** Windows NetIf Inspector (`pwdtt-wg`), PWDTT Adapter + минимальный шов в `backend/wg.go`, User-mode WFP Kill Switch & IPv6 Leak Guard (ABI проверен на Windows 11 x64).
- **T08 — Ядро маршрутизатора (`internal/splittunnel/router`):**
  - `interface.go`: контракт `TrafficRouter`, `RouterStats`, `FlowKey`, `SocketKey`.
  - `nat_table.go`: потокобезопасная ограниченная по памяти SNAT/DNAT таблица (16k сессий, разруливание коллизий портов, отдельные TTL для TCP 120с и UDP/QUIC 45с).
  - `staging_queue.go`: буфер ожидания первого пакета (25 мс) для устранения гонки `SOCKET` vs `NETWORK` (Risk R01).

## Что сейчас работает
- Все модули `settings`, `appmgr`, `vpn` и `router` (`wfp` + `nat_table` + `staging_queue`) проходят `go test -race` и кросс-компилируются под Windows x64.

## Что ещё не реализовано
- Фаза 3: `T09` (WinDivert 2.2 Loader + SOCKET listener + TCP/UDP table fallback), `T10` (WinDivert NETWORK SNAT/DNAT + Checksum + Orchestrator), `T11` (DNS Guard).
- Фаза 4: `T15` (Upstream Sync & AI-Report), `T12`–`T14` (Wails Service, UI SplitTunnelView, CI и E2E в VM).

## Следующая задача
- **ID:** `T09 — WinDivert 2.2 loader и SOCKET listener (отслеживание PID для TCP и UDP/QUIC)`
- **Цель:** Реализовать проверку SHA-256 официального WinDivert 2.2, загрузку `WinDivert.dll`, чтение `WINDIVERT_LAYER_SOCKET` и резервный опрос `GetExtendedTcpTable`/`GetExtendedUdpTable`.
- **Файлы для изучения:**
  - `AGENTS.md`
  - `docs/STATUS.md`
  - `docs/DECISIONS/ADR-001-routing-tech.md`
  - `docs/modules/traffic-router.md`
