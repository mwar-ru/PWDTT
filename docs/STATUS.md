# STATUS — PWDTT Per-App Split Tunneling (Windows 11)

**Последнее обновление:** Реализована подзадача T02 (Settings Store)
**Активная ветка:** `feat/windows-split-tunnel` (форк `https://github.com/mwar-ru/PWDTT`)
**Синхронизация с upstream:** Базовый релиз `luminescq/PWDTT` (`v1.6.0`)

## Что уже реализовано
- Проведён анализ архитектуры `luminescq/PWDTT` (Go + Wails + WireGuard/Wintun + VK TURN/DTLS).
- Утверждены ключевые решения:
  - Технология: гибрид WinDivert 2.2 (WHQL) + user-mode WFP Kill Switch (ADR-001).
  - Политика утечек: настраиваемый Kill Switch с `fail-closed` по умолчанию (ADR-002).
  - Режимы v1: `global`, `include` (только выбранные), `exclude` (кроме выбранных).
  - Граница изоляции от upstream (ADR-004).
  - Кастомный путь `split_tunnel.json` (OneDrive + портативные `%Env%`-пути), отказ от Linux в v1 и встроенный AI-апдейтер M6 (ADR-005).
- Составлена декомпозиция на 15 независимых подзадач (`T01`–`T15`).
- **T02 — Settings Store:**
  - Реализован пакет `internal/splittunnel/settings`.
  - Реализована схема v1 (JSON настроек `split_tunnel.json`).
  - Локальный bootstrap-указатель (`%AppData%/PWDTT/split_tunnel_bootstrap.json`).
  - Поддержка кастомного пути (`SetConfigPath`) с возможностью переноса существующего файла.
  - Сворачивание и разворачивание системных путей в портативные `%Env%` переменные (`%ProgramFiles%`, `%ProgramFiles(x86)%`, `%LocalAppData%`, `%AppData%`, `%UserProfile%`, `%SystemRoot%`, `%WinDir%`).
  - Атомарная запись с retry-механизмом и fallback к кэшу Last-Known-Good (`split_tunnel_last_known_good.json`).
  - 100% прохождение unit-тестов с `-race` (`go test -race ./internal/splittunnel/settings/...`).

## Что сейчас работает
- Оригинальный функционал PWDTT не изменялся.
- Пакет `internal/splittunnel/settings` полностью функционален и протестирован.
- Готова полная проектная документация, ADR-001..005 и паспорта всех 6 модулей.

## Что ещё не реализовано
- Фаза 1: `T03` (Policy Engine), `T04` (Process Tree Windows).
- Фаза 2: `T05` (NetIf Inspector), `T06` (Upstream Adapter), `T07` (WFP Kill Switch).
- Фаза 3: `T08`–`T11` (NAT/Staging Queue, WinDivert SOCKET/NETWORK, DNS Guard).
- Фаза 4: `T15` (Upstream Sync & AI-Report), `T12`–`T14` (Wails Service, UI SplitTunnelView, CI и E2E в VM).

## Известные ограничения и проблемы
- При жёстком краше `pwdtt.exe` динамическая WFP-сессия удаляется Windows (честное ограничение описано в ADR-002; предусмотрена опция Persistent WFP + утилита сброса).
- Системный DNS (UDP/53) в Windows 11 выполняется через `svchost.exe` — используется политика VPN DNS Guard (ADR-003).

## Следующая задача
- **ID:** `T03 — Policy Engine`
- **Цель:** Реализовать модуль Policy Engine (`internal/splittunnel/appmgr`) для вычисления и применения правил маршрутизации трафика (режимы Global/Include/Exclude, проверка совпадения процессов).
- **Файлы для изучения:**
  - `AGENTS.md`
  - `docs/STATUS.md`
  - `docs/modules/application-manager.md`


