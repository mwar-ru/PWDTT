# ADR-003: DNS, QUIC (HTTP/3) и дочерние процессы браузеров

- **Статус:** Принято

## 1. Многопроцессные браузеры
- Chrome и Edge выполняют сетевые запросы из дочернего `NetworkService` процесса; Firefox использует `Socket Process`.
- `Application Manager` матчит процессы по каноническому пути EXE (`QueryFullProcessImageNameW`) + проверяет цепочку родительских PID (`PPID tree`) с привязкой к `CreationTime` процесса.

## 2. QUIC (HTTP/3 over UDP:443)
- WinDivert `SOCKET` отслеживает UDP bind/connect; таблица NAT поддерживает UDP-сессии с TTL 120 сек.
- Staging Queue (20 мс) удерживает первый UDP/SYN пакет, если событие `SOCKET` слегка отстаёт от `NETWORK`.

## 3. DNS на Windows 11
- **Browser DoH/DoT:** выполняется самим процессом браузера → автоматически следует правилу браузера.
- **System DNS (UDP/TCP 53):** выполняется службой `svchost.exe` (`Dnscache`). По умолчанию включён режим `tunnel_dns`: при наличии хотя бы одного VPN-приложения системный порт 53 направляется в DNS туннеля для предотвращения DNS-утечек (переключаемо в настройках).

