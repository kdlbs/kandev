package repoclone

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

func TestGitCmdWithHTTPHeaderKeepsCredentialOutOfArguments(t *testing.T) {
	t.Parallel()
	cloner := &Cloner{}
	header := "Authorization: Basic c2VjcmV0"
	cmd := cloner.gitCmdWithHTTPHeader(context.Background(), header, "clone", "https://dev.azure.com/acme/p/_git/r")
	if strings.Contains(strings.Join(cmd.Args, " "), "c2VjcmV0") {
		t.Fatal("credential leaked into command arguments")
	}
	found := false
	for _, value := range cmd.Env {
		if value == "GIT_CONFIG_VALUE_0="+header {
			found = true
		}
	}
	if !found {
		t.Fatal("authorization header was not provided through the Git child environment")
	}
}

func TestEnsureClonedWithBasicAuthKeepsCredentialScopedToGitChild(t *testing.T) {
	tests := []struct {
		name   string
		cancel bool
	}{
		{name: "git failure"},
		{name: "context cancellation", cancel: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			binDir := t.TempDir()
			capturePath := filepath.Join(t.TempDir(), "git-env")
			fakeGit := "#!/bin/sh\nprintf '%s' \"$GIT_CONFIG_VALUE_0\" > \"$CAPTURE_PATH\"\n" +
				"if [ \"$BLOCK_GIT\" = 1 ]; then exec sleep 10; fi\nexit 1\n"
			if err := os.WriteFile(filepath.Join(binDir, "git"), []byte(fakeGit), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("CAPTURE_PATH", capturePath)
			t.Setenv("GIT_CONFIG_VALUE_0", "")
			if tc.cancel {
				t.Setenv("BLOCK_GIT", "1")
			}
			cloner := NewCloner(Config{BasePath: t.TempDir()}, ProtocolHTTPS, "", logger.Default())
			ctx := context.Background()
			var cancel context.CancelFunc
			if tc.cancel {
				ctx, cancel = context.WithTimeout(ctx, 20*time.Millisecond)
				defer cancel()
			}
			_, err := cloner.EnsureClonedWithBasicAuth(
				ctx, "https://dev.azure.com/acme/p/_git/r", "p", "r", "kandev", "secret-pat",
			)
			if err == nil {
				t.Fatal("expected git clone error")
			}
			if tc.cancel && ctx.Err() != context.DeadlineExceeded {
				t.Fatalf("expected cancelled clone context, got %v", ctx.Err())
			}
			captured, readErr := os.ReadFile(capturePath)
			if readErr != nil {
				t.Fatal(readErr)
			}
			expectedCredential := base64.StdEncoding.EncodeToString([]byte("kandev:secret-pat"))
			if !strings.Contains(string(captured), expectedCredential) {
				t.Fatal("credential was not passed to Git child")
			}
			if os.Getenv("GIT_CONFIG_VALUE_0") != "" {
				t.Fatal("credential escaped into the parent process environment")
			}
		})
	}
}
