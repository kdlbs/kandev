package lifecycle

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle/skill"
)

func TestSSHWindowsPathClassificationAndCleaning(t *testing.T) {
	tests := []struct {
		raw       string
		wantWin   bool
		wantClean string
		wantSFTP  string
	}{
		{
			raw:       `c:/Users/Kandev User/.kandev/../.kandev/tasks/task-1/`,
			wantWin:   true,
			wantClean: `C:\Users\Kandev User\.kandev\tasks\task-1`,
			wantSFTP:  `/C:/Users/Kandev User/.kandev/tasks/task-1`,
		},
		{
			raw:       `/C:/Users/kandev/bin/agentctl.exe`,
			wantWin:   true,
			wantClean: `C:\Users\kandev\bin\agentctl.exe`,
			wantSFTP:  `/C:/Users/kandev/bin/agentctl.exe`,
		},
		{
			raw:       `C:\`,
			wantWin:   true,
			wantClean: `C:\`,
			wantSFTP:  `/C:/`,
		},
		{
			raw:       `\\buildbox\share\kandev\..\tasks\task-9`,
			wantWin:   true,
			wantClean: `\\buildbox\share\tasks\task-9`,
			wantSFTP:  `//buildbox/share/tasks/task-9`,
		},
		{
			raw:       `/home/kandev/.kandev/tasks/task-1`,
			wantWin:   false,
			wantClean: "",
			wantSFTP:  `/home/kandev/.kandev/tasks/task-1`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			if got := isWindowsRemotePath(tc.raw); got != tc.wantWin {
				t.Fatalf("isWindowsRemotePath(%q) = %v, want %v", tc.raw, got, tc.wantWin)
			}
			if tc.wantWin {
				if got := cleanWindowsRemotePath(tc.raw); got != tc.wantClean {
					t.Fatalf("cleanWindowsRemotePath(%q) = %q, want %q", tc.raw, got, tc.wantClean)
				}
			}
			if got := toSFTPPath(tc.raw); got != tc.wantSFTP {
				t.Fatalf("toSFTPPath(%q) = %q, want %q", tc.raw, got, tc.wantSFTP)
			}
		})
	}
}

func TestSSHWindowsPathComparisonsAndDriveRoots(t *testing.T) {
	winPlatform := SSHRemotePlatform{GOOS: "windows", GOARCH: "amd64"}
	if !sshRemotePathsEqual(`c:/users/kandev user/.kandev/tasks/task-1/`, `C:\Users\Kandev User\.kandev\tasks\task-1`, winPlatform) {
		t.Fatal("expected Git slash/lowercase Windows path to match PowerShell canonical path")
	}
	if sshRemotePathsEqual(`/Users/KandevUser/task`, `/users/kandevuser/task`, SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}) {
		t.Fatal("POSIX paths must remain case-sensitive")
	}
	for _, root := range []string{"/C:", "/C:/", "c:", "D:/"} {
		if !isSFTPWindowsDriveRoot(root) {
			t.Fatalf("expected %q to be recognized as SFTP Windows drive root", root)
		}
	}
	for _, nonRoot := range []string{"/C:/Users", "/home", "/"} {
		if isSFTPWindowsDriveRoot(nonRoot) {
			t.Fatalf("did not expect %q to be recognized as SFTP Windows drive root", nonRoot)
		}
	}
}

func TestSSHWindowsQuoteArgRoundTrip(t *testing.T) {
	quoted := quoteWindowsArg(`C:\Users\Kandev User\dir with "quote"\`)
	wantQuoted := `"C:\Users\Kandev User\dir with \"quote\"\\"`
	if quoted != wantQuoted {
		t.Fatalf("quoteWindowsArg = %s, want %s", quoted, wantQuoted)
	}
	taskDir := `C:\Users\Kandev User\tasks\task 1`
	if workdir, ok := windowsAgentctlWorkdir(`agentctl.exe --workdir ` + quoteWindowsArg(taskDir)); !ok || workdir != taskDir {
		t.Fatalf("windowsAgentctlWorkdir round trip = %q, %v; want %q", workdir, ok, taskDir)
	}
}

func TestSSHWindowsCommandLineFlagExtractionAndMatching(t *testing.T) {
	// Exact Win32_Process.CommandLine of agentctl.exe started by the WMI cmd.exe
	// wrapper on a Windows 11 host: two spaces before --workdir and a trailing
	// space left by the stripped redirection.
	realLaunch := `"C:\Users\Kandev User\AppData\Local\Temp\kandev wo4\bin\agentctl.exe"  --workdir "C:\Users\Kandev User\AppData\Local\Temp\kandev wo4\tasks\task 1" `
	if !remoteWindowsAgentctlCommandLineMatches(realLaunch, `C:\Users\Kandev User\AppData\Local\Temp\kandev wo4\tasks\task 1`) {
		t.Fatal("identity check rejected the command line of a WMI-launched agentctl.exe")
	}
	taskDir := `C:\Users\Kandev User\.kandev\tasks\task-1`
	cases := []struct {
		name    string
		cmdline string
		want    bool
	}{
		{
			name:    "quoted path with spaces",
			cmdline: `"C:\Users\Kandev User\.kandev\bin\agentctl.exe" --workdir "C:\Users\Kandev User\.kandev\tasks\task-1"`,
			want:    true,
		},
		{
			name:    "drive letter case and separators normalize",
			cmdline: `"C:\kandev\agentctl.exe" --workdir "c:/users/kandev user/.kandev/tasks/task-1"`,
			want:    true,
		},
		{
			name:    "sibling prefix must not match",
			cmdline: `"C:\Users\Kandev User\.kandev\bin\agentctl.exe" --workdir "C:\Users\Kandev User\.kandev\tasks\task-10"`,
			want:    false,
		},
		{
			name:    "no quoted workdir",
			cmdline: `"C:\Users\Kandev User\.kandev\bin\agentctl.exe" --workdir=C:\Users\Kandev User\.kandev\tasks\task-1`,
			want:    false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := remoteWindowsAgentctlCommandLineMatches(tc.cmdline, taskDir); got != tc.want {
				t.Fatalf("remoteWindowsAgentctlCommandLineMatches(%q) = %v, want %v", tc.cmdline, got, tc.want)
			}
		})
	}
}

type recordingSkillWorkspace struct {
	written map[string]string
	removed []string
}

func (w *recordingSkillWorkspace) WriteFile(_ context.Context, target string, content []byte, _ os.FileMode) error {
	if w.written == nil {
		w.written = make(map[string]string)
	}
	w.written[target] = string(content)
	return nil
}

func (w *recordingSkillWorkspace) RemoveAll(_ context.Context, target string) error {
	w.removed = append(w.removed, target)
	return nil
}

// Skill files stay under the Windows task directory. Paths keep the Windows form
// until the SFTP boundary, so the leading \\ of a UNC share is never collapsed.
func TestSSHWindowsMaterializeSkillManifestTaskDir(t *testing.T) {
	manifestBytes, err := json.Marshal(skill.Manifest{
		ProjectSkillDir: ".claude/skills",
		Skills:          []skill.Skill{{Slug: "win-deploy", Content: "# Deploy skill\n"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, taskDir := range []string{`C:\Users\Kandev User\.kandev\tasks\task-1`, `\\srv\share\kandev\tasks\task-1`} {
		ws := &recordingSkillWorkspace{}
		if err := materializeSSHSkillManifest(context.Background(), ws, taskDir, string(manifestBytes)); err != nil {
			t.Fatalf("materializeSSHSkillManifest(%q): %v", taskDir, err)
		}
		if got := ws.written[taskDir+`\.claude\skills\kandev-win-deploy\SKILL.md`]; got != "# Deploy skill\n" {
			t.Errorf("skill under %q = %q, written %#v", taskDir, got, ws.written)
		}
	}
}

func TestSSHWindowsCreateInstanceRequestJournalPathSeparator(t *testing.T) {
	req := &ExecutorCreateRequest{
		InstanceID:            "inst-1",
		SessionID:             "sess-1",
		TaskID:                "task-1",
		DurableJournalOwnerID: "owner-1",
	}
	winCreate := buildSSHCreateInstanceRequest(req, `C:\Users\Kandev User\.kandev\tasks\task-1`, `C:\Users\Kandev User\.kandev\bin\agentctl.exe`)
	wantWinJournal := `C:\Users\Kandev User\.kandev\tasks\task-1\.kandev\agentctl-journals\owner-1\delivery.bbolt`
	if winCreate.DurableJournalPath != wantWinJournal {
		t.Fatalf("Windows DurableJournalPath = %q, want %q", winCreate.DurableJournalPath, wantWinJournal)
	}
}

// A root that is neither a drive, UNC, nor home-relative path would become a
// relative directory under the SSH working directory, so it fails first.
// @covers AC-EXECUTORS-SSH-WINDOWS-001.7
func TestSSHWindowsTaskDirRejectsNonWindowsWorkdirRoot(t *testing.T) {
	for _, root := range []string{"/srv/kandev", "kandev", `D:kandev`} {
		server := newFakeSSHServer(t, func(string, string) sshExecResult {
			t.Errorf("root %q must be rejected before any remote command", root)
			return sshOK
		})
		_, err := ensureWindowsRemoteTaskDir(context.Background(), server.dial(t), root, "task-1")
		if err == nil || !strings.Contains(err.Error(), root) {
			t.Errorf("ensureWindowsRemoteTaskDir(%q) error = %v, want a rejection naming the root", root, err)
		}
	}
}
