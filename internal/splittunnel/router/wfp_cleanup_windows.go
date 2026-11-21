//go:build windows

package router

import (
	"fmt"
	"unsafe"
)

// ResetWfpRules safely enumerates and deletes ALL Windows Filtering Platform filters,
// sublayer, and provider belonging strictly to PWDTT's dedicated Provider GUID.
// Safe to call on startup (to recover from a prior SIGKILL crash in persistent_guard mode)
// or manually from UI / CLI recovery tools.
func ResetWfpRules() error {
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

	// Enumerate and delete all filters owned by pwdttProviderGUID
	var tmpl fwpmFilterEnumTemplate0
	tmpl.ProviderKey = &pwdttProviderGUID

	var enumHandle uintptr
	r1, _, _ = procFwpmFilterCreateEnumHandle0.Call(
		engineHandle,
		uintptr(unsafe.Pointer(&tmpl)),
		uintptr(unsafe.Pointer(&enumHandle)),
	)
	if r1 == 0 && enumHandle != 0 {
		defer procFwpmFilterDestroyEnumHandle0.Call(engineHandle, enumHandle)

		const batchSize = 64
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
				if f != nil && f.FilterId != 0 {
					_, _, _ = procFwpmFilterDeleteById0.Call(engineHandle, uintptr(f.FilterId))
				}
			}

			memPtr := uintptr(unsafe.Pointer(entriesPtr))
			_, _, _ = procFwpmFreeMemory0.Call(uintptr(unsafe.Pointer(&memPtr)))
		}
	}

	// Delete our dedicated sublayer and provider (ignore NOT_FOUND)
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
