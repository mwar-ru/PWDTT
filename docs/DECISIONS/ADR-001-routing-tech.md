# ADR-001: Выбор технологии маршрутизации для Windows 11

- **Статус:** Принято
- **Контекст:** Требуется Per-App Split Tunneling поверх существующего WireGuard/Wintun туннеля PWDTT на Windows 11 (Secure Boot ON) без возможности разрабатывать и подписывать собственный драйвер ядра (EV Code Signing + Microsoft WHQL).

## Решение

Использовать гибридную схему за абстрактным интерфейсом `TrafficRouter`:
1. **WinDivert 2.2** (официальный WHQL-подписанный `WinDivert64.sys`) — два хэндла:
   - `WINDIVERT_LAYER_SOCKET` для привязки `(Proto, LocalIP, LocalPort)` ↔ `ProcessId` (TCP и UDP/QUIC).
   - `WINDIVERT_LAYER_NETWORK` для перехвата пакетов целевых процессов, stateful SNAT/DNAT и реинжекции по `IfIdx` (Wintun или физический NIC).
2. **User-mode WFP (`fwpuclnt.dll`)** — для Fail-Closed Kill Switch и блокировки IPv6 при IPv4-only туннеле (не требует сторонних драйверов).

## Почему отклонены альтернативы
- **Собственный WFP Callout Driver:** требует EV/WHQL-подписи; оставлен как перспективная реализация `TrafficRouter` v2.
- **Чистый user-mode WFP:** не поддерживает `ALE_BIND_REDIRECT` / `ALE_CONNECT_REDIRECT` без драйвера ядра.
- **WinpkFilter / NDIS LWF:** лицензионные ограничения и избыточный L2-уровень.
- **Локальный SOCKS5 / второй TUN:** игнорируется многими приложениями и UDP/QUIC либо дублирует WireGuard-стек PWDTT.

## Последствия
- В режиме `Include` трафик невыбранных приложений идёт по нативному стеку Windows с нулевым оверхедом.
- Необходима проверка SHA-256 поставляемых бинарников WinDivert и внедрение Staging Queue (15–30 мс) против гонки первого пакета.

