//go:build !windows

package updater

// OpenReportInEditor validates the report path on non-Windows platforms.
func OpenReportInEditor(reportsDir, reportPath string) error {
	_, err := ValidateReportPath(reportsDir, reportPath)
	return err
}
