// Package workspacedescriptor validates and compares the workspace location a
// provider reports for an environment. It never touches a filesystem: the path
// names a location on the provider's worker, parsed by the environment's
// declared platform.
package workspacedescriptor

import (
	"errors"
	"strings"
)

const (
	maxPathBytes     = 1024
	maxIdentityBytes = 128
)

// Descriptor is the recorded workspace location and opaque identity.
type Descriptor struct {
	Path     string `json:"path"`
	Identity string `json:"identity"`
}

type form int

const (
	formPOSIX form = iota
	formWindows
)

// platformForm reads the operating system from a `goos-goarch` or
// `goos/goarch` platform string.
func platformForm(platform string) (form, error) {
	goos := strings.ToLower(strings.TrimSpace(platform))
	if i := strings.IndexAny(goos, "-/"); i >= 0 {
		goos = goos[:i]
	}
	switch goos {
	case "linux", "darwin":
		return formPOSIX, nil
	case "windows":
		return formWindows, nil
	}
	return formPOSIX, errors.New("workspace platform is unsupported")
}

// Validate checks both fields of a descriptor.
func Validate(platform, path, identity string) error {
	if err := ValidatePath(platform, path); err != nil {
		return err
	}
	return ValidateIdentity(identity)
}

// ValidateIdentity accepts 1 to 128 characters of [A-Za-z0-9._:-].
func ValidateIdentity(identity string) error {
	if identity == "" || len(identity) > maxIdentityBytes {
		return errors.New("workspace identity must be 1 to 128 characters")
	}
	for i := 0; i < len(identity); i++ {
		c := identity[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '.', c == '_', c == ':', c == '-':
		default:
			return errors.New("workspace identity contains a character outside [A-Za-z0-9._:-]")
		}
	}
	return nil
}

// ValidatePath checks a workspace path against the platform's absolute form.
func ValidatePath(platform, path string) error {
	_, err := segments(platform, path)
	return err
}

// NormalizedPath returns the comparison form of a valid path: repeated
// separators collapsed and the trailing separator removed; on Windows also
// lower-cased with `\` separators.
func NormalizedPath(platform, path string) (string, error) {
	parts, err := segments(platform, path)
	if err != nil {
		return "", err
	}
	if kind, _ := platformForm(platform); kind == formWindows {
		return strings.ToLower(strings.Join(parts, `\`)), nil
	}
	return "/" + strings.Join(parts, "/"), nil
}

// Conflicts reports whether two descriptors of one installation collide on the
// normalized path or on the identity. A path that fails validation never
// compares equal by path.
func Conflicts(platform string, a, b Descriptor) bool {
	if a.Identity != "" && a.Identity == b.Identity {
		return true
	}
	left, errLeft := NormalizedPath(platform, a.Path)
	right, errRight := NormalizedPath(platform, b.Path)
	return errLeft == nil && errRight == nil && left == right
}

// segments returns the non-empty path segments; for Windows the first element
// is the drive prefix such as `C:`.
func segments(platform, path string) ([]string, error) {
	kind, err := platformForm(platform)
	if err != nil {
		return nil, err
	}
	if path == "" || len(path) > maxPathBytes {
		return nil, errors.New("workspace path must be 1 to 1024 bytes")
	}
	for i := 0; i < len(path); i++ {
		if path[i] < 0x20 || path[i] == 0x7f {
			return nil, errors.New("workspace path contains a control character")
		}
	}
	if kind == formWindows {
		return windowsSegments(path)
	}
	return posixSegments(path)
}

func posixSegments(path string) ([]string, error) {
	if path[0] != '/' {
		return nil, errors.New("workspace path must be absolute")
	}
	return cleanSegments(strings.Split(path, "/"), nil)
}

func windowsSegments(path string) ([]string, error) {
	if strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, "//") {
		return nil, errors.New("workspace path must not be a UNC path")
	}
	if len(path) < 3 || !isDriveLetter(path[0]) || path[1] != ':' || (path[2] != '\\' && path[2] != '/') {
		return nil, errors.New("workspace path must be drive-letter absolute")
	}
	parts := strings.FieldsFunc(path[3:], func(r rune) bool { return r == '\\' || r == '/' })
	return cleanSegments(parts, []string{path[:2]})
}

func cleanSegments(parts, prefix []string) ([]string, error) {
	out := append([]string(nil), prefix...)
	for _, part := range parts {
		switch part {
		case "":
		case ".", "..":
			return nil, errors.New("workspace path contains a dot segment")
		default:
			out = append(out, part)
		}
	}
	if len(out) == len(prefix) {
		return nil, errors.New("workspace path must not be a root")
	}
	return out, nil
}

func isDriveLetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
