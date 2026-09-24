package workspaces

import (
	"encoding/binary"
	"errors"
	"unicode/utf16"
)

func decodeWindowsReparsePath(data []byte, returned uint32, pathOffset int) (string, error) {
	if returned > uint32(len(data)) || int(returned) < pathOffset || pathOffset < 0 {
		return "", errors.New("reparse point data is truncated")
	}
	nameOffset := binary.LittleEndian.Uint16(data[8:10])
	nameLength := binary.LittleEndian.Uint16(data[10:12])
	if nameLength == 0 {
		return "", errors.New("reparse point target name is empty")
	}
	start := pathOffset + int(nameOffset)
	end := start + int(nameLength)
	if start < pathOffset || end > int(returned) || nameLength%2 != 0 {
		return "", errors.New("reparse point path is invalid")
	}
	units := make([]uint16, nameLength/2)
	for index := range units {
		units[index] = binary.LittleEndian.Uint16(data[start+index*2 : start+index*2+2])
	}
	return string(utf16.Decode(units)), nil
}
