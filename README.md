# mwPWDTT_windows

Desktop-клиент для **Windows 10 / 11 (x64)** для маршрутизации WireGuard-трафика через VK Calls TURN-серверы с поприложенческим разделением трафика (**Per-App Split Tunneling**) и встроенным режимом **SOCKS5-прокси для mihomo**.

Форк проекта [luminescq/PWDTT](https://github.com/luminescq/PWDTT) (база `v1.6.0`).

> **Важно:** Программа предназначена для личного использования, обучения и тестирования сетевых технологий.

---

## Возможности

### Основной туннель и клиент
- **Ссылки `wdtt://` и `qwdtt://`** — быстрый импорт профилей (в том числе вставкой из буфера обмена).
- **Гибкий транспорт** — выбор режима обфускации (`Audio` / `Video`), транспорта TURN (`UDP` / `TCP`) и мощности (числа воркеров).
- **Автовосстановление Wintun** — при сбое создания TUN-интерфейса приложение автоматически очищает залипший драйвер Wintun и повторяет подключение.
- **Удобный интерфейс** — системный трей, автозапуск с Windows, светлая и тёмная темы.

### Режим SOCKS5-прокси для mihomo (Clash Meta)
- **Чистый userspace-туннель (`wireguard/netstack`)** — поднимает локальный SOCKS5-прокси (`127.0.0.1:10808`, поддержка **TCP + UDP**) без создания Wintun-адаптера и без изменения системных маршрутов Windows.
- **Автоматическая защита от конфликтов** — при включении режима прокси приложение запоминает текущие настройки Split Tunneling, скрыто приостанавливает WinDivert и WFP Kill Switch (сохраняется и после перезагрузки ПК), а при выключении автоматически восстанавливает все прежние параметры.
- **Кнопка `YAML`** в настройках — копирует готовый фрагмент конфига для mihomo в буфер обмена.

### Per-App Split Tunneling
- **Три режима работы** — `include` (только выбранные приложения через VPN), `exclude` (весь трафик, кроме выбранных) и `global` (весь трафик через VPN).
- **Правила по имени `.exe`** (`Telegram.exe`, `firefox.exe`) с поддержкой дочерних процессов (браузеры, Electron-приложения) — не ломаются после обновлений программ.
- **WinDivert 2.2 + проверка целостности** — маршрутизация пакетов напрямую в `wireguard-go` с TCP MSS clamping и проверкой SHA-256 драйвера по `CHECKSUMS.sha256`.
- **WFP Kill Switch и DNS Guard**:
  - Режимы `Fail-Closed` / `Fail-Open`, защита от IPv6-утечек и опция **«Игнорировать Kill Switch при ручном отключении»**.
  - Перенаправление UDP/53 выбранных программ в DNS туннеля (DoH браузеров на 443 не затрагивается).
  - Кнопка **«Сбросить WFP-правила»** для мгновенной очистки фильтров.
- **Пользовательский путь к конфигу** — файл `split_tunnel.json` можно разместить и подключить из любой выбранной папки.

### Обновления
- **Встроенное автообновление** с GitHub Releases и просмотр лога обновления (**«Лог обн.»** в настройках).
- **Пропуск и приостановка обновлений** — кнопка **«Пропустить эту версию»** в окне обновления и тумблер **«Приостановить обновления»** в настройках (пропускает следующую версию и снова уведомляет только при выходе ещё более новой).
- **Upstream Sync** — проверка новых релизов `luminescq/PWDTT` и безопасный dry-run проверки слияния.

---

## Установка

1. Скачай последний архив `mwPWDTT_windows-amd64-v*.zip` на странице **[Releases](https://github.com/mwar-ru/mwPWDTT_windows/releases)**.
2. Распакуй **все файлы** в отдельную папку (например, `C:\Apps\mwPWDTT_windows\`). Рядом с `mwPWDTT_windows.exe` должны лежать:
   - `mwPWDTT_windows.exe`
   - `WinDivert.dll`
   - `WinDivert64.sys`
   - `CHECKSUMS.sha256`
3. Запусти `mwPWDTT_windows.exe` и подтверди UAC (права администратора нужны для Wintun, WinDivert и WFP).

> **Windows Defender / антивирус:** Перед загрузкой `WinDivert.dll` и `WinDivert64.sys` приложение сверяет их SHA-256 хеши с `CHECKSUMS.sha256`. Если антивирус блокирует драйвер, добавь папку приложения в исключения.

---

## Быстрый старт (обычный режим TUN + Split Tunneling)

1. Добавь сервер по ссылке `wdtt://` (или вставь её через `Ctrl+V` в окне приложения).
2. При необходимости открой раздел **Split Tunneling**, включи тумблер, выбери режим (`include` или `exclude`) и добавь нужные программы через **«Выбрать из процессов»** или **«Добавить .exe»**.
3. Нажми **Подключить** на главном экране.

---

## Подключение к mihomo (SOCKS5-прокси)

Режим прокси позволяет использовать `mwPWDTT_windows` как обычный исходящий узел (`outbound`) внутри **mihomo** без конфликтов TUN-интерфейсов и маршрутов.

1. Открой **Настройки** в `mwPWDTT_windows` и включи **«Прокси для mihomo (SOCKS5)»** (порт по умолчанию — `10808`). Split Tunneling и WFP Kill Switch будут автоматически приостановлены на время работы этого режима.
2. Подключись к своему серверу на главном экране.
3. Добавь узел `PWDTT` и правило исключения для самого процесса в конфиг **mihomo**:

```yaml
proxies:
  - name: "PWDTT"
    type: socks5
    server: 127.0.0.1
    port: 10808
    udp: true

proxy-groups:
  - name: "PROXY"
    type: select
    proxies:
      - "PWDTT"
      - DIRECT

rules:
  # Обязательно при включённом TUN в mihomo, чтобы трафик VK TURN не зациклился
  - PROCESS-NAME,mwPWDTT_windows.exe,DIRECT
  - MATCH,PROXY
```

Когда выключишь **«Прокси для mihomo»** в настройках, прежние настройки Split Tunneling восстановятся автоматически.

---

## Требования

- **Windows 10 / 11 (x64)** с правами администратора.
- **WebView2 Runtime** (обычно уже встроен в Windows 10/11).
- Ссылка `wdtt://` / `qwdtt://` на сервер с развёрнутым [vk-tunnel-server](https://github.com/luminescq/vk-tunnel-server).

---

## Сборка из исходников (Windows)

Требуются **Go 1.26+**, **Node.js 22+** и [Wails CLI v2](https://wails.io/docs/getting-started/installation).

```powershell
# 1. Установка зависимостей, тесты и сборка фронтенда
Push-Location .\frontend
npm ci
npm run test
npm run build
Pop-Location

# 2. Go-тесты
go test ./backend/... ./internal/splittunnel/...

# 3. Сборка mwPWDTT_windows.exe (вшивает фронтенд, иконку и UAC-манифест)
wails build -platform windows/amd64 -s

# 4. Копирование WinDivert рядом с бинарником
Copy-Item -Force .\third_party\windivert\WinDivert.dll, .\third_party\windivert\WinDivert64.sys, .\third_party\windivert\CHECKSUMS.sha256 .\build\bin\
```

Готовые файлы появятся в папке `build\bin\`.

---

## Документация

- [docs/MANUAL.md](docs/MANUAL.md) — подробное руководство пользователя и FAQ.
- [docs/VM_TESTING_GUIDE.md](docs/VM_TESTING_GUIDE.md) — инструкция по проверке Split Tunneling и WFP на Windows.

## Благодарности и лицензия

Основано на [luminescq/PWDTT](https://github.com/luminescq/PWDTT). Распространяется по лицензии [LICENSE](LICENSE).