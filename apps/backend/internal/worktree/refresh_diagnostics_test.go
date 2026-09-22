package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.14
func TestRefreshDiagnosticClassification(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		runErr     error
		contextErr error
		wantCode   string
		wantDetail string
		wantNoText string
	}{
		{name: "SSH public key", output: "Permission denied (publickey).", runErr: errors.New("exit status 128"), wantCode: refreshDiagnosticSSHKey, wantDetail: "public key"},
		{name: "SSH host key", output: "Host key verification failed.", runErr: errors.New("exit status 128"), wantCode: refreshDiagnosticSSHHost, wantDetail: "host"},
		{name: "DNS", output: "Could not resolve host: example.invalid", runErr: errors.New("exit status 128"), wantCode: refreshDiagnosticDNS, wantDetail: "host name"},
		{name: "connection", output: "Connection refused", runErr: errors.New("exit status 128"), wantCode: refreshDiagnosticConnection, wantDetail: "connection"},
		{name: "TLS", output: "SSL certificate problem: unable to get local issuer certificate", runErr: errors.New("exit status 128"), wantCode: refreshDiagnosticTLS, wantDetail: "TLS"},
		{name: "missing ref", output: "couldn't find remote ref feature/missing", runErr: errors.New("exit status 128"), wantCode: refreshDiagnosticMissingRef, wantDetail: "remote branch"},
		{name: "non-fast-forward", output: "rejected: non-fast-forward", runErr: errors.New("exit status 128"), wantCode: refreshDiagnosticNonFastForward, wantDetail: "non-fast-forward"},
		{name: "pull fast-forward refusal", output: "fatal: Not possible to fast-forward, aborting.", runErr: errors.New("exit status 128"), wantCode: refreshDiagnosticNonFastForward, wantDetail: "local branch", wantNoText: "remote"},
		{name: "tag clobber is not branch refusal", output: "! [rejected] v1 -> v1 (would clobber existing tag)", runErr: errors.New("exit status 128"), wantCode: refreshDiagnosticUnknown, wantDetail: "unknown reason"},
		{name: "repository access", output: "Repository not found.", runErr: errors.New("exit status 128"), wantCode: refreshDiagnosticRepository, wantDetail: "accessed"},
		{name: "lock", output: "fatal: Unable to create '.git/index.lock': File exists.", runErr: errors.New("exit status 128"), wantCode: refreshDiagnosticLock, wantDetail: "repository lock"},
		{name: "generic lock", output: "fatal: unable to create '.git/refs/heads/main.lock': File exists.", runErr: errors.New("exit status 128"), wantCode: refreshDiagnosticLock, wantDetail: "repository lock"},
		{name: "authentication", output: "terminal prompts disabled", runErr: errors.New("exit status 128"), wantCode: refreshDiagnosticAuthentication, wantDetail: "authenticate"},
		{name: "unknown", output: "unexpected backend detail: token-secret", runErr: errors.New("exit status 128"), wantCode: refreshDiagnosticUnknown, wantDetail: "unknown reason", wantNoText: "token-secret"},
		{name: "timeout wins", output: "Permission denied (publickey)", runErr: context.DeadlineExceeded, contextErr: context.DeadlineExceeded, wantCode: refreshDiagnosticTimeout, wantDetail: "timed out"},
		{name: "cancellation wins", output: "Permission denied (publickey)", runErr: context.Canceled, contextErr: context.Canceled, wantCode: refreshDiagnosticCanceled, wantDetail: "canceled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyRefreshDiagnostic(tt.output, tt.runErr, tt.contextErr)
			if got.code != tt.wantCode {
				t.Fatalf("diagnostic code = %q, want %q", got.code, tt.wantCode)
			}
			if !strings.Contains(strings.ToLower(got.detail), strings.ToLower(tt.wantDetail)) {
				t.Fatalf("diagnostic detail = %q, want it to contain %q", got.detail, tt.wantDetail)
			}
			if tt.wantNoText != "" && strings.Contains(got.detail, tt.wantNoText) {
				t.Fatalf("diagnostic detail contains raw output %q: %q", tt.wantNoText, got.detail)
			}
		})
	}
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.14
func TestBaseRefreshDiagnosticEmission(t *testing.T) {
	tests := []struct {
		name          string
		gitScript     string
		wantRef       string
		wantCode      string
		wantOperation string
		secret        string
	}{
		{
			name: "fetch authentication failure",
			gitScript: `
case "${1:-}" in
  fetch)
    echo "fatal: could not read Username for 'https://token:fetch-secret@example.invalid/repo.git': terminal prompts disabled" >&2
    exit 128
    ;;
  rev-parse)
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`,
			wantRef:       "main",
			wantCode:      "authentication_failed",
			wantOperation: "fetch",
			secret:        "fetch-secret",
		},
		{
			name: "fetch non-fast-forward update",
			gitScript: `
case "${1:-}" in
  rev-parse)
    exit 0
    ;;
  fetch)
    echo "From https://example.invalid/repo.git" >&2
    echo " ! [rejected]        main -> main (non-fast-forward)" >&2
    echo "error: some local refs could not be updated" >&2
    exit 128
    ;;
  *)
    exit 0
    ;;
esac
`,
			wantRef:       "main",
			wantCode:      "non_fast_forward",
			wantOperation: "fetch",
		},
		{
			name: "pull TLS failure",
			gitScript: `
case "${1:-}" in
  fetch)
    exit 0
    ;;
  pull)
    echo "fatal: unable to access 'https://token:pull-secret@example.invalid/repo.git': SSL certificate problem" >&2
    exit 128
    ;;
  rev-parse)
    if [ "${2:-}" = "--abbrev-ref" ]; then
      echo "main"
    fi
    exit 0
    ;;
  merge-base)
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`,
			wantRef:       "origin/main",
			wantCode:      "tls_verification_failed",
			wantOperation: "pull",
			secret:        "pull-secret",
		},
		{
			name: "pull fast-forward refusal",
			gitScript: `
case "${1:-}" in
  fetch)
    exit 0
    ;;
  pull)
    echo "fatal: Not possible to fast-forward, aborting." >&2
    exit 128
    ;;
  rev-parse)
    if [ "${2:-}" = "--abbrev-ref" ]; then
      echo "main"
    fi
    exit 0
    ;;
  merge-base)
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`,
			wantRef:       "origin/main",
			wantCode:      "non_fast_forward",
			wantOperation: "pull",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scriptDir := writeFakeGitScript(t, tt.gitScript)
			t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

			log, logs := newObservedWorktreeLogger(t)
			mgr, err := NewManager(newTestConfig(t), newMockStore(), log)
			if err != nil {
				t.Fatalf("NewManager() error = %v", err)
			}

			ref, err := mgr.pullBaseBranch(context.Background(), t.TempDir(), "main", nil)
			if err != nil {
				t.Fatalf("pullBaseBranch() error = %v", err)
			}
			if ref != tt.wantRef {
				t.Fatalf("pullBaseBranch() ref = %q, want %q", ref, tt.wantRef)
			}

			entries := logs.FilterMessage("git refresh failed").All()
			if len(entries) != 1 {
				t.Fatalf("observed %d refresh diagnostics, want 1; logs = %v", len(entries), logs.All())
			}
			fields := entries[0].ContextMap()
			if fields["operation"] != tt.wantOperation {
				t.Fatalf("operation = %v, want %q", fields["operation"], tt.wantOperation)
			}
			if fields["branch"] != "main" {
				t.Fatalf("branch = %v, want main", fields["branch"])
			}
			if fields["diagnostic_code"] != tt.wantCode {
				t.Fatalf("diagnostic_code = %v, want %q", fields["diagnostic_code"], tt.wantCode)
			}
			if fields["detail"] == "" {
				t.Fatal("diagnostic detail is empty")
			}
			for _, entry := range logs.All() {
				if strings.Contains(entry.Message, tt.secret) || strings.Contains(entry.Message, "https://token:") {
					t.Fatalf("log message contains secret material: %q", entry.Message)
				}
				for key, value := range entry.ContextMap() {
					if strings.Contains(key, tt.secret) || strings.Contains(key, "https://token:") || strings.Contains(fmt.Sprint(value), tt.secret) {
						t.Fatalf("log field %q contains secret material: %v", key, value)
					}
				}
			}
		})
	}
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.14
func TestRefreshDiagnosticsPreservePolicy(t *testing.T) {
	scriptDir := writeFakeGitScript(t, `
case "${1:-}" in
  rev-parse)
    exit 1
    ;;
  fetch)
    echo "fatal: unexpected backend detail token-secret" >&2
    exit 128
    ;;
  *)
    exit 0
    ;;
esac
`)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	log, logs := newObservedWorktreeLogger(t)
	mgr, err := NewManager(newTestConfig(t), newMockStore(), log)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	var events []SyncProgressEvent
	_, _, err = mgr.pullBaseBranchWithPolicy(
		context.Background(), t.TempDir(), "main", "", true, captureSyncProgress(&events),
	)
	if !errors.Is(err, ErrGitCommandFailed) {
		t.Fatalf("pullBaseBranchWithPolicy() error = %v, want ErrGitCommandFailed", err)
	}
	if strings.Contains(err.Error(), "token-secret") {
		t.Fatalf("returned error exposed raw Git output: %v", err)
	}
	if len(events) != 2 || events[1].Status != SyncProgressFailed {
		t.Fatalf("sync progress = %#v, want running then failed", events)
	}
	if events[1].Error != "git_command_failed" || strings.Contains(events[1].Output, "token-secret") {
		t.Fatalf("sync progress exposed raw failure details: %#v", events[1])
	}

	entries := logs.FilterMessage("git refresh failed").All()
	if len(entries) != 1 {
		t.Fatalf("observed %d refresh diagnostics, want 1; logs = %v", len(entries), logs.All())
	}
	fields := entries[0].ContextMap()
	if fields["operation"] != "fetch" || fields["branch"] != "main" || fields["reason"] != "git_command_failed" {
		t.Fatalf("diagnostic fields = %v, want fetch/main/git_command_failed", fields)
	}
	if fields["diagnostic_code"] != refreshDiagnosticUnknown {
		t.Fatalf("diagnostic_code = %v, want %q", fields["diagnostic_code"], refreshDiagnosticUnknown)
	}
	for _, entry := range logs.All() {
		if strings.Contains(entry.Message, "token-secret") || strings.Contains(fmt.Sprint(entry.ContextMap()), "token-secret") {
			t.Fatalf("log exposed raw Git output: %#v", entry)
		}
	}

	t.Run("configured fallback failure", func(t *testing.T) {
		scriptDir := writeFakeGitScript(t, `
case "${1:-}" in
  rev-parse)
    exit 1
    ;;
  fetch)
    case "${4:-}" in
      feature/missing)
        echo "fatal: couldn't find remote ref feature/missing" >&2
        ;;
      main)
        echo "fatal: could not read Username for 'https://token:fallback-secret@example.invalid/repo.git': terminal prompts disabled" >&2
        ;;
    esac
    exit 128
    ;;
  *)
    exit 0
    ;;
esac
`)
		t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		log, logs := newObservedWorktreeLogger(t)
		mgr, err := NewManager(newTestConfig(t), newMockStore(), log)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}
		var events []SyncProgressEvent
		_, _, err = mgr.pullBaseBranchWithFallback(
			context.Background(), t.TempDir(), "feature/missing", "main", captureSyncProgress(&events),
		)
		if !errors.Is(err, ErrAuthFailed) {
			t.Fatalf("pullBaseBranchWithFallback() error = %v, want ErrAuthFailed", err)
		}
		if strings.Contains(err.Error(), "fallback-secret") || strings.Contains(fmt.Sprint(events), "fallback-secret") {
			t.Fatalf("fallback failure exposed raw Git output: error=%v events=%#v", err, events)
		}
		entries := logs.FilterMessage("git refresh failed").All()
		if len(entries) != 2 {
			t.Fatalf("observed %d refresh diagnostics, want primary and fallback failures", len(entries))
		}
		if entries[0].ContextMap()["branch"] != "feature/missing" || entries[0].ContextMap()["diagnostic_code"] != refreshDiagnosticMissingRef {
			t.Fatalf("primary diagnostic fields = %v", entries[0].ContextMap())
		}
		if entries[1].ContextMap()["branch"] != "main" || entries[1].ContextMap()["diagnostic_code"] != refreshDiagnosticAuthentication {
			t.Fatalf("fallback diagnostic fields = %v", entries[1].ContextMap())
		}
		for _, entry := range logs.All() {
			if strings.Contains(fmt.Sprint(entry.ContextMap()), "fallback-secret") {
				t.Fatalf("fallback diagnostic exposed raw Git output: %#v", entry)
			}
		}
	})
}
