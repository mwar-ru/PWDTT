//go:build windows

package router

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// WinDivert 2.2 constants from official windivert.h
const (
	winDivertInvalidHandle = ^uintptr(0)

	winDivertLayerNetwork        uint32 = 0
	winDivertLayerNetworkForward uint32 = 1
	winDivertLayerFlow           uint32 = 2
	winDivertLayerSocket         uint32 = 3
	winDivertLayerReflect        uint32 = 4

	winDivertEventNetworkPacket uint8 = 0
	winDivertEventSocketBind    uint8 = 3
	winDivertEventSocketConnect uint8 = 4
	winDivertEventSocketListen  uint8 = 5
	winDivertEventSocketAccept  uint8 = 6
	winDivertEventSocketClose   uint8 = 7

	winDivertFlagSniff    uint64 = 0x0001
	winDivertFlagDrop     uint64 = 0x0002
	winDivertFlagRecvOnly uint64 = 0x0004
	winDivertFlagSendOnly uint64 = 0x0008

	winDivertShutdownRecv uint32 = 0x1
	winDivertShutdownSend uint32 = 0x2
	winDivertShutdownBoth uint32 = 0x3
)

// winDivertAddress matches WinDivert 2.2 WINDIVERT_ADDRESS (exact size: 80 bytes).
type winDivertAddress struct {
	Timestamp int64
	Flags     uint32   // bitfields: Layer(0..7), Event(8..15), Sniffed(16), Outbound(17), Loopback(18), Impostor(19), IPv6(20), IP/TCP/UDPChecksum(21..23)
	Reserved2 uint32
	Data      [64]byte // union { Network, Flow, Socket, Reflect }
}

// winDivertDataNetwork matches WINDIVERT_DATA_NETWORK (8 bytes at Data[0:8]).
type winDivertDataNetwork struct {
	IfIdx    uint32
	SubIfIdx uint32
}

// winDivertDataSocket matches WINDIVERT_DATA_SOCKET (64 bytes at Data[0:64]).
type winDivertDataSocket struct {
	EndpointId       uint64
	ParentEndpointId uint64
	ProcessId        uint32
	LocalAddr        [4]uint32
	RemoteAddr       [4]uint32
	LocalPort        uint16
	RemotePort       uint16
	Protocol         uint8
	_                [7]byte
}

func (a *winDivertAddress) Layer() uint8    { return uint8(a.Flags & 0xFF) }
func (a *winDivertAddress) Event() uint8    { return uint8((a.Flags >> 8) & 0xFF) }
func (a *winDivertAddress) Sniffed() bool   { return (a.Flags & (1 << 16)) != 0 }
func (a *winDivertAddress) Outbound() bool  { return (a.Flags & (1 << 17)) != 0 }
func (a *winDivertAddress) Loopback() bool  { return (a.Flags & (1 << 18)) != 0 }
func (a *winDivertAddress) Impostor() bool  { return (a.Flags & (1 << 19)) != 0 }
func (a *winDivertAddress) IPv6() bool      { return (a.Flags & (1 << 20)) != 0 }

func (a *winDivertAddress) SetOutbound(v bool) {
	if v {
		a.Flags |= (1 << 17)
	} else {
		a.Flags &^= (1 << 17)
	}
}

func (a *winDivertAddress) SetImpostor(v bool) {
	if v {
		a.Flags |= (1 << 19)
	} else {
		a.Flags &^= (1 << 19)
	}
}

func (a *winDivertAddress) Network() *winDivertDataNetwork {
	return (*winDivertDataNetwork)(unsafe.Pointer(&a.Data[0]))
}

func (a *winDivertAddress) Socket() *winDivertDataSocket {
	return (*winDivertDataSocket)(unsafe.Pointer(&a.Data[0]))
}

// WinDivertDLL wraps a loaded and integrity-verified WinDivert 2.2 DLL.
type WinDivertDLL struct {
	dllPath string
	sysPath string
	dll     *windows.LazyDLL

	procOpen          *windows.LazyProc
	procRecv          *windows.LazyProc
	procSend          *windows.LazyProc
	procShutdown      *windows.LazyProc
	procClose         *windows.LazyProc
	procCalcChecksums *windows.LazyProc
}

// LoadWinDivert locates WinDivert.dll and WinDivert64.sys, verifies optional SHA-256 hashes,
// and loads WinDivert.dll lazily.
func LoadWinDivert(searchDir string, expectedHashes map[string]string) (*WinDivertDLL, error) {
	dir := searchDir
	if dir == "" {
		exe, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("resolve executable dir: %w", err)
		}
		dir = filepath.Dir(exe)
	}

	dllPath := filepath.Join(dir, "WinDivert.dll")
	sysPath := filepath.Join(dir, "WinDivert64.sys")

	if err := VerifyFileSHA256(dllPath, expectedHashes["windivert.dll"]); err != nil {
		return nil, fmt.Errorf("WinDivert.dll integrity check failed: %w", err)
	}
	if err := VerifyFileSHA256(sysPath, expectedHashes["windivert64.sys"]); err != nil {
		return nil, fmt.Errorf("WinDivert64.sys integrity check failed: %w", err)
	}

	dll := windows.NewLazyDLL(dllPath)
	if err := dll.Load(); err != nil {
		return nil, fmt.Errorf("failed to load WinDivert.dll (check architecture/antivirus): %w", err)
	}

	return &WinDivertDLL{
		dllPath:           dllPath,
		sysPath:           sysPath,
		dll:               dll,
		procOpen:          dll.NewProc("WinDivertOpen"),
		procRecv:          dll.NewProc("WinDivertRecv"),
		procSend:          dll.NewProc("WinDivertSend"),
		procShutdown:      dll.NewProc("WinDivertShutdown"),
		procClose:         dll.NewProc("WinDivertClose"),
		procCalcChecksums: dll.NewProc("WinDivertHelperCalcChecksums"),
	}, nil
}

// Open opens a WinDivert handle on the given layer with a filter expression.
func (w *WinDivertDLL) Open(filter string, layer uint32, priority int16, flags uint64) (uintptr, error) {
	cFilter, err := syscall.BytePtrFromString(filter)
	if err != nil {
		return 0, err
	}
	h, _, callErr := w.procOpen.Call(
		uintptr(unsafe.Pointer(cFilter)),
		uintptr(layer),
		uintptr(uint16(priority)),
		uintptr(flags),
	)
	if h == winDivertInvalidHandle || h == 0 {
		return 0, classifyWinDivertOpenError(callErr)
	}
	return h, nil
}

func classifyWinDivertOpenError(err error) error {
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return fmt.Errorf("WinDivertOpen access denied (run PWDTT as Administrator): %w", err)
	}
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
		return fmt.Errorf("WinDivert64.sys not found or quarantined by antivirus/EDR: %w", err)
	}
	if errors.Is(err, windows.ERROR_INVALID_IMAGE_HASH) {
		return fmt.Errorf("WinDivert64.sys driver signature rejected by Windows: %w", err)
	}
	return fmt.Errorf("WinDivertOpen failed: %w", err)
}

// Recv reads a packet or socket event from a WinDivert handle.
func (w *WinDivertDLL) Recv(handle uintptr, buf []byte, addr *winDivertAddress) (uint32, error) {
	var pktPtr uintptr
	var pktLen uint32
	if len(buf) > 0 {
		pktPtr = uintptr(unsafe.Pointer(&buf[0]))
		pktLen = uint32(len(buf))
	}
	var recvLen uint32
	r1, _, callErr := w.procRecv.Call(
		handle,
		pktPtr,
		uintptr(pktLen),
		uintptr(unsafe.Pointer(&recvLen)),
		uintptr(unsafe.Pointer(addr)),
	)
	if r1 == 0 {
		return 0, callErr
	}
	return recvLen, nil
}

// Send injects a packet via a WinDivert handle.
func (w *WinDivertDLL) Send(handle uintptr, pkt []byte, addr *winDivertAddress) (uint32, error) {
	if len(pkt) == 0 {
		return 0, nil
	}
	var sendLen uint32
	r1, _, callErr := w.procSend.Call(
		handle,
		uintptr(unsafe.Pointer(&pkt[0])),
		uintptr(len(pkt)),
		uintptr(unsafe.Pointer(&sendLen)),
		uintptr(unsafe.Pointer(addr)),
	)
	if r1 == 0 {
		return 0, callErr
	}
	return sendLen, nil
}

// CalcChecksums recalculates IPv4/TCP/UDP checksums via WinDivertHelperCalcChecksums.
func (w *WinDivertDLL) CalcChecksums(pkt []byte, addr *winDivertAddress, flags uint64) {
	if len(pkt) == 0 {
		return
	}
	_, _, _ = w.procCalcChecksums.Call(
		uintptr(unsafe.Pointer(&pkt[0])),
		uintptr(len(pkt)),
		uintptr(unsafe.Pointer(addr)),
		uintptr(flags),
	)
}

// Shutdown unblocks pending Recv/Send calls cleanly during graceful shutdown.
func (w *WinDivertDLL) Shutdown(handle uintptr, how uint32) error {
	if handle == 0 || handle == winDivertInvalidHandle {
		return nil
	}
	r1, _, callErr := w.procShutdown.Call(handle, uintptr(how))
	if r1 == 0 {
		return callErr
	}
	return nil
}

// Close releases a WinDivert handle.
func (w *WinDivertDLL) Close(handle uintptr) error {
	if handle == 0 || handle == winDivertInvalidHandle {
		return nil
	}
	r1, _, callErr := w.procClose.Call(handle, 0)
	if r1 == 0 {
		return callErr
	}
	return nil
}
