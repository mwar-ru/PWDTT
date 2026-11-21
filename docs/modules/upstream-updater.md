# Паспорт модуля: Upstream Sync & AI-Report Engine (M6)

- **Каталог:** `internal/splittunnel/updater/`
- **Зависимости:** `M1 Settings Store`.

## Метаданные сборки (ldflags)
- `BaseUpstreamTag` (текущая база: `v1.6.0`, обновляется при синхронизации)
- `BaseUpstreamCommit` (короткий SHA базового коммита)
- `UpstreamReleasesURL` (`https://github.com/luminescq/PWDTT/releases`)
- `ForkRepoURL` (`https://github.com/mwar-ru/PWDTT`)

## Сценарии работы
1. **Проверка релиза:** запрос к GitHub Releases API + Compare API для списка изменённых файлов в зоне 4 швов (`wg_windows_init.go`, `app.go`, `main.go`, `frontend/src/App.tsx`).
2. **Попытка автообновления:**
   - Если вышел бинарный релиз форка под новый upstream → скачивание и проверка SHA-256.
   - Если указана локальная папка исходников (Git + Go) → пробный `git merge` во временном `worktree` + `go test ./internal/splittunnel/...`.
3. **Генерация AI-отчёта:** запись в `%AppData%/PWDTT/reports/upstream-update-<from>-to-<to>.md` (diff конфликта, лог сборки, `STATUS.md` и готовый промпт для ИИ-агента) + кнопка открытия файла в системном редакторе (`ShellExecute`).

