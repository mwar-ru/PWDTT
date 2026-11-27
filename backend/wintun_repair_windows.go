//go:build windows

package backend

import (
"context"
"fmt"
"os"
"os/exec"
"path/filepath"
"syscall"
"time"
)

func runHiddenSysCmd(timeout time.Duration, exeName string, args ...string) (string, error) {
ctx, cancel := context.WithTimeout(context.Background(), timeout)
defer cancel()

bin := exeName
if sysRoot := os.Getenv("SystemRoot"); sysRoot != "" {
cand := filepath.Join(sysRoot, "System32", exeName)
if _, err := os.Stat(cand); err == nil {
bin = cand
}
}

cmd := exec.CommandContext(ctx, bin, args...)
cmd.SysProcAttr = &syscall.SysProcAttr{
HideWindow:    true,
CreationFlags: 0x08000000, // CREATE_NO_WINDOW
}
out, err := cmd.CombinedOutput()
return string(out), err
}

// repairWintunDriver removes a stuck/corrupted Wintun service and DriverStore package
// so wintun.dll can cleanly reinstall a fresh signed driver on retry.
// Aborts safely if Wintun is currently RUNNING (used by another active VPN).
func repairWintunDriver(logf func(string)) error {
if out, err := runHiddenSysCmd(8*time.Second, "sc.exe", "query", "Wintun"); err == nil {
if isWintunServiceRunning(out) {
return fmt.Errorf("служба Wintun занята другим активным VPN, пропускаем сброс")
}
}

_, _ = runHiddenSysCmd(8*time.Second, "sc.exe", "stop", "Wintun")
if _, err := runHiddenSysCmd(8*time.Second, "sc.exe", "delete", "Wintun"); err == nil {
logf("Удалена зависшая служба Wintun")
}

enumOut, err := runHiddenSysCmd(15*time.Second, "pnputil.exe", "/enum-drivers")
if err != nil && enumOut == "" {
return fmt.Errorf("pnputil /enum-drivers: %w", err)
}

oems := parseWintunOemInfs(enumOut)
if len(oems) == 0 {
logf("Пакеты wintun.inf в DriverStore не найдены, пробуем повторное создание TUN")
}
for _, oem := range oems {
if _, derr := runHiddenSysCmd(15*time.Second, "pnputil.exe", "/delete-driver", oem, "/uninstall"); derr != nil {
logf(fmt.Sprintf("Не удалось удалить %s: %v", oem, derr))
} else {
logf(fmt.Sprintf("Удалён старый драйвер Wintun (%s)", oem))
}
}

time.Sleep(500 * time.Millisecond)
return nil
}
