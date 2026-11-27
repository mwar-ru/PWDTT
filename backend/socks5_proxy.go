package backend

import (
"context"
"encoding/binary"
"errors"
"fmt"
"io"
"net"
"strconv"
"sync"
"time"
)

type tunnelDialer interface {
DialContext(ctx context.Context, network, address string) (net.Conn, error)
LookupContextHost(ctx context.Context, host string) ([]string, error)
}

type socks5Server struct {
ln     net.Listener
dialer tunnelDialer
ctx    context.Context
cancel context.CancelFunc
wg     sync.WaitGroup
}

func startSocks5Server(listenAddr string, dialer tunnelDialer) (*socks5Server, error) {
ln, err := net.Listen("tcp4", listenAddr)
if err != nil {
return nil, fmt.Errorf("listen socks5 %s: %w", listenAddr, err)
}
ctx, cancel := context.WithCancel(context.Background())
s := &socks5Server{ln: ln, dialer: dialer, ctx: ctx, cancel: cancel}
s.wg.Add(1)
go s.acceptLoop()
return s, nil
}

func (s *socks5Server) Addr() string { return s.ln.Addr().String() }

func (s *socks5Server) Close() error {
s.cancel()
err := s.ln.Close()
s.wg.Wait()
return err
}

func (s *socks5Server) acceptLoop() {
defer s.wg.Done()
for {
c, err := s.ln.Accept()
if err != nil {
return
}
s.wg.Add(1)
go func(conn net.Conn) {
defer s.wg.Done()
defer conn.Close()
s.handleConn(conn)
}(c)
}
}

func (s *socks5Server) handleConn(c net.Conn) {
_ = c.SetDeadline(time.Now().Add(15 * time.Second))

hdr := make([]byte, 2)
if _, err := io.ReadFull(c, hdr); err != nil || hdr[0] != 0x05 {
return
}
methods := make([]byte, hdr[1])
if _, err := io.ReadFull(c, methods); err != nil {
return
}
noAuth := false
for _, m := range methods {
if m == 0x00 {
noAuth = true
break
}
}
if !noAuth {
_, _ = c.Write([]byte{0x05, 0xFF})
return
}
if _, err := c.Write([]byte{0x05, 0x00}); err != nil {
return
}

req := make([]byte, 4)
if _, err := io.ReadFull(c, req); err != nil || req[0] != 0x05 {
return
}
host, port, err := readSocksAddr(c, req[3])
if err != nil {
sendSocksReply(c, 0x08, net.IPv4zero, 0)
return
}
_ = c.SetDeadline(time.Time{})

switch req[1] {
case 0x01:
s.handleTCPConnect(c, host, port)
case 0x03:
s.handleUDPAssociate(c)
default:
sendSocksReply(c, 0x07, net.IPv4zero, 0)
}
}

func (s *socks5Server) handleTCPConnect(c net.Conn, host string, port uint16) {
dialCtx, cancel := context.WithTimeout(s.ctx, 20*time.Second)
defer cancel()

target := net.JoinHostPort(host, strconv.Itoa(int(port)))
up, err := s.dialer.DialContext(dialCtx, "tcp", target)
if err != nil {
sendSocksReply(c, 0x05, net.IPv4zero, 0)
return
}
defer up.Close()

sendSocksReply(c, 0x00, net.IPv4(127, 0, 0, 1), 0)

done := make(chan struct{}, 2)
go func() {
_, _ = io.Copy(up, c)
done <- struct{}{}
}()
go func() {
_, _ = io.Copy(c, up)
done <- struct{}{}
}()
select {
case <-done:
case <-s.ctx.Done():
}
}

func (s *socks5Server) handleUDPAssociate(tcpConn net.Conn) {
udpLn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
if err != nil {
sendSocksReply(tcpConn, 0x01, net.IPv4zero, 0)
return
}
defer udpLn.Close()

localAddr := udpLn.LocalAddr().(*net.UDPAddr)
sendSocksReply(tcpConn, 0x00, net.IPv4(127, 0, 0, 1), uint16(localAddr.Port))

assocCtx, assocCancel := context.WithCancel(s.ctx)
defer assocCancel()

go func() {
buf := make([]byte, 16)
for {
if _, rerr := tcpConn.Read(buf); rerr != nil {
assocCancel()
_ = udpLn.Close()
return
}
}
}()

var (
mu         sync.Mutex
upstreams  = make(map[string]net.Conn)
clientAddr *net.UDPAddr
)
defer func() {
mu.Lock()
for _, u := range upstreams {
_ = u.Close()
}
mu.Unlock()
}()

buf := make([]byte, 65535)
for {
n, cAddr, rerr := udpLn.ReadFromUDP(buf)
if rerr != nil {
return
}
mu.Lock()
clientAddr = cAddr
mu.Unlock()

host, port, payload, perr := parseSocksUDPDatagram(buf[:n])
if perr != nil || len(payload) == 0 {
continue
}

dstKey := net.JoinHostPort(host, strconv.Itoa(int(port)))
mu.Lock()
up, ok := upstreams[dstKey]
mu.Unlock()

if !ok {
dialHost := host
if net.ParseIP(host) == nil {
rCtx, rCancel := context.WithTimeout(assocCtx, 5*time.Second)
ips, lerr := s.dialer.LookupContextHost(rCtx, host)
rCancel()
if lerr != nil || len(ips) == 0 {
continue
}
dialHost = ips[0]
}
dCtx, dCancel := context.WithTimeout(assocCtx, 8*time.Second)
newUp, derr := s.dialer.DialContext(dCtx, "udp", net.JoinHostPort(dialHost, strconv.Itoa(int(port))))
dCancel()
if derr != nil {
continue
}

mu.Lock()
if existing, exists := upstreams[dstKey]; exists {
_ = newUp.Close()
up = existing
} else {
upstreams[dstKey] = newUp
up = newUp
go func(key, h string, p uint16, conn net.Conn) {
defer func() {
_ = conn.Close()
mu.Lock()
delete(upstreams, key)
mu.Unlock()
}()
rbuf := make([]byte, 65535)
for {
_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
rn, err := conn.Read(rbuf)
if err != nil {
return
}
pkt, err := buildSocksUDPDatagram(h, p, rbuf[:rn])
if err != nil {
return
}
mu.Lock()
ca := clientAddr
mu.Unlock()
if ca != nil {
_, _ = udpLn.WriteToUDP(pkt, ca)
}
}
}(dstKey, host, port, up)
}
mu.Unlock()
}

_, _ = up.Write(payload)
}
}

func readSocksAddr(r io.Reader, atyp byte) (string, uint16, error) {
var host string
switch atyp {
case 0x01:
b := make([]byte, 4)
if _, err := io.ReadFull(r, b); err != nil {
return "", 0, err
}
host = net.IP(b).String()
case 0x03:
lb := make([]byte, 1)
if _, err := io.ReadFull(r, lb); err != nil {
return "", 0, err
}
b := make([]byte, lb[0])
if _, err := io.ReadFull(r, b); err != nil {
return "", 0, err
}
host = string(b)
case 0x04:
b := make([]byte, 16)
if _, err := io.ReadFull(r, b); err != nil {
return "", 0, err
}
host = net.IP(b).String()
default:
return "", 0, errors.New("unsupported atyp")
}
pb := make([]byte, 2)
if _, err := io.ReadFull(r, pb); err != nil {
return "", 0, err
}
return host, binary.BigEndian.Uint16(pb), nil
}

func sendSocksReply(w io.Writer, rep byte, ip net.IP, port uint16) {
ip4 := ip.To4()
if ip4 == nil {
ip4 = net.IPv4zero
}
_, _ = w.Write([]byte{0x05, rep, 0x00, 0x01, ip4[0], ip4[1], ip4[2], ip4[3], byte(port >> 8), byte(port)})
}

func parseSocksUDPDatagram(pkt []byte) (string, uint16, []byte, error) {
if len(pkt) < 7 || pkt[0] != 0x00 || pkt[1] != 0x00 || pkt[2] != 0x00 {
return "", 0, nil, errors.New("invalid socks5 udp header")
}
pos := 4
var host string
switch pkt[3] {
case 0x01:
if len(pkt) < pos+6 {
return "", 0, nil, errors.New("short ipv4")
}
host = net.IP(pkt[pos : pos+4]).String()
pos += 4
case 0x03:
if len(pkt) < pos+1 {
return "", 0, nil, errors.New("short domain len")
}
dlen := int(pkt[pos])
pos++
if len(pkt) < pos+dlen+2 {
return "", 0, nil, errors.New("short domain")
}
host = string(pkt[pos : pos+dlen])
pos += dlen
case 0x04:
if len(pkt) < pos+18 {
return "", 0, nil, errors.New("short ipv6")
}
host = net.IP(pkt[pos : pos+16]).String()
pos += 16
default:
return "", 0, nil, errors.New("unsupported atyp")
}
port := binary.BigEndian.Uint16(pkt[pos : pos+2])
return host, port, pkt[pos+2:], nil
}

func buildSocksUDPDatagram(host string, port uint16, payload []byte) ([]byte, error) {
out := []byte{0x00, 0x00, 0x00}
if ip := net.ParseIP(host); ip != nil {
if ip4 := ip.To4(); ip4 != nil {
out = append(out, 0x01)
out = append(out, ip4...)
} else {
out = append(out, 0x04)
out = append(out, ip.To16()...)
}
} else {
if len(host) > 255 {
return nil, errors.New("domain too long")
}
out = append(out, 0x03, byte(len(host)))
out = append(out, []byte(host)...)
}
out = append(out, byte(port>>8), byte(port))
out = append(out, payload...)
return out, nil
}
