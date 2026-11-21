# STATUS — PWDTT Per-App Split Tunneling (Windows 11)

**Последнее обновление:** Реализована подзадача T07 (User-mode WFP Kill Switch & IPv6 Leak Guard)
**Активная ветка:** `feat/windows-split-tunnel` (форк `https://github.com/mwar-ru/PWDTT`)
**Синхронизация с upstream:** Базовый релиз `luminescq/PWDTT` (`v1.6.0`)

## Что уже реализовано
- **Фаза 0 & Фаза 1 завершены (`T01`–`T04`):** документация, Settings Store (OneDrive + `%Env%`), Policy Engine + Anti-Loop, Windows Process Tree Resolver + Scanner.
- **Фаза 2 завершена (`T05`–`T07`):**
  - `T05`: WinAPI инспектор сетевых интерфейсов (`pwdtt-wg` + физический NIC + детектор конкурирующих VPN).
  - `T06`: Адаптер `PWDTTAdapter` + минимальный шов в `backend/wg.go` (~25 строк, нулевое окно утечки при teardown).
  - `T07`: User-mode WFP Kill Switch (`internal/splittunnel/router/wfp_plan.go`, `wfp_guard_windows.go`, `wfp_cleanup_windows.go`, `wfp_guard_stub.go`) с точным x64 ABI `fwpuclnt.dll`, транзакциями, `dynamic`/`persistent_guard`, защитой от IPv6-утечек и `ResetWfpRules()`.

## Что сейчас работает
- Все модули Фаз 1 и 2 (`settings`, `appmgr`, `vpn`, `router/wfp`) проходят `go test -race` и кросс-компилируются под Windows x64; оригинальные `./tests/...` PWDTT зелёные.

## Что ещё не реализовано
- Фаза 3: `T08` (Контракт `TrafficRouter`, NAT-таблица, Staging Queue), `T09` (WinDivert SOCKET), `T10` (WinDivert NETWORK + SNAT/DNAT), `T11` (DNS Guard).
- Фаза 4: `T15` (Upstream Sync & AI-Report), `T12`–`T14` (Wails Service, UI SplitTunnelView, CI и E2E в VM).

## Следующая задача
- **ID:** `T08 — Контракт TrafficRouter, stateful NAT-таблица и Staging Queue`
- **Цель:** Реализовать платформонезависимое ядро сопоставления потоков (TCP/UDP/QUIC), таблицу SNAT/DNAT с TTL/LRU-очисткой и буфер ожидания первого пакета (`StagingQueue`, 15–30 мс) + бенчмарки и `-race` тесты.
- **Файлы для изучения:**
  - `AGENTS.md`
  - `docs/STATUS.md`
  - `docs/modules/traffic-router.md`
