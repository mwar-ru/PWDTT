//go:build !windows

package router

type stubWFPGuard struct{}

func (stubWFPGuard) Apply(WFPGuardConfig, bool) error { return nil }
func (stubWFPGuard) Clear() error                     { return nil }
func (stubWFPGuard) ResetStale() error                { return nil }
func (stubWFPGuard) Close() error                     { return nil }
func (stubWFPGuard) ActiveFilterCount() int           { return 0 }

// NewWindowsWFPGuard returns a no-op guard on non-Windows platforms.
func NewWindowsWFPGuard() WFPGuard { return stubWFPGuard{} }

// ResetWfpRules is a no-op on non-Windows platforms.
func ResetWfpRules() error { return nil }
