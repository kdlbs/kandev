//go:build !windows

package subproc

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

func TestManagedGitCredentialFillPTY(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(config, []byte("[credential]\n\thelper = !false\n"), 0o600); err != nil {
		t.Fatalf("write isolated git config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cmd := NewGitCommand(ctx, "credential", "fill")
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=" + os.Getenv("PATH"),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=1",
		"GCM_INTERACTIVE=Always",
		"GCM_GUI_PROMPT=1",
		"GIT_ASKPASS=",
		"SSH_ASKPASS=",
	}
	PrepareGitCommand(cmd)

	terminal, err := pty.Start(cmd)
	if err != nil {
		t.Fatalf("start git under controlling PTY: %v", err)
	}
	t.Cleanup(func() {
		_ = terminal.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	if _, err := terminal.Write([]byte("protocol=https\nhost=example.invalid\n\n")); err != nil {
		t.Fatalf("write credential request: %v", err)
	}

	type result struct {
		output []byte
		err    error
		wait   error
	}
	finished := make(chan result, 1)
	go func() {
		output, readErr := io.ReadAll(terminal)
		finished <- result{output: output, err: readErr, wait: cmd.Wait()}
	}()

	var got result
	select {
	case got = <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("managed Git credential fill remained attached to the controlling PTY")
	}
	if got.wait == nil {
		t.Fatal("credential fill unexpectedly succeeded without credentials")
	}
	if got.err != nil && !strings.Contains(got.err.Error(), "input/output error") {
		t.Fatalf("read PTY output: %v", got.err)
	}
}
