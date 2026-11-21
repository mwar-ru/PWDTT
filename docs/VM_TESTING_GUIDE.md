# Сборка и проверка на Windows 11 VM

## 1. Подготовка WinDivert 2.2 (WHQL-подписанная сборка)

1. Скачайте официальный архив WinDivert 2.2.2 (A) с https://reqrypt.org/windivert.html (папка x64).
2. Скопируйте WinDivert.dll и WinDivert64.sys в third_party/windivert/ (или рядом с pwdtt.exe).
3. Сгенерируйте SHA-256 чексуммы в PowerShell:

    $d = (Get-FileHash .\third_party\windivert\WinDivert.dll -Algorithm SHA256).Hash.ToLower()
    $s = (Get-FileHash .\third_party\windivert\WinDivert64.sys -Algorithm SHA256).Hash.ToLower()
    @("$d  WinDivert.dll", "$s  WinDivert64.sys") | Set-Content -Encoding ASCII .\third_party\windivert\CHECKSUMS.sha256

## 2. Сборка pwdtt.exe на Windows

    cd C:\AI-Builds\PWDTT\frontend
    npm ci
    npm run build

    cd C:\AI-Builds\PWDTT
    go build -tags production -ldflags="-H windowsgui -s -w" -o pwdtt.exe .

Положите рядом с pwdtt.exe файлы WinDivert.dll, WinDivert64.sys и CHECKSUMS.sha256 (или оставьте их в third_party\windivert\).

## 3. E2E-проверка на Windows 11 VM

1. Запустите pwdtt.exe от имени Администратора.
2. Откройте вкладку Split Tunnel:
   - Включите Split Tunneling, режим "Только выбранные" (include).
   - Добавьте firefox.exe и msedge.exe.
   - Оставьте Kill Switch = Fail-Closed, DNS = DNS туннеля.
3. Подключите VPN на главной вкладке.
4. Запустите в PowerShell (Администратор): .\scripts\verify_split_tunnel_vm.ps1
5. Проверьте вручную:
   - Firefox / Edge: https://api.ipify.org -> IP VPN; https://ipleak.net -> без утечек DNS и IPv6.
   - Chrome: https://api.ipify.org -> прямой IP провайдера и полная скорость.
   - Kill Switch: отключите VPN -> Firefox блокируется, Chrome работает.
   - OneDrive Sync: перенесите split_tunnel.json в OneDrive, подключите на втором ПК через "Подключить существующий".
   - Чистота WFP: закройте PWDTT и убедитесь, что Active PWDTT WFP filter entries == 0.
