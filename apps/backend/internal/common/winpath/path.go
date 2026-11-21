// Package winpath parses Windows paths and opens them without following reparse points.
package winpath

import (
	"errors"
	"strings"
)

var errPathNotAbsolute = errors.New("windows path must be absolute")

// PathParts splits an absolute Windows path into its volume root and components.
func PathParts(path string) (string, []string, error) {
	volume, parts, ok := splitWindowsPath(path)
	if !ok {
		return "", nil, errPathNotAbsolute
	}
	return volume + `\`, parts, nil
}

// NTPath converts a Windows path to the NT object-manager namespace.
func NTPath(path string) string {
	clean, ok := cleanWindowsPath(path)
	if !ok {
		clean = strings.ReplaceAll(path, "/", `\`)
	}
	if strings.HasPrefix(clean, `\\?\`) {
		return `\??\` + strings.TrimPrefix(clean, `\\?\`)
	}
	if strings.HasPrefix(clean, `\\`) {
		return `\??\UNC\` + strings.TrimPrefix(clean, `\\`)
	}
	return `\??\` + clean
}

// IsVolumeRoot reports whether path names a drive, share, or device-volume root.
func IsVolumeRoot(path string) bool {
	_, parts, ok := splitWindowsPath(path)
	return ok && len(parts) == 0
}

func cleanWindowsPath(path string) (string, bool) {
	volume, parts, ok := splitWindowsPath(path)
	if !ok {
		return "", false
	}
	clean := volume + `\`
	if len(parts) > 0 {
		clean += strings.Join(parts, `\`)
	}
	return clean, true
}

func splitWindowsPath(path string) (string, []string, bool) {
	path = strings.ReplaceAll(path, "/", `\`)
	switch {
	case strings.HasPrefix(path, `\\?\`):
		return splitExtendedWindowsPath(path)
	case strings.HasPrefix(path, `\\`):
		components := splitWindowsComponents(path[2:])
		if len(components) < 2 {
			return "", nil, false
		}
		volume := `\\` + components[0] + `\` + components[1]
		return volume, cleanWindowsComponents(components[2:]), true
	case len(path) >= 3 && isASCIIAlpha(path[0]) && path[1] == ':' && path[2] == '\\':
		return path[:2], cleanWindowsComponents(splitWindowsComponents(path[3:])), true
	default:
		return "", nil, false
	}
}

func splitExtendedWindowsPath(path string) (string, []string, bool) {
	components := splitWindowsComponents(path[4:])
	if len(components) == 0 {
		return "", nil, false
	}
	if strings.EqualFold(components[0], "UNC") {
		if len(components) < 3 {
			return "", nil, false
		}
		volume := `\\?\` + components[0] + `\` + components[1] + `\` + components[2]
		return volume, cleanWindowsComponents(components[3:]), true
	}
	volume := `\\?\` + components[0]
	return volume, cleanWindowsComponents(components[1:]), true
}

func splitWindowsComponents(path string) []string {
	return strings.FieldsFunc(path, func(char rune) bool { return char == '\\' })
}

func cleanWindowsComponents(components []string) []string {
	clean := make([]string, 0, len(components))
	for _, component := range components {
		switch component {
		case "", ".":
			continue
		case "..":
			if len(clean) > 0 {
				clean = clean[:len(clean)-1]
			}
		default:
			clean = append(clean, component)
		}
	}
	if len(clean) == 0 {
		return nil
	}
	return clean
}

func isASCIIAlpha(char byte) bool {
	return char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z'
}
