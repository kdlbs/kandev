package winpath

import (
	"reflect"
	"testing"
)

func TestPathPartsUsesWindowsVolumeRoots(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		root  string
		parts []string
	}{
		{name: "drive root", path: `C:\`, root: `C:\`},
		{name: "drive child", path: `C:\one\two`, root: `C:\`, parts: []string{"one", "two"}},
		{name: "drive cleans dot segments", path: `C:\one\.\two\..\three`, root: `C:\`, parts: []string{"one", "three"}},
		{name: "ordinary UNC root", path: `\\server\share\`, root: `\\server\share\`},
		{name: "ordinary UNC child", path: `\\server\share\folder\child`, root: `\\server\share\`, parts: []string{"folder", "child"}},
		{name: "extended UNC root", path: `\\?\UNC\server\share\`, root: `\\?\UNC\server\share\`},
		{name: "extended UNC child", path: `\\?\UNC\server\share\folder`, root: `\\?\UNC\server\share\`, parts: []string{"folder"}},
		{name: "extended UNC cleans dot segments", path: `\\?\UNC\server\share\one\..\two`, root: `\\?\UNC\server\share\`, parts: []string{"two"}},
		{name: "extended drive root", path: `\\?\C:\`, root: `\\?\C:\`},
		{name: "extended drive child", path: `\\?\C:\one`, root: `\\?\C:\`, parts: []string{"one"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, parts, err := PathParts(test.path)
			if err != nil {
				t.Fatalf("PathParts(%q): %v", test.path, err)
			}
			if root != test.root {
				t.Errorf("root = %q, want %q", root, test.root)
			}
			if !reflect.DeepEqual(parts, test.parts) {
				t.Errorf("parts = %#v, want %#v", parts, test.parts)
			}
		})
	}
}

func TestPathPartsRejectsRelativePaths(t *testing.T) {
	for _, path := range []string{`relative\path`, `C:relative`, `\\server`} {
		if _, _, err := PathParts(path); err == nil {
			t.Errorf("PathParts(%q) succeeded, want an error", path)
		}
	}
}

func TestNTPathPreservesDriveAndUNCNamespaces(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: `C:\`, want: `\??\C:\`},
		{path: `C:\one\two`, want: `\??\C:\one\two`},
		{path: `\\server\share\`, want: `\??\UNC\server\share\`},
		{path: `\\?\UNC\server\share\`, want: `\??\UNC\server\share\`},
		{path: `\\?\UNC\server\share\child`, want: `\??\UNC\server\share\child`},
		{path: `\\?\C:\child`, want: `\??\C:\child`},
	}
	for _, test := range tests {
		if got := NTPath(test.path); got != test.want {
			t.Errorf("NTPath(%q) = %q, want %q", test.path, got, test.want)
		}
	}
}

func TestIsVolumeRoot(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: `C:\`, want: true},
		{path: `\\server\share\`, want: true},
		{path: `\\?\UNC\server\share\`, want: true},
		{path: `\\?\C:\`, want: true},
		{path: `C:\cache`, want: false},
		{path: `\\server\share\cache`, want: false},
		{path: `\\server`, want: false},
		{path: `relative`, want: false},
	}
	for _, test := range tests {
		if got := IsVolumeRoot(test.path); got != test.want {
			t.Errorf("IsVolumeRoot(%q) = %t, want %t", test.path, got, test.want)
		}
	}
}

func TestPathPartsHandlesSeparatorsAndRootParents(t *testing.T) {
	tests := []struct {
		path  string
		parts []string
	}{
		{path: "C:\\..\\one", parts: []string{"one"}},
		{path: "C:/one/two", parts: []string{"one", "two"}},
	}
	for _, test := range tests {
		root, parts, err := PathParts(test.path)
		if err != nil {
			t.Fatalf("PathParts(%q): %v", test.path, err)
		}
		if root != "C:\\" {
			t.Errorf("root = %q, want %q", root, "C:\\")
		}
		if !reflect.DeepEqual(parts, test.parts) {
			t.Errorf("parts = %#v, want %#v", parts, test.parts)
		}
	}
}
