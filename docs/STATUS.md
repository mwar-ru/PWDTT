# STATUS — PWDTT Per-App Split Tunneling (Windows 11)

**Последнее обновление:** Реализована подзадача T04 (Windows Process Tree & Scanner)
**Активная ветка:** `feat/windows-split-tunnel` (форк `https://github.com/mwar-ru/PWDTT`)
**Синхронизация с upstream:** Базовый релиз `luminescq/PWDTT` (`v1.6.0`)

## Что уже реализовано
- Проведён анализ архитектуры `luminescq/PWDTT` (Go + Wails + WireGuard/Wintun + VK TURN/DTLS).
- Утверждены ключевые решения (ADR-001..005): WinDivert 2.2 (WHQL) + user-mode WFP Kill Switch (`fail-closed` по умолчанию), 3 режима (`global`/`include`/`exclude`), кастомный путь `split_tunnel.json` (OneDrive + `%Env%`), встроенный AI-апдейтер M6, Windows 11 only в v1.
- **T01 — Фундамент и документация:** настроены `fork`/`upstream`, ветка `feat/windows-split-tunnel`, `AGENTS.md` и `docs/*`.
- **T02 — Settings Store (`internal/splittunnel/settings`):** схема v1, bootstrap-указатель, кастомный путь (OneDrive + защита от перезаписи на втором ПК), портативные `%Env%`-пути, атомарная запись с retry и LKG-кэш.
- **T03 — Policy Engine (`internal/splittunnel/appmgr`):** каноническая нормализация Windows-путей (`\\?\`, `\??\`, `%Env%`, `\Device\...`), `PolicyEngine` для 3 режимов, наследование дочерними процессами и anti-loop защита `pwdtt.exe`.
- **T04 — Windows Process Tree & Scanner (`internal/splittunnel/appmgr`):**
  - `process_types.go`: потокобезопасный `ProcessResolver` с кэшем, защитой от PID-reuse по `CreationTime`, защитой от циклов PPID и группировкой многопроцессных приложений (`ListRunningApps`).
  - `process_tree_windows.go`: WinAPI (`PROCESS_QUERY_LIMITED_INFORMATION`, `QueryFullProcessImageName`, `GetProcessTimes`, кэшированный `CreateToolhelp32Snapshot`).
  - `process_tree_stub.go`: кроссплатформенная заглушка для Linux/macOS сборок.

## Что сейчас работает
- Оригинальный функционал PWDTT не изменялся.
- Фаза 1 (`T02`, `T03`, `T04`) полностью завершена: пакеты `settings` и `appmgr` проходят тесты с `-race` и кросс-компилируются под Windows x64.

## Что ещё не реализовано
- Фаза 2: `T05` (NetIf Inspector), `T06` (Upstream Adapter), `T07` (WFP Kill Switch).
- Фаза 3: `T08`–`T11` (NAT/Staging Queue, WinDivert SOCKET/NETWORK, DNS Guard).
- Фаза 4: `T15` (Upstream Sync & AI-Report), `T12`–`T14` (Wails Service, UI SplitTunnelView, CI и E2E в VM).

## Известные ограничения и проблемы
- При жёстком краше `pwdtt.exe` динамическая WFP-сессия удаляется Windows (честное ограничение описано в ADR-002; предусмотрена опция Persistent WFP + утилита сброса).
- Системный DNS (UDP/53) в Windows 11 выполняется через `svchost.exe` — используется политика VPN DNS Guard (ADR-003).

## Следующая задача
- **ID:** `T05 — Инспектор сетевых интерфейсов Windows (Wintun и физический NIC)`
- **Цель:** Реализовать определение `IfIdx`, `LUID`, IPv4/IPv6 и шлюза активного физического адаптера и виртуального интерфейса Wintun (`internal/splittunnel/vpn/types.go`, `netif_windows.go`, тесты).
- **Файлы для изучения:**
  - `AGENTS.md`
  - `docs/STATUS.md`
  - `docs/modules/vpn-integration.md`
