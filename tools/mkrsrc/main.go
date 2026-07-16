// mkrsrc generates cmd/alt-ime-go/rsrc_windows_amd64.syso: a COFF object
// whose .rsrc section embeds the PerMonitorV2 manifest and the multi-size
// application icon. The Go linker includes *_windows_amd64.syso files from
// the main package directory in the PE image automatically, so no
// third-party resource compiler is required.
//
// Regenerate after editing cmd/alt-ime-go/alt-ime.manifest or
// assets/alt-ime-icon.ico (from the repository root, because the paths are
// root-relative):
//
//	go run ./tools/mkrsrc
package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sort"
)

const (
	manifestFile = "cmd/alt-ime-go/alt-ime.manifest"
	iconFile     = "assets/alt-ime-icon.ico"
	sysoFile     = "cmd/alt-ime-go/rsrc_windows_amd64.syso"

	imageFileMachineAMD64 = 0x8664
	imageRelAMD64Addr32NB = 0x0003 // 32-bit address without image base (RVA)
	imageSymClassStatic   = 3
	// IMAGE_SCN_CNT_INITIALIZED_DATA | IMAGE_SCN_MEM_READ
	rsrcSectionFlags = 0x40000040

	rtIcon      = 3
	rtGroupIcon = 14
	rtManifest  = 24

	appIconResID  = 1
	manifestResID = 1
	langNeutral   = 0
	langEnUS      = 0x0409

	subdirBit = 0x80000000

	fileHeaderSize    = 20
	sectionHeaderSize = 40
	relocationSize    = 10
	symbolSize        = 18
)

type resource struct {
	typeID uint32
	id     uint32
	lang   uint32
	data   []byte

	langDirOff   uint32
	dataEntryOff uint32
	dataOff      uint32
}

type resourceType struct {
	id     uint32
	dirOff uint32
	items  []*resource
}

type icoImage struct {
	width      byte
	height     byte
	colorCount byte
	planes     uint16
	bitCount   uint16
	data       []byte
}

func main() {
	manifest, err := os.ReadFile(manifestFile)
	if err != nil {
		fatal(err)
	}
	ico, err := os.ReadFile(iconFile)
	if err != nil {
		fatal(err)
	}
	resources, err := makeResources(manifest, ico)
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(sysoFile, buildObject(resources), 0o644); err != nil {
		fatal(err)
	}
	fmt.Printf("mkrsrc: wrote %s (%d resources: manifest + %d icon images + group)\n",
		sysoFile, len(resources), len(resources)-2)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "mkrsrc:", err)
	os.Exit(1)
}

func makeResources(manifest, ico []byte) ([]*resource, error) {
	images, err := parseICO(ico)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", iconFile, err)
	}
	resources := make([]*resource, 0, len(images)+2)
	for i, img := range images {
		resources = append(resources, &resource{
			typeID: rtIcon,
			id:     uint32(i + 1),
			lang:   langNeutral,
			data:   img.data,
		})
	}
	resources = append(resources,
		&resource{typeID: rtGroupIcon, id: appIconResID, lang: langNeutral, data: buildGroupIcon(images)},
		&resource{typeID: rtManifest, id: manifestResID, lang: langEnUS, data: manifest},
	)
	return resources, nil
}

func parseICO(data []byte) ([]icoImage, error) {
	if len(data) < 6 {
		return nil, errors.New("truncated ICONDIR")
	}
	le := binary.LittleEndian
	if le.Uint16(data[0:2]) != 0 || le.Uint16(data[2:4]) != 1 {
		return nil, errors.New("not an icon file")
	}
	count := int(le.Uint16(data[4:6]))
	if count == 0 || len(data) < 6+16*count {
		return nil, errors.New("invalid image directory")
	}
	images := make([]icoImage, 0, count)
	for i := 0; i < count; i++ {
		off := 6 + i*16
		size := uint64(le.Uint32(data[off+8 : off+12]))
		imageOff := uint64(le.Uint32(data[off+12 : off+16]))
		if size == 0 || imageOff > uint64(len(data)) || size > uint64(len(data))-imageOff {
			return nil, fmt.Errorf("invalid image %d bounds", i)
		}
		images = append(images, icoImage{
			width:      data[off],
			height:     data[off+1],
			colorCount: data[off+2],
			planes:     le.Uint16(data[off+4 : off+6]),
			bitCount:   le.Uint16(data[off+6 : off+8]),
			data:       data[imageOff : imageOff+size],
		})
	}
	return images, nil
}

// buildGroupIcon converts ICONDIR entries to the RT_GROUP_ICON format. The
// final WORD in each entry identifies the corresponding RT_ICON resource.
func buildGroupIcon(images []icoImage) []byte {
	buf := new(bytes.Buffer)
	le := binary.LittleEndian
	put := func(v any) { _ = binary.Write(buf, le, v) }
	put(uint16(0))
	put(uint16(1))
	put(uint16(len(images)))
	for i, img := range images {
		put(img.width)
		put(img.height)
		put(img.colorCount)
		put(uint8(0))
		put(img.planes)
		put(img.bitCount)
		put(uint32(len(img.data)))
		put(uint16(i + 1))
	}
	return buf.Bytes()
}

// buildObject lays out a three-level resource directory tree
// (type -> id -> language), data entries, aligned resource payloads, and one
// IMAGE_REL_AMD64_ADDR32NB relocation for every data entry.
func buildObject(resources []*resource) []byte {
	sort.Slice(resources, func(i, j int) bool {
		if resources[i].typeID != resources[j].typeID {
			return resources[i].typeID < resources[j].typeID
		}
		if resources[i].id != resources[j].id {
			return resources[i].id < resources[j].id
		}
		return resources[i].lang < resources[j].lang
	})

	var types []*resourceType
	for _, r := range resources {
		if len(types) == 0 || types[len(types)-1].id != r.typeID {
			types = append(types, &resourceType{id: r.typeID})
		}
		types[len(types)-1].items = append(types[len(types)-1].items, r)
	}

	cursor := uint32(16 + 8*len(types)) // root directory and its entries
	for _, typ := range types {
		typ.dirOff = cursor
		cursor += uint32(16 + 8*len(typ.items))
	}
	for _, r := range resources {
		r.langDirOff = cursor
		cursor += 24 // directory header + one language entry
	}
	for _, r := range resources {
		r.dataEntryOff = cursor
		cursor += 16
	}
	cursor = align4(cursor)
	for _, r := range resources {
		r.dataOff = cursor
		cursor += uint32(len(r.data))
		cursor = align4(cursor)
	}

	section := make([]byte, cursor)
	putDir(section, 0, uint16(len(types)))
	for i, typ := range types {
		entry := 16 + i*8
		put32(section, entry, typ.id)
		put32(section, entry+4, subdirBit|typ.dirOff)
		putDir(section, typ.dirOff, uint16(len(typ.items)))
		for j, r := range typ.items {
			itemEntry := int(typ.dirOff) + 16 + j*8
			put32(section, itemEntry, r.id)
			put32(section, itemEntry+4, subdirBit|r.langDirOff)

			putDir(section, r.langDirOff, 1)
			put32(section, int(r.langDirOff)+16, r.lang)
			put32(section, int(r.langDirOff)+20, r.dataEntryOff)

			put32(section, int(r.dataEntryOff), r.dataOff)
			put32(section, int(r.dataEntryOff)+4, uint32(len(r.data)))
			copy(section[r.dataOff:], r.data)
		}
	}

	rawSize := uint32(len(section))
	relocPtr := uint32(fileHeaderSize+sectionHeaderSize) + rawSize
	symbolPtr := relocPtr + uint32(relocationSize*len(resources))

	buf := new(bytes.Buffer)
	le := binary.LittleEndian
	put := func(v any) { _ = binary.Write(buf, le, v) }

	// IMAGE_FILE_HEADER
	put(uint16(imageFileMachineAMD64))
	put(uint16(1))
	put(uint32(0)) // reproducible TimeDateStamp
	put(symbolPtr)
	put(uint32(1))
	put(uint16(0))
	put(uint16(0))

	// IMAGE_SECTION_HEADER ".rsrc"
	var name [8]byte
	copy(name[:], ".rsrc")
	put(name)
	put(uint32(0))
	put(uint32(0))
	put(rawSize)
	put(uint32(fileHeaderSize + sectionHeaderSize))
	put(relocPtr)
	put(uint32(0))
	put(uint16(len(resources)))
	put(uint16(0))
	put(uint32(rsrcSectionFlags))
	buf.Write(section)

	for _, r := range resources {
		put(r.dataEntryOff)
		put(uint32(0)) // .rsrc symbol index
		put(uint16(imageRelAMD64Addr32NB))
	}

	// IMAGE_SYMBOL ".rsrc"
	put(name)
	put(uint32(0))
	put(int16(1))
	put(uint16(0))
	put(uint8(imageSymClassStatic))
	put(uint8(0))
	put(uint32(4)) // empty COFF string table

	return buf.Bytes()
}

func putDir(dst []byte, off uint32, idEntries uint16) {
	put16(dst, int(off)+14, idEntries)
}

func put16(dst []byte, off int, value uint16) {
	binary.LittleEndian.PutUint16(dst[off:off+2], value)
}

func put32(dst []byte, off int, value uint32) {
	binary.LittleEndian.PutUint32(dst[off:off+4], value)
}

func align4(value uint32) uint32 {
	return (value + 3) &^ 3
}
