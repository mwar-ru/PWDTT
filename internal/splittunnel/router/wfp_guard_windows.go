//go:build windows

package router

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Dedicated static GUIDs for PWDTT Split Tunneling WFP Provider and Sublayer.
var (
	pwdttProviderGUID = windows.GUID{
		Data1: 0x8B4C2A10,
		Data2: 0x7D3E,
		Data3: 0x4E91,
		Data4: [8]byte{0x9A, 0x55, 0x2F, 0x7C, 0x8E, 0x4B, 0x10, 0xA1},
	}
	pwdttSublayerGUID = windows.GUID{
		Data1: 0x8B4C2A10,
		Data2: 0x7D3E,
		Data3: 0x4E91,
		Data4: [8]byte{0x9A, 0x55, 0x2F, 0x7C, 0x8E, 0x4B, 0x10, 0xA2},
	}

	// Official Windows WFP Layer and Condition GUIDs (fwpmu.h)
	fwpmLayerALEAuthConnectV4 = windows.GUID{
		Data1: 0xc38d57d1,
		Data2: 0x05a7,
		Data3: 0x4c33,
		Data4: [8]byte{0x90, 0x4f, 0x7f, 0xbc, 0xee, 0xe6, 0x0e, 0x82},
	}
	fwpmLayerALEAuthConnectV6 = windows.GUID{
		Data1: 0x4a72393b,
		Data2: 0x319f,
		Data3: 0x44bc,
		Data4: [8]byte{0x84, 0xc3, 0xba, 0x54, 0xdc, 0xb3, 0xb6, 0xb4},
	}
	fwpmConditionALEAppID = windows.GUID{
		Data1: 0xd78e1e87,
		Data2: 0x8644,
		Data3: 0x4ea5,
		Data4: [8]byte{0x94, 0x37, 0xd8, 0x09, 0xec, 0xef, 0xc9, 0x71},
	}
)

const (
	rpcCAuthnDefault = 0xFFFFFFFF

	fwpmSessionFlagDynamic    = 0x00000001
	fwpmProviderFlagPersist   = 0x00000001
	fwpmSublayerFlagPersist   = 0x00000001
	fwpmFilterFlagPersist     = 0x00000001

	fwpActionBlock  = 0x00001001 // FWP_ACTION_FLAG_TERMINATING | 0x0001
	fwpActionPermit = 0x00001002 // FWP_ACTION_FLAG_TERMINATING | 0x0002

	fwpMatchEqual     = 0
	fwpDataTypeUint8  = 1
	fwpDataTypeBlob   = 12

	fwpEAlreadyExists    = 0x80320009
	fwpEFilterNotFound   = 0x80320003
	fwpEProviderNotFound = 0x80320005
	fwpESublayerNotFound = 0x80320007
)

var (
	modFwpuclnt = windows.NewLazySystemDLL("fwpuclnt.dll")

	procFwpmEngineOpen0              = modFwpuclnt.NewProc("FwpmEngineOpen0")
	procFwpmEngineClose0             = modFwpuclnt.NewProc("FwpmEngineClose0")
	procFwpmTransactionBegin0        = modFwpuclnt.NewProc("FwpmTransactionBegin0")
	procFwpmTransactionCommit0       = modFwpuclnt.NewProc("FwpmTransactionCommit0")
	procFwpmTransactionAbort0        = modFwpuclnt.NewProc("FwpmTransactionAbort0")
	procFwpmProviderAdd0             = modFwpuclnt.NewProc("FwpmProviderAdd0")
	procFwpmProviderDeleteByKey0     = modFwpuclnt.NewProc("FwpmProviderDeleteByKey0")
	procFwpmSubLayerAdd0             = modFwpuclnt.NewProc("FwpmSubLayerAdd0")
	procFwpmSubLayerDeleteByKey0     = modFwpuclnt.NewProc("FwpmSubLayerDeleteByKey0")
	procFwpmFilterAdd0               = modFwpuclnt.NewProc("FwpmFilterAdd0")
	procFwpmFilterDeleteById0        = modFwpuclnt.NewProc("FwpmFilterDeleteById0")
	procFwpmFilterCreateEnumHandle0  = modFwpuclnt.NewProc("FwpmFilterCreateEnumHandle0")
	procFwpmFilterEnum0              = modFwpuclnt.NewProc("FwpmFilterEnum0")
	procFwpmFilterDestroyEnumHandle0 = modFwpuclnt.NewProc("FwpmFilterDestroyEnumHandle0")
	procFwpmGetAppIdFromFileName0    = modFwpuclnt.NewProc("FwpmGetAppIdFromFileName0")
	procFwpmFreeMemory0              = modFwpuclnt.NewProc("FwpmFreeMemory0")
)

// Exact MSVC windows/amd64 ABI layouts for fwpuclnt.dll structures.

type fwpByteBlob struct {
	Size uint32
	_    uint32
	Data *byte
}

type fwpmDisplayData0 struct {
	Name        *uint16
	Description *uint16
}

type fwpmSession0 struct {
	SessionKey           windows.GUID
	DisplayData          fwpmDisplayData0
	Flags                uint32
	TxnWaitTimeoutInMSec uint32
	ProcessId            uint32
	_                    uint32
	Sid                  *windows.SID
	Username             *uint16
	KernelMode           uint32
	_                    uint32
}

type fwpmProvider0 struct {
	ProviderKey  windows.GUID
	DisplayData  fwpmDisplayData0
	Flags        uint32
	_            uint32
	ProviderData fwpByteBlob
	ServiceName  *uint16
}

type fwpmSublayer0 struct {
	SubLayerKey  windows.GUID
	DisplayData  fwpmDisplayData0
	Flags        uint32
	_            uint32
	ProviderKey  *windows.GUID
	ProviderData fwpByteBlob
	Weight       uint16
	_            [6]byte
}

type fwpValue0 struct {
	Type  uint32
	_     uint32
	Value uintptr
}

type fwpmFilterCondition0 struct {
	FieldKey       windows.GUID
	MatchType      uint32
	_              uint32
	ConditionValue fwpValue0
}

type fwpmAction0 struct {
	Type       uint32
	FilterType windows.GUID
}

// fwpmFilter0 matches MSVC x64 sizeof(FWPM_FILTER0) == 200 bytes.
// ProviderContextKey uses [2]uint64 to preserve the 8-byte alignment of C union { UINT64; GUID }.
type fwpmFilter0 struct {
	FilterKey           windows.GUID          // offset 0   (16)
	DisplayData         fwpmDisplayData0      // offset 16  (16)
	Flags               uint32                // offset 32  (4 + 4 pad)
	_                   uint32                // offset 36
	ProviderKey         *windows.GUID         // offset 40  (8)
	ProviderData        fwpByteBlob           // offset 48  (16)
	LayerKey            windows.GUID          // offset 64  (16)
	SubLayerKey         windows.GUID          // offset 80  (16)
	Weight              fwpValue0             // offset 96  (16)
	NumFilterConditions uint32                // offset 112 (4 + 4 pad)
	_                   uint32                // offset 116
	FilterCondition     *fwpmFilterCondition0 // offset 120 (8)
	Action              fwpmAction0           // offset 128 (20 + 4 pad)
	_                   uint32                // offset 148
	ProviderContextKey  [2]uint64             // offset 152 (16, 8-byte aligned)
	Reserved            *windows.GUID         // offset 168 (8)
	FilterId            uint64                // offset 176 (8)
	EffectiveWeight     fwpValue0             // offset 184 (16)
}

type fwpmFilterEnumTemplate0 struct {
	ProviderKey             *windows.GUID
	LayerKey                windows.GUID
	EnumType                uint32
	Flags                   uint32
	ProviderContextTemplate uintptr
	NumFilterConditions     uint32
	_                       uint32
	FilterCondition         *fwpmFilterCondition0
	ActionMask              uint32
	_                       uint32
	CalloutKey              *windows.GUID
}

// WindowsWFPGuard implements WFPGuard using user-mode fwpuclnt.dll.
type WindowsWFPGuard struct {
	mu             sync.Mutex
	engineHandle   uintptr
	persistentMode bool
	activeIDs      []uint64
}

// NewWindowsWFPGuard creates a Windows user-mode WFP Kill Switch controller.
func NewWindowsWFPGuard() WFPGuard {
	return &WindowsWFPGuard{}
}

func (g *WindowsWFPGuard) ensureSessionLocked(persistent bool) error {
	if g.engineHandle != 0 && g.persistentMode == persistent {
		return nil
	}
	if g.engineHandle != 0 {
		g.clearFiltersLocked()
		_, _, _ = procFwpmEngineClose0.Call(g.engineHandle)
		g.engineHandle = 0
	}

	namePtr, _ := windows.UTF16PtrFromString("PWDTT Split Tunneling WFP Session")
	descPtr, _ := windows.UTF16PtrFromString("Per-App Kill Switch and IPv6 Leak Guard")

	var session fwpmSession0
	session.DisplayData = fwpmDisplayData0{Name: namePtr, Description: descPtr}
	if !persistent {
		session.Flags = fwpmSessionFlagDynamic
	}

	var handle uintptr
	r1, _, _ := procFwpmEngineOpen0.Call(
		0,
		rpcCAuthnDefault,
		0,
		uintptr(unsafe.Pointer(&session)),
		uintptr(unsafe.Pointer(&handle)),
	)
	if r1 != 0 {
		return fmt.Errorf("FwpmEngineOpen0 failed: 0x%x", r1)
	}

	g.engineHandle = handle
	g.persistentMode = persistent

	if err := g.ensureProviderAndSublayerLocked(persistent); err != nil {
		_, _, _ = procFwpmEngineClose0.Call(g.engineHandle)
		g.engineHandle = 0
		return err
	}
	setActiveWFPGuard(g)
	return nil
}

func (g *WindowsWFPGuard) ensureProviderAndSublayerLocked(persistent bool) error {
	provName, _ := windows.UTF16PtrFromString("PWDTT Split Tunneling")
	provDesc, _ := windows.UTF16PtrFromString("Isolated provider for PWDTT per-app Kill Switch")
	var prov fwpmProvider0
	prov.ProviderKey = pwdttProviderGUID
	prov.DisplayData = fwpmDisplayData0{Name: provName, Description: provDesc}
	if persistent {
		prov.Flags = fwpmProviderFlagPersist
	}

	r1, _, _ := procFwpmProviderAdd0.Call(g.engineHandle, uintptr(unsafe.Pointer(&prov)), 0)
	if r1 != 0 && r1 != fwpEAlreadyExists {
		return fmt.Errorf("FwpmProviderAdd0 failed: 0x%x", r1)
	}

	subName, _ := windows.UTF16PtrFromString("PWDTT Split Tunneling Sublayer")
	subDesc, _ := windows.UTF16PtrFromString("High-priority sublayer for PWDTT fail-closed rules")
	var sub fwpmSublayer0
	sub.SubLayerKey = pwdttSublayerGUID
	sub.DisplayData = fwpmDisplayData0{Name: subName, Description: subDesc}
	sub.ProviderKey = &pwdttProviderGUID
	sub.Weight = 0xFFFE
	if persistent {
		sub.Flags = fwpmSublayerFlagPersist
	}

	r1, _, _ = procFwpmSubLayerAdd0.Call(g.engineHandle, uintptr(unsafe.Pointer(&sub)), 0)
	if r1 != 0 && r1 != fwpEAlreadyExists {
		return fmt.Errorf("FwpmSubLayerAdd0 failed: 0x%x", r1)
	}
	return nil
}

// Apply atomically replaces PWDTT's active WFP filters with the computed filter plan.
func (g *WindowsWFPGuard) Apply(cfg WFPGuardConfig, tunnelUp bool) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	specs := BuildWFPFilterPlan(cfg, tunnelUp)
	if len(specs) == 0 {
		if g.engineHandle != 0 {
			g.clearFiltersLocked()
		}
		return nil
	}

	persistent := specs[0].Persistent
	if err := g.ensureSessionLocked(persistent); err != nil {
		return err
	}

	if r1, _, _ := procFwpmTransactionBegin0.Call(g.engineHandle, 0); r1 != 0 {
		return fmt.Errorf("FwpmTransactionBegin0 failed: 0x%x", r1)
	}

	// Remove previous filters within the same transaction for seamless swap
	for _, id := range g.activeIDs {
		_, _, _ = procFwpmFilterDeleteById0.Call(g.engineHandle, uintptr(id))
	}

	newIDs := make([]uint64, 0, len(specs))
	for _, spec := range specs {
		id, err := g.addFilterLocked(spec)
		if err != nil {
			_, _, _ = procFwpmTransactionAbort0.Call(g.engineHandle)
			return err
		}
		if id != 0 {
			newIDs = append(newIDs, id)
		}
	}

	if r1, _, _ := procFwpmTransactionCommit0.Call(g.engineHandle); r1 != 0 {
		_, _, _ = procFwpmTransactionAbort0.Call(g.engineHandle)
		return fmt.Errorf("FwpmTransactionCommit0 failed: 0x%x", r1)
	}

	g.activeIDs = newIDs
	return nil
}

func (g *WindowsWFPGuard) addFilterLocked(spec WFPFilterSpec) (uint64, error) {
	namePtr, err := windows.UTF16PtrFromString(spec.Name)
	if err != nil {
		return 0, err
	}

	var layerGUID windows.GUID
	switch spec.Layer {
	case LayerALEAuthConnectV6:
		layerGUID = fwpmLayerALEAuthConnectV6
	default:
		layerGUID = fwpmLayerALEAuthConnectV4
	}

	var actionType uint32 = fwpActionBlock
	if spec.Action == ActionPermit {
		actionType = fwpActionPermit
	}

	var conditions []fwpmFilterCondition0
	var appBlob *fwpByteBlob
	if spec.AppPath != "" {
		appPathPtr, pErr := windows.UTF16PtrFromString(spec.AppPath)
		if pErr != nil {
			return 0, pErr
		}
		r1, _, _ := procFwpmGetAppIdFromFileName0.Call(
			uintptr(unsafe.Pointer(appPathPtr)),
			uintptr(unsafe.Pointer(&appBlob)),
		)
		if r1 != 0 {
			return 0, nil
		}
		defer func(blob *fwpByteBlob) {
			if blob != nil {
				ptr := uintptr(unsafe.Pointer(blob))
				_, _, _ = procFwpmFreeMemory0.Call(uintptr(unsafe.Pointer(&ptr)))
			}
		}(appBlob)

		conditions = append(conditions, fwpmFilterCondition0{
			FieldKey:  fwpmConditionALEAppID,
			MatchType: fwpMatchEqual,
			ConditionValue: fwpValue0{
				Type:  fwpDataTypeBlob,
				Value: uintptr(unsafe.Pointer(appBlob)),
			},
		})
	}

	var filter fwpmFilter0
	filter.DisplayData = fwpmDisplayData0{Name: namePtr}
	filter.ProviderKey = &pwdttProviderGUID
	filter.LayerKey = layerGUID
	filter.SubLayerKey = pwdttSublayerGUID
	filter.Weight = fwpValue0{
		Type:  fwpDataTypeUint8,
		Value: uintptr(spec.Weight),
	}
	filter.Action = fwpmAction0{Type: actionType}
	if spec.Persistent {
		filter.Flags = fwpmFilterFlagPersist
	}
	if len(conditions) > 0 {
		filter.NumFilterConditions = uint32(len(conditions))
		filter.FilterCondition = &conditions[0]
	}

	var filterID uint64
	r1, _, _ := procFwpmFilterAdd0.Call(
		g.engineHandle,
		uintptr(unsafe.Pointer(&filter)),
		0,
		uintptr(unsafe.Pointer(&filterID)),
	)
	if r1 != 0 {
		return 0, fmt.Errorf("FwpmFilterAdd0(%s) failed: 0x%x", spec.Name, r1)
	}
	return filterID, nil
}

func (g *WindowsWFPGuard) clearFiltersLocked() {
	if g.engineHandle == 0 || len(g.activeIDs) == 0 {
		g.activeIDs = nil
		return
	}
	_, _, _ = procFwpmTransactionBegin0.Call(g.engineHandle, 0)
	for _, id := range g.activeIDs {
		_, _, _ = procFwpmFilterDeleteById0.Call(g.engineHandle, uintptr(id))
	}
	_, _, _ = procFwpmTransactionCommit0.Call(g.engineHandle)
	g.activeIDs = nil
}

// Clear removes all active WFP filters created by this guard.
func (g *WindowsWFPGuard) Clear() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.clearFiltersLocked()
	return nil
}

// ResetStale removes any orphaned persistent PWDTT WFP filters left by a previous crash.
func (g *WindowsWFPGuard) ResetStale() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.clearFiltersLocked()
	if g.engineHandle != 0 {
		_, _, _ = procFwpmEngineClose0.Call(g.engineHandle)
		g.engineHandle = 0
	}
	return resetWfpRulesEngine()
}

// Close clears filters and closes the WFP engine session.
func (g *WindowsWFPGuard) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.clearFiltersLocked()
	if g.engineHandle != 0 {
		_, _, _ = procFwpmEngineClose0.Call(g.engineHandle)
		g.engineHandle = 0
	}
	return nil
}

// ActiveFilterCount returns the number of currently installed WFP filters.
func (g *WindowsWFPGuard) ActiveFilterCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.activeIDs)
}

var _ = syscall.Errno(0)
