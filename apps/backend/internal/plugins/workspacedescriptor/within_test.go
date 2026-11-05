package workspacedescriptor

import "testing"

func TestWithinAcceptsPathsInsideTheWorkspaceRoot(t *testing.T) {
	cases := []struct {
		name, platform, root, path string
		want                       bool
	}{
		{"absolute child", "linux/amd64", "/work/env", "/work/env/repo", true},
		{"absolute nested", "linux/amd64", "/work/env", "/work/env/a/b", true},
		{"root itself", "linux/amd64", "/work/env", "/work/env", true},
		{"relative child", "linux/amd64", "/work/env", "repo", true},
		{"relative nested", "linux/amd64", "/work/env", "a/b", true},
		{"sibling with shared prefix", "linux/amd64", "/work/env", "/work/env2/repo", false},
		{"other tree", "linux/amd64", "/work/env", "/etc", false},
		{"parent segment escape", "linux/amd64", "/work/env", "../env2", false},
		{"embedded parent segment", "linux/amd64", "/work/env", "/work/env/../env2", false},
		{"empty path", "linux/amd64", "/work/env", "", false},
		{"control character", "linux/amd64", "/work/env", "/work/env/a\x00b", false},
		{"windows child", "windows/amd64", `C:\work\env`, `C:\work\env\repo`, true},
		{"windows case-insensitive", "windows/amd64", `C:\work\env`, `c:\WORK\ENV\repo`, true},
		{"windows relative", "windows/amd64", `C:\work\env`, `repo\sub`, true},
		{"windows other drive", "windows/amd64", `C:\work\env`, `D:\work\env\repo`, false},
		{"windows unc", "windows/amd64", `C:\work\env`, `\\host\share`, false},
		{"invalid root", "linux/amd64", "work", "work/repo", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Within(tc.platform, tc.root, tc.path); got != tc.want {
				t.Fatalf("Within(%q, %q, %q) = %v, want %v", tc.platform, tc.root, tc.path, got, tc.want)
			}
		})
	}
}
