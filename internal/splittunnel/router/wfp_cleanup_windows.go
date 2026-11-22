//go:build windows

package router

import (
	"fmt"
	"sync"
	"unsafe"
)

var (
	activeWFPGuardMu sync.Mutex
	activeWFPGuard   *WindowsWFPGuard
)

func setActiveWFPGuard(g *WindowsWFPGuard) {
	activeWFPGuardMu.Lock()
	activeWFPGuard = g
	activeWFPGuardMu.Unlock()
}

// ResetWfpRules closes any active in-process WFP session first (releasing dynamic objects),
// then deletes ALL Windows Filtering Platform filters, sublayer, and provider belonging to PWDTT.
func ResetWfpRules() error {
	activeWFPGuardMu.Lock()
	g := activeWFPGuard
	activeWFPGuardMu.Unlock()

	if g != nil {
		_ = g.Close()
	}
	return resetWfpRulesEngine()
}

func resetWfpRulesEngine() error {
	var engineHandle uintptr
	r1, _, _ := procFwpmEngineOpen0.Call(
		0,
		rpcCAuthnDefault,
		0,
		0,
		uintptr(unsafe.Pointer(&engineHandle)),
	)
	if r1 != 0 {
		return fmt.Errorf("FwpmEngineOpen0 for ResetWfpRules failed: 0x%x", r1)
	}
	defer procFwpmEngineClose0.Call(engineHandle)

	if r1, _, _ := procFwpmTransactionBegin0.Call(engineHandle, 0); r1 != 0 {
		return fmt.Errorf("FwpmTransactionBegin0 failed: 0x%x", r1)
	}

	var enumHandle uintptr
	r1, _, _ = procFwpmFilterCreateEnumHandle0.Call(
		engineHandle,
		0,
		uintptr(unsafe.Pointer(&enumHandle)),
	)
	if r1 != 0 {
		_, _, _ = procFwpmTransactionAbort0.Call(engineHandle)
		return fmt.Errorf("FwpmFilterCreateEnumHandle0 failed: 0x%x", r1)
	}

	var toDelete []uint64
	const batchSize = 128
	for {
		var entriesPtr **fwpmFilter0
		var returned uint32
		r1, _, _ = procFwpmFilterEnum0.Call(
			engineHandle,
			enumHandle,
			batchSize,
			uintptr(unsafe.Pointer(&entriesPtr)),
			uintptr(unsafe.Pointer(&returned)),
		)
		if r1 != 0 || returned == 0 || entriesPtr == nil {
			break
		}

		entries := unsafe.Slice(entriesPtr, returned)
		for _, f := range entries {
			if f == nil || f.FilterId == 0 {
				continue
			}
			matchesProvider := f.ProviderKey != nil && *f.ProviderKey == pwdttProviderGUID
			matchesSublayer := f.SubLayerKey == pwdttSublayerGUID
			if matchesProvider || matchesSublayer {
				toDelete = append(toDelete, f.FilterId)
			}
		}

		memPtr := uintptr(unsafe.Pointer(entriesPtr))
		_, _, _ = procFwpmFreeMemory0.Call(uintptr(unsafe.Pointer(&memPtr)))
	}
	_, _, _ = procFwpmFilterDestroyEnumHandle0.Call(engineHandle, enumHandle)

	for _, id := range toDelete {
		r1, _, _ = procFwpmFilterDeleteById0.Call(engineHandle, uintptr(id))
		if r1 != 0 && r1 != fwpEFilterNotFound {
			_, _, _ = procFwpmTransactionAbort0.Call(engineHandle)
			return fmt.Errorf("FwpmFilterDeleteById0(%d) failed: 0x%x", id, r1)
		}
	}

	r1, _, _ = procFwpmSubLayerDeleteByKey0.Call(engineHandle, uintptr(unsafe.Pointer(&pwdttSublayerGUID)))
	if r1 != 0 && r1 != fwpESublayerNotFound && r1 != fwpEFilterNotFound {
		_, _, _ = procFwpmTransactionAbort0.Call(engineHandle)
		return fmt.Errorf("FwpmSubLayerDeleteByKey0 failed: 0x%x", r1)
	}

	r1, _, _ = procFwpmProviderDeleteByKey0.Call(engineHandle, uintptr(unsafe.Pointer(&pwdttProviderGUID)))
	if r1 != 0 && r1 != fwpEProviderNotFound {
		_, _, _ = procFwpmTransactionAbort0.Call(engineHandle)
		return fmt.Errorf("FwpmProviderDeleteByKey0 failed: 0x%x", r1)
	}

	if r1, _, _ := procFwpmTransactionCommit0.Call(engineHandle); r1 != 0 {
		_, _, _ = procFwpmTransactionAbort0.Call(engineHandle)
		return fmt.Errorf("FwpmTransactionCommit0 failed: 0x%x", r1)
	}
	return nil
}
