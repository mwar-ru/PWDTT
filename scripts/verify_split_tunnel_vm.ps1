[CmdletBinding()]
param(
    [string]$WinDivertDir = ".\third_party\windivert",
    [string]$TunnelIface = "pwdtt-wg"
)

$ErrorActionPreference = "Stop"
Write-Host "=== PWDTT Split Tunneling VM Verification ===" -ForegroundColor Cyan

$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
$isAdmin = $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Warning "Run from an Administrator PowerShell to inspect WFP filters and routes."
} else {
    Write-Host "[OK] Running as Administrator" -ForegroundColor Green
}

foreach ($name in @("WinDivert.dll", "WinDivert64.sys", "CHECKSUMS.sha256")) {
    $f = Join-Path $WinDivertDir $name
    if (Test-Path $f) {
        $hash = (Get-FileHash -Algorithm SHA256 -Path $f).Hash.ToLower()
        Write-Host "[OK] $f ($hash)" -ForegroundColor Green
    } else {
        Write-Warning "[MISSING] $f"
    }
}

$wgAdapter = Get-NetIPInterface -InterfaceAlias $TunnelIface -AddressFamily IPv4 -ErrorAction SilentlyContinue
if ($wgAdapter) {
    Write-Host "[OK] Adapter '$TunnelIface': IfIndex=$($wgAdapter.ifIndex), Metric=$($wgAdapter.InterfaceMetric)" -ForegroundColor Green
    $def = Get-NetRoute -InterfaceIndex $wgAdapter.ifIndex -DestinationPrefix "0.0.0.0/0" -ErrorAction SilentlyContinue
    if ($def) {
        Write-Host "[INFO] 0.0.0.0/0 route present on $TunnelIface (global/exclude mode)" -ForegroundColor Yellow
    } else {
        Write-Host "[OK] No 0.0.0.0/0 route on $TunnelIface (include mode native speed)" -ForegroundColor Green
    }
} else {
    Write-Host "[INFO] Adapter '$TunnelIface' is not up (connect VPN in PWDTT first)" -ForegroundColor Yellow
}

if ($isAdmin) {
    $wfpXml = Join-Path $env:TEMP "pwdtt_wfp_check.xml"
    netsh wfp show filters file="$wfpXml" | Out-Null
    if (Test-Path $wfpXml) {
        $found = Select-String -Path $wfpXml -Pattern "PWDTT Split Tunnel" -SimpleMatch
        Write-Host "[INFO] Active PWDTT WFP filter entries: $($found.Count)" -ForegroundColor Cyan
        Remove-Item -Force $wfpXml -ErrorAction SilentlyContinue
    }
}
