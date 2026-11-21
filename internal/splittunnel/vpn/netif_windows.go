//go:build windows

package vpn

import (
	"fmt"
	"net/netip"
	"unsafe"

	"golang.org/x/sys/windows"
)

type windowsInterfaceProvider struct{}

// NewWindowsInspector returns an Inspector backed by Windows iphlpapi GetAdaptersAddresses.
func NewWindowsInspector(tunnelNameHint string) *Inspector {
	return NewInspector(&windowsInterfaceProvider{}, tunnelNameHint)
}

func (p *windowsInterfaceProvider) ListInterfaces() ([]InterfaceInfo, error) {
	const flags = windows.GAA_FLAG_INCLUDE_PREFIX | windows.GAA_FLAG_INCLUDE_GATEWAYS

	size := uint32(16 * 1024)
	var buf []byte
	for attempt := 0; attempt < 4; attempt++ {
		buf = make([]byte, size)
		err := windows.GetAdaptersAddresses(
			windows.AF_UNSPEC,
			flags,
			0,
			(*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])),
			&size,
		)
		if err == nil {
			break
		}
		if err == windows.ERROR_BUFFER_OVERFLOW {
			continue
		}
		return nil, fmt.Errorf("GetAdaptersAddresses: %w", err)
	}

	var result []InterfaceInfo
	for cur := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])); cur != nil; cur = cur.Next {
		name := windows.UTF16PtrToString(cur.FriendlyName)
		desc := windows.UTF16PtrToString(cur.Description)
		isLoopback := cur.IfType == windows.IF_TYPE_SOFTWARE_LOOPBACK
		isVirt, isVPN := ClassifyVirtualAdapter(name, desc)
		if cur.IfType == windows.IF_TYPE_TUNNEL {
			isVirt = true
		}

		info := InterfaceInfo{
			IfIndex:        cur.IfIndex,
			LUID:           cur.Luid,
			Name:           name,
			Description:    desc,
			Metric:         cur.Ipv4Metric,
			MTU:            cur.Mtu,
			IsUp:           cur.OperStatus == windows.IfOperStatusUp,
			IsLoopback:     isLoopback,
			IsVirtual:      isVirt,
			IsCompetingVPN: isVPN,
		}

		// Unicast IPv4 / IPv6 addresses
		for ua := cur.FirstUnicastAddress; ua != nil; ua = ua.Next {
			addr, ok := socketAddrToNetip(ua.Address)
			if !ok {
				continue
			}
			if addr.Is4() {
				// Prefer non-APIPA (169.254.x.x) IPv4 if multiple exist
				if !info.IPv4.IsValid() || (info.IPv4.IsLinkLocalUnicast() && !addr.IsLinkLocalUnicast()) {
					info.IPv4 = addr
				}
			} else if addr.Is6() {
				// Prefer global IPv6 over link-local fe80::
				if !info.IPv6.IsValid() || (info.IPv6.IsLinkLocalUnicast() && !addr.IsLinkLocalUnicast()) {
					info.IPv6 = addr
				}
			}
		}

		// Default gateways
		for gw := cur.FirstGatewayAddress; gw != nil; gw = gw.Next {
			addr, ok := socketAddrToNetip(gw.Address)
			if !ok {
				continue
			}
			if addr.Is4() && !info.GatewayIPv4.IsValid() {
				info.GatewayIPv4 = addr
			} else if addr.Is6() && !info.GatewayIPv6.IsValid() {
				info.GatewayIPv6 = addr
			}
		}

		// DNS servers
		for dns := cur.FirstDnsServerAddress; dns != nil; dns = dns.Next {
			addr, ok := socketAddrToNetip(dns.Address)
			if !ok {
				continue
			}
			info.DNSServers = append(info.DNSServers, addr)
		}

		result = append(result, info)
	}

	return result, nil
}

func socketAddrToNetip(sa windows.SocketAddress) (netip.Addr, bool) {
	ip := sa.IP()
	if len(ip) == 0 {
		return netip.Addr{}, false
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}
