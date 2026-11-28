package backend

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
)

type TrayIconVariant int

const (
	TrayIconIdle TrayIconVariant = iota
	TrayIconProxyDirect
	TrayIconConnecting
	TrayIconProxyVPN
	TrayIconSplitVPN
	TrayIconGlobalVPN
)

// SelectTrayIconVariant picks the tray icon variant matching the main screen status colors.
func SelectTrayIconVariant(st TrayState) TrayIconVariant {
	switch st.TunnelState {
	case "connected":
		if st.ProxyMode {
			return TrayIconProxyVPN
		}
		if st.SplitSupported && st.SplitEnabled {
			return TrayIconSplitVPN
		}
		return TrayIconGlobalVPN
	case "connecting":
		return TrayIconConnecting
	default:
		if st.ProxyMode {
			return TrayIconProxyDirect
		}
		return TrayIconIdle
	}
}

type trayBadgeStyle struct {
	ringColor color.NRGBA
	ringWidth float64 // fraction of icon size
	hasDot    bool
	dotColor  color.NRGBA
}

var trayVariantStyles = map[TrayIconVariant]trayBadgeStyle{
	// Subtle dim violet ring when SOCKS5 is running in DIRECT standby mode
	TrayIconProxyDirect: {
		ringColor: color.NRGBA{R: 139, G: 92, B: 246, A: 185},
		ringWidth: 0.065,
		hasDot:    false,
	},
	// Amber ring + dot while connecting
	TrayIconConnecting: {
		ringColor: color.NRGBA{R: 245, G: 158, B: 11, A: 255},
		ringWidth: 0.08,
		hasDot:    true,
		dotColor:  color.NRGBA{R: 251, G: 191, B: 36, A: 255},
	},
	// Vivid violet ring + glowing dot when connected in mihomo SOCKS5 VPN mode
	TrayIconProxyVPN: {
		ringColor: color.NRGBA{R: 139, G: 92, B: 246, A: 255},
		ringWidth: 0.085,
		hasDot:    true,
		dotColor:  color.NRGBA{R: 167, G: 139, B: 250, A: 255},
	},
	// Emerald ring + dot when connected with Split Tunneling active
	TrayIconSplitVPN: {
		ringColor: color.NRGBA{R: 16, G: 185, B: 129, A: 255},
		ringWidth: 0.08,
		hasDot:    true,
		dotColor:  color.NRGBA{R: 52, G: 211, B: 153, A: 255},
	},
	// Bright blue ring + dot when connected in standard Global TUN mode
	TrayIconGlobalVPN: {
		ringColor: color.NRGBA{R: 59, G: 130, B: 246, A: 255},
		ringWidth: 0.08,
		hasDot:    true,
		dotColor:  color.NRGBA{R: 96, G: 165, B: 250, A: 255},
	},
}

// BuildTrayIcons pre-generates cached Windows .ico byte slices for every status variant.
// Falls back cleanly to fallbackICO if basePNG cannot be decoded.
func BuildTrayIcons(basePNG, fallbackICO []byte) map[TrayIconVariant][]byte {
	out := map[TrayIconVariant][]byte{
		TrayIconIdle: fallbackICO,
	}
	src, err := png.Decode(bytes.NewReader(basePNG))
	if err != nil {
		for v := range trayVariantStyles {
			out[v] = fallbackICO
		}
		return out
	}

	for v, style := range trayVariantStyles {
		ico, err := renderStatusICO(src, style)
		if err != nil || len(ico) == 0 {
			out[v] = fallbackICO
		} else {
			out[v] = ico
		}
	}
	return out
}

func renderStatusICO(src image.Image, style trayBadgeStyle) ([]byte, error) {
	sizes := []int{32, 64}
	pngs := make([][]byte, 0, len(sizes))
	for _, sz := range sizes {
		img := renderStatusFrame(src, sz, style)
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return nil, err
		}
		pngs = append(pngs, buf.Bytes())
	}
	return encodeMultiPNGICO(sizes, pngs), nil
}

func renderStatusFrame(src image.Image, size int, style trayBadgeStyle) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	fSize := float64(size)
	ringPx := math.Max(2.0, fSize*style.ringWidth)
	inset := int(math.Round(ringPx * 0.72))
	inner := size - inset*2
	if inner < 8 {
		inner = size
		inset = 0
	}

	// 1. Draw scaled base app icon inside the ring with bilinear filtering
	scaled := scaleBilinearNRGBA(src, inner, inner)
	for y := 0; y < inner; y++ {
		for x := 0; x < inner; x++ {
			dst.SetNRGBA(inset+x, inset+y, scaled.NRGBAAt(x, y))
		}
	}

	// 2. Draw smooth anti-aliased rounded-square status border ring
	outerHalf := fSize/2.0 - 0.5
	innerHalf := outerHalf - ringPx
	outerRad := fSize * 0.24
	innerRad := math.Max(2.0, outerRad-ringPx*0.75)
	cx, cy := fSize/2.0, fSize/2.0

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			px := float64(x) + 0.5 - cx
			py := float64(y) + 0.5 - cy
			dOut := sdfRoundedBox(px, py, outerHalf, outerHalf, outerRad)
			dIn := sdfRoundedBox(px, py, innerHalf, innerHalf, innerRad)

			// Inside outer box (dOut <= 0) and outside inner box (dIn >= 0)
			aOut := clamp01(0.5 - dOut)
			aIn := clamp01(0.5 + dIn)
			cov := aOut * aIn
			if cov > 0.001 {
				c := style.ringColor
				c.A = uint8(math.Round(float64(c.A) * cov))
				blendOver(dst, x, y, c)
			}
		}
	}

	// 3. Draw high-contrast status dot in bottom-right corner
	if style.hasDot {
		dotR := math.Max(2.5, fSize*0.115)
		haloR := dotR + math.Max(1.0, fSize*0.035)
		dotCX := fSize - haloR - 0.5
		dotCY := fSize - haloR - 0.5
		haloCol := color.NRGBA{R: 15, G: 23, B: 42, A: 235}

		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				dx := (float64(x) + 0.5) - dotCX
				dy := (float64(y) + 0.5) - dotCY
				dist := math.Hypot(dx, dy)

				if dist <= haloR+1.0 {
					aHalo := clamp01(haloR - dist + 0.5)
					if aHalo > 0.001 {
						hc := haloCol
						hc.A = uint8(math.Round(float64(hc.A) * aHalo))
						blendOver(dst, x, y, hc)
					}
					aDot := clamp01(dotR - dist + 0.5)
					if aDot > 0.001 {
						dc := style.dotColor
						dc.A = uint8(math.Round(float64(dc.A) * aDot))
						blendOver(dst, x, y, dc)
					}
				}
			}
		}
	}

	return dst
}

func sdfRoundedBox(px, py, bx, by, r float64) float64 {
	qx := math.Abs(px) - (bx - r)
	qy := math.Abs(py) - (by - r)
	outDist := math.Hypot(math.Max(qx, 0), math.Max(qy, 0))
	inDist := math.Min(math.Max(qx, qy), 0)
	return outDist + inDist - r
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func blendOver(dst *image.NRGBA, x, y int, src color.NRGBA) {
	if src.A == 0 {
		return
	}
	bg := dst.NRGBAAt(x, y)
	sa := float64(src.A) / 255.0
	ba := float64(bg.A) / 255.0
	outA := sa + ba*(1.0-sa)
	if outA <= 0 {
		return
	}
	r := (float64(src.R)*sa + float64(bg.R)*ba*(1.0-sa)) / outA
	g := (float64(src.G)*sa + float64(bg.G)*ba*(1.0-sa)) / outA
	b := (float64(src.B)*sa + float64(bg.B)*ba*(1.0-sa)) / outA
	dst.SetNRGBA(x, y, color.NRGBA{
		R: uint8(math.Round(r)),
		G: uint8(math.Round(g)),
		B: uint8(math.Round(b)),
		A: uint8(math.Round(outA * 255.0)),
	})
}

func scaleBilinearNRGBA(src image.Image, w, h int) *image.NRGBA {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	if sw <= 0 || sh <= 0 || w <= 0 || h <= 0 {
		return out
	}

	nrgba := image.NewNRGBA(image.Rect(0, 0, sw, sh))
	for y := 0; y < sh; y++ {
		for x := 0; x < sw; x++ {
			nrgba.Set(x, y, src.At(sb.Min.X+x, sb.Min.Y+y))
		}
	}

	for y := 0; y < h; y++ {
		fy := (float64(y)+0.5)*float64(sh)/float64(h) - 0.5
		y0 := int(math.Floor(fy))
		y1 := y0 + 1
		wy := fy - float64(y0)
		if y0 < 0 {
			y0 = 0
		}
		if y1 >= sh {
			y1 = sh - 1
		}
		for x := 0; x < w; x++ {
			fx := (float64(x)+0.5)*float64(sw)/float64(w) - 0.5
			x0 := int(math.Floor(fx))
			x1 := x0 + 1
			wx := fx - float64(x0)
			if x0 < 0 {
				x0 = 0
			}
			if x1 >= sw {
				x1 = sw - 1
			}
			c00 := nrgba.NRGBAAt(x0, y0)
			c10 := nrgba.NRGBAAt(x1, y0)
			c01 := nrgba.NRGBAAt(x0, y1)
			c11 := nrgba.NRGBAAt(x1, y1)

			lerp := func(a, b, c, d uint8) uint8 {
				top := float64(a)*(1-wx) + float64(b)*wx
				bot := float64(c)*(1-wx) + float64(d)*wx
				return uint8(math.Round(top*(1-wy) + bot*wy))
			}
			out.SetNRGBA(x, y, color.NRGBA{
				R: lerp(c00.R, c10.R, c01.R, c11.R),
				G: lerp(c00.G, c10.G, c01.G, c11.G),
				B: lerp(c00.B, c10.B, c01.B, c11.B),
				A: lerp(c00.A, c10.A, c01.A, c11.A),
			})
		}
	}
	return out
}

func encodeMultiPNGICO(sizes []int, pngs [][]byte) []byte {
	count := len(sizes)
	headerLen := 6 + count*16
	total := headerLen
	for _, p := range pngs {
		total += len(p)
	}
	buf := make([]byte, total)
	// ICONDIR: reserved=0, type=1 (ICO), count
	binary.LittleEndian.PutUint16(buf[0:2], 0)
	binary.LittleEndian.PutUint16(buf[2:4], 1)
	binary.LittleEndian.PutUint16(buf[4:6], uint16(count))

	offset := headerLen
	for i, sz := range sizes {
		entry := buf[6+i*16 : 6+(i+1)*16]
		if sz >= 256 {
			entry[0] = 0
			entry[1] = 0
		} else {
			entry[0] = byte(sz)
			entry[1] = byte(sz)
		}
		entry[2] = 0                                  // color palette
		entry[3] = 0                                  // reserved
		binary.LittleEndian.PutUint16(entry[4:6], 1)  // planes
		binary.LittleEndian.PutUint16(entry[6:8], 32) // bpp
		binary.LittleEndian.PutUint32(entry[8:12], uint32(len(pngs[i])))
		binary.LittleEndian.PutUint32(entry[12:16], uint32(offset))
		copy(buf[offset:], pngs[i])
		offset += len(pngs[i])
	}
	return buf
}
