# STATUS — PWDTT Per-App Split Tunneling (Windows 11)

**Последнее обновление:** Реализована подзадача T09 (WinDivert 2.2 Loader, SHA-256 & SOCKET Tracker)
**Активная ветка:** `feat/windows-split-tunnel` (форк `https://github.com/mwar-ru/PWDTT`)
**Синхронизация с upstream:** Базовый релиз `luminescq/PWDTT` (`v1.6.0`)

## Что уже реализовано
- **Фаза 0 & Фаза 1 завершены (`T01`–`T04`):** документация, Settings Store (OneDrive + `%Env%`), Policy Engine + Anti-Loop, Windows Process Tree Resolver + Scanner.
- **Фаза 2 завершена (`T05`–`T07`):** Windows NetIf Inspector (`pwdtt-wg`), PWDTT Adapter + минимальный шов в `backend/wg.go`, User-mode WFP Kill Switch & IPv6 Leak Guard (ABI проверен на Windows 11 x64).
- **T08:** Контракт `TrafficRouter`, потокобезопасная NAT-таблица (TCP 120с / UDP-QUIC 45с) и `StagingQueue` (25 мс).
- **T09 — WinDivert 2.2 Loader и SOCKET Tracker (`internal/splittunnel/router`):**
  - `socket_table.go`: потокобезопасная карта `(Proto, LocalPort) → PID` + проверка целостности SHA-256.
  - `windivert_loader_windows.go`: 80-байтовый ABI WinDivert 2.2, безопасная загрузка DLL и диагностика блокировок AV/EDR.
  - `windivert_socket_windows.go`: поток `WINDIVERT_LAYER_SOCKET` + резервный опрос `GetExtendedTcpTable`/`GetExtendedUdpTable`.
  - `third_party/windivert/CHECKSUMS.sha256`: шаблон фиксации хешей официального WHQL WinDivert 2.2.2.

## Что сейчас работает
- Все модули `settings`, `appmgr`, `vpn` и `router` (`wfp`, `nat_table`, `staging_queue`, `socket_table`, `windivert_loader/socket`) проходят `go test -race` и кросс-компилируются под Windows x64.

## Что ещё не реализовано
- Фаза 3: `T10` (WinDivert NETWORK SNAT/DNAT + IPv4/TCP/UDP Checksum + Orchestrator), `T11` (DNS Guard).
- Фаза 4: `T15` (Upstream Sync & AI-Report), `T12`–`T14` (Wails Service, UI SplitTunnelView, CI и E2E в VM).

## Следующая задача
- **ID:** `T10 — WinDivert NETWORK packet engine (SNAT/DNAT, контрольные суммы и Orchestrator)`
- **Цель:** Реализовать разбор/перезапись IPv4 TCP/UDP пакетов (SNAT/DNAT), пересчёт контрольных сумм, пакетный цикл `WINDIVERT_LAYER_NETWORK` с `Impostor`/anti-loop и оркестратор `WindowsTrafficRouter`.
- **Файлы для изучения:**
  - `AGENTS.md`
  - `docs/STATUS.md`
  - `docs/modules/traffic-router.md`
