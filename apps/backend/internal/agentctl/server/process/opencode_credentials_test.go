package process

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/opencode"
	tools "github.com/kandev/kandev/internal/tools/installer"
)

func TestOpenCodeCredentialCommandRetriesTransientNPMFailureOnline(t *testing.T) {
	runtime := opencode.Runtime{Family: opencode.FamilyV2, Source: opencode.SourceManaged, Version: "2.1.2"}
	args, err := runtime.CredentialCommand("export")
	if err != nil {
		t.Fatal(err)
	}
	spec := tools.CommandSpec{Path: args[0], Args: args[1:], Dir: t.TempDir()}
	calls := 0

	stdout, stderr, err := runOpenCodeCredentialReadCommandWithRetry(
		context.Background(), runtime, spec,
		func(_ context.Context, got tools.CommandSpec) ([]byte, []byte, error) {
			calls++
			wantArgs := append([]string(nil), spec.Args...)
			if calls == 1 {
				if !reflect.DeepEqual(got.Args, wantArgs) {
					t.Fatalf("first command args = %#v, want %#v", got.Args, wantArgs)
				}
				return nil, []byte("npm error code ECONNRESET\n"), errors.New("exit status 1")
			}
			for i, arg := range wantArgs {
				if arg == openCodeCredentialPreferOfflineArg {
					wantArgs[i] = openCodeCredentialPreferOnlineArg
				}
			}
			if !reflect.DeepEqual(got.Args, wantArgs) {
				t.Fatalf("retry command args = %#v, want %#v", got.Args, wantArgs)
			}
			return []byte("[]"), nil, nil
		},
	)
	if err != nil {
		t.Fatalf("credential command retry: %v (stderr %q)", err, stderr)
	}
	if calls != 2 {
		t.Fatalf("command calls = %d, want one offline and one online attempt", calls)
	}
	if string(stdout) != "[]" {
		t.Fatalf("credential command output = %q, want retry output", stdout)
	}
}

func TestOpenCodeCredentialReadRetriesExactETARGETOnline(t *testing.T) {
	runtime := opencode.Runtime{Family: opencode.FamilyV2, Source: opencode.SourceManaged, Version: "2.1.2"}
	args, err := runtime.CredentialCommand("export")
	if err != nil {
		t.Fatal(err)
	}
	spec := tools.CommandSpec{Path: args[0], Args: args[1:], Dir: t.TempDir()}
	calls := 0
	_, _, err = runOpenCodeCredentialReadCommandWithRetry(
		context.Background(), runtime, spec,
		func(_ context.Context, got tools.CommandSpec) ([]byte, []byte, error) {
			calls++
			if calls == 1 {
				return nil, []byte("npm error code ETARGET\nnpm error notarget No matching version found for @opencode/cli@2.1.2.\n"), errors.New("exit status 1")
			}
			wantArgs := append([]string(nil), spec.Args...)
			for i, arg := range wantArgs {
				if arg == openCodeCredentialPreferOfflineArg {
					wantArgs[i] = openCodeCredentialPreferOnlineArg
				}
			}
			if !reflect.DeepEqual(got.Args, wantArgs) {
				t.Fatalf("retry command args = %#v, want prefer-online command", got.Args)
			}
			return []byte("[]"), nil, nil
		},
	)
	if err != nil {
		t.Fatalf("credential read retry: %v", err)
	}
	if calls != 2 {
		t.Fatalf("command calls = %d, want one offline and one online attempt", calls)
	}
}

func TestOpenCodeCredentialReadDoesNotRetryUntrustedFailure(t *testing.T) {
	runtime := opencode.Runtime{Family: opencode.FamilyV2, Source: opencode.SourceManaged, Version: "2.1.2"}
	args, err := runtime.CredentialCommand("export")
	if err != nil {
		t.Fatal(err)
	}
	spec := tools.CommandSpec{Path: args[0], Args: args[1:], Dir: t.TempDir()}
	cases := []struct {
		name   string
		stderr string
	}{
		{name: "permanent npm error", stderr: "npm error code E401\n"},
		{name: "unclassified error", stderr: "authentication failed\n"},
		{name: "unrelated ETARGET", stderr: "npm error code ETARGET\nnpm error notarget No matching version found for dependency@1.0.0.\n"},
		{name: "release age policy", stderr: "npm error code ETARGET\nnpm error notarget No matching version found for @opencode/cli@2.1.2 with a date before 2026-09-24T11:00:00Z.\n"},
		{name: "multiple npm codes", stderr: "npm error code ECONNRESET\nnpm error code E401\n"},
		{name: "oversized diagnostic", stderr: "npm error code ECONNRESET\n" + strings.Repeat("diagnostic\n", 2000)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			_, _, err := runOpenCodeCredentialReadCommandWithRetry(
				context.Background(), runtime, spec,
				func(_ context.Context, _ tools.CommandSpec) ([]byte, []byte, error) {
					calls++
					return nil, []byte(tc.stderr), errors.New("exit status 1")
				},
			)
			if err == nil {
				t.Fatal("credential command failure was hidden")
			}
			if calls != 1 {
				t.Fatalf("command calls = %d, want no retry", calls)
			}
		})
	}
}

func TestOpenCodeCredentialAdmissionWaitsForProcessTreeReap(t *testing.T) {
	for _, tc := range []struct {
		name     string
		reapErr  error
		wantOpen bool
	}{
		{name: "clean reap"},
		{name: "failed reap", reapErr: errors.New("process tree remains alive"), wantOpen: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager := &Manager{credentialImport: true}
			done := make(chan struct{})
			proc := &PipedProcess{Done: done, process: &commandProcess{reapErr: tc.reapErr}}
			cleanupDone := manager.holdCredentialAdmissionUntilReaped(proc)
			if err := manager.releaseOpenCodeCredentialAdmission(); err != nil {
				t.Fatalf("release credential admission: %v", err)
			}

			manager.startMu.Lock()
			if !manager.credentialImport || !manager.credentialImportBlocked {
				t.Fatal("credential admission opened before process tree reaping completed")
			}
			manager.startMu.Unlock()

			close(done)
			select {
			case <-cleanupDone:
			case <-time.After(time.Second):
				t.Fatal("credential cleanup did not observe process tree completion")
			}

			manager.startMu.Lock()
			defer manager.startMu.Unlock()
			if got := manager.credentialImport; got != tc.wantOpen {
				t.Fatalf("credential admission blocked = %v, want %v", got, tc.wantOpen)
			}
		})
	}
}

func TestOpenCodeCredentialStatusRejectsStoppingAgent(t *testing.T) {
	manager := &Manager{}
	manager.status.Store(StatusStopping)

	if _, err := manager.OpenCodeCredentialStatus(t.Context()); err == nil {
		t.Fatal("credential status accepted an agent that is still stopping")
	}
}
