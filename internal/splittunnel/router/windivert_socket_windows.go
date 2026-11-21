//go:build windows

package router

import (
	"context"
	"fmt"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	iphlpAFInet            = 2
	tcpTableOwnerPIDAll    = 5
	udpTableOwnerPID       = 1
	socketFallbackMinGap   = 50 * time.Millisecond
)

var (
	modIphlpapi             = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetExtendedTcpTable = modIphlpapi.NewProc("GetExtendedTcpTable")
	procGetExtendedUdpTable = modIphlpapi.NewProc("GetExtendedUdpTable")
)

type mibTCPRowOwnerPID struct {
	State      uint32
	LocalAddr  uint32
	LocalPort  uint32
	RemoteAddr uint32
	RemotePort uint32
	OwningPid  uint32
}

type mibUDPRowOwnerPID struct {
	LocalAddr uint32
	LocalPort uint32
	OwningPid uint32
}

func ntohsFromUint32(raw uint32) uint16 {
	return uint16(((raw & 0xFF) << 8) | ((raw >> 8) & 0xFF))
}

// QueryWindowsSocketTable reads all active IPv4 TCP and UDP local-port -> PID mappings
// from Windows iphlpapi.dll. Works without Administrator privileges and serves both
// initial seeding and fallback resolution for first-packet races.
func QueryWindowsSocketTable() (map[SocketKey]uint32, error) {
	out := make(map[SocketKey]uint32, 256)

	if err := readTCPTableInto(out); err != nil {
		return nil, err
	}
	if err := readUDPTableInto(out); err != nil {
		return nil, err
	}
	return out, nil
}

func readTCPTableInto(dst map[SocketKey]uint32) error {
	var size uint32
	for attempt := 0; attempt < 4; attempt++ {
		var buf []byte
		var ptr uintptr
		if size > 0 {
			buf = make([]byte, size)
			ptr = uintptr(unsafe.Pointer(&buf[0]))
		}
		r1, _, _ := procGetExtendedTcpTable.Call(
			ptr,
			uintptr(unsafe.Pointer(&size)),
			0, // unsorted (faster)
			iphlpAFInet,
			tcpTableOwnerPIDAll,
			0,
		)
		if r1 == uintptr(windows.ERROR_INSUFFICIENT_BUFFER) {
			continue
		}
		if r1 != 0 {
			return fmt.Errorf("GetExtendedTcpTable failed: %d", r1)
		}
		if len(buf) < 4 {
			return nil
		}
		count := *(*uint32)(unsafe.Pointer(&buf[0]))
		rowSize := unsafe.Sizeof(mibTCPRowOwnerPID{})
		if uintptr(len(buf)) < 4+uintptr(count)*rowSize {
			return nil
		}
		rows := unsafe.Slice((*mibTCPRowOwnerPID)(unsafe.Pointer(&buf[4])), count)
		for _, row := range rows {
			port := ntohsFromUint32(row.LocalPort)
			if port != 0 && row.OwningPid != 0 {
				dst[SocketKey{Proto: ProtoTCP, LocalPort: port}] = row.OwningPid
			}
		}
		return nil
	}
	return fmt.Errorf("GetExtendedTcpTable buffer retry exhausted")
}

func readUDPTableInto(dst map[SocketKey]uint32) error {
	var size uint32
	for attempt := 0; attempt < 4; attempt++ {
		var buf []byte
		var ptr uintptr
		if size > 0 {
			buf = make([]byte, size)
			ptr = uintptr(unsafe.Pointer(&buf[0]))
		}
		r1, _, _ := procGetExtendedUdpTable.Call(
			ptr,
			uintptr(unsafe.Pointer(&size)),
			0,
			iphlpAFInet,
			udpTableOwnerPID,
			0,
		)
		if r1 == uintptr(windows.ERROR_INSUFFICIENT_BUFFER) {
			continue
		}
		if r1 != 0 {
			return fmt.Errorf("GetExtendedUdpTable failed: %d", r1)
		}
		if len(buf) < 4 {
			return nil
		}
		count := *(*uint32)(unsafe.Pointer(&buf[0]))
		rowSize := unsafe.Sizeof(mibUDPRowOwnerPID{})
		if uintptr(len(buf)) < 4+uintptr(count)*rowSize {
			return nil
		}
		rows := unsafe.Slice((*mibUDPRowOwnerPID)(unsafe.Pointer(&buf[4])), count)
		for _, row := range rows {
			port := ntohsFromUint32(row.LocalPort)
			if port != 0 && row.OwningPid != 0 {
				dst[SocketKey{Proto: ProtoUDP, LocalPort: port}] = row.OwningPid
			}
		}
		return nil
	}
	return fmt.Errorf("GetExtendedUdpTable buffer retry exhausted")
}

// SocketTracker streams WINDIVERT_LAYER_SOCKET events into SocketTable with iphlpapi fallback.
type SocketTracker struct {
	dll   *WinDivertDLL
	table *SocketTable

	mu               sync.Mutex
	handle           uintptr
	lastFallbackScan time.Time
	doneCh           chan struct{}
}

// NewSocketTracker creates a SocketTracker backed by WinDivert SOCKET layer and iphlpapi.
func NewSocketTracker(dll *WinDivertDLL, table *SocketTable) *SocketTracker {
	return &SocketTracker{
		dll:   dll,
		table: table,
	}
}

// Start seeds existing OS sockets and launches the WinDivert SOCKET listener goroutine.
func (t *SocketTracker) Start(ctx context.Context) error {
	if initial, err := QueryWindowsSocketTable(); err == nil {
		t.table.BulkSeed(initial)
	}

	h, err := t.dll.Open(
		"tcp or udp",
		winDivertLayerSocket,
		0,
		winDivertFlagSniff|winDivertFlagRecvOnly,
	)
	if err != nil {
		return err
	}

	done := make(chan struct{})
	t.mu.Lock()
	t.handle = h
	t.doneCh = done
	t.mu.Unlock()

	go t.runLoop(ctx, h, done)
	return nil
}

func (t *SocketTracker) runLoop(ctx context.Context, handle uintptr, done chan struct{}) {
	defer close(done)
	var addr winDivertAddress

	for {
		if ctx.Err() != nil {
			return
		}
		_, err := t.dll.Recv(handle, nil, &addr)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			// Handle was shut down or closed
			return
		}

		sock := addr.Socket()
		proto := Protocol(sock.Protocol)
		if proto != ProtoTCP && proto != ProtoUDP {
			continue
		}
		if sock.LocalPort == 0 {
			continue
		}

		key := SocketKey{Proto: proto, LocalPort: sock.LocalPort}
		switch addr.Event() {
		case winDivertEventSocketBind, winDivertEventSocketConnect, winDivertEventSocketAccept:
			if sock.ProcessId != 0 {
				t.table.Set(key, sock.ProcessId)
			}
		case winDivertEventSocketClose:
			t.table.Delete(key)
		}
	}
}

// LookupWithFallback resolves key from SocketTable, falling back to a rate-limited
// GetExtendedTcpTable/UdpTable refresh on cache miss (handles first-packet race).
func (t *SocketTracker) LookupWithFallback(key SocketKey) (uint32, bool) {
	if pid, ok := t.table.Lookup(key); ok {
		return pid, true
	}

	t.mu.Lock()
	shouldScan := time.Since(t.lastFallbackScan) >= socketFallbackMinGap
	if shouldScan {
		t.lastFallbackScan = time.Now()
	}
	t.mu.Unlock()

	if shouldScan {
		if fresh, err := QueryWindowsSocketTable(); err == nil {
			t.table.BulkSeed(fresh)
		}
	}
	return t.table.Lookup(key)
}

// Stop gracefully shuts down and closes the WinDivert SOCKET handle.
func (t *SocketTracker) Stop() error {
	t.mu.Lock()
	h := t.handle
	done := t.doneCh
	t.handle = 0
	t.doneCh = nil
	t.mu.Unlock()

	if h != 0 {
		_ = t.dll.Shutdown(h, winDivertShutdownBoth)
		_ = t.dll.Close(h)
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}
	return nil
}
