# Паспорт модуля: Settings Store (M1)

- **Каталог:** `internal/splittunnel/settings/`
- **Зависимости:** только стандартная библиотека Go.
- **Локальный указатель:** `%AppData%/PWDTT/split_tunnel_bootstrap.json` → хранит путь к целевому `split_tunnel.json` (по умолчанию `%AppData%/PWDTT/split_tunnel.json`, можно указать папку OneDrive).

## Схема v1 (JSON с портативными путями)
```json
{
  "schema_version": 1,
  "enabled": true,
  "mode": "include",
  "kill_switch": "fail_closed",
  "wfp_crash_mode": "dynamic",
  "system_dns": "tunnel_dns",
  "portable_env_paths": true,
  "apps": [
    { "name": "Firefox", "exe_path": "%ProgramFiles%\\Mozilla Firefox\\firefox.exe", "match_children": true },
    { "name": "Microsoft Edge", "exe_path": "%ProgramFiles(x86)%\\Microsoft\\Edge\\Application\\msedge.exe", "match_children": true }
  ]
}
```

## Критерии завершения (T02)
- Выбор/перенос кастомного пути конфига через `SetConfigPath` с переносом существующего файла.
- Сворачивание/разворачивание `%ProgramFiles%`, `%LocalAppData%`, `%AppData%`, `%UserProfile%`.
- Атомарная запись с retry при блокировке OneDrive + локальный кэш last-known-good.
- 100% покрытие unit-тестами (`go test -race`).

