# mwPWDTT_windows

Desktop-клиент для **Windows 10 / 11 (x64)** для маршрутизации WireGuard-трафика через VK Calls TURN-серверы с полноценным **поприложенческим разделением трафика (Per-App Split Tunneling)**.

Форк проекта [luminescq/PWDTT](https://github.com/luminescq/PWDTT) (база `v1.6.0`).

> **Важно:** Программа предназначена для личного использования, обучения и тестирования сетевых технологий.

## Возможности

- **Per-App Split Tunneling** — режимы `include` (только выбранные приложения через VPN), `exclude` (все кроме выбранных) и `global` (весь трафик).
- **Сопоставление по имени `.exe`** (`Telegram.exe`, `firefox.exe`) — правила не ломаются после обновлений Microsoft Store и при переносе конфига на другой ПК; поддерживается наследование для дочерних процессов браузеров и Electron.
- **Прямой туннельный pipe + WinDivert 2.2** — маршрутизация пакетов напрямую в `wireguard-go` с автоматическим TCP MSS clamping и проверкой целостности драйвера по `CHECKSUMS.sha256`.
- **Защита от утечек (WFP + DNS Guard)**:
  - WFP Kill Switch (`Fail-Closed` / `Fail-Open`) и блокировка IPv6-утечек при IPv4-туннеле.
  - Опция **«Игнорировать Kill Switch при ручном отключении»** — при статусе «Отключён» выбранные приложения работают напрямую (`Direct`), а при обрыве туннеля срабатывает Kill Switch.
  - Кнопка **«Сбросить WFP-правила»** для мгновенной очистки фильтров.
  - Перенаправление UDP/53 выбранных программ в DNS туннеля (DoH браузеров на 443 не затрагивается).
- **Синхронизация между ПК (OneDrive)** — выбор пользовательского пути для `split_tunnel.json`.
- **Обновление ядра (Upstream Sync)** — проверка новых релизов `luminescq/PWDTT` и безопасный Dry-Run проверки слияния.
- **Все базовые функции PWDTT** — профили, системный трей, автозапуск с Windows, светлая/тёмная тема, встроенные `wg-turn-client` и `wireguard-go`.

## Скачивание и установка

1. Скачай последний архив `mwPWDTT_windows-amd64-v*.zip` на странице **[Releases](https://github.com/mwar-ru/mwPWDTT_windows/releases)**.
2. Распакуй **все файлы** в отдельную папку (например, `C:\Apps\mwPWDTT_windows\`). Рядом с `mwPWDTT_windows.exe` должны находиться:
   - `mwPWDTT_windows.exe`
   - `WinDivert.dll`
   - `WinDivert64.sys`
   - `CHECKSUMS.sha256`
3. Запусти `mwPWDTT_windows.exe` и подтверди запрос прав администратора (UAC) — они требуются для адаптера Wintun, драйвера WinDivert и правил Windows Filtering Platform (WFP).

> **Windows Defender / антивирус:** Перед загрузкой `WinDivert.dll` и `WinDivert64.sys` приложение сверяет их SHA-256 хеши с `CHECKSUMS.sha256`. Если антивирус блокирует драйвер, добавь папку приложения в исключения.

## Требования

- **Windows 10 / 11 (x64)** с правами администратора.
- **WebView2 Runtime** (обычно уже встроен в Windows 10/11).
- Ссылка на звонок VK и сервер с развёрнутым [vk-tunnel-server](https://github.com/luminescq/vk-tunnel-server).

## Быстрый старт

1. На главном экране укажи параметры профиля (ссылка VK Calls и конфиг WireGuard).
2. Открой раздел **Split Tunneling**, включи тумблер и выбери режим **Только выбранные приложения (`include`)**.
3. Добавь нужные программы через **«Выбрать из процессов»** или **«Добавить .exe»** (в конфиге сохранятся переносимые имена вроде `Telegram.exe`, `firefox.exe`).
4. Нажми **Подключить** на главном экране и проверь IP на `https://api.ipify.org` в выбранном и обычном браузерах.

Подробная инструкция по настройке Kill Switch, OneDrive-синхронизации и решению проблем — в **[docs/MANUAL.md](docs/MANUAL.md)**.

## Сборка из исходников (Windows)

Требуются Go 1.25+, Node.js 20+ и [Wails CLI v2](https://wails.io/docs/getting-started/installation).

~~~powershell
# 1. Сборка фронтенда
Push-Location .\frontend
npm ci
npm run test
npm run build
Pop-Location

# 2. Сборка mwPWDTT_windows.exe (вшивает иконку и UAC-манифест)
wails build -platform windows/amd64 -s

# 3. Копирование WinDivert рядом с бинарником
Copy-Item -Force .\third_party\windivert\WinDivert.dll, .\third_party\windivert\WinDivert64.sys, .\third_party\windivert\CHECKSUMS.sha256 .\build\bin\
~~~

Готовые файлы будут в папке `build\bin\`.

## Документация

- [docs/MANUAL.md](docs/MANUAL.md) — полное руководство пользователя и FAQ.
- [docs/VM_TESTING_GUIDE.md](docs/VM_TESTING_GUIDE.md) — инструкция по проверке Split Tunneling и WFP на Windows.

## Благодарности и лицензия

Основано на [luminescq/PWDTT](https://github.com/luminescq/PWDTT). Распространяется по лицензии [LICENSE](LICENSE).