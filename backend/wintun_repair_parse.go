package backend

import (
"regexp"
"strings"
)

var (
reOemInf    = regexp.MustCompile(`(?i)\boem\d+\.inf\b`)
reWintunInf = regexp.MustCompile(`(?i)\bwintun\.inf\b`)
reScRunning = regexp.MustCompile(`(?i):\s*4\s+RUNNING\b`)
)

func shouldAttemptWintunRepair(err error) bool {
if err == nil {
return false
}
msg := strings.ToLower(err.Error())
for _, needle := range []string{
"cannot find the file specified",
"error creating interface",
"device attached to the system is not functioning",
"cr_invalid_devnode",
"error creating wintun",
} {
if strings.Contains(msg, needle) {
return true
}
}
return false
}

// isWintunServiceRunning checks `sc query Wintun` output for STATE 4 RUNNING (locale-independent).
func isWintunServiceRunning(scOutput string) bool {
return reScRunning.MatchString(scOutput)
}

// parseWintunOemInfs extracts all published oemXX.inf names associated with wintun.inf
// from `pnputil /enum-drivers` output regardless of Windows UI language or CP866 encoding.
func parseWintunOemInfs(output string) []string {
norm := strings.ReplaceAll(output, "\r\n", "\n")
blocks := strings.Split(norm, "\n\n")
seen := make(map[string]struct{})
var result []string
for _, block := range blocks {
if !reWintunInf.MatchString(block) {
continue
}
if oem := reOemInf.FindString(block); oem != "" {
key := strings.ToLower(oem)
if _, ok := seen[key]; !ok {
seen[key] = struct{}{}
result = append(result, key)
}
}
}
return result
}
