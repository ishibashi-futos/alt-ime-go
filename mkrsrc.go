//go:build ignore

// mkrsrc.go generates rsrc_windows_amd64.syso: a minimal COFF object whose
// .rsrc section embeds alt-ime.manifest as RT_MANIFEST resource ID 1
// (language 0x0409). The Go linker links *_windows_amd64.syso files into the
// PE image automatically, which is how the PerMonitorV2 DPI manifest reaches
// the executable without any third-party resource tool.
//
// Regenerate after editing alt-ime.manifest:
//
//	go run mkrsrc.go
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
)

const (
	manifestFile = "alt-ime.manifest"
	sysoFile     = "rsrc_windows_amd64.syso"

	imageFileMachineAMD64 = 0x8664
	imageRelAMD64Addr32NB = 0x0003 // 32-bit address without image base (RVA)
	imageSymClassStatic   = 3
	// IMAGE_SCN_CNT_INITIALIZED_DATA | IMAGE_SCN_MEM_READ
	rsrcSectionFlags = 0x40000040

	rtManifest    = 24
	manifestResID = 1
	langEnUS      = 0x0409

	subdirBit = 0x80000000

	fileHeaderSize    = 20
	sectionHeaderSize = 40
	relocationSize    = 10
	symbolSize        = 18
)

func main() {
	manifest, err := os.ReadFile(manifestFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mkrsrc:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(sysoFile, buildObject(manifest), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "mkrsrc:", err)
		os.Exit(1)
	}
	fmt.Printf("mkrsrc: wrote %s (%d bytes of manifest)\n", sysoFile, len(manifest))
}

// buildObject lays out the .rsrc section as a three-level resource directory
// (type -> id -> language) followed by one data entry and the manifest bytes:
//
//	 0 directory (type)          16 entry: ID 24 -> subdir 24
//	24 directory (id)            40 entry: ID 1  -> subdir 48
//	48 directory (language)      64 entry: ID 0x0409 -> data entry 72
//	72 data entry (RVA, size, code page, reserved)
//	88 manifest bytes
//
// The data entry's RVA field holds the section-relative offset (88) and is
// fixed up by the linker through an IMAGE_REL_AMD64_ADDR32NB relocation
// against the .rsrc section symbol.
func buildObject(manifest []byte) []byte {
	const dataEntryOff = 72
	const dataOff = 88
	rawSize := uint32(dataOff + len(manifest))
	relocPtr := uint32(fileHeaderSize + sectionHeaderSize + rawSize)
	symbolPtr := relocPtr + relocationSize

	buf := new(bytes.Buffer)
	le := binary.LittleEndian
	put := func(v any) { binary.Write(buf, le, v) }

	// IMAGE_FILE_HEADER
	put(uint16(imageFileMachineAMD64))
	put(uint16(1)) // NumberOfSections
	put(uint32(0)) // TimeDateStamp (kept 0 for reproducible output)
	put(symbolPtr) // PointerToSymbolTable
	put(uint32(1)) // NumberOfSymbols
	put(uint16(0)) // SizeOfOptionalHeader
	put(uint16(0)) // Characteristics

	// IMAGE_SECTION_HEADER ".rsrc"
	var name [8]byte
	copy(name[:], ".rsrc")
	put(name)
	put(uint32(0))                                  // VirtualSize
	put(uint32(0))                                  // VirtualAddress
	put(rawSize)                                    // SizeOfRawData
	put(uint32(fileHeaderSize + sectionHeaderSize)) // PointerToRawData
	put(relocPtr)                                   // PointerToRelocations
	put(uint32(0))                                  // PointerToLinenumbers
	put(uint16(1))                                  // NumberOfRelocations
	put(uint16(0))                                  // NumberOfLinenumbers
	put(uint32(rsrcSectionFlags))                   // Characteristics

	// Section data: resource directory tree.
	putDir := func(idEntries uint16) {
		put(uint32(0)) // Characteristics
		put(uint32(0)) // TimeDateStamp
		put(uint16(0)) // MajorVersion
		put(uint16(0)) // MinorVersion
		put(uint16(0)) // NumberOfNamedEntries
		put(idEntries) // NumberOfIdEntries
	}
	putDir(1)
	put(uint32(rtManifest))
	put(uint32(subdirBit | 24))
	putDir(1)
	put(uint32(manifestResID))
	put(uint32(subdirBit | 48))
	putDir(1)
	put(uint32(langEnUS))
	put(uint32(dataEntryOff))

	// IMAGE_RESOURCE_DATA_ENTRY
	put(uint32(dataOff)) // OffsetToData: section-relative, relocated to an RVA
	put(uint32(len(manifest)))
	put(uint32(0)) // CodePage
	put(uint32(0)) // Reserved

	buf.Write(manifest)

	// IMAGE_RELOCATION for the data entry's OffsetToData field.
	put(uint32(dataEntryOff)) // VirtualAddress
	put(uint32(0))            // SymbolTableIndex (.rsrc)
	put(uint16(imageRelAMD64Addr32NB))

	// IMAGE_SYMBOL ".rsrc"
	put(name)
	put(uint32(0)) // Value
	put(int16(1))  // SectionNumber
	put(uint16(0)) // Type
	put(uint8(imageSymClassStatic))
	put(uint8(0)) // NumberOfAuxSymbols

	// Empty COFF string table (its 4-byte length field includes itself).
	put(uint32(4))

	return buf.Bytes()
}
