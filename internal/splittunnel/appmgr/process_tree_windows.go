//go:build windows

package appmgr

import (
	"fmt"
	"os"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const ppidSnapshotTTL = 500 * time.Millisecond

type windowsProcessBackend struct {
	mu           sync.Mutex
	ppidMap      map[uint32]uint32
	snapshotTime time.Time
}

// NewWindowsProcessResolver returns a ProcessResolver backed by Windows WinAPI.
func NewWindowsProcessResolver() *ProcessResolver {
	return NewProcessResolverWithBackend(&windowsProcessBackend{
		ppidMap: make(map[uint32]uint32, 256),
	})
}

// SelfExecutablePath returns the normalized canonical path of the current PWDTT binary.
func SelfExecutablePath() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	return NormalizeWindowsPath(p)
}

func (b *windowsProcessBackend) LookupProcess(pid uint32) (rawProcessInfo, error) {
	if pid == 0 {
		return rawProcessInfo{}, ErrProcessNotFound
	}

	exePath, creationTime, err := queryProcessImageAndCreation(pid)
	if err != nil {
		return rawProcessInfo{}, err
	}

	ppid := b.lookupPPID(pid)
	return rawProcessInfo{
		PID:          pid,
		PPID:         ppid,
		CreationTime: creationTime,
		ExePath:      exePath,
	}, nil
}

func (b *windowsProcessBackend) ListProcesses() ([]rawProcessInfo, error) {
	snapshot, err := captureProcessSnapshot()
	if err != nil {
		return nil, err
	}

	b.mu.Lock()
	b.ppidMap = snapshot
	b.snapshotTime = time.Now()
	b.mu.Unlock()

	results := make([]rawProcessInfo, 0, len(snapshot))
	for pid, ppid := range snapshot {
		if pid == 0 || pid == 4 {
			continue
		}
		exePath, creationTime, qErr := queryProcessImageAndCreation(pid)
		if qErr != nil || exePath == "" {
			// Gracefully skip Protected Process Light (PPL) or exited processes
			continue
		}
		results = append(results, rawProcessInfo{
			PID:          pid,
			PPID:         ppid,
			CreationTime: creationTime,
			ExePath:      exePath,
		})
	}

	return results, nil
}

func (b *windowsProcessBackend) lookupPPID(pid uint32) uint32 {
	b.mu.Lock()
	defer b.mu.Unlock()

	if time.Since(b.snapshotTime) < ppidSnapshotTTL {
		if ppid, ok := b.ppidMap[pid]; ok {
			return ppid
		}
	}

	if snap, err := captureProcessSnapshot(); err == nil {
		b.ppidMap = snap
		b.snapshotTime = time.Now()
		return snap[pid]
	}

	return b.ppidMap[pid]
}

func captureProcessSnapshot() (map[uint32]uint32, error) {
	handle, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("CreateToolhelp32Snapshot: %w", err)
	}
	defer windows.CloseHandle(handle)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	if err := windows.Process32First(handle, &entry); err != nil {
		return nil, fmt.Errorf("Process32First: %w", err)
	}

	ppids := make(map[uint32]uint32, 256)
	for {
		ppids[entry.ProcessID] = entry.ParentProcessID
		if err := windows.Process32Next(handle, &entry); err != nil {
			break
		}
	}
	return ppids, nil
}

func queryProcessImageAndCreation(pid uint32) (string, uint64, error) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", 0, ErrProcessNotFound
	}
	defer windows.CloseHandle(handle)

	buf := make([]uint16, 1024)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(handle, 0, &buf[0], &size); err != nil {
		return "", 0, ErrProcessNotFound
	}
	exePath := windows.UTF16ToString(buf[:size])

	var creation, exit, kernel, user windows.Filetime
	var creationTicks uint64
	if err := windows.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err == nil {
		creationTicks = (uint64(creation.HighDateTime) << 32) | uint64(creation.LowDateTime)
	}

	return exePath, creationTicks, nil
}
