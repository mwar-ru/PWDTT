# ROADMAP — План поэтапной разработки

## Граф зависимостей

```
T01 (Docs & Git)
 ├──► T02 (Settings) ──► T03 (Policy) ──► T04 (Process Tree) ──┐
 │                          │                                  ├──► T07 (WFP Guard) ──┐
 │                          └──► T08 (NAT & Staging Queue) ────┼──► T09 (WinDivert    ├──► T10 (Network Engine) ──► T11 (DNS Guard) ──► T12 (Wails Service) ──► T13 (UI) ──► T14 (CI & VM E2E)
 └──► T05 (NetIf Inspector) ──► T06 (Upstream Adapter) ────────┴──── SOCKET) ─────────┘
```

## Перечень фаз и подзадач

- **Фаза 0 — Фундамент:**
  - `T01`: Настройка fork/upstream и развёртывание `AGENTS.md` + `docs/*`.
- **Фаза 1 — Settings & App Manager (можно тестировать без админ-прав):**
  - `T02`: `Settings Store` (`split_tunnel.json`, schema v1, atomic save).
  - `T03`: Нормализация путей EXE и чистый `Policy Engine` (Global/Include/Exclude + anti-loop).
  - `T04`: Трекер процессов и дерева дочерних PID на Windows (Firefox/Chrome/Edge).
- **Фаза 2 — VPN Integration & WFP Kill Switch:**
  - `T05`: Инспектор интерфейсов Windows (Wintun + физический NIC, метрики, детекция других VPN).
  - `T06`: Адаптер `pwdtt_adapter.go` + минимальные хуки в `wg_windows_init.go` и `app.go`.
  - `T07`: User-mode WFP Kill Switch (`ALE_AUTH_CONNECT_V4/V6`, блокировка IPv6 при IPv4-only туннеле).
- **Фаза 3 — WinDivert Traffic Router & DNS/QUIC:**
  - `T08`: Интерфейс `TrafficRouter`, потокобезопасная NAT-таблица (TCP/UDP/QUIC) и Staging Queue.
  - `T09`: Загрузчик WinDivert 2.2 (SHA-256) + `WINDIVERT_LAYER_SOCKET` listener.
  - `T10`: `WINDIVERT_LAYER_NETWORK` packet engine (SNAT/DNAT, checksums, реинжекция по `IfIdx`).
  - `T11`: `DNS Guard` (обработка `svchost.exe:53` + совместимость с браузерным DoH).
- **Фаза 4 — Upstream Updater, UI Integration, CI и E2E:**
  - `T15`: `Upstream Sync & AI-Report` (базовый релиз, проверка GitHub API, попытка обновления, AI-отчёт и открытие в редакторе).
  - `T12`: `SplitTunnelService` (Windows + `!windows` stub) и регистрация в `main.go`.
  - `T13`: UI-экран `SplitTunnelView.tsx` (выбор пути JSON + блок обновлений upstream) и подключение в `App.tsx`.
  - `T14`: GitHub Actions CI, PowerShell-скрипт E2E-проверки в Windows 11 VM и аудит утечек.

