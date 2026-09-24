package workspaces

import (
	"encoding/binary"
	"errors"
	"unicode/utf16"
)

func decodeWindowsReparsePath(data []byte, returned uint32, pathOffset int) (string, error) {
	if returned > uint32(len(data)) || int(returned) < pathOffset || pathOffset < 16 {
		return "", errors.New("reparse point data is truncated")
	}
	declaredEnd := 8 + int(binary.LittleEndian.Uint16(data[4:6]))
	if declaredEnd < pathOffset || declaredEnd > int(returned) {
		return "", errors.New("reparse point payload is truncated")
	}
	nameOffset := binary.LittleEndian.Uint16(data[8:10])
	nameLength := binary.LittleEndian.Uint16(data[10:12])
	if nameLength == 0 || nameOffset%2 != 0 || nameLength%2 != 0 {
		return "", errors.New("reparse point target name is invalid")
	}
	start := pathOffset + int(nameOffset)
	end := start + int(nameLength)
	if start < pathOffset || end > declaredEnd {
		return "", errors.New("reparse point path is invalid")
	}
	units := make([]uint16, nameLength/2)
	for index := range units {
		units[index] = binary.LittleEndian.Uint16(data[start+index*2 : start+index*2+2])
	}
	if err := validateWindowsReparseTargetUnits(units); err != nil {
		return "", err
	}
	return string(utf16.Decode(units)), nil
}

func validateWindowsReparseTargetUnits(units []uint16) error {
	for index := 0; index < len(units); index++ {
		unit := units[index]
		if unit == 0 {
			return errors.New("reparse point target name contains NUL")
		}
		if unit >= 0xD800 && unit <= 0xDBFF {
			if index+1 >= len(units) || units[index+1] < 0xDC00 || units[index+1] > 0xDFFF {
				return errors.New("reparse point target name contains invalid UTF-16")
			}
			index++
			continue
		}
		if unit >= 0xDC00 && unit <= 0xDFFF {
			return errors.New("reparse point target name contains invalid UTF-16")
		}
	}
	return nil
}
