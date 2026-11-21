# STATUS — PWDTT Per-App Split Tunneling (Windows 11)

**Последнее обновление:** Реализована подзадача T03 (Path Normalization & Policy Engine)
**Активная ветка:** `feat/windows-split-tunnel` (форк `https://github.com/mwar-ru/PWDTT`)
**Синхронизация с upstream:** Базовый релиз `luminescq/PWDTT` (`v1.6.0`)

## Что уже реализовано
- Проведён анализ архитектуры `luminescq/PWDTT` (Go + Wails + WireGuard/Wintun + VK TURN/DTLS).
- Утверждены ключевые решения (ADR-001..005): WinDivert 2.2 (WHQL) + user-mode WFP Kill Switch (`fail-closed` по умолчанию), 3 режима (`global`/`include`/`exclude`), кастомный путь `split_tunnel.json` (OneDrive + `%Env%`), встроенный AI-апдейтер M6, Windows 11 only в v1.
- **T01 — Фундамент и документация:** настроены `fork`/`upstream`, ветка `feat/windows-split-tunnel`, `AGENTS.md` и `docs/*`.
- **T02 — Settings Store (`internal/splittunnel/settings`):** схема v1, bootstrap-указатель, кастомный путь (OneDrive с сохранением существующего конфига при подключении второго ПК), портативные `%Env%`-пути, атомарная запись с retry и LKG-кэш.
- **T03 — Policy Engine (`internal/splittunnel/appmgr`):**
  - `path_normalize.go`: каноническая нормализация Windows-путей (`\\?\`, `\??\`, `..`, `%Env%`, `\Device\HarddiskVolumeX`).
  - `policy_eval.go`: потокобезопасный `PolicyEngine` для `global`/`include`/`exclude`, наследование дочерними процессами (`MatchChildren`) и жёсткая anti-loop защита для `pwdtt.exe`.
  - Матричные и конкурентные тесты (`-race`) + проверка кросс-компиляции `GOOS=windows`.

## Что сейчас работает
- Оригинальный функционал PWDTT не изменялся.
- Пакеты `internal/splittunnel/settings` и `internal/splittunnel/appmgr` полностью протестированы (`go test -race`) и собираются под Windows x64.

## Что ещё не реализовано
- Фаза 1: `T04` (Windows Process Tree & Scanner).
- Фаза 2: `T05` (NetIf Inspector), `T06` (Upstream Adapter), `T07` (WFP Kill Switch).
- Фаза 3: `T08`–`T11` (NAT/Staging Queue, WinDivert SOCKET/NETWORK, DNS Guard).
- Фаза 4: `T15` (Upstream Sync & AI-Report), `T12`–`T14` (Wails Service, UI SplitTunnelView, CI и E2E в VM).

## Известные ограничения и проблемы
- При жёстком краше `pwdtt.exe` динамическая WFP-сессия удаляется Windows (честное ограничение описано в ADR-002; предусмотрена опция Persistent WFP + утилита сброса).
- Системный DNS (UDP/53) в Windows 11 выполняется через `svchost.exe` — используется политика VPN DNS Guard (ADR-003).

## Следующая задача
- **ID:** `T04 — Трекер процессов и дерева дочерних PID на Windows`
- **Цель:** Реализовать `ProcessResolver` (PID → EXE-путь + `PPID` + `CreationTime` кэш) и сканер запущенных приложений (`internal/splittunnel/appmgr/process_tree_windows.go`, `process_scanner_windows.go`).
- **Файлы для изучения:**
  - `AGENTS.md`
  - `docs/STATUS.md`
  - `docs/modules/application-manager.md`
