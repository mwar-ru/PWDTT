# Паспорт модуля: Application Manager (M2)

- **Каталог:** `internal/splittunnel/appmgr/`
- **Зависимости:** `M1 Settings Store`.

## Публичный интерфейс
- `ResolvePID(pid uint32) (ProcessIdentity, bool)`
- `MatchPolicy(pid uint32, cfg settings.Config) RouteDecision` (`RouteVPN`, `RouteDirect`, `RouteBypassSelf`)
- `ListRunningApps() ([]DiscoveredApp, error)`

## Ключевые инварианты
- Собственный процесс `pwdtt.exe` и его дочерние процессы всегда возвращают `RouteBypassSelf` (защита от петли маршрутизации).
- Кэш PID валидируется по `CreationTime` процесса.
- Поддерживается наследование правил по дереву `PPID` для `firefox.exe`, `chrome.exe`, `msedge.exe` и Electron-приложений.

