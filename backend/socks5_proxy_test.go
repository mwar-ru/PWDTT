package backend

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"
)

type loopbackDialer struct{}

func (loopbackDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, network, address)
}

func (loopbackDialer) LookupContextHost(ctx context.Context, host string) ([]string, error) {
	return net.DefaultResolver.LookupHost(ctx, host)
}

func TestSocks5TCPConnectAndUDPRoundtrip(t *testing.T) {
	echoLn, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echoLn.Close()
	go func() {
		for {
			c, err := echoLn.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}(c)
		}
	}()

	srv, err := startSocks5Server("127.0.0.1:0", loopbackDialer{})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	c, err := net.Dial("tcp4", srv.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	_, _ = c.Write([]byte{0x05, 0x01, 0x00})
	gr := make([]byte, 2)
	if _, err := io.ReadFull(c, gr); err != nil || gr[0] != 0x05 || gr[1] != 0x00 {
		t.Fatalf("bad greeting: %v %x", err, gr)
	}

	ea := echoLn.Addr().(*net.TCPAddr)
	_, _ = c.Write([]byte{0x05, 0x01, 0x00, 0x01, 127, 0, 0, 1, byte(ea.Port >> 8), byte(ea.Port)})
	rep := make([]byte, 10)
	if _, err := io.ReadFull(c, rep); err != nil || rep[1] != 0x00 {
		t.Fatalf("bad connect reply: %v %x", err, rep)
	}

	msg := []byte("hello-mihomo")
	_, _ = c.Write(msg)
	got := make([]byte, len(msg))
	if _, err := io.ReadFull(c, got); err != nil || !bytes.Equal(got, msg) {
		t.Fatalf("echo mismatch: got=%q err=%v", got, err)
	}

	pkt, err := buildSocksUDPDatagram("example.com", 53, []byte("dns-query"))
	if err != nil {
		t.Fatal(err)
	}
	h, p, pl, err := parseSocksUDPDatagram(pkt)
	if err != nil || h != "example.com" || p != 53 || string(pl) != "dns-query" {
		t.Fatalf("udp parse mismatch: h=%q p=%d pl=%q err=%v", h, p, pl, err)
	}
}

func TestSocks5CloseImmediateWithActiveUDPAndTCP(t *testing.T) {
	// Start an idle TCP upstream that never closes on its own
	tcpLn, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tcpLn.Close()
	go func() {
		c, err := tcpLn.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_, _ = io.Copy(io.Discard, c)
	}()

	srv, err := startSocks5Server("127.0.0.1:0", directDialer{})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Open active idle TCP CONNECT session
	tcpClient, err := net.Dial("tcp", srv.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer tcpClient.Close()
	_, _ = tcpClient.Write([]byte{0x05, 0x01, 0x00})
	resp := make([]byte, 2)
	_, _ = io.ReadFull(tcpClient, resp)

	tcpPort := uint16(tcpLn.Addr().(*net.TCPAddr).Port)
	req := []byte{0x05, 0x01, 0x00, 0x01, 127, 0, 0, 1, byte(tcpPort >> 8), byte(tcpPort)}
	_, _ = tcpClient.Write(req)
	reply := make([]byte, 10)
	if _, err := io.ReadFull(tcpClient, reply); err != nil || reply[1] != 0x00 {
		t.Fatalf("tcp connect failed: err=%v rep=%x", err, reply[1])
	}

	// 2. Open active idle UDP ASSOCIATE session (simulates mihomo udp: true)
	udpCtrl, err := net.Dial("tcp", srv.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer udpCtrl.Close()
	_, _ = udpCtrl.Write([]byte{0x05, 0x01, 0x00})
	_, _ = io.ReadFull(udpCtrl, resp)
	_, _ = udpCtrl.Write([]byte{0x05, 0x03, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
	if _, err := io.ReadFull(udpCtrl, reply); err != nil || reply[1] != 0x00 {
		t.Fatalf("udp associate failed: err=%v rep=%x", err, reply[1])
	}

	// 3. Close() MUST return immediately without hanging
	start := time.Now()
	if err := srv.Close(); err != nil {
		t.Fatalf("srv.Close() error: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 400*time.Millisecond {
		t.Fatalf("srv.Close() took too long (%v) with active TCP+UDP sessions", elapsed)
	}
}

func TestWGProxyApplyAndTeardownRealNetstack(t *testing.T) {
	// Valid 32-byte base64 WG keys (test-only dummy keys)
	conf := `[Interface]
PrivateKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=
Address = 10.66.66.2/32
DNS = 1.1.1.1
MTU = 1280

[Peer]
PublicKey = BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB=
Endpoint = 127.0.0.1:51820
AllowedIPs = 0.0.0.0/0
`
	// Pick a free ephemeral port
	tmpLn, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := tmpLn.Addr().(*net.TCPAddr).Port
	_ = tmpLn.Close()

	w := &WG{}
	w.SetProxyMode(true, port)
	defer w.StopProxyServer()

	// Cycle 1: apply -> teardown (must stay in DIRECT standby without panic or hang)
	if err := w.applyProxy(conf, func(string) {}); err != nil {
		t.Fatalf("applyProxy #1 failed: %v", err)
	}
	start := time.Now()
	if !w.teardownProxy() {
		t.Fatalf("teardownProxy #1 returned false")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("teardownProxy #1 too slow: %v", elapsed)
	}
	if w.activeSocks == nil {
		t.Fatalf("expected SOCKS5 standby listener to remain open in DIRECT mode")
	}

	// Cycle 2: re-apply reusing standby listener -> disable proxy mode -> teardown
	if err := w.applyProxy(conf, func(string) {}); err != nil {
		t.Fatalf("applyProxy #2 failed: %v", err)
	}
	w.SetProxyMode(false, port)
	if !w.teardownProxy() {
		t.Fatalf("teardownProxy #2 returned false")
	}
	if w.activeSocks != nil {
		t.Fatalf("expected SOCKS5 listener closed when proxyMode=false")
	}
}
