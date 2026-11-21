# STATUS — PWDTT Per-App Split Tunneling (Windows 11)

**Последнее обновление:** Реализована подзадача T05 (Windows Network Interface Inspector)
**Активная ветка:** `feat/windows-split-tunnel` (форк `https://github.com/mwar-ru/PWDTT`)
**Синхронизация с upstream:** Базовый релиз `luminescq/PWDTT` (`v1.6.0`)

## Что уже реализовано
- Проведён анализ архитектуры `luminescq/PWDTT` (Go + Wails + WireGuard/Wintun + VK TURN/DTLS).
- Утверждены ключевые решения (ADR-001..005): WinDivert 2.2 (WHQL) + user-mode WFP Kill Switch (`fail-closed` по умолчанию), 3 режима (`global`/`include`/`exclude`), кастомный путь `split_tunnel.json` (OneDrive + `%Env%`), встроенный AI-апдейтер M6, Windows 11 only в v1.
- **Фаза 0 & Фаза 1 завершены (`T01`–`T04`):** документация, Settings Store (OneDrive + `%Env%`), Policy Engine + Anti-Loop, Windows Process Tree Resolver + Scanner (проверено на живой Windows 11).
- **T05 — Инспектор сетевых интерфейсов (`internal/splittunnel/vpn`):**
  - `types.go`: `TunnelState`, `InterfaceInfo`, `NetworkTopology`, `SelectTopology`, детектор конкурирующих VPN (`ClassifyVirtualAdapter` отличает VPN от WSL/Docker/Hyper-V).
  - `netif_windows.go`: WinAPI `GetAdaptersAddresses` (`IfIdx`, `LUID`, IPv4/IPv6, шлюзы, DNS, метрика).
  - `netif_stub.go`: кроссплатформенная заглушка для Linux/macOS.

## Что сейчас работает
- Оригинальный функционал PWDTT не изменялся.
- Пакеты `settings`, `appmgr` и `vpn` проходят unit-тесты с `-race` и кросс-компилируются под Windows x64.

## Что ещё не реализовано
- Фаза 2: `T06` (Upstream Adapter), `T07` (WFP Kill Switch).
- Фаза 3: `T08`–`T11` (NAT/Staging Queue, WinDivert SOCKET/NETWORK, DNS Guard).
- Фаза 4: `T15` (Upstream Sync & AI-Report), `T12`–`T14` (Wails Service, UI SplitTunnelView, CI и E2E в VM).

## Известные ограничения и проблемы
- При жёстком краше `pwdtt.exe` динамическая WFP-сессия удаляется Windows (честное ограничение описано в ADR-002; предусмотрена опция Persistent WFP + утилита сброса).
- Системный DNS (UDP/53) в Windows 11 выполняется через `svchost.exe` — используется политика VPN DNS Guard (ADR-003).

## Следующая задача
- **ID:** `T06 — Минимальный адаптер интеграции с туннелем upstream PWDTT`
- **Цель:** Подключить события жизненного цикла туннеля и управление default route Wintun с минимальным патчем в `backend/wg_windows_init.go` и `backend/app.go`.
- **Файлы для изучения:**
  - `backend/wg_windows_init.go`
  - `backend/wg.go`
  - `backend/app.go`
