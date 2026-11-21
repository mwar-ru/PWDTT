# Паспорт модуля: VPN Integration (M3)

- **Каталог:** `internal/splittunnel/vpn/`
- **Зависимости:** минимальный хук в `wg_windows_init.go` и `app.go`.

## Контракт `TunnelInfo`
- `State`: `Disconnected | Starting | Connected | Reconnecting`
- `TunIfIdx`, `TunLUID`, `TunIPv4`, `TunIPv6`, `TunnelDNS`
- `PhysIfIdx`, `PhysIPv4`, `PhysGateway`
- `Conflicts []string` (обнаруженные сторонние TUN/TAP адаптеры)

## Ограничение
Единственный Go-модуль, знающий о структурах upstream PWDTT (локализовано в `pwdtt_adapter.go`).

