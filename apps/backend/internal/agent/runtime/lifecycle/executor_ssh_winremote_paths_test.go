package lifecycle

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle/skill"
	"github.com/kandev/kandev/internal/githubauth"
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

func TestSSHWindowsQuoteArgAndCredentialHelper(t *testing.T) {
	quoted := quoteWindowsArg(`C:\Users\Kandev User\dir with "quote"\`)
	wantQuoted := `"C:\Users\Kandev User\dir with \"quote\"\\"`
	if quoted != wantQuoted {
		t.Fatalf("quoteWindowsArg = %s, want %s", quoted, wantQuoted)
	}
	tokens := splitWindowsCommandLine(`agentctl.exe --workdir ` + quoted)
	if len(tokens) != 3 || tokens[2] != `C:\Users\Kandev User\dir with "quote"\` {
		t.Fatalf("round-trip splitWindowsCommandLine = %#v", tokens)
	}

	req := &ExecutorCreateRequest{Env: map[string]string{
		envKeyGitHubCredentialBrokerURL: "https://kandev.example/api/v1/github/credentials/resolve",
		envKeyGitHubCredentialLease:     "lease",
		"GIT_CONFIG_COUNT":              "1",
		"GIT_CONFIG_KEY_0":              "credential.https://github.com.helper",
		"GIT_CONFIG_VALUE_0":            githubauth.ManagedGitCredentialHelper,
	}}
	got := sshRemoteContributionEnv(req, `C:\Users\Kandev User\.kandev\bin\agentctl.exe`)
	wantHelper := `!'C:/Users/Kandev User/.kandev/bin/agentctl.exe' git-credential`
	if got["GIT_CONFIG_VALUE_0"] != wantHelper {
		t.Fatalf("Windows git credential helper = %q, want %q", got["GIT_CONFIG_VALUE_0"], wantHelper)
	}
}

func TestSSHWindowsCommandLineFlagExtractionAndMatching(t *testing.T) {
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
			name:    "equals syntax and forward slashes",
			cmdline: `C:\kandev\agentctl.exe --workdir="c:/users/kandev user/.kandev/tasks/task-1"`,
			want:    true,
		},
		{
			name:    "sibling prefix must not match",
			cmdline: `"C:\Users\Kandev User\.kandev\bin\agentctl.exe" --workdir "C:\Users\Kandev User\.kandev\tasks\task-10"`,
			want:    false,
		},
		{
			name:    "unrelated executable must not match",
			cmdline: `"C:\Windows\System32\notepad.exe" --workdir "C:\Users\Kandev User\.kandev\tasks\task-1"`,
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

func TestSSHWindowsMaterializeSkillManifestTaskDir(t *testing.T) {
	manifestBytes, err := json.Marshal(skill.Manifest{
		ProjectSkillDir: ".claude/skills",
		Skills: []skill.Skill{
			{Slug: "win-deploy", Content: "# Deploy skill\n"},
		},
	})
	if err != nil {
		t.Fatalf("marshal skill manifest: %v", err)
	}
	ws := &recordingSkillWorkspace{}
	err = materializeSSHSkillManifest(
		context.Background(),
		ws,
		`C:\Users\Kandev User\.kandev\tasks\task-1`,
		string(manifestBytes),
	)
	if err != nil {
		t.Fatalf("materializeSSHSkillManifest with Windows taskDir: %v", err)
	}
	var sawSkill bool
	for p, body := range ws.written {
		if strings.HasPrefix(p, "/C:/Users/Kandev User/.kandev/tasks/task-1/.claude/skills/") && strings.HasSuffix(p, "/SKILL.md") {
			sawSkill = body == "# Deploy skill\n"
		}
	}
	if !sawSkill {
		t.Fatalf("expected Windows SFTP skill write under /C:/..., got %#v", ws.written)
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

	posixCreate := buildSSHCreateInstanceRequest(req, "/home/kandev/.kandev/tasks/task-1", "/home/kandev/.kandev/bin/agentctl")
	wantPosixJournal := "/home/kandev/.kandev/tasks/task-1/.kandev/agentctl-journals/owner-1/delivery.bbolt"
	if posixCreate.DurableJournalPath != wantPosixJournal {
		t.Fatalf("POSIX DurableJournalPath = %q, want %q", posixCreate.DurableJournalPath, wantPosixJournal)
	}
}
