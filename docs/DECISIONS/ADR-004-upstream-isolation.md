# ADR-004: Изоляция от upstream luminescq/PWDTT

- **Статус:** Принято

## Стратегия
- Ветка `main` остаётся чистым зеркалом `upstream/main`.
- Разработка ведётся в `feat/windows-split-tunnel` с коммитами `split-tunnel(<module>): ...`.
- Все интеграционные изменения в файлах upstream ограничены 4 точками (`wg_windows_init.go`, `app.go`, `main.go`, `frontend/src/App.tsx`) и суммарно занимают <80 строк.

## Точки, которые всё равно могут потребовать адаптации при обновлении upstream
1. Рефакторинг инициализации Wintun или мьютекса `stateMu` в `wg_windows_init.go` / `app.go` → правится только `internal/splittunnel/vpn/pwdtt_adapter.go`.
2. Обновление Wails v2 → v3 в `main.go` → правится только регистрация `SplitTunnelService`.
3. Редизайн меню в `frontend/src/App.tsx` → правится только ссылка на `<SplitTunnelView />`.

