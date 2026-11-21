# STATUS — PWDTT Per-App Split Tunneling (Windows 11)

**Последнее обновление:** Этап 1 завершён (Исследование и проектирование)
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

## Что сейчас работает
- Оригинальный функционал PWDTT не изменялся.
- Готова полная проектная документация, ADR-001..005 и паспорта всех 6 модулей.

## Что ещё не реализовано
- Фаза 0: `T01` (инициализация ветки и перенос `AGENTS.md` / `docs/` в fork).
- Фаза 1: `T02` (Settings Store + OneDrive), `T03` (Policy Engine), `T04` (Process Tree Windows).
- Фаза 2: `T05` (NetIf Inspector), `T06` (Upstream Adapter), `T07` (WFP Kill Switch).
- Фаза 3: `T08`–`T11` (NAT/Staging Queue, WinDivert SOCKET/NETWORK, DNS Guard).
- Фаза 4: `T15` (Upstream Sync & AI-Report), `T12`–`T14` (Wails Service, UI SplitTunnelView, CI и E2E в VM).

## Известные ограничения и проблемы
- При жёстком краше `pwdtt.exe` динамическая WFP-сессия удаляется Windows (честное ограничение описано в ADR-002; предусмотрена опция Persistent WFP + утилита сброса).
- Системный DNS (UDP/53) в Windows 11 выполняется через `svchost.exe` — используется политика VPN DNS Guard (ADR-003).

## Следующая задача
- **ID:** `T01` (затем `T02 — Settings Store`)
- **Цель:** Создать ветку `feat/windows-split-tunnel`, подключить remote `upstream` и перенести пакет документации `AGENTS.md` + `docs/*`.
- **Файлы для изучения:**
  - `AGENTS.md`
  - `docs/ROADMAP.md` (раздел `T01` и `T02`)
  - `docs/modules/settings.md` (для перехода к `T02`)

