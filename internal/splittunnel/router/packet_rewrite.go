package router

import (
	"encoding/binary"
	"net/netip"
)

// ParsedIPv4Packet describes the header fields of an IPv4 TCP or UDP packet.
type ParsedIPv4Packet struct {
	IHL     int // IPv4 header length in bytes (>= 20)
	Proto   Protocol
	SrcIP   netip.Addr
	DstIP   netip.Addr
	SrcPort uint16
	DstPort uint16
}

// ParseIPv4Transport inspects a raw IPv4 packet and extracts 5-tuple metadata for TCP/UDP.
func ParseIPv4Transport(pkt []byte) (ParsedIPv4Packet, bool) {
	if len(pkt) < 20 {
		return ParsedIPv4Packet{}, false
	}
	version := pkt[0] >> 4
	if version != 4 {
		return ParsedIPv4Packet{}, false
	}
	ihl := int(pkt[0]&0x0F) * 4
	if ihl < 20 || len(pkt) < ihl+8 {
		return ParsedIPv4Packet{}, false
	}

	proto := Protocol(pkt[9])
	if proto != ProtoTCP && proto != ProtoUDP {
		return ParsedIPv4Packet{}, false
	}
	if proto == ProtoTCP && len(pkt) < ihl+20 {
		return ParsedIPv4Packet{}, false
	}

	var src4, dst4 [4]byte
	copy(src4[:], pkt[12:16])
	copy(dst4[:], pkt[16:20])

	return ParsedIPv4Packet{
		IHL:     ihl,
		Proto:   proto,
		SrcIP:   netip.AddrFrom4(src4),
		DstIP:   netip.AddrFrom4(dst4),
		SrcPort: binary.BigEndian.Uint16(pkt[ihl : ihl+2]),
		DstPort: binary.BigEndian.Uint16(pkt[ihl+2 : ihl+4]),
	}, true
}

// RewriteIPv4SNAT updates the source IPv4 address and source TCP/UDP port in-place
// and recalculates IPv4 + L4 checksums.
func RewriteIPv4SNAT(pkt []byte, ihl int, proto Protocol, newSrcIP netip.Addr, newSrcPort uint16) bool {
	if !newSrcIP.Is4() || len(pkt) < ihl+8 {
		return false
	}
	ip4 := newSrcIP.As4()
	copy(pkt[12:16], ip4[:])
	binary.BigEndian.PutUint16(pkt[ihl:ihl+2], newSrcPort)
	RecalculateIPv4Checksums(pkt, ihl, proto)
	return true
}

// RewriteIPv4DNAT updates the destination IPv4 address and destination TCP/UDP port in-place
// and recalculates IPv4 + L4 checksums.
func RewriteIPv4DNAT(pkt []byte, ihl int, proto Protocol, newDstIP netip.Addr, newDstPort uint16) bool {
	if !newDstIP.Is4() || len(pkt) < ihl+8 {
		return false
	}
	ip4 := newDstIP.As4()
	copy(pkt[16:20], ip4[:])
	binary.BigEndian.PutUint16(pkt[ihl+2:ihl+4], newDstPort)
	RecalculateIPv4Checksums(pkt, ihl, proto)
	return true
}

// RecalculateIPv4Checksums computes fresh RFC 791 IPv4 header checksum and RFC 793/768 TCP/UDP checksum.
func RecalculateIPv4Checksums(pkt []byte, ihl int, proto Protocol) {
	if len(pkt) < ihl || ihl < 20 {
		return
	}

	// 1. IPv4 header checksum
	pkt[10] = 0
	pkt[11] = 0
	ipSum := onesComplementSum(pkt[:ihl], 0)
	binary.BigEndian.PutUint16(pkt[10:12], ^ipSum)

	// 2. TCP / UDP checksum with IPv4 pseudo-header
	l4 := pkt[ihl:]
	l4Len := len(l4)
	if proto == ProtoTCP && l4Len >= 20 {
		l4[16] = 0
		l4[17] = 0
		pseudo := ipv4PseudoHeaderSum(pkt[12:16], pkt[16:20], uint8(ProtoTCP), uint16(l4Len))
		finalSum := onesComplementSum(l4, pseudo)
		binary.BigEndian.PutUint16(l4[16:18], ^finalSum)
	} else if proto == ProtoUDP && l4Len >= 8 {
		l4[6] = 0
		l4[7] = 0
		pseudo := ipv4PseudoHeaderSum(pkt[12:16], pkt[16:20], uint8(ProtoUDP), uint16(l4Len))
		finalSum := onesComplementSum(l4, pseudo)
		csum := ^finalSum
		if csum == 0 {
			csum = 0xFFFF // RFC 768: 0x0000 means no checksum, transmit 0xFFFF instead
		}
		binary.BigEndian.PutUint16(l4[6:8], csum)
	}
}

func ipv4PseudoHeaderSum(srcIP, dstIP []byte, proto uint8, l4Len uint16) uint32 {
	var sum uint32
	sum += uint32(binary.BigEndian.Uint16(srcIP[0:2]))
	sum += uint32(binary.BigEndian.Uint16(srcIP[2:4]))
	sum += uint32(binary.BigEndian.Uint16(dstIP[0:2]))
	sum += uint32(binary.BigEndian.Uint16(dstIP[2:4]))
	sum += uint32(proto)
	sum += uint32(l4Len)
	return sum
}

func onesComplementSum(data []byte, initial uint32) uint16 {
	sum := initial
	n := len(data)
	for i := 0; i+1 < n; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(data[i : i+2]))
	}
	if n%2 == 1 {
		sum += uint32(data[n-1]) << 8
	}
	for (sum >> 16) != 0 {
		sum = (sum & 0xFFFF) + (sum >> 16)
	}
	return uint16(sum)
}

// VerifyIPv4HeaderChecksum returns true if the IPv4 header checksum validates to 0xFFFF.
func VerifyIPv4HeaderChecksum(pkt []byte) bool {
	if len(pkt) < 20 {
		return false
	}
	ihl := int(pkt[0]&0x0F) * 4
	if ihl < 20 || len(pkt) < ihl {
		return false
	}
	return onesComplementSum(pkt[:ihl], 0) == 0xFFFF
}

// VerifyIPv4L4Checksum returns true if the TCP or UDP checksum validates to 0xFFFF.
func VerifyIPv4L4Checksum(pkt []byte) bool {
	p, ok := ParseIPv4Transport(pkt)
	if !ok {
		return false
	}
	l4 := pkt[p.IHL:]
	sum := ipv4PseudoHeaderSum(pkt[12:16], pkt[16:20], uint8(p.Proto), uint16(len(l4)))
	return onesComplementSum(l4, sum) == 0xFFFF
}

// ClampTCPSYNMSS clamps the TCP MSS option in IPv4 SYN packets to maxMSS (e.g. 1360 for WireGuard MTU 1420)
// to prevent PMTUD blackholes when steering traffic from a 1500-MTU physical NIC into a 1420-MTU tunnel.
func ClampTCPSYNMSS(pkt []byte, ihl int, proto Protocol, maxMSS uint16) bool {
	if proto != ProtoTCP || len(pkt) < ihl+20 || maxMSS == 0 {
		return false
	}
	tcp := pkt[ihl:]
	if tcp[13]&0x02 == 0 {
		return false
	}
	dataOffset := int(tcp[12]>>4) * 4
	if dataOffset <= 20 || len(tcp) < dataOffset {
		return false
	}
	opts := tcp[20:dataOffset]
	for i := 0; i < len(opts); {
		kind := opts[i]
		if kind == 0 {
			break
		}
		if kind == 1 {
			i++
			continue
		}
		if i+1 >= len(opts) {
			break
		}
		optLen := int(opts[i+1])
		if optLen < 2 || i+optLen > len(opts) {
			break
		}
		if kind == 2 && optLen == 4 {
			curMSS := binary.BigEndian.Uint16(opts[i+2 : i+4])
			if curMSS > maxMSS {
				binary.BigEndian.PutUint16(opts[i+2:i+4], maxMSS)
				return true
			}
			return false
		}
		i += optLen
	}
	return false
}
