//go:build linux || darwin

package subproc

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// goldenGit is what one managed Git helper handed to the git executable.
type goldenGit struct {
	Args  []string
	Env   []string
	Count int
}

// captureGoldenGit runs one representative command through each helper class
// against a recording git executable and returns its argument vector,
// environment and process count.
func captureGoldenGit(t *testing.T) map[string]goldenGit {
	t.Helper()
	bin := t.TempDir()
	work := t.TempDir()
	script := "#!/bin/sh\n" +
		"{ echo ARGS; for a in \"$@\"; do echo \"$a\"; done; echo ENV; env | sort; echo END; } >> \"$KANDEV_GOLDEN_LOG\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o700); err != nil {
		t.Fatalf("write recording git: %v", err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	classes := map[string]GitWorkClass{
		"interactive": GitInteractive,
		"lifecycle":   GitLifecycle,
		"background":  GitBackground,
	}
	got := make(map[string]goldenGit, len(classes))
	for name, class := range classes {
		logPath := filepath.Join(t.TempDir(), "calls.log")
		cmd := NewGitCommand(context.Background(), "status", "--porcelain")
		cmd.Dir = work
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + bin, "KANDEV_GOLDEN_LOG=" + logPath}
		if err := RunGitClass(context.Background(), class, cmd); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		raw, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("%s: read call log: %v", name, err)
		}
		got[name] = parseGoldenLog(string(raw), bin, work)
	}
	return got
}

func parseGoldenLog(raw, bin, work string) goldenGit {
	raw = strings.ReplaceAll(raw, bin, "<BIN>")
	raw = strings.ReplaceAll(raw, work, "<WORK>")
	var out goldenGit
	section := ""
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		switch line {
		case "ARGS", "ENV":
			section = line
			continue
		case "END":
			out.Count++
			continue
		}
		if out.Count > 0 {
			continue
		}
		if section == "ARGS" {
			out.Args = append(out.Args, line)
		} else if !goldenEnvNoise(line) {
			out.Env = append(out.Env, line)
		}
	}
	sort.Strings(out.Env)
	return out
}

func goldenEnvNoise(line string) bool {
	for _, prefix := range []string{"PATH=", "PWD=", "OLDPWD=", "SHLVL=", "_=", "KANDEV_GOLDEN_LOG="} {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}
