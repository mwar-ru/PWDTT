package router

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSocketTableSetLookupDeleteCallbackAndSweep(t *testing.T) {
	var mappedKeys []SocketKey
	var mappedPIDs []uint32
	now := time.Now()

	st := NewSocketTable(func(key SocketKey, pid uint32) {
		mappedKeys = append(mappedKeys, key)
		mappedPIDs = append(mappedPIDs, pid)
	})
	st.nowFn = func() time.Time { return now }

	k1 := SocketKey{Proto: ProtoUDP, LocalPort: 53001}
	k2 := SocketKey{Proto: ProtoTCP, LocalPort: 53002}

	st.Set(k1, 4242)
	if pid, ok := st.Lookup(k1); !ok || pid != 4242 {
		t.Fatalf("expected PID 4242, got %d (ok=%v)", pid, ok)
	}
	if len(mappedKeys) != 1 || mappedKeys[0] != k1 || mappedPIDs[0] != 4242 {
		t.Fatalf("expected onMapped callback triggered for k1")
	}

	st.BulkSeed(map[SocketKey]uint32{k2: 9999})
	if pid, ok := st.Lookup(k2); !ok || pid != 9999 {
		t.Fatalf("expected BulkSeed PID 9999, got %d (ok=%v)", pid, ok)
	}
	if len(mappedKeys) != 1 {
		t.Fatalf("BulkSeed should not spam onMapped callbacks")
	}

	st.Delete(k1)
	if _, ok := st.Lookup(k1); ok {
		t.Fatalf("expected k1 deleted")
	}

	removed := st.SweepExpired(now.Add(10*time.Minute), 5*time.Minute)
	if removed != 1 || st.Len() != 0 {
		t.Fatalf("expected k2 swept, removed=%d len=%d", removed, st.Len())
	}
}

func TestSocketTableConcurrentRace(t *testing.T) {
	st := NewSocketTable(nil)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			k := SocketKey{Proto: ProtoTCP, LocalPort: uint16(30000 + id)}
			for j := 0; j < 200; j++ {
				st.Set(k, uint32(1000+id))
				st.Lookup(k)
				if j%25 == 0 {
					st.Delete(k)
				}
			}
		}(i)
	}
	wg.Wait()
}

func TestVerifyFileSHA256AndParseManifest(t *testing.T) {
	dir := t.TempDir()
	samplePath := filepath.Join(dir, "WinDivert.dll")
	content := []byte("official-windivert-2.2-binary-payload")
	if err := os.WriteFile(samplePath, content, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	sum := sha256.Sum256(content)
	hexSum := hex.EncodeToString(sum[:])

	manifest := "# comment line\n" + hexSum + "  WinDivert.dll\n"
	parsed := ParseChecksumManifest(manifest)
	if parsed["windivert.dll"] != hexSum {
		t.Fatalf("ParseChecksumManifest failed: %+v", parsed)
	}

	if err := VerifyFileSHA256(samplePath, parsed["windivert.dll"]); err != nil {
		t.Fatalf("expected valid SHA-256 verification, got error: %v", err)
	}

	// Tampered hash MUST fail
	if err := VerifyFileSHA256(samplePath, "0000000000000000000000000000000000000000000000000000000000000000"); err == nil {
		t.Fatalf("expected SHA-256 mismatch error")
	}
}
