# Руководство пользователя mwPWDTT_windows

Форк [luminescq/PWDTT](https://github.com/luminescq/PWDTT) для Windows 10 / 11 x64 с поприложенческим разделением трафика (**Per-App Split Tunneling**) по имени `.exe`, защитой от утечек (WFP Kill Switch + DNS Guard), синхронизацией списка программ между ПК (OneDrive) и проверкой обновлений из upstream.

Репозиторий и релизы: **https://github.com/mwar-ru/mwPWDTT_windows/releases**

## 1. Установка из готового релиза (Windows 10 / 11 x64)

1. Скачай архив `mwPWDTT_windows-amd64-v*.zip` со страницы [Releases](https://github.com/mwar-ru/mwPWDTT_windows/releases).
2. Распакуй **все файлы** в отдельную папку (например, `C:\Apps\mwPWDTT_windows\`). Рядом с `mwPWDTT_windows.exe` обязательно должны лежать:
   - `mwPWDTT_windows.exe`
   - `WinDivert.dll`
   - `WinDivert64.sys`
   - `CHECKSUMS.sha256`
3. Запусти `mwPWDTT_windows.exe` — UAC автоматически запросит права администратора (нужны для Wintun, WinDivert и правил WFP).

> Перед загрузкой `WinDivert.dll` и `WinDivert64.sys` проверяются их контрольные суммы SHA-256 по `CHECKSUMS.sha256`. Если антивирус/Defender блокирует драйвер, добавь папку приложения в исключения.

## 2. Быстрый старт

1. На главном экране настрой профиль (ссылка VK Calls + параметры WireGuard-сервера).
2. Перейди в **Split Tunneling** и включи тумблер **Split Tunneling**.
3. Выбери режим:
   - **Только выбранные приложения (`include`)** *(рекомендуется)* — через VPN идут только программы из списка; остальные работают напрямую.
   - **Все, кроме выбранных (`exclude`)** — весь трафик через VPN, кроме указанных программ.
   - **Весь трафик (`global`)** — стандартный режим без разделения.
4. Добавь программы через **«Выбрать из процессов»** или **«Добавить .exe»**:
   - Приложения сохраняются и сопоставляются по **имени файла** (`Telegram.exe`, `firefox.exe`), поэтому правила не ломаются при обновлениях Microsoft Store и при переносе между ПК.
   - Опция **Дочерние процессы** подхватывает подпроцессы браузеров и Electron-приложений.
5. Подключи VPN на главном экране и проверь IP на `https://api.ipify.org` в выбранном и обычном браузерах.

## 3. Kill Switch (WFP) и защита DNS

- **Kill Switch (`Fail-Closed` / `Fail-Open`)**: при обрыве туннеля `Fail-Closed` блокирует выбранные приложения через WFP и WinDivert и закрывает IPv6-утечки при IPv4-туннеле.
- **Игнорировать Kill Switch при ручном отключении** *(включено по умолчанию)*: при статусе **«Отключён»** выбранные программы работают напрямую (`Direct`), а при обрыве или ошибке туннеля Kill Switch срабатывает.
- **Режим WFP-фильтров**: `Dynamic` *(рекомендуется)* автоматически снимает правила при закрытии приложения; `Persistent Guard` сохраняет блокировку до следующего запуска или сброса.
- **Сбросить WFP-правила**: принудительно удаляет все фильтры приложения из Windows Filtering Platform.
- **Системный DNS**: `DNS туннеля` перенаправляет UDP/53 выбранных приложений в DNS WireGuard; DoH браузеров (443) не затрагивается.

## 4. Синхронизация между ПК (OneDrive)

В разделе **Split Tunneling** нажми **«Сменить путь»** и укажи `split_tunnel.json` в папке OneDrive. Благодаря хранению правил по имени `name.exe` один конфиг работает на всех твоих ПК с разными путями установки.

## 5. Обновление ядра (Upstream Sync)

Внизу экрана **Split Tunneling** можно проверить новые релизы `luminescq/PWDTT` и выполнить безопасный **Dry-Run** проверки слияния в изолированном git worktree.

## 6. Сборка из исходников (Windows)

~~~powershell
Push-Location .\frontend
npm ci; npm run test; npm run build
Pop-Location
wails build -platform windows/amd64 -s
Copy-Item -Force .\third_party\windivert\WinDivert.dll, .\third_party\windivert\WinDivert64.sys, .\third_party\windivert\CHECKSUMS.sha256 .\build\bin\
~~~

## 7. Решение проблем

- **Пропал интернет в выбранных программах после отключения (`ERR_NETWORK_ACCESS_DENIED`)**: проверь галочку **«Игнорировать Kill Switch при ручном отключении»** и нажми **«Сбросить WFP-правила»**.
- **Ошибка загрузки WinDivert**: проверь наличие `WinDivert.dll`, `WinDivert64.sys` и `CHECKSUMS.sha256` рядом с `mwPWDTT_windows.exe` и права администратора.