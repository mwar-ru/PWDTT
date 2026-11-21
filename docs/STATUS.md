# STATUS — PWDTT Per-App Split Tunneling (Windows 11)

**Последнее обновление:** Реализована подзадача T06 (Upstream PWDTT Tunnel Adapter & Minimal Seam)
**Активная ветка:** `feat/windows-split-tunnel` (форк `https://github.com/mwar-ru/PWDTT`)
**Синхронизация с upstream:** Базовый релиз `luminescq/PWDTT` (`v1.6.0`)

## Что уже реализовано
- **Фаза 0 & Фаза 1 завершены (`T01`–`T04`):** документация, Settings Store (OneDrive + `%Env%`), Policy Engine + Anti-Loop, Windows Process Tree Resolver + Scanner.
- **T05 — Инспектор сетевых интерфейсов (`internal/splittunnel/vpn`):** WinAPI `GetAdaptersAddresses`, выбор физического NIC и Wintun (`pwdtt-wg`), детектор конкурирующих VPN.
- **T06 — Адаптер интеграции с туннелем PWDTT (`internal/splittunnel/vpn/pwdtt_adapter.go` + шов в `backend/wg.go`):**
  - Интерфейс `TunnelHooks` и потокобезопасный `PWDTTAdapter` с подпиской на `TunnelEvent` (`Starting`/`Up`/`Down`/`Error`) и автоматическим чтением топологии `pwdtt-wg`.
  - Минимальный шов в `backend/wg.go` (~25 строк): в режиме `include` Wintun поднимается с метрикой `9999` без захвата default route (`0.0.0.0/0`, `0.0.0.0/1`, `128.0.0.0/1`, `::/0`), а `StateDown` отправляется ДО закрытия Wintun (нулевое окно утечки для Kill Switch).
  - При незарегистрированных хуках или в `global`-режиме поведение PWDTT 100% идентично оригиналу.

## Что сейчас работает
- Оригинальные тесты PWDTT (`./tests/...`) и все наши модули (`settings`, `appmgr`, `vpn`) проходят `go test -race` и компилируются под Windows x64.

## Что ещё не реализовано
- Фаза 2: `T07` (User-mode WFP Kill Switch & IPv6 leak protection).
- Фаза 3: `T08`–`T11` (NAT/Staging Queue, WinDivert SOCKET/NETWORK, DNS Guard).
- Фаза 4: `T15` (Upstream Sync & AI-Report), `T12`–`T14` (Wails Service, UI SplitTunnelView, CI и E2E в VM).

## Следующая задача
- **ID:** `T07 — User-mode WFP Kill Switch и защита от IPv6-утечек`
- **Цель:** Реализовать управление WFP фильтрами (`fwpuclnt.dll`) по `ALE_APP_ID` с выделенным Provider/Sublayer GUID и безопасной очисткой (`ResetWfpRules`).
- **Файлы для изучения:**
  - `AGENTS.md`
  - `docs/STATUS.md`
  - `docs/DECISIONS/ADR-002-fail-closed-killswitch.md`
  - `docs/modules/traffic-router.md`
