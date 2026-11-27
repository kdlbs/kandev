package lifecycle

import "testing"

// TestSSHSafeRelativePathRejectsEscapes covers paths that stay inside the
// project root on POSIX but climb out on Windows, where a segment is trimmed
// before it resolves.
func TestSSHSafeRelativePathRejectsEscapes(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "plain relative path", value: "skills/review/SKILL.md", want: true},
		{name: "leading space before parent", value: " ../outside.txt", want: false},
		{name: "space-padded parent segment", value: "skills/ .. /outside.txt", want: false},
		{name: "tab-padded parent segment", value: "skills/\t..\t/outside.txt", want: false},
		{name: "plain parent", value: "../outside.txt", want: false},
		{name: "absolute path", value: "/etc/passwd", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, got := sshSafeRelativePath(tt.value)
			if got != tt.want {
				t.Fatalf("sshSafeRelativePath(%q) ok = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}
