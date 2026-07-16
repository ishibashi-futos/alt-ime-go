// mkicon generates the project icon in SVG, PNG, and multi-size ICO
// formats using only the Go standard library.
//
// Regenerate after changing the geometry or palette below (from the
// repository root, because the output paths are root-relative):
//
//	go run ./tools/mkicon
package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"math"
	"os"
	"path/filepath"
)

const (
	iconSVG = "assets/alt-ime-icon.svg"
	iconPNG = "assets/alt-ime-icon.png"
	iconICO = "assets/alt-ime-icon.ico"

	canvasSize = 256.0
	samples    = 4
)

var iconSizes = []int{16, 20, 24, 32, 40, 48, 64, 128, 256}

var (
	charcoal = rgba{r: 0x30, g: 0x32, b: 0x36, a: 0xff}
	white    = rgba{r: 0xf7, g: 0xf7, b: 0xf2, a: 0xff}
)

const svgSource = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256" role="img" aria-label="alt-ime icon">
  <rect x="12" y="12" width="232" height="232" rx="44" fill="#303236"/>
  <g fill="none" stroke="#f7f7f2" stroke-linecap="round" stroke-linejoin="round">
    <path d="M54 174 88 82l34 92M68 139h40" stroke-width="13"/>
    <path d="M142 106h65M163 84l-5 88M199 118l-55 57" stroke-width="11"/>
    <ellipse cx="174" cy="146" rx="37" ry="31" stroke-width="11"/>
  </g>
</svg>
`

type point struct{ x, y float64 }

type rgba struct{ r, g, b, a byte }

func main() {
	if err := os.MkdirAll(filepath.Dir(iconICO), 0o755); err != nil {
		fatal(err)
	}
	if err := os.WriteFile(iconSVG, []byte(svgSource), 0o644); err != nil {
		fatal(err)
	}

	encoded := make([][]byte, 0, len(iconSizes))
	for _, size := range iconSizes {
		data := encodePNG(size, renderIcon(size))
		encoded = append(encoded, data)
		if size == 256 {
			if err := os.WriteFile(iconPNG, data, 0o644); err != nil {
				fatal(err)
			}
		}
	}
	if err := os.WriteFile(iconICO, buildICO(encoded), 0o644); err != nil {
		fatal(err)
	}
	fmt.Printf("mkicon: wrote %s, %s, and %s (%d sizes)\n", iconSVG, iconPNG, iconICO, len(iconSizes))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "mkicon:", err)
	os.Exit(1)
}

func renderIcon(size int) []byte {
	pixels := make([]byte, size*size*4)
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var opaque, red, green, blue int
			for sy := 0; sy < samples; sy++ {
				for sx := 0; sx < samples; sx++ {
					x := (float64(px) + (float64(sx)+0.5)/samples) * canvasSize / float64(size)
					y := (float64(py) + (float64(sy)+0.5)/samples) * canvasSize / float64(size)
					c, ok := sampleIcon(x, y)
					if !ok {
						continue
					}
					opaque++
					red += int(c.r)
					green += int(c.g)
					blue += int(c.b)
				}
			}
			if opaque == 0 {
				continue
			}
			off := (py*size + px) * 4
			pixels[off] = uint8(red / opaque)
			pixels[off+1] = uint8(green / opaque)
			pixels[off+2] = uint8(blue / opaque)
			pixels[off+3] = uint8(opaque * 255 / (samples * samples))
		}
	}
	return pixels
}

func sampleIcon(x, y float64) (rgba, bool) {
	if !insideRoundedRect(x, y, 12, 12, 244, 244, 44) {
		return rgba{}, false
	}

	// A = direct input; the hand-drawn kana-like mark = Japanese input.
	if onSegment(x, y, point{54, 174}, point{88, 82}, 6.5) ||
		onSegment(x, y, point{88, 82}, point{122, 174}, 6.5) ||
		onSegment(x, y, point{68, 139}, point{108, 139}, 6.5) ||
		onSegment(x, y, point{142, 106}, point{207, 106}, 5.5) ||
		onSegment(x, y, point{163, 84}, point{158, 172}, 5.5) ||
		onSegment(x, y, point{199, 118}, point{144, 175}, 5.5) ||
		onEllipse(x, y, 174, 146, 37, 31, 5.5) {
		return white, true
	}
	return charcoal, true
}

// encodePNG writes an 8-bit RGBA PNG. Keeping this tiny encoder here avoids
// depending on image/png, which is not present in every minimal Go toolchain.
func encodePNG(size int, pixels []byte) []byte {
	raw := make([]byte, size*(1+size*4))
	for y := 0; y < size; y++ {
		dst := y * (1 + size*4)
		raw[dst] = 0 // PNG filter: None
		copy(raw[dst+1:dst+1+size*4], pixels[y*size*4:(y+1)*size*4])
	}
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	_, _ = zw.Write(raw)
	_ = zw.Close()

	var ihdr [13]byte
	binary.BigEndian.PutUint32(ihdr[0:4], uint32(size))
	binary.BigEndian.PutUint32(ihdr[4:8], uint32(size))
	ihdr[8] = 8 // bit depth
	ihdr[9] = 6 // RGBA

	var out bytes.Buffer
	out.Write([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a})
	writePNGChunk(&out, "IHDR", ihdr[:])
	writePNGChunk(&out, "IDAT", compressed.Bytes())
	writePNGChunk(&out, "IEND", nil)
	return out.Bytes()
}

func writePNGChunk(out *bytes.Buffer, kind string, data []byte) {
	_ = binary.Write(out, binary.BigEndian, uint32(len(data)))
	out.WriteString(kind)
	out.Write(data)
	checksum := crc32.NewIEEE()
	_, _ = checksum.Write([]byte(kind))
	_, _ = checksum.Write(data)
	_ = binary.Write(out, binary.BigEndian, checksum.Sum32())
}

func insideRoundedRect(x, y, left, top, right, bottom, radius float64) bool {
	cx := math.Max(left+radius, math.Min(x, right-radius))
	cy := math.Max(top+radius, math.Min(y, bottom-radius))
	return math.Hypot(x-cx, y-cy) <= radius
}

func onSegment(x, y float64, a, b point, radius float64) bool {
	dx, dy := b.x-a.x, b.y-a.y
	length2 := dx*dx + dy*dy
	t := ((x-a.x)*dx + (y-a.y)*dy) / length2
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(x-(a.x+t*dx), y-(a.y+t*dy)) <= radius
}

func onEllipse(x, y, cx, cy, rx, ry, radius float64) bool {
	// A short polyline is deterministic and accurate enough at icon sizes.
	const steps = 48
	previous := point{cx + rx, cy}
	for i := 1; i <= steps; i++ {
		angle := float64(i) * 2 * math.Pi / steps
		next := point{cx + rx*math.Cos(angle), cy + ry*math.Sin(angle)}
		if onSegment(x, y, previous, next, radius) {
			return true
		}
		previous = next
	}
	return false
}

func buildICO(images [][]byte) []byte {
	const headerSize = 6
	const entrySize = 16
	offset := headerSize + entrySize*len(images)
	buf := new(bytes.Buffer)
	le := binary.LittleEndian
	put := func(v any) { _ = binary.Write(buf, le, v) }

	put(uint16(0))
	put(uint16(1)) // icon
	put(uint16(len(images)))
	for i, data := range images {
		size := iconSizes[i]
		dimension := uint8(size)
		if size == 256 {
			dimension = 0
		}
		put(dimension)
		put(dimension)
		put(uint8(0)) // color count: true color
		put(uint8(0))
		put(uint16(1))
		put(uint16(32))
		put(uint32(len(data)))
		put(uint32(offset))
		offset += len(data)
	}
	for _, data := range images {
		buf.Write(data)
	}
	return buf.Bytes()
}
