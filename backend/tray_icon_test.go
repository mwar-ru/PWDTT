package backend

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestSelectTrayIconVariant(t *testing.T) {
	cases := []struct {
		name string
		st   TrayState
		want TrayIconVariant
	}{
		{"idle_default", TrayState{TunnelState: "idle"}, TrayIconIdle},
		{"idle_proxy_direct", TrayState{TunnelState: "idle", ProxyMode: true}, TrayIconProxyDirect},
		{"connecting", TrayState{TunnelState: "connecting", ProxyMode: true}, TrayIconConnecting},
		{"connected_proxy_vpn", TrayState{TunnelState: "connected", ProxyMode: true}, TrayIconProxyVPN},
		{"connected_split_vpn", TrayState{TunnelState: "connected", SplitSupported: true, SplitEnabled: true}, TrayIconSplitVPN},
		{"connected_global_vpn", TrayState{TunnelState: "connected", SplitSupported: true, SplitEnabled: false}, TrayIconGlobalVPN},
	}
	for _, tc := range cases {
		if got := SelectTrayIconVariant(tc.st); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestBuildTrayIconsProducesValidMultiPNGICO(t *testing.T) {
	// Create a dummy 64x64 source PNG
	src := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: 30, G: 60, B: 120, A: 255})
		}
	}
	var baseBuf bytes.Buffer
	if err := png.Encode(&baseBuf, src); err != nil {
		t.Fatal(err)
	}
	fallback := []byte{0, 0, 1, 0, 0, 0}

	icons := BuildTrayIcons(baseBuf.Bytes(), fallback)
	variants := []TrayIconVariant{
		TrayIconProxyDirect,
		TrayIconConnecting,
		TrayIconProxyVPN,
		TrayIconSplitVPN,
		TrayIconGlobalVPN,
	}
	for _, v := range variants {
		ico := icons[v]
		if len(ico) < 38 {
			t.Fatalf("variant %v: ico too short (%d)", v, len(ico))
		}
		if binary.LittleEndian.Uint16(ico[0:2]) != 0 ||
			binary.LittleEndian.Uint16(ico[2:4]) != 1 ||
			binary.LittleEndian.Uint16(ico[4:6]) != 2 {
			t.Fatalf("variant %v: invalid ICO header", v)
		}
		for i := 0; i < 2; i++ {
			entry := ico[6+i*16 : 6+(i+1)*16]
			size := binary.LittleEndian.Uint32(entry[8:12])
			off := binary.LittleEndian.Uint32(entry[12:16])
			img, err := png.Decode(bytes.NewReader(ico[off : off+size]))
			if err != nil {
				t.Fatalf("variant %v entry %d: invalid PNG inside ICO: %v", v, i, err)
			}
			wantDim := 32
			if i == 1 {
				wantDim = 64
			}
			if img.Bounds().Dx() != wantDim || img.Bounds().Dy() != wantDim {
				t.Fatalf("variant %v entry %d: got %dx%d, want %d", v, i, img.Bounds().Dx(), img.Bounds().Dy(), wantDim)
			}
		}
	}
}
