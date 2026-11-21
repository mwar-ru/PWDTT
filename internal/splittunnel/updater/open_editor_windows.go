//go:build windows

package updater

import (
	"os/exec"
)

// OpenReportInEditor opens a validated Markdown report in the user's default Windows editor.
func OpenReportInEditor(reportsDir, reportPath string) error {
	absPath, err := ValidateReportPath(reportsDir, reportPath)
	if err != nil {
		return err
	}
	// Launch directly via rundll32 FileProtocolHandler (no cmd /c shell interpolation).
	cmd := exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", absPath)
	if err := cmd.Start(); err != nil {
		return exec.Command("notepad.exe", absPath).Start()
	}
	return nil
}
