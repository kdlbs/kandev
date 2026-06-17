package readselector

import "testing"

func TestSplit(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantPath  string
		wantStart int
		wantCount int
	}{
		{"no selector", "config.json", "config.json", 0, 0},
		{"single line", "apps/web/lib/utils.ts:50", "apps/web/lib/utils.ts", 50, 0},
		{"open ended", "main.go:50-", "main.go", 50, 0},
		{"closed range", "apps/backend/internal/sentry/handlers.go:43-94", "apps/backend/internal/sentry/handlers.go", 43, 52},
		{"plus span", "main.go:50+150", "main.go", 50, 150},
		{"anchor", "main.go:20+1", "main.go", 20, 1},
		{"multi range", "main.go:5-16,960-973", "main.go", 5, 12},
		{"raw mode", "main.go:raw", "main.go", 0, 0},
		{"conflicts mode", "main.go:conflicts", "main.go", 0, 0},
		{"range then raw", "main.go:2-4:raw", "main.go", 2, 3},
		{"raw then range", "main.go:raw:2-4", "main.go", 2, 3},
		{"absolute path", "/home/u/.kandev/x/handlers.go:43-94", "/home/u/.kandev/x/handlers.go", 43, 52},
		{"absolute external", "/home/clem/.claude/CLAUDE.md:93-113", "/home/clem/.claude/CLAUDE.md", 93, 21},
		{"no extension single line", "Makefile:10", "Makefile", 10, 0},
		// Real OMP reads observed in the wild (multi-range, zero-length range,
		// range+mode combo, and a "~"-prefixed home path).
		{"omp multi range zero-len", "x/README.md:16-20,32-40,69-69,85-96", "x/README.md", 16, 5},
		{"omp range plus raw combo", "x/README.md:69+1:raw", "x/README.md", 69, 1},
		{"omp tilde multi range", "~/.kandev/x/README.md:27-28,35-36,66-66", "~/.kandev/x/README.md", 27, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, start, count := Split(tt.raw)
			if path != tt.wantPath {
				t.Errorf("path = %q, want %q", path, tt.wantPath)
			}
			if start != tt.wantStart {
				t.Errorf("startLine = %d, want %d", start, tt.wantStart)
			}
			if count != tt.wantCount {
				t.Errorf("lineCount = %d, want %d", count, tt.wantCount)
			}
		})
	}
}

// TestSplit_NoFalsePositives verifies that paths whose colon suffix is not a
// valid omp selector are returned untouched, so legitimate filenames and
// non-omp agent paths never lose data.
func TestSplit_NoFalsePositives(t *testing.T) {
	unchanged := []string{
		"",
		"src/file.ts",
		"foo.go:bar",       // non-numeric, non-keyword suffix
		"foo.go:1.2",       // float-like, not a line spec
		`C:\Users\me\a.go`, // windows drive letter: suffix is not a line spec
		"dir:1/file.go",    // colon lives in a directory component, not the file
		":43-94",           // empty base
		"main.go:",         // empty selector
		"main.go:-5",       // negative / malformed start
		"main.go:0",        // zero is not a valid 1-based line
		"main.go:10-5",     // descending range
		"main.go:5,abc",    // one invalid segment invalidates the list
	}
	for _, raw := range unchanged {
		t.Run(raw, func(t *testing.T) {
			path, start, count := Split(raw)
			if path != raw || start != 0 || count != 0 {
				t.Errorf("Split(%q) = (%q, %d, %d), want (%q, 0, 0)", raw, path, start, count, raw)
			}
		})
	}
}
