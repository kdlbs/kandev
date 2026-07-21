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
		// Windows path: drive-letter colon must not be read as the selector
		// boundary; the trailing "\\" segment + ":43-94" selector is stripped.
		{"windows absolute", `C:\Users\me\handlers.go:43-94`, `C:\Users\me\handlers.go`, 43, 52},
		// "N+0" is intentionally accepted (count 0, anchor at N) and stripped, so
		// the link still opens; rejecting it would leave the selector on the path.
		{"plus zero anchors", "main.go:50+0", "main.go", 50, 0},
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
		`C:43`,             // windows drive-relative path, not a selector
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

// TestSplitFiles covers omp reads that reference several comma-joined files in a
// single read call (e.g. "a.yaml:1-80,b.yaml:1-80"). Each file becomes its own
// entry so the UI can render a separate, openable link per file. A comma segment
// that is purely a line spec ("960-973") stays an extra range of the preceding
// file, and a comma that merely lives inside a directory name falls back to the
// single-file Split result.
func TestSplitFiles(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []File
	}{
		{"single no selector", "config.json", []File{{Path: "config.json"}}},
		{
			"single with range",
			"apps/backend/internal/sentry/handlers.go:43-94",
			[]File{{Path: "apps/backend/internal/sentry/handlers.go", StartLine: 43, LineCount: 52}},
		},
		{"single multi-range stays one file", "main.go:5-16,960-973", []File{{Path: "main.go", StartLine: 5, LineCount: 12}}},
		{
			"two files with ranges",
			"deployments/cluster-tools/values.backupprod.yaml:1-80,deployments/cluster-tools/values.au-backupprod.yaml:1-80",
			[]File{
				{Path: "deployments/cluster-tools/values.backupprod.yaml", StartLine: 1, LineCount: 80},
				{Path: "deployments/cluster-tools/values.au-backupprod.yaml", StartLine: 1, LineCount: 80},
			},
		},
		{"two bare-extension files", "a.go:1,b.go:2", []File{{Path: "a.go", StartLine: 1}, {Path: "b.go", StartLine: 2}}},
		{
			"multi-range first file then second file",
			"a.go:5-16,40-80,b.go:10",
			[]File{{Path: "a.go", StartLine: 5, LineCount: 12}, {Path: "b.go", StartLine: 10}},
		},
		{"files with mode selectors", "a.go:2-4:raw,b.go:raw", []File{{Path: "a.go", StartLine: 2, LineCount: 3}, {Path: "b.go"}}},
		{"second file without range still openable", "a/x.yaml:1-80,b/y.yaml", []File{{Path: "a/x.yaml", StartLine: 1, LineCount: 80}, {Path: "b/y.yaml"}}},
		{"comma inside directory name falls back to single", "a,b/foo.go:1-80", []File{{Path: "a,b/foo.go", StartLine: 1, LineCount: 80}}},
		{
			"hyphenated dirs two files",
			"deployments/tailscale-ingress-extras/values.prod.yaml:1-180,deployments/tailscale-ingress-extras/values.staging.yaml:1-180",
			[]File{
				{Path: "deployments/tailscale-ingress-extras/values.prod.yaml", StartLine: 1, LineCount: 180},
				{Path: "deployments/tailscale-ingress-extras/values.staging.yaml", StartLine: 1, LineCount: 180},
			},
		},
		{
			"hyphenated dirs legacy half-stripped trailing range",
			"deployments/tailscale-ingress-extras/values.prod.yaml:1-180,deployments/tailscale-ingress-extras/values.staging.yaml",
			[]File{
				{Path: "deployments/tailscale-ingress-extras/values.prod.yaml", StartLine: 1, LineCount: 180},
				{Path: "deployments/tailscale-ingress-extras/values.staging.yaml"},
			},
		},
		// A comma inside a single filename must not be split into phantom files:
		// "src/foo" is fileish only via its separator (no selector, no extension),
		// so it is not a file boundary and the whole path stays one File.
		{"comma inside filename falls back to single", "src/foo,bar.go:1-20", []File{{Path: "src/foo,bar.go", StartLine: 1, LineCount: 20}}},
		{
			"two files first with extension still splits",
			"src/a.go,src/b.go:1-20",
			[]File{{Path: "src/a.go"}, {Path: "src/b.go", StartLine: 1, LineCount: 20}},
		},
		{"extensionless first file with selector still splits", "Makefile:10,foo.go:20", []File{{Path: "Makefile", StartLine: 10}, {Path: "foo.go", StartLine: 20}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SplitFiles(tt.raw)
			if len(got) != len(tt.want) {
				t.Fatalf("SplitFiles(%q) = %+v (%d files), want %d", tt.raw, got, len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("file[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
