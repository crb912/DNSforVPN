// genicons renders the DNSforVPN icon set: PWA PNG icons and a Windows
// .ico for installers/shortcuts. No external dependencies — the logo is
// drawn procedurally (gradient rounded square + white drop) with 4x
// supersampling for anti-aliasing.
//
// Run from the repository root:
//
//	go run ./tools/genicons
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

const ss = 4 // supersampling factor

var (
	gradTop = color.NRGBA{0x1e, 0x88, 0xe5, 0xff} // #1e88e5
	gradBot = color.NRGBA{0x0d, 0x47, 0xa1, 0xff} // #0d47a1
	white   = color.NRGBA{0xff, 0xff, 0xff, 0xff}
	clear   = color.NRGBA{0, 0, 0, 0}
)

type geom struct {
	size float64 // logical size (before supersampling)
	// drop geometry, relative to size
	apexX, apexY   float64
	baseLX, baseRX float64
	baseY          float64
	circleX        float64
	circleY        float64
	circleR        float64
	holeR          float64
	cornerR        float64
	maskable       bool
}

func defaultGeom(size float64, maskable bool) geom {
	scale := 1.0
	if maskable {
		scale = 0.8 // keep artwork inside the maskable safe zone
	}
	cx := size / 2
	return geom{
		size:     size,
		apexX:    cx,
		apexY:    cx - 0.33*size*scale,
		baseLX:   cx - 0.14*size*scale,
		baseRX:   cx + 0.14*size*scale,
		baseY:    cx + 0.06*size*scale,
		circleX:  cx,
		circleY:  cx + 0.12*size*scale,
		circleR:  0.21 * size * scale,
		holeR:    0.09 * size * scale,
		cornerR:  0.18 * size,
		maskable: maskable,
	}
}

// grad returns the background gradient color at relative height t in [0,1].
func grad(t float64) color.NRGBA {
	lerp := func(a, b uint8) uint8 {
		return uint8(float64(a) + (float64(b)-float64(a))*t)
	}
	return color.NRGBA{lerp(gradTop.R, gradBot.R), lerp(gradTop.G, gradBot.G), lerp(gradTop.B, gradBot.B), 0xff}
}

func sign(px, py, ax, ay, bx, by float64) float64 {
	return (px-bx)*(ay-by) - (ax-bx)*(py-by)
}

func inTriangle(px, py float64, g geom) bool {
	d1 := sign(px, py, g.apexX, g.apexY, g.baseLX, g.baseY)
	d2 := sign(px, py, g.baseLX, g.baseY, g.baseRX, g.baseY)
	d3 := sign(px, py, g.baseRX, g.baseY, g.apexX, g.apexY)
	hasNeg := d1 < 0 || d2 < 0 || d3 < 0
	hasPos := d1 > 0 || d2 > 0 || d3 > 0
	return !(hasNeg && hasPos)
}

func dist(px, py, cx, cy float64) float64 {
	dx, dy := px-cx, py-cy
	return dx*dx + dy*dy // squared; compare against r*r
}

func inRoundedRect(px, py float64, g geom) bool {
	s := g.size
	r := g.cornerR
	if px < 0 || py < 0 || px > s || py > s {
		return false
	}
	// Corner circles.
	for _, c := range [][2]float64{{r, r}, {s - r, r}, {r, s - r}, {s - r, s - r}} {
		inCornerBox := (px < r || px > s-r) && (py < r || py > s-r)
		if inCornerBox {
			if (px-c[0])*(px-c[0])+(py-c[1])*(py-c[1]) > r*r {
				// Only reject when this is the nearest corner.
				if (px < s/2) == (c[0] < s/2) && (py < s/2) == (c[1] < s/2) {
					return false
				}
			}
		}
	}
	return true
}

func render(logicalSize int, maskable bool) *image.NRGBA {
	S := logicalSize * ss
	g := defaultGeom(float64(S), maskable)
	img := image.NewNRGBA(image.Rect(0, 0, S, S))

	for y := 0; y < S; y++ {
		for x := 0; x < S; x++ {
			px, py := float64(x), float64(y)
			var col color.NRGBA
			if g.maskable || inRoundedRect(px, py, g) {
				col = grad(py / float64(S))
			} else {
				col = clear
			}
			inDrop := inTriangle(px, py, g) ||
				dist(px, py, g.circleX, g.circleY) < g.circleR*g.circleR
			if inDrop {
				col = white
			}
			if dist(px, py, g.circleX, g.circleY) < g.holeR*g.holeR {
				col = grad(py / float64(S))
			}
			img.SetNRGBA(x, y, col)
		}
	}
	return downsample(img, logicalSize)
}

func downsample(src *image.NRGBA, size int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a uint32
			for dy := 0; dy < ss; dy++ {
				for dx := 0; dx < ss; dx++ {
					c := src.NRGBAAt(x*ss+dx, y*ss+dy)
					r += uint32(c.R)
					g += uint32(c.G)
					b += uint32(c.B)
					a += uint32(c.A)
				}
			}
			n := uint32(ss * ss)
			out.SetNRGBA(x, y, color.NRGBA{
				uint8(r / n), uint8(g / n), uint8(b / n), uint8(a / n),
			})
		}
	}
	return out
}

func writePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// writeICO writes a single-image ICO embedding PNG data (valid for
// Windows Vista and later).
func writeICO(path string, img image.Image) error {
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		return err
	}
	b := img.Bounds().Dx()
	w, h := byte(b), byte(b)
	if b >= 256 {
		w, h = 0, 0 // 0 means 256 in the ICO format
	}

	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, uint16(0)) // reserved
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1)) // type: icon
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1)) // image count
	buf.WriteByte(w)                                       // width
	buf.WriteByte(h)                                       // height
	buf.WriteByte(0)                                       // palette colors
	buf.WriteByte(0)                                       // reserved
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))  // color planes
	_ = binary.Write(&buf, binary.LittleEndian, uint16(32)) // bits per pixel
	_ = binary.Write(&buf, binary.LittleEndian, uint32(pngBuf.Len()))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(22)) // data offset
	buf.Write(pngBuf.Bytes())

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0644)
}

func main() {
	out := []struct {
		path     string
		size     int
		maskable bool
	}{
		{"frontend/public/icons/icon-192.png", 192, false},
		{"frontend/public/icons/icon-512.png", 512, false},
		{"frontend/public/icons/icon-512-maskable.png", 512, true},
	}
	for _, o := range out {
		if err := writePNG(o.path, render(o.size, o.maskable)); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		fmt.Println("wrote", o.path)
	}
	if err := writeICO("deploy/windows/dnsforvpn.ico", render(256, false)); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Println("wrote deploy/windows/dnsforvpn.ico")
}
