package backend

import (
"errors"
"reflect"
"testing"
)

func TestShouldAttemptWintunRepair(t *testing.T) {
if !shouldAttemptWintunRepair(errors.New("Error creating interface: The system cannot find the file specified.")) {
t.Fatal("expected true for file-not-found CreateTUN error")
}
if shouldAttemptWintunRepair(errors.New("permission denied")) {
t.Fatal("expected false for unrelated error")
}
if shouldAttemptWintunRepair(nil) {
t.Fatal("expected false for nil error")
}
}

func TestIsWintunServiceRunning(t *testing.T) {
stopped := "SERVICE_NAME: Wintun\n        STATE              : 1  STOPPED\n        WIN32_EXIT_CODE    : 31  (0x1f)\n"
if isWintunServiceRunning(stopped) {
t.Fatal("expected STOPPED service to return false")
}
running := "SERVICE_NAME: Wintun\n        STATE              : 4  RUNNING\n"
if !isWintunServiceRunning(running) {
t.Fatal("expected RUNNING service to return true")
}
}

func TestParseWintunOemInfs_MultiLocale(t *testing.T) {
sample := "Published Name:     oem12.inf\r\nOriginal Name:      other.inf\r\n\r\n" +
"╬яєсышъютрээюх шь :     oem92.inf\r\n╚ёїюфэюх шь :      wintun.inf\r\n╚ь  яюёЄрт∙шър:      WireGuard LLC\r\n\r\n" +
"Published Name:     OEM45.INF\nOriginal Name:      wintun.inf\nProvider Name:      WireGuard LLC\n"
got := parseWintunOemInfs(sample)
want := []string{"oem92.inf", "oem45.inf"}
if !reflect.DeepEqual(got, want) {
t.Fatalf("parseWintunOemInfs() = %v, want %v", got, want)
}
}
