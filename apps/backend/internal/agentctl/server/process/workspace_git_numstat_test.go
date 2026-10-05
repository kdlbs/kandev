package process

import (
	"reflect"
	"testing"
)

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.36
func TestParseWorkspaceNumstatZ(t *testing.T) {
	for _, path := range []string{
		"plain.txt", "café.txt", "literal => name.txt", "dir/{old => new}/file.txt",
		" leading.txt", "trailing.txt ", "tab\tname.txt", "line\nname.txt",
		"quote\"name.txt", "back\\slash.txt", "raw-\xff.txt",
	} {
		t.Run(path, func(t *testing.T) {
			input := "2\t1\t" + path + "\x00"
			entry, rest, ok := parseWorkspaceNumstatZ(input)
			if !ok || entry != (numstatEntry{path: path, additions: 2, deletions: 1}) || rest != "" {
				t.Fatalf("parse(%q) = %+v, %q, %v", input, entry, rest, ok)
			}
		})
	}
	tests := []struct {
		name  string
		input string
		want  []numstatEntry
	}{
		{"empty", "", nil},
		{"adjacent renames and ordinary", "1\t0\t\x00old.txt\x00new.txt\x002\t3\t\x00dir/old/f.txt\x00dir/new/f.txt\x004\t5\tnext.txt\x00", []numstatEntry{
			{path: "new.txt", additions: 1}, {path: "dir/new/f.txt", additions: 2, deletions: 3},
			{path: "next.txt", additions: 4, deletions: 5},
		}},
		{"rename destination is literal syntax", "0\t0\t\x00old\x00{literal => dest}\t\n \x00", []numstatEntry{{path: "{literal => dest}\t\n "}}},
		{"binary and unchanged", "-\t-\tbinary.bin\x000\t0\tmode.txt\x000\t0\t\x00old\x00new\x00", []numstatEntry{
			{path: "binary.bin"}, {path: "mode.txt"}, {path: "new"},
		}},
		{"unterminated ordinary", "1\t0\tfile.txt", nil},
		{"missing columns", "file.txt\x00", nil},
		{"missing rename origin", "1\t0\t\x00", nil},
		{"missing rename destination", "1\t0\t\x00old\x00", nil},
		{"unterminated rename destination", "1\t0\t\x00old\x00new", nil},
		{"empty rename destination", "1\t0\t\x00old\x00\x00", nil},
		{"complete prefix then incomplete rename", "1\t0\tgood\x002\t0\t\x00old\x00", []numstatEntry{{path: "good", additions: 1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []numstatEntry
			for output := tt.input; output != ""; {
				entry, rest, ok := parseWorkspaceNumstatZ(output)
				if !ok {
					break
				}
				if len(rest) >= len(output) {
					t.Fatal("parser did not consume a complete record")
				}
				got = append(got, entry)
				output = rest
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("entries = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.33, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.36
func TestParseWorkspaceNumstatZRejectsInvalidCounts(t *testing.T) {
	for _, counts := range []string{
		"bad\t0", "0\tbad", "\t0", "0\t", "-\t0", "0\t-",
		"-1\t0", "0\t-1", "999999999999999999999999\t0", "0\t999999999999999999999999",
		"+1\t0", " 1\t0", "0\t1 ",
	} {
		t.Run(counts, func(t *testing.T) {
			for _, path := range []string{"file.txt\x00", "\x00old\x00new\x00"} {
				if entry, rest, ok := parseWorkspaceNumstatZ(counts + "\t" + path); ok {
					t.Errorf("invalid counts accepted: %+v, rest %q", entry, rest)
				}
			}
		})
	}
}
