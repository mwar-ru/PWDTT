package vpn

import (
	"os"
	"sync"

	"golang.zx2c4.com/wireguard/tun"
)

const (
	tunPipeQueueCap    = 4096
	tunPipeInnerOffset = 16
	tunPipeMaxPacket   = 65535
)

// InboundPacketHandler inspects decrypted IP packets coming out of WireGuard (tun.Device.Write).
// If handled == true, the router has DNAT'd and re-injected the packet into Windows via WinDivert.
type InboundPacketHandler func(pkt []byte) (handled bool)

var (
	pipeMu         sync.RWMutex
	activePipe     *splitTunDevice
	inboundHandler InboundPacketHandler
)

// SetInboundPacketHandler registers the router's inbound tunnel packet handler (nil to clear).
func SetInboundPacketHandler(h InboundPacketHandler) {
	pipeMu.Lock()
	inboundHandler = h
	pipeMu.Unlock()
}

// InjectOutboundPacket pushes an SNAT'd raw IP packet directly into WireGuard's tun.Device.Read loop,
// bypassing Windows outbound routing table limitations for WinDivert.
func InjectOutboundPacket(pkt []byte) bool {
	if len(pkt) == 0 {
		return false
	}
	pipeMu.RLock()
	p := activePipe
	pipeMu.RUnlock()
	if p == nil {
		return false
	}
	return p.enqueueOutbound(pkt)
}

// HasActiveTunPipe reports whether a wrapped WireGuard tun.Device is currently active.
func HasActiveTunPipe() bool {
	pipeMu.RLock()
	ok := activePipe != nil
	pipeMu.RUnlock()
	return ok
}

// WrapTunDevice wraps a WireGuard tun.Device so split-tunneling can inject outbound packets
// directly into wireguard-go and intercept decrypted inbound replies before Wintun.
func WrapTunDevice(inner tun.Device) tun.Device {
	if inner == nil {
		return nil
	}
	d := &splitTunDevice{
		inner:  inner,
		txCh:   make(chan []byte, tunPipeQueueCap),
		doneCh: make(chan struct{}),
	}
	pipeMu.Lock()
	activePipe = d
	pipeMu.Unlock()

	go d.pumpInnerRead()
	return d
}

type splitTunDevice struct {
	inner     tun.Device
	txCh      chan []byte
	doneCh    chan struct{}
	closeOnce sync.Once

	errMu   sync.Mutex
	readErr error
}

var _ tun.Device = (*splitTunDevice)(nil)

func (d *splitTunDevice) enqueueOutbound(pkt []byte) bool {
	cp := make([]byte, len(pkt))
	copy(cp, pkt)
	select {
	case <-d.doneCh:
		return false
	case d.txCh <- cp:
		return true
	default:
		return false
	}
}

func (d *splitTunDevice) pumpInnerRead() {
	batch := d.inner.BatchSize()
	if batch < 1 {
		batch = 1
	}
	bufs := make([][]byte, batch)
	sizes := make([]int, batch)
	for i := range bufs {
		bufs[i] = make([]byte, tunPipeInnerOffset+tunPipeMaxPacket)
	}

	for {
		n, err := d.inner.Read(bufs, sizes, tunPipeInnerOffset)
		if err != nil {
			d.errMu.Lock()
			d.readErr = err
			d.errMu.Unlock()
			d.markClosed()
			return
		}
		for i := 0; i < n; i++ {
			sz := sizes[i]
			if sz <= 0 || tunPipeInnerOffset+sz > len(bufs[i]) {
				continue
			}
			cp := make([]byte, sz)
			copy(cp, bufs[i][tunPipeInnerOffset:tunPipeInnerOffset+sz])
			select {
			case <-d.doneCh:
				return
			case d.txCh <- cp:
			default:
			}
		}
	}
}

func (d *splitTunDevice) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	if len(bufs) == 0 || len(sizes) == 0 {
		return 0, nil
	}
	maxBatch := len(bufs)
	if len(sizes) < maxBatch {
		maxBatch = len(sizes)
	}

	select {
	case <-d.doneCh:
		d.errMu.Lock()
		err := d.readErr
		d.errMu.Unlock()
		if err != nil {
			return 0, err
		}
		return 0, os.ErrClosed
	case pkt := <-d.txCh:
		n := 0
		if offset+len(pkt) <= len(bufs[0]) {
			copy(bufs[0][offset:], pkt)
			sizes[0] = len(pkt)
			n = 1
		}
		for n < maxBatch {
			select {
			case more := <-d.txCh:
				if offset+len(more) <= len(bufs[n]) {
					copy(bufs[n][offset:], more)
					sizes[n] = len(more)
					n++
				}
			default:
				return n, nil
			}
		}
		return n, nil
	}
}

func (d *splitTunDevice) Write(bufs [][]byte, offset int) (int, error) {
	pipeMu.RLock()
	h := inboundHandler
	pipeMu.RUnlock()

	if h == nil {
		return d.inner.Write(bufs, offset)
	}

	var passthrough [][]byte
	for _, buf := range bufs {
		if offset >= len(buf) {
			continue
		}
		pkt := buf[offset:]
		if h(pkt) {
			continue
		}
		passthrough = append(passthrough, buf)
	}
	if len(passthrough) > 0 {
		if _, err := d.inner.Write(passthrough, offset); err != nil {
			return len(bufs) - len(passthrough), err
		}
	}
	return len(bufs), nil
}

func (d *splitTunDevice) File() *os.File          { return d.inner.File() }
func (d *splitTunDevice) MTU() (int, error)       { return d.inner.MTU() }
func (d *splitTunDevice) Name() (string, error)   { return d.inner.Name() }
func (d *splitTunDevice) Events() <-chan tun.Event { return d.inner.Events() }
func (d *splitTunDevice) BatchSize() int          { return d.inner.BatchSize() }

func (d *splitTunDevice) markClosed() {
	d.closeOnce.Do(func() {
		pipeMu.Lock()
		if activePipe == d {
			activePipe = nil
		}
		pipeMu.Unlock()
		close(d.doneCh)
	})
}

func (d *splitTunDevice) Close() error {
	d.markClosed()
	return d.inner.Close()
}
