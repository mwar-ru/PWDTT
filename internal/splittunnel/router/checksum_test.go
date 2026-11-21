package router

import (
	"encoding/binary"
	"net/netip"
	"testing"
)

func buildSampleIPv4UDPPacket(srcIP, dstIP [4]byte, srcPort, dstPort uint16, payload []byte) []byte {
	totalLen := 20 + 8 + len(payload)
	pkt := make([]byte, totalLen)
	pkt[0] = 0x45 // IPv4, IHL=5 (20 bytes)
	binary.BigEndian.PutUint16(pkt[2:4], uint16(totalLen))
	pkt[8] = 64 // TTL
	pkt[9] = byte(ProtoUDP)
	copy(pkt[12:16], srcIP[:])
	copy(pkt[16:20], dstIP[:])

	// UDP header
	binary.BigEndian.PutUint16(pkt[20:22], srcPort)
	binary.BigEndian.PutUint16(pkt[22:24], dstPort)
	binary.BigEndian.PutUint16(pkt[24:26], uint16(8+len(payload)))
	copy(pkt[28:], payload)

	RecalculateIPv4Checksums(pkt, 20, ProtoUDP)
	return pkt
}

func buildSampleIPv4TCPPacket(srcIP, dstIP [4]byte, srcPort, dstPort uint16, payload []byte) []byte {
	totalLen := 20 + 20 + len(payload)
	pkt := make([]byte, totalLen)
	pkt[0] = 0x45
	binary.BigEndian.PutUint16(pkt[2:4], uint16(totalLen))
	pkt[8] = 64
	pkt[9] = byte(ProtoTCP)
	copy(pkt[12:16], srcIP[:])
	copy(pkt[16:20], dstIP[:])

	// TCP header (20 bytes, data offset = 5)
	binary.BigEndian.PutUint16(pkt[20:22], srcPort)
	binary.BigEndian.PutUint16(pkt[22:24], dstPort)
	pkt[32] = 0x50 // Data offset = 5 (20 bytes)
	pkt[33] = 0x02 // SYN flag
	binary.BigEndian.PutUint16(pkt[34:36], 65535)
	copy(pkt[40:], payload)

	RecalculateIPv4Checksums(pkt, 20, ProtoTCP)
	return pkt
}

func TestRewriteIPv4SNATAndDNATChecksumsUDPQUIC(t *testing.T) {
	pkt := buildSampleIPv4UDPPacket([4]byte{192, 168, 1, 42}, [4]byte{1, 1, 1, 1}, 54321, 443, []byte("QUIC-INITIAL-ODD"))

	parsed, ok := ParseIPv4Transport(pkt)
	if !ok || parsed.Proto != ProtoUDP || parsed.SrcPort != 54321 || parsed.DstPort != 443 {
		t.Fatalf("unexpected parsed UDP packet: %+v (ok=%v)", parsed, ok)
	}
	if !VerifyIPv4HeaderChecksum(pkt) || !VerifyIPv4L4Checksum(pkt) {
		t.Fatalf("initial UDP checksums invalid")
	}

	// SNAT outbound -> Wintun IP 10.66.66.2:60001
	tunIP := netip.MustParseAddr("10.66.66.2")
	if !RewriteIPv4SNAT(pkt, parsed.IHL, parsed.Proto, tunIP, 60001) {
		t.Fatalf("RewriteIPv4SNAT failed")
	}
	afterSNAT, ok := ParseIPv4Transport(pkt)
	if !ok || afterSNAT.SrcIP != tunIP || afterSNAT.SrcPort != 60001 {
		t.Fatalf("SNAT rewrite mismatch: %+v", afterSNAT)
	}
	if !VerifyIPv4HeaderChecksum(pkt) || !VerifyIPv4L4Checksum(pkt) {
		t.Fatalf("UDP checksums invalid after SNAT")
	}

	// Simulate reply packet and DNAT back to 192.168.1.42:54321
	reply := buildSampleIPv4UDPPacket([4]byte{1, 1, 1, 1}, [4]byte{10, 66, 66, 2}, 443, 60001, []byte("QUIC-HANDSHAKE"))
	rp, _ := ParseIPv4Transport(reply)
	physIP := netip.MustParseAddr("192.168.1.42")
	if !RewriteIPv4DNAT(reply, rp.IHL, rp.Proto, physIP, 54321) {
		t.Fatalf("RewriteIPv4DNAT failed")
	}
	afterDNAT, _ := ParseIPv4Transport(reply)
	if afterDNAT.DstIP != physIP || afterDNAT.DstPort != 54321 {
		t.Fatalf("DNAT rewrite mismatch: %+v", afterDNAT)
	}
	if !VerifyIPv4HeaderChecksum(reply) || !VerifyIPv4L4Checksum(reply) {
		t.Fatalf("UDP checksums invalid after DNAT")
	}
}

func TestRewriteIPv4SNATAndDNATChecksumsTCP(t *testing.T) {
	pkt := buildSampleIPv4TCPPacket([4]byte{192, 168, 1, 10}, [4]byte{93, 184, 216, 34}, 51234, 443, []byte("TLS-CLIENT-HELLO!"))

	if !VerifyIPv4HeaderChecksum(pkt) || !VerifyIPv4L4Checksum(pkt) {
		t.Fatalf("initial TCP checksums invalid")
	}

	parsed, ok := ParseIPv4Transport(pkt)
	if !ok || parsed.Proto != ProtoTCP {
		t.Fatalf("failed to parse TCP packet")
	}

	tunIP := netip.MustParseAddr("10.66.66.2")
	if !RewriteIPv4SNAT(pkt, parsed.IHL, parsed.Proto, tunIP, 55555) {
		t.Fatalf("TCP SNAT failed")
	}
	if !VerifyIPv4HeaderChecksum(pkt) || !VerifyIPv4L4Checksum(pkt) {
		t.Fatalf("TCP checksums invalid after SNAT")
	}

	// Verify odd-length payload & DNAT back
	physIP := netip.MustParseAddr("192.168.1.10")
	if !RewriteIPv4DNAT(pkt, parsed.IHL, parsed.Proto, physIP, 443) {
		t.Fatalf("TCP DNAT failed")
	}
	if !VerifyIPv4HeaderChecksum(pkt) || !VerifyIPv4L4Checksum(pkt) {
		t.Fatalf("TCP checksums invalid after DNAT")
	}
}
