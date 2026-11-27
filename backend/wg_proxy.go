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

func normalizeProxyPort(port int) int {
	if port <= 0 || port > 65535 {
		return DefaultProxyPort
	}
	return port
}

func (w *WG) SetProxyMode(enabled bool, port int) {
	w.stateMu.Lock()
	defer w.stateMu.Unlock()
	w.proxyMode = enabled
	w.proxyPort = normalizeProxyPort(port)
}

func (w *WG) isProxyMode() bool {
	w.stateMu.Lock()
	defer w.stateMu.Unlock()
	return w.proxyMode
}

// SyncProxyStandby keeps 127.0.0.1:<port> listening in DIRECT passthrough mode
// when mihomo proxy mode is enabled (even while VPN is disconnected), and stops
// the listener completely when proxy mode is disabled.
func (w *WG) SyncProxyStandby(enabled bool, port int) error {
	w.stateMu.Lock()
	defer w.stateMu.Unlock()

	port = normalizeProxyPort(port)
	w.proxyMode = enabled
	w.proxyPort = port

	if !enabled {
		if w.activeSocks != nil && !w.proxyActive {
			_ = w.activeSocks.Close()
			w.activeSocks = nil
		}
		return nil
	}

	wantAddr := fmt.Sprintf("127.0.0.1:%d", port)
	if w.activeSocks != nil {
		if w.activeSocks.listenAddr == wantAddr {
			return nil
		}
		d := w.activeSocks.getDialer()
		_ = w.activeSocks.Close()
		w.activeSocks = nil
		srv, err := startSocks5Server(wantAddr, d)
		if err != nil {
			return err
		}
		w.activeSocks = srv
		return nil
	}

	srv, err := startSocks5Server(wantAddr, directDialer{})
	if err != nil {
		return err
	}
	w.activeSocks = srv
	return nil
}

// StopProxyServer unconditionally closes the local SOCKS5 listener (e.g. on app exit).
func (w *WG) StopProxyServer() {
	w.stateMu.Lock()
	defer w.stateMu.Unlock()
	w.proxyMode = false
	if w.activeSocks != nil {
		_ = w.activeSocks.Close()
		w.activeSocks = nil
	}
}

func (w *WG) applyProxy(conf string, logf wgLogFunc) error {
	w.stateMu.Lock()
	defer w.stateMu.Unlock()

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

	port := normalizeProxyPort(w.proxyPort)
	wantAddr := fmt.Sprintf("127.0.0.1:%d", port)
	if w.activeSocks != nil && w.activeSocks.listenAddr == wantAddr {
		w.activeSocks.SetDialer(tnet, true)
	} else {
		if w.activeSocks != nil {
			_ = w.activeSocks.Close()
			w.activeSocks = nil
		}
		srv, err := startSocks5Server(wantAddr, tnet)
		if err != nil {
			dev.Close()
			_ = tunDev.Close()
			w.activeDevice = nil
			w.activeTun = nil
			return err
		}
		w.activeSocks = srv
	}
	w.proxyActive = true
	logf(fmt.Sprintf("SOCKS5-прокси переключён на VPN %s (TCP + UDP) — готов для mihomo", w.activeSocks.Addr()))
	return nil
}

func (w *WG) teardownProxy() bool {
	w.stateMu.Lock()
	defer w.stateMu.Unlock()
	if !w.proxyActive {
		return false
	}
	if w.activeSocks != nil {
		if w.proxyMode {
			// Keep SOCKS5 listening in DIRECT passthrough mode while proxyMode stays enabled.
			w.activeSocks.SetDialer(directDialer{}, true)
		} else {
			_ = w.activeSocks.Close()
			w.activeSocks = nil
		}
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
