package backend

import (
"bytes"
"context"
"io"
"net"
"testing"
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
