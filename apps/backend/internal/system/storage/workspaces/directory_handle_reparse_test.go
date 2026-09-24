package workspaces

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

func TestDecodeWindowsReparsePathUsesSubstituteName(t *testing.T) {
	buffer := windowsSymlinkReparseBuffer(`\??\C:\actual-target`, `C:\display-name`)
	got, err := decodeWindowsReparsePath(buffer, uint32(len(buffer)), 20)
	if err != nil {
		t.Fatal(err)
	}
	if want := `\??\C:\actual-target`; got != want {
		t.Fatalf("decodeWindowsReparsePath() = %q, want %q", got, want)
	}
}

func TestDecodeWindowsReparsePathRejectsMissingSubstituteName(t *testing.T) {
	buffer := windowsSymlinkReparseBuffer("", `C:\display-name`)
	if _, err := decodeWindowsReparsePath(buffer, uint32(len(buffer)), 20); err == nil {
		t.Fatal("decodeWindowsReparsePath() accepted a display name without a target name")
	}
}

func TestDecodeWindowsReparsePathRejectsMalformedSubstituteNameMetadata(t *testing.T) {
	t.Run("odd offset", func(t *testing.T) {
		buffer := windowsSymlinkReparseBuffer(`\??\C:\actual-target`, `C:\display-name`)
		binary.LittleEndian.PutUint16(buffer[8:10], 1)
		if _, err := decodeWindowsReparsePath(buffer, uint32(len(buffer)), 20); err == nil {
			t.Fatal("decodeWindowsReparsePath() accepted an odd UTF-16 byte offset")
		}
	})
	t.Run("outside declared payload", func(t *testing.T) {
		buffer := windowsSymlinkReparseBuffer(`\??\C:\actual-target`, `C:\display-name`)
		binary.LittleEndian.PutUint16(buffer[4:6], 12)
		if _, err := decodeWindowsReparsePath(buffer, uint32(len(buffer)), 20); err == nil {
			t.Fatal("decodeWindowsReparsePath() accepted a name outside the declared reparse payload")
		}
	})
}

func windowsSymlinkReparseBuffer(substituteName, printName string) []byte {
	substitute := utf16.Encode([]rune(substituteName))
	print := utf16.Encode([]rune(printName))
	pathBuffer := make([]byte, (len(substitute)+len(print))*2)
	for index, unit := range substitute {
		binary.LittleEndian.PutUint16(pathBuffer[index*2:index*2+2], unit)
	}
	printOffset := len(substitute) * 2
	for index, unit := range print {
		binary.LittleEndian.PutUint16(pathBuffer[printOffset+index*2:printOffset+index*2+2], unit)
	}
	buffer := make([]byte, 20+len(pathBuffer))
	binary.LittleEndian.PutUint16(buffer[4:6], uint16(len(buffer)-8))
	binary.LittleEndian.PutUint16(buffer[8:10], 0)
	binary.LittleEndian.PutUint16(buffer[10:12], uint16(len(substitute)*2))
	binary.LittleEndian.PutUint16(buffer[12:14], uint16(printOffset))
	binary.LittleEndian.PutUint16(buffer[14:16], uint16(len(print)*2))
	copy(buffer[20:], pathBuffer)
	return buffer
}
