# Паспорт модуля: UI Integration (M5)

- **Каталоги:** `internal/splittunnel/service_*.go`, `frontend/src/splittunnel/`
- **Зависимости:** `M1 Settings`, `M2 Application Manager`, `M4 Traffic Router`.

## Принцип разделения
- UI не содержит логики маршрутизации и не обращается к WinAPI напрямую.
- Все изменения настроек передаются через `SplitTunnelService.SaveConfig(cfg)`, а актуальный статус защиты и конфликты приходят через Wails-событие `splittunnel:status`.
- Фильтрация по доменам, сайтам и IP намеренно отсутствует.

