package vpn

import (
	"bytes"
	"os"
	"sync"
	"testing"

	"golang.zx2c4.com/wireguard/tun"
)

type fakeTunDevice struct {
	mu      sync.Mutex
	inCh    chan []byte
	written [][]byte
	closed  bool
	events  chan tun.Event
}

func newFakeTunDevice() *fakeTunDevice {
	return &fakeTunDevice{
		inCh:   make(chan []byte, 16),
		events: make(chan tun.Event, 1),
	}
}

func (f *fakeTunDevice) File() *os.File          { return nil }
func (f *fakeTunDevice) MTU() (int, error)        { return 1420, nil }
func (f *fakeTunDevice) Name() (string, error)    { return "pwdtt-wg", nil }
func (f *fakeTunDevice) Events() <-chan tun.Event { return f.events }
func (f *fakeTunDevice) BatchSize() int           { return 4 }

func (f *fakeTunDevice) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	pkt, ok := <-f.inCh
	if !ok {
		return 0, os.ErrClosed
	}
	copy(bufs[0][offset:], pkt)
	sizes[0] = len(pkt)
	return 1, nil
}

func (f *fakeTunDevice) Write(bufs [][]byte, offset int) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, b := range bufs {
		cp := append([]byte(nil), b[offset:]...)
		f.written = append(f.written, cp)
	}
	return len(bufs), nil
}

func (f *fakeTunDevice) Close() error {
	f.mu.Lock()
	if !f.closed {
		f.closed = true
		close(f.inCh)
	}
	f.mu.Unlock()
	return nil
}

func TestWrapTunDeviceInjectAndInterceptInbound(t *testing.T) {
	inner := newFakeTunDevice()
	wrapped := WrapTunDevice(inner)
	defer func() {
		SetInboundPacketHandler(nil)
		_ = wrapped.Close()
	}()

	if !HasActiveTunPipe() {
		t.Fatal("expected active tun pipe")
	}

	want := []byte{0x45, 0x00, 0x00, 0x20, 0xaa, 0xbb}
	if !InjectOutboundPacket(want) {
		t.Fatal("InjectOutboundPacket returned false")
	}
	bufs := [][]byte{make([]byte, 128)}
	sizes := []int{0}
	n, err := wrapped.Read(bufs, sizes, 16)
	if err != nil || n != 1 {
		t.Fatalf("wrapped.Read failed: n=%d err=%v", n, err)
	}
	if !bytes.Equal(bufs[0][16:16+sizes[0]], want) {
		t.Fatalf("read payload mismatch: got %x want %x", bufs[0][16:16+sizes[0]], want)
	}

	var intercepted [][]byte
	SetInboundPacketHandler(func(pkt []byte) bool {
		if len(pkt) > 0 && pkt[0] == 0xAA {
			intercepted = append(intercepted, append([]byte(nil), pkt...))
			return true
		}
		return false
	})

	p1 := append(make([]byte, 16), 0xAA, 0x01, 0x02)
	p2 := append(make([]byte, 16), 0xBB, 0x03, 0x04)
	wn, err := wrapped.Write([][]byte{p1, p2}, 16)
	if err != nil || wn != 2 {
		t.Fatalf("wrapped.Write failed: wn=%d err=%v", wn, err)
	}
	if len(intercepted) != 1 || intercepted[0][0] != 0xAA {
		t.Fatalf("expected 1 intercepted packet, got %d", len(intercepted))
	}
	inner.mu.Lock()
	gotPass := len(inner.written)
	inner.mu.Unlock()
	if gotPass != 1 {
		t.Fatalf("expected 1 passthrough packet to inner Wintun, got %d", gotPass)
	}

	_ = wrapped.Close()
	if HasActiveTunPipe() {
		t.Fatal("expected active pipe cleared on Close")
	}
}
