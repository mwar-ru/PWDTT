//go:build windows

package main

import (
	"syscall"

	"golang.org/x/sys/windows"
)

var (
	kernel32DLL     = windows.NewLazySystemDLL("kernel32.dll")
	procGetProcAddr = kernel32DLL.NewProc("GetProcAddress")
	uxthemeDLL      = windows.NewLazySystemDLL("uxtheme.dll")
)

// enableNativeMenuTheme enables automatic Windows 10/11 Dark & Light theme
// rendering for Win32 popup menus (including the system tray context menu).
func enableNativeMenuTheme() {
	if err := uxthemeDLL.Load(); err != nil {
		return
	}
	h := uxthemeDLL.Handle()
	// Ordinal 135: SetPreferredAppMode(AllowDark = 1)
	if addr, _, _ := procGetProcAddr.Call(h, uintptr(135)); addr != 0 {
		_, _, _ = syscall.SyscallN(addr, 1)
	}
	// Ordinal 136: FlushMenuThemes()
	if addr, _, _ := procGetProcAddr.Call(h, uintptr(136)); addr != 0 {
		_, _, _ = syscall.SyscallN(addr)
	}
}

func flushNativeMenuTheme() {
	if err := uxthemeDLL.Load(); err != nil {
		return
	}
	if addr, _, _ := procGetProcAddr.Call(uxthemeDLL.Handle(), uintptr(136)); addr != 0 {
		_, _, _ = syscall.SyscallN(addr)
	}
}
