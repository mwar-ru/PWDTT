package router

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

const defaultSocketOwnerTTL = 5 * time.Minute

// SocketOwner stores PID attribution for a local TCP or UDP port.
type SocketOwner struct {
	PID       uint32
	UpdatedAt time.Time
}

// SocketTable maps local transport endpoints (Proto, LocalPort) to owning PIDs.
type SocketTable struct {
	mu       sync.RWMutex
	owners   map[SocketKey]SocketOwner
	onMapped func(key SocketKey, pid uint32)
	nowFn    func() time.Time
}

// NewSocketTable creates a thread-safe SocketTable with an optional mapping callback.
func NewSocketTable(onMapped func(key SocketKey, pid uint32)) *SocketTable {
	return &SocketTable{
		owners:   make(map[SocketKey]SocketOwner, 512),
		onMapped: onMapped,
		nowFn:    time.Now,
	}
}

// Set records or updates the owning PID for key and triggers onMapped outside the lock.
func (st *SocketTable) Set(key SocketKey, pid uint32) {
	if pid == 0 || key.LocalPort == 0 {
		return
	}
	now := st.nowFn()
	st.mu.Lock()
	st.owners[key] = SocketOwner{PID: pid, UpdatedAt: now}
	cb := st.onMapped
	st.mu.Unlock()

	if cb != nil {
		cb(key, pid)
	}
}

// Delete removes a closed socket mapping.
func (st *SocketTable) Delete(key SocketKey) {
	st.mu.Lock()
	delete(st.owners, key)
	st.mu.Unlock()
}

// Lookup returns the owning PID for key, if known.
func (st *SocketTable) Lookup(key SocketKey) (uint32, bool) {
	st.mu.RLock()
	owner, ok := st.owners[key]
	st.mu.RUnlock()
	if !ok || owner.PID == 0 {
		return 0, false
	}
	return owner.PID, true
}

// BulkSeed populates initial or refreshed OS socket mappings without triggering per-entry callbacks.
func (st *SocketTable) BulkSeed(entries map[SocketKey]uint32) {
	if len(entries) == 0 {
		return
	}
	now := st.nowFn()
	st.mu.Lock()
	for k, pid := range entries {
		if pid != 0 && k.LocalPort != 0 {
			st.owners[k] = SocketOwner{PID: pid, UpdatedAt: now}
		}
	}
	st.mu.Unlock()
}

// SweepExpired removes stale socket entries older than ttl.
func (st *SocketTable) SweepExpired(now time.Time, ttl time.Duration) int {
	if ttl <= 0 {
		ttl = defaultSocketOwnerTTL
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	removed := 0
	for k, owner := range st.owners {
		if now.Sub(owner.UpdatedAt) > ttl {
			delete(st.owners, k)
			removed++
		}
	}
	return removed
}

// Len returns the number of active socket-to-PID entries.
func (st *SocketTable) Len() int {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return len(st.owners)
}

// ParseChecksumManifest parses standard sha256sum format (`<hex>  <filename>`) ignoring comments.
func ParseChecksumManifest(content string) map[string]string {
	out := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}
		hashHex := strings.ToLower(strings.TrimSpace(fields[0]))
		fileName := strings.TrimPrefix(strings.TrimSpace(fields[1]), "*")
		if len(hashHex) == 64 && fileName != "" {
			out[strings.ToLower(fileName)] = hashHex
		}
	}
	return out
}

// VerifyFileSHA256 verifies that the file at path matches expectedHex (case-insensitive).
// If expectedHex is empty, verification only checks that the file exists and is readable.
func VerifyFileSHA256(path, expectedHex string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot open %s: %w", path, err)
	}
	defer f.Close()

	expected := strings.ToLower(strings.TrimSpace(expectedHex))
	if expected == "" {
		return nil
	}

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hashing %s failed: %w", path, err)
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expected {
		return fmt.Errorf("SHA-256 mismatch for %s: got %s, want %s", path, actual, expected)
	}
	return nil
}
