package copyfiles

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

// @covers AC-WORKSPACES-COPYFILES-GLOB-TOKENIZATION-001.1
// @covers AC-WORKSPACES-COPYFILES-GLOB-TOKENIZATION-001.2
// @covers AC-WORKSPACES-COPYFILES-GLOB-TOKENIZATION-001.4
func TestParseSpecs_GlobTokenization(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, spec string
		want       []PatternSpec
	}{
		{"unclosed class", `config/[a.env, .env.local`, toSpecs(`config/[a.env`, `.env.local`)},
		{"class comma", `config/[a,b].env, .env.local`, toSpecs(`config/[a,b].env`, `.env.local`)},
		{"class opening brace", `config/[{].env, .env.local`, toSpecs(`config/[{].env`, `.env.local`)},
		{"class closing brace", `config/{[}],dev}.env, .env.local`, toSpecs(`config/{[}],dev}.env`, `.env.local`)},
		{"negated class", `config/[!a,b{].env, .env.local`, toSpecs(`config/[!a,b{].env`, `.env.local`)},
		{"caret negation", `config/[^a,b{].env, .env.local`, toSpecs(`config/[^a,b{].env`, `.env.local`)},
		{"nested alternation", `config/{a,{b,c}}.env, .env.local`, toSpecs(`config/{a,{b,c}}.env`, `.env.local`)},
		{"nested class brackets", `config/[[,].env, .env.local`, toSpecs(`config/[[,].env`, `.env.local`)},
		{"suffix precedence", `config/[a,b].env:symlink, config/[a,b].env, literal::symlink, .env:`, []PatternSpec{
			{Pattern: `config/[a,b].env`, Symlink: true}, {Pattern: `literal:symlink`}, {Pattern: `.env:`},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseSpecs(tc.spec); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ParseSpecs(%q) = %#v, want %#v", tc.spec, got, tc.want)
			}
		})
	}
}

// @covers AC-WORKSPACES-COPYFILES-GLOB-TOKENIZATION-001.2
func TestParseSpecs_NativeEscapes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		spec           string
		posix, windows []string
	}{
		{`config/\{.env, .env.local`, []string{`config/\{.env`, `.env.local`}, []string{`config/\{.env, .env.local`}},
		{`config\, .env.local`, []string{`config\, .env.local`}, []string{`config\`, `.env.local`}},
		{`config/a\,b.env, .env.local`, []string{`config/a\,b.env`, `.env.local`}, []string{`config/a\`, `b.env`, `.env.local`}},
		{`config/\[a,b.env, .env.local`, []string{`config/\[a`, `b.env`, `.env.local`}, []string{`config/\[a`, `b.env`, `.env.local`}},
		{`config/[\],{].env, .env.local`, []string{`config/[\],{].env`, `.env.local`}, []string{`config/[\]`, `{].env, .env.local`}},
		{`config/{a\},b}.env, .env.local`, []string{`config/{a\},b}.env`, `.env.local`}, []string{`config/{a\}`, `b}.env`, `.env.local`}},
		{`config\[a,b].env, .env.local`, []string{`config\[a`, `b].env`, `.env.local`}, []string{`config\[a,b].env`, `.env.local`}},
		{`config/\\{a,b}.env, .env.local`, []string{`config/\\{a,b}.env`, `.env.local`}, []string{`config/\\{a,b}.env`, `.env.local`}},
	}
	for _, tc := range cases {
		t.Run(tc.spec, func(t *testing.T) {
			want := tc.posix
			if runtime.GOOS == "windows" {
				want = tc.windows
			}
			if got := Parse(tc.spec); !reflect.DeepEqual(got, want) {
				t.Fatalf("Parse(%q) = %#v, want %#v", tc.spec, got, want)
			}
		})
	}
}

// @covers AC-WORKSPACES-COPYFILES-GLOB-TOKENIZATION-001.1
// @covers AC-WORKSPACES-COPYFILES-GLOB-TOKENIZATION-001.2
// @covers AC-WORKSPACES-COPYFILES-GLOB-TOKENIZATION-001.3
func TestCopyPlan_GlobTokenization(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, spec string
		files      []string
		posix      bool
	}{
		{"class comma", `config/[a,b].env`, []string{"config/a.env", "config/b.env"}, false},
		{"class brace", `config/[{].env, .env.local`, []string{"config/{.env", ".env.local"}, false},
		{"escaped brace", `config/\{.env, .env.local`, []string{"config/{.env", ".env.local"}, true},
		{"escaped exact comma", `config/a\,b.env, .env.local`, []string{"config/a,b.env", ".env.local"}, true},
		{"escaped comma", `config/a\,b*.env, .env.local`, []string{"config/a,b.env", ".env.local"}, true},
		{"escaped directory comma", `config\,local/*.env, .env.local`, []string{"config,local/a.env", "config,local/b.env", ".env.local"}, true},
		{"escaped directory and alternation commas", `config\,local/{a\,b*,c}.env, .env.local`, []string{"config,local/a,b.env", "config,local/c.env", ".env.local"}, true},
		{"escaped directory brace and comma", `config\{\,local/*.env, .env.local`, []string{"config{,local/a.env", ".env.local"}, true},
		{"negated class", `config/[!a,b{].env, .env.local`, []string{"config/c.env", ".env.local"}, false},
		{"nested alternation", `config/{a,{b,c}}.env, .env.local`, []string{"config/a.env", "config/b.env", "config/c.env", ".env.local"}, false},
		{"escaped class close", `config/[\],{].env, .env.local`, []string{"config/].env", "config/,.env", "config/{.env", ".env.local"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.posix && runtime.GOOS == "windows" {
				t.Skip("POSIX escape syntax")
			}
			src, dst := t.TempDir(), t.TempDir()
			want := make(map[string]string, len(tc.files))
			for _, rel := range tc.files {
				want[rel] = "content:" + rel
				writeFile(t, filepath.Join(src, filepath.FromSlash(rel)), want[rel], 0o600)
			}
			if err := ValidateSpec(tc.spec); err != nil {
				t.Fatal(err)
			}
			copied, warnings, err := Copy(context.Background(), src, dst, ParseSpecs(tc.spec), nil)
			if err != nil || len(warnings) != 0 {
				t.Errorf("Copy: warnings=%v err=%v", warnings, err)
			}
			copiedBytes := make(map[string]string, len(copied))
			for _, rel := range copied {
				copiedBytes[rel] = readFile(t, filepath.Join(dst, filepath.FromSlash(rel)))
			}
			if !reflect.DeepEqual(copiedBytes, want) {
				t.Errorf("Copy = %v, want %v", copiedBytes, want)
			}
			entries, warnings, err := Plan(context.Background(), src, Parse(tc.spec), nil)
			if err != nil || len(warnings) != 0 {
				t.Errorf("Plan: warnings=%v err=%v", warnings, err)
			}
			plannedBytes := make(map[string]string, len(entries))
			for _, entry := range entries {
				plannedBytes[entry.RelPath] = string(entry.Content)
			}
			if !reflect.DeepEqual(plannedBytes, want) {
				t.Errorf("Plan = %v, want %v", plannedBytes, want)
			}
		})
	}
}

// @covers AC-WORKSPACES-COPYFILES-GLOB-TOKENIZATION-001.3
// @covers AC-WORKSPACES-COPYFILES-GLOB-TOKENIZATION-001.4
func TestCopyPlan_GlobSymlinkPrecedence(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native links require Windows privileges")
	}
	for _, tc := range []struct {
		spec        string
		wantSymlink bool
	}{
		{`config/[a,b].env:symlink, config/*.env, .env.local:symlink`, true},
		{`config/*.env, config/[a,b].env:symlink, .env.local:symlink`, false},
	} {
		t.Run(tc.spec, func(t *testing.T) {
			src, dst := t.TempDir(), t.TempDir()
			for _, rel := range []string{"config/a.env", "config/b.env", ".env.local"} {
				writeFile(t, filepath.Join(src, rel), rel, 0o600)
			}
			copied, warnings, err := Copy(context.Background(), src, dst, ParseSpecs(tc.spec), nil)
			if err != nil || len(warnings) != 0 || len(copied) != 3 {
				t.Fatalf("Copy = %v warnings=%v err=%v", copied, warnings, err)
			}
			for _, rel := range copied {
				info, err := os.Lstat(filepath.Join(dst, rel))
				if err != nil {
					t.Fatal(err)
				}
				wantSymlink := tc.wantSymlink || rel == ".env.local"
				if got := info.Mode()&os.ModeSymlink != 0; got != wantSymlink {
					t.Errorf("%s symlink=%v, want %v", rel, got, wantSymlink)
				}
				if got := readFile(t, filepath.Join(dst, rel)); got != rel {
					t.Errorf("%s content=%q", rel, got)
				}
			}
			entries, warnings, err := Plan(context.Background(), src, Parse(tc.spec), nil)
			if err != nil || len(warnings) != 0 || len(entries) != 3 {
				t.Fatalf("Plan = %v warnings=%v err=%v", entries, warnings, err)
			}
			for _, entry := range entries {
				if string(entry.Content) != entry.RelPath {
					t.Errorf("Plan content=%q for %s", entry.Content, entry.RelPath)
				}
			}
		})
	}
}

// @covers AC-WORKSPACES-COPYFILES-GLOB-TOKENIZATION-001.4
func TestValidateSpec_GlobAdjacentSuffix(t *testing.T) {
	t.Parallel()
	for _, spec := range []string{`config/[a,b].env, :symlink`, `config/[{].env, :symlink`} {
		if err := ValidateSpec(spec); err == nil {
			t.Errorf("ValidateSpec(%q) accepted missing suffix path", spec)
		}
	}
}

// @covers AC-WORKSPACES-COPYFILES-GLOB-TOKENIZATION-001.3
func TestCopyPlan_UnclosedClassKeepsFollowingEntries(t *testing.T) {
	t.Parallel()
	src, dst := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(src, ".env.local"), "LOCAL=1", 0o600)
	spec := `config/[a.env, .env.local`
	copied, warnings, err := Copy(context.Background(), src, dst, ParseSpecs(spec), nil)
	if err != nil || len(warnings) != 1 {
		t.Errorf("Copy warnings=%v err=%v", warnings, err)
	}
	if !reflect.DeepEqual(copied, []string{".env.local"}) {
		t.Errorf("Copy=%v, want .env.local", copied)
	}
	entries, warnings, err := Plan(context.Background(), src, Parse(spec), nil)
	if err != nil || len(warnings) != 1 {
		t.Errorf("Plan warnings=%v err=%v", warnings, err)
	}
	if len(entries) != 1 || entries[0].RelPath != ".env.local" || string(entries[0].Content) != "LOCAL=1" {
		t.Errorf("Plan=%v, want .env.local bytes", entries)
	}
}

// @covers AC-WORKSPACES-COPYFILES-GLOB-TOKENIZATION-001.2
func TestCopyPlan_EscapedExactCommaPreservesLiteralPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX literal backslash filename")
	}
	src, dst := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(src, `a\,b.env`), "LITERAL=1", 0o600)
	writeFile(t, filepath.Join(src, "a,b.env"), "ESCAPED=1", 0o600)
	spec := `a\,b.env`
	copied, warnings, err := Copy(context.Background(), src, dst, ParseSpecs(spec), nil)
	if err != nil || len(warnings) != 0 || !reflect.DeepEqual(copied, []string{spec}) {
		t.Fatalf("Copy=%v warnings=%v err=%v", copied, warnings, err)
	}
	if got := readFile(t, filepath.Join(dst, spec)); got != "LITERAL=1" {
		t.Errorf("Copy content=%q", got)
	}
	entries, warnings, err := Plan(context.Background(), src, Parse(spec), nil)
	if err != nil || len(warnings) != 0 || len(entries) != 1 {
		t.Fatalf("Plan=%v warnings=%v err=%v", entries, warnings, err)
	}
	if entries[0].RelPath != spec || string(entries[0].Content) != "LITERAL=1" {
		t.Errorf("Plan=%v", entries)
	}
}
