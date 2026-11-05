package sandbox

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSandboxPolicyCeiling(t *testing.T) {
	ceiling := Grants{Filesystem: []FilesystemGrant{{Path: "/workspace", Writable: true}, {Path: "/runtime"}}, Network: []NetworkGrant{{Host: "api.example.test", Port: 443, Protocol: "https"}}, KandevMCP: true}
	for _, tc := range []struct {
		name   string
		grants Grants
		denied bool
	}{
		{"nested read", Grants{Filesystem: []FilesystemGrant{{Path: "/workspace/src"}}}, false},
		{"nested write", Grants{Filesystem: []FilesystemGrant{{Path: "/workspace/src", Writable: true}}}, false},
		{"read to write", Grants{Filesystem: []FilesystemGrant{{Path: "/runtime", Writable: true}}}, true},
		{"prefix sibling", Grants{Filesystem: []FilesystemGrant{{Path: "/workspace-secrets"}}}, true},
		{"parent root", Grants{Filesystem: []FilesystemGrant{{Path: "/"}}}, true},
		{"exact route", Grants{Network: []NetworkGrant{{Host: "api.example.test", Port: 443, Protocol: "https"}}}, false},
		{"wrong port", Grants{Network: []NetworkGrant{{Host: "api.example.test", Port: 80, Protocol: "http"}}}, true},
		{"subdomain", Grants{Network: []NetworkGrant{{Host: "evil.api.example.test", Port: 443, Protocol: "https"}}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := WithinCeiling(tc.grants, ceiling)
			if errors.Is(err, ErrOutsideCeiling) != tc.denied {
				t.Fatalf("denied=%v, error=%v", tc.denied, err)
			}
		})
	}
	if !errors.Is(WithinCeiling(Grants{KandevMCP: true}, Grants{}), ErrOutsideCeiling) {
		t.Fatal("MCP grant bypassed ceiling")
	}
}

func TestSandboxPolicyCanonicalize(t *testing.T) {
	root := t.TempDir()
	safe := filepath.Join(root, "work")
	secret := filepath.Join(root, "control")
	for _, path := range []string{safe, secret} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(safe, "escape")
	if err := os.Symlink(secret, link); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		grants Grants
	}{
		{"relative", Grants{Filesystem: []FilesystemGrant{{Path: "work"}}}},
		{"missing", Grants{Filesystem: []FilesystemGrant{{Path: filepath.Join(root, "missing")}}}},
		{"symlink", Grants{Filesystem: []FilesystemGrant{{Path: link}}}},
		{"protected ancestor", Grants{Filesystem: []FilesystemGrant{{Path: root}}}},
		{"protected descendant", Grants{Filesystem: []FilesystemGrant{{Path: secret}}}},
		{"wildcard", Grants{Network: []NetworkGrant{{Host: "*", Port: 443, Protocol: "https"}}}},
		{"subdomain wildcard", Grants{Network: []NetworkGrant{{Host: "*.example.test", Port: 443, Protocol: "https"}}}},
		{"URL", Grants{Network: []NetworkGrant{{Host: "https://example.test/path", Port: 443, Protocol: "https"}}}},
		{"all ports", Grants{Network: []NetworkGrant{{Host: "example.test", Protocol: "https"}}}},
		{"unknown protocol", Grants{Network: []NetworkGrant{{Host: "example.test", Port: 443, Protocol: "any"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Canonicalize(tc.grants, []string{secret})
			if !errors.Is(err, ErrInvalidPolicy) {
				t.Fatalf("accepted invalid grant: %v", err)
			}
		})
	}
	grants := Grants{Filesystem: []FilesystemGrant{{Path: safe}, {Path: safe, Writable: true}}, Network: []NetworkGrant{{Host: "API.EXAMPLE.TEST", Port: 443, Protocol: "https"}}}
	got, err := Canonicalize(grants, []string{secret})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Filesystem) != 1 || !got.Filesystem[0].Writable || got.Network[0].Host != "api.example.test" {
		t.Fatalf("not canonical: %+v", got)
	}
	if grants.Network[0].Host != "API.EXAMPLE.TEST" {
		t.Fatal("mutated input")
	}
}

func TestSandboxPolicyProtectedAlias(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "control")
	alias := filepath.Join(root, "alias")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
	_, err := Canonicalize(Grants{Filesystem: []FilesystemGrant{{Path: target}}}, []string{filepath.Join(alias, "future-secret")})
	if !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("protected alias exposed: %v", err)
	}
}
