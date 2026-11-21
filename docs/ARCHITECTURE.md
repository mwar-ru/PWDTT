# Архитектура PWDTT Per-App Split Tunneling (Windows 11)

## 1. Принцип изоляции от upstream PWDTT

Оригинальный `luminescq/PWDTT` отвечает за Wails-оболочку, авторизацию VK, парсинг `wdtt://`/`qwdtt://`, обфускацию RTP/DTLS и поднятие WireGuard/Wintun интерфейса.
Наш функционал изолирован в пакете `internal/splittunnel/` и каталоге `frontend/src/splittunnel/` и подключается через **3 минимальных шва**:

1. **Шов туннеля (`wg_windows_init.go`, `app.go`):** передача событий жизненного цикла (`Starting`, `Up`, `Reconnecting`, `Down`) и опциональный пропуск установки глобального маршрута `0.0.0.0/0` в режиме `Include`.
2. **Шов Wails (`main.go`):** регистрация отдельного сервиса `SplitTunnelService` в `Bind` и вызов `Shutdown()`.
3. **Шов UI (`frontend/src/App.tsx`):** подключение пункта навигации и рендер `<SplitTunnelView />` на Windows.

## 2. Модульная схема

```
[UI Integration (M5)] ──► [Settings Store (M1)]
         │                         │
         ▼                         ▼
[Traffic Router (M4)] ◄── [Application Manager (M2)]
         ▲
         │
[VPN Integration (M3)] ◄── (Шов upstream: wg_windows_init.go / app.go)
```

## 3. Краткое назначение модулей

| Модуль | Каталог | Ответственность |
| :--- | :--- | :--- |
| **M1. Settings Store** | `internal/splittunnel/settings/` | Выбор кастомного пути (OneDrive), портативные `%Env%`-пути, `split_tunnel.json` v1, атомарная запись |
| **M2. Application Manager** | `internal/splittunnel/appmgr/` | Нормализация EXE-путей, дерево дочерних PID браузеров, оценка правил + anti-loop |
| **M3. VPN Integration** | `internal/splittunnel/vpn/` | Чтение Wintun/Phys NIC (`IfIdx`, `LUID`, `IP`), адаптер к PWDTT, health-check |
| **M4. Traffic Router** | `internal/splittunnel/router/` | WinDivert `SOCKET`/`NETWORK` (SNAT/DNAT), Staging Queue, user-mode WFP Kill Switch, DNS Guard |
| **M5. UI Integration** | `internal/splittunnel/service_*.go`, `frontend/src/splittunnel/` | Wails-фасад, Windows-only сборка + stub для Linux/macOS, экран управления и блок обновлений |
| **M6. Upstream Sync & AI-Report** | `internal/splittunnel/updater/` | Фиксация релиза upstream, проверка новых релизов, попытка автообновления и генерация отчёта для ИИ-агента |

## 4. Поддерживаемые режимы v1

- **`global` (Весь трафик):** штатное поведение PWDTT, WinDivert не активируется.
- **`include` (Только выбранные):** физический адаптер остаётся маршрутом по умолчанию; WinDivert перенаправляет в Wintun только трафик выбранных приложений (и их дочерних процессов).
- **`exclude` (Кроме выбранных):** Wintun остаётся маршрутом по умолчанию; WinDivert перенаправляет на физический адаптер только трафик приложений-исключений.

