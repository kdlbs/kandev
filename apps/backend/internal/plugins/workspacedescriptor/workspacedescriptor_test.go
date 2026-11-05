package workspacedescriptor

import (
	"strings"
	"testing"
)

func TestValidatePathPerPlatform(t *testing.T) {
	long := "/" + strings.Repeat("a", 1024)
	cases := []struct {
		name     string
		platform string
		path     string
		wantErr  bool
	}{
		{"posix absolute", "linux-amd64", "/srv/work/env-1", false},
		{"posix darwin", "darwin-arm64", "/Users/w/env", false},
		{"posix repeated and trailing separators", "linux-amd64", "/srv//work/", false},
		{"windows drive path", "windows-amd64", `C:\work\env`, false},
		{"windows forward slashes", "windows-amd64", "d:/work/env", false},
		{"slash separated platform", "windows/amd64", `C:\work`, false},
		{"empty", "linux-amd64", "", true},
		{"relative", "linux-amd64", "work/env", true},
		{"root only", "linux-amd64", "/", true},
		{"root only repeated", "linux-amd64", "///", true},
		{"windows root only", "windows-amd64", `C:\`, true},
		{"windows drive without separator", "windows-amd64", "C:", true},
		{"windows UNC", "windows-amd64", `\\server\share\w`, true},
		{"windows UNC forward", "windows-amd64", "//server/share/w", true},
		{"windows posix form", "windows-amd64", "/srv/work", true},
		{"posix windows form", "linux-amd64", `C:\work`, true},
		{"NUL", "linux-amd64", "/srv/a\x00b", true},
		{"control character", "linux-amd64", "/srv/a\nb", true},
		{"DEL", "linux-amd64", "/srv/a\x7fb", true},
		{"dot segment", "linux-amd64", "/srv/./w", true},
		{"parent segment", "linux-amd64", "/srv/../w", true},
		{"trailing dot segment", "linux-amd64", "/srv/w/.", true},
		{"windows parent segment", "windows-amd64", `C:\w\..\x`, true},
		{"exactly 1024 bytes", "linux-amd64", "/" + strings.Repeat("a", 1023), false},
		{"over 1024 bytes", "linux-amd64", long, true},
		{"unsupported platform", "plan9-amd64", "/srv/w", true},
		{"empty platform", "", "/srv/w", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePath(tc.platform, tc.path)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidatePath(%q, %q) error = %v, wantErr %v", tc.platform, tc.path, err, tc.wantErr)
			}
		})
	}
}

func TestValidateIdentity(t *testing.T) {
	cases := map[string]bool{
		"a":                      true,
		"alloc-1.b_c:d":          true,
		"alloc-1.b:c":            true,
		strings.Repeat("a", 128): true,
		strings.Repeat("a", 129): false,
		"":                       false,
		"has space":              false,
		"slash/inside":           false,
		"tab\t":                  false,
		"unicodé":                false,
		"UPPER.lower-09:x":       true,
	}
	for identity, ok := range cases {
		if err := ValidateIdentity(identity); (err == nil) != ok {
			t.Errorf("ValidateIdentity(%q) error = %v, want ok=%v", identity, err, ok)
		}
	}
}

func TestNormalizedPath(t *testing.T) {
	cases := []struct {
		platform string
		a, b     string
		same     bool
	}{
		{"linux-amd64", "/srv/w", "/srv//w/", true},
		{"linux-amd64", "/srv/w", "/srv/W", false},
		{"darwin-arm64", "/srv/w", "/srv/w//", true},
		{"windows-amd64", `C:\Work\Env`, `c:/work//env/`, true},
		{"windows-amd64", `C:\Work\Env`, `D:\Work\Env`, false},
		{"windows-amd64", `C:\Work\Env`, `C:\Work\Env2`, false},
	}
	for _, tc := range cases {
		a, errA := NormalizedPath(tc.platform, tc.a)
		b, errB := NormalizedPath(tc.platform, tc.b)
		if errA != nil || errB != nil {
			t.Fatalf("normalize failed: %v %v", errA, errB)
		}
		if (a == b) != tc.same {
			t.Errorf("%s: %q vs %q normalized to %q and %q, same=%v want %v", tc.platform, tc.a, tc.b, a, b, a == b, tc.same)
		}
	}
	if _, err := NormalizedPath("linux-amd64", "relative"); err == nil {
		t.Fatal("an invalid path must not normalize")
	}
}

func TestValidateChecksBothFields(t *testing.T) {
	if err := Validate("linux-amd64", "/srv/w", "id-1"); err != nil {
		t.Fatal(err)
	}
	if err := Validate("linux-amd64", "/srv/w", ""); err == nil {
		t.Fatal("a missing identity must be rejected")
	}
	if err := Validate("linux-amd64", "", "id-1"); err == nil {
		t.Fatal("a missing path must be rejected")
	}
}

func TestConflictsWith(t *testing.T) {
	a := Descriptor{Path: "/srv/w", Identity: "id-1"}
	if Conflicts("linux-amd64", a, Descriptor{Path: "/srv/w/", Identity: "id-2"}) != true {
		t.Fatal("the same normalized path must conflict")
	}
	if Conflicts("linux-amd64", a, Descriptor{Path: "/srv/x", Identity: "id-1"}) != true {
		t.Fatal("the same identity must conflict")
	}
	if Conflicts("linux-amd64", a, Descriptor{Path: "/srv/x", Identity: "id-2"}) {
		t.Fatal("distinct path and identity must not conflict")
	}
	win := Descriptor{Path: `C:\Work`, Identity: "w-1"}
	if !Conflicts("windows-amd64", win, Descriptor{Path: "c:/work/", Identity: "w-2"}) {
		t.Fatal("Windows paths compare case and separator insensitively")
	}
}
