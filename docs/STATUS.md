# STATUS — PWDTT Per-App Split Tunneling (Windows 11)

**Последнее обновление:** Реализована подзадача T10 (WinDivert NETWORK SNAT/DNAT, Checksums & Orchestrator)
**Активная ветка:** `feat/windows-split-tunnel` (форк `https://github.com/mwar-ru/PWDTT`)
**Синхронизация с upstream:** Базовый релиз `luminescq/PWDTT` (`v1.6.0`)

## Что уже реализовано
- **Фаза 0 & Фаза 1 завершены (`T01`–`T04`):** документация, Settings Store (OneDrive + `%Env%`), Policy Engine + Anti-Loop, Windows Process Tree Resolver + Scanner.
- **Фаза 2 завершена (`T05`–`T07`):** Windows NetIf Inspector (`pwdtt-wg`), PWDTT Adapter + минимальный шов в `backend/wg.go`, User-mode WFP Kill Switch & IPv6 Leak Guard (ABI проверен на Windows 11 x64).
- **Фаза 3 (в процессе: `T08`–`T10` готовы):**
  - `T08`: Контракт `TrafficRouter`, NAT-таблица и `StagingQueue` (25 мс).
  - `T09`: WinDivert 2.2 Loader (SHA-256), `WINDIVERT_LAYER_SOCKET` и `GetExtendedTcpTable`/`UdpTable` (проверено на живой Windows 11).
  - `T10`: `packet_rewrite.go` (IPv4/TCP/UDP SNAT/DNAT + RFC 791/793/768 контрольные суммы), `windivert_network_windows.go` (`!loopback and !impostor`, fast-path NAT, staging drain, fail-closed drop), `orchestrator_windows.go` + `orchestrator_stub.go`.

## Что сейчас работает
- Полный цикл `WindowsTrafficRouter` (WFP + WinDivert SOCKET + WinDivert NETWORK + NAT + Staging) компилируется под Windows x64 и проходит `-race` тесты на Linux/Windows.

## Что ещё не реализовано
- Фаза 3: `T11` (DNS Guard — перехват системного UDP/53 `svchost.exe` + сохранение браузерного DoH).
- Фаза 4: `T15` (Upstream Sync & AI-Report), `T12`–`T14` (Wails Service, UI SplitTunnelView, CI и E2E в VM).

## Следующая задача
- **ID:** `T11 — Модуль DNS Guard (svchost.exe порт 53 + браузерный DoH)`
- **Цель:** Реализовать `dns_guard.go` и подключить `PacketHook` в `WindowsTrafficRouter`, чтобы в политике `tunnel_dns` системные UDP/53 запросы перенаправлялись в DNS туннеля (с DNAT ответа обратно), без петли для DNS самого PWDTT.
- **Файлы для изучения:**
  - `AGENTS.md`
  - `docs/STATUS.md`
  - `docs/DECISIONS/ADR-003-dns-quic-browsers.md`
