package backend

import (
"fmt"
"net/netip"
"strings"

"golang.zx2c4.com/wireguard/conn"
"golang.zx2c4.com/wireguard/device"
"golang.zx2c4.com/wireguard/tun/netstack"
)

const DefaultProxyPort = 10808

func (w *WG) SetProxyMode(enabled bool, port int) {
w.stateMu.Lock()
defer w.stateMu.Unlock()
w.proxyMode = enabled
if port <= 0 || port > 65535 {
port = DefaultProxyPort
}
w.proxyPort = port
}

func (w *WG) isProxyMode() bool {
w.stateMu.Lock()
defer w.stateMu.Unlock()
return w.proxyMode
}

func (w *WG) applyProxy(conf string, logf wgLogFunc) error {
w.stateMu.Lock()
defer w.stateMu.Unlock()

if w.activeSocks != nil {
_ = w.activeSocks.Close()
w.activeSocks = nil
}
if w.activeDevice != nil {
w.activeDevice.Close()
w.activeDevice = nil
}
if w.activeTun != nil {
_ = w.activeTun.Close()
w.activeTun = nil
}

addr, mtuStr, _, wgConf := parseWGConfig(conf)
if addr == "" {
return fmt.Errorf("Address not found in wg config")
}
mtu := 1280
if mtuStr != "" {
fmt.Sscanf(mtuStr, "%d", &mtu)
}

localIPs := parseNetstackAddrs(addr)
if len(localIPs) == 0 {
return fmt.Errorf("no valid IP in Address=%q", addr)
}
dnsIPs := parseNetstackDNS(conf)

logf(fmt.Sprintf("Запуск userspace WireGuard (netstack, mtu=%d, без Wintun)", mtu))
tunDev, tnet, err := netstack.CreateNetTUN(localIPs, dnsIPs, mtu)
if err != nil {
return fmt.Errorf("netstack CreateNetTUN: %w", err)
}
w.activeTun = tunDev

logger := &device.Logger{
Verbosef: func(format string, args ...interface{}) {},
Errorf:   func(format string, args ...interface{}) { logf(fmt.Sprintf(format, args...)) },
}
dev := device.NewDevice(tunDev, conn.NewDefaultBind(), logger)
w.activeDevice = dev

uapi := uapiConf(wgConf)
if err := dev.IpcSetOperation(strings.NewReader(uapi)); err != nil {
return fmt.Errorf("IpcSet: %w", err)
}
if err := dev.Up(); err != nil {
return fmt.Errorf("device up: %w", err)
}

port := w.proxyPort
if port <= 0 || port > 65535 {
port = DefaultProxyPort
}
srv, err := startSocks5Server(fmt.Sprintf("127.0.0.1:%d", port), tnet)
if err != nil {
dev.Close()
_ = tunDev.Close()
w.activeDevice = nil
w.activeTun = nil
return err
}
w.activeSocks = srv
w.proxyActive = true
logf(fmt.Sprintf("SOCKS5-прокси запущен на %s (TCP + UDP) — готов для mihomo", srv.Addr()))
return nil
}

func (w *WG) teardownProxy() bool {
w.stateMu.Lock()
defer w.stateMu.Unlock()
if !w.proxyActive && w.activeSocks == nil {
return false
}
if w.activeSocks != nil {
_ = w.activeSocks.Close()
w.activeSocks = nil
}
if w.activeDevice != nil {
w.activeDevice.Close()
w.activeDevice = nil
}
if w.activeTun != nil {
_ = w.activeTun.Close()
w.activeTun = nil
}
w.proxyActive = false
return true
}

func parseNetstackAddrs(raw string) []netip.Addr {
var out []netip.Addr
for _, part := range strings.Split(raw, ",") {
p := strings.TrimSpace(part)
if p == "" {
continue
}
if pfx, err := netip.ParsePrefix(p); err == nil {
out = append(out, pfx.Addr())
} else if ip, err := netip.ParseAddr(p); err == nil {
out = append(out, ip)
}
}
return out
}

func parseNetstackDNS(conf string) []netip.Addr {
var out []netip.Addr
for _, line := range strings.Split(conf, "\n") {
line = strings.TrimSpace(line)
if !strings.HasPrefix(strings.ToLower(line), "dns") {
continue
}
if _, val, ok := strings.Cut(line, "="); ok {
for _, part := range strings.Split(val, ",") {
if ip, err := netip.ParseAddr(strings.TrimSpace(part)); err == nil {
out = append(out, ip)
}
}
}
}
if len(out) == 0 {
out = []netip.Addr{netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("8.8.8.8")}
}
return out
}
