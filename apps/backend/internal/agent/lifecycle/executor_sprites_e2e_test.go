//go:build sprites_e2e

package lifecycle

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/remoteauth"
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/secrets"
)

// SPRITES_API_TOKEN="" go test -tags sprites_e2e -run TestSpritesE2E_CredentialsAndAuth -v -timeout 600s ./internal/agent/lifecycle/

var spritesInteractive = flag.Bool("sprites.interactive", false, "block after setup for manual sprite debugging")

func TestSpritesE2E_FullFlow(t *testing.T) {
	t.Skip("")
	token := os.Getenv("SPRITES_API_TOKEN")
	if token == "" {
		t.Skip("SPRITES_API_TOKEN not set, skipping sprites e2e test")
	}

	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:      "debug",
		Format:     "console",
		OutputPath: "stdout",
	})
	require.NoError(t, err)

	resolver := NewAgentctlResolver(log)
	agentRegistry := registry.NewRegistry(log)
	agentRegistry.LoadDefaults()
	spritesExec := NewSpritesExecutor(&noopSecretStore{}, agentRegistry, resolver, spritesAgentctlPort, log)

	req := &ExecutorCreateRequest{
		InstanceID: "e2e-" + randomSuffix(),
		TaskID:     "e2e-task",
		SessionID:  "e2e-session",
		Env: map[string]string{
			"SPRITES_API_TOKEN": token,
		},
		Metadata: map[string]interface{}{
			"repository_clone_url": "https://github.com/gin-gonic/gin",
			"repository_branch":    "master",
		},
		OnProgress: func(step PrepareStep, stepIndex int, totalSteps int) {
			t.Logf("[%d/%d] %s — %s", stepIndex+1, totalSteps, step.Name, step.Status)
			if step.Output != "" {
				t.Logf("  output: %s", step.Output)
			}
			if step.Error != "" {
				t.Logf("  ERROR: %s", step.Error)
			}
		},
	}

	ctx := context.Background()
	instance, err := spritesExec.CreateInstance(ctx, req)
	require.NoError(t, err)
	defer func() {
		t.Log("Cleaning up sprite instance...")
		_ = spritesExec.StopInstance(context.Background(), instance, true)
	}()

	// Verify agentctl health via port-forwarded proxy
	localPort := instance.Metadata["local_port"].(int)
	t.Logf("Checking agentctl health at http://127.0.0.1:%d/health ...", localPort)

	// Retry health check — proxy WebSocket setup may take a moment
	var healthErr error
	for i := 0; i < 5; i++ {
		healthCtx, healthCancel := context.WithTimeout(ctx, 5*time.Second)
		healthErr = instance.Client.Health(healthCtx)
		healthCancel()
		if healthErr == nil {
			break
		}
		t.Logf("  health attempt %d failed: %v", i+1, healthErr)
		time.Sleep(1 * time.Second)
	}
	require.NoError(t, healthErr)
	t.Log("agentctl health: OK")

	// Verify agentctl status (may 404 if no instance is configured yet — that's OK)
	statusCtx, statusCancel := context.WithTimeout(ctx, 10*time.Second)
	defer statusCancel()
	status, err := instance.Client.GetStatus(statusCtx)
	if err != nil {
		t.Logf("agentctl status: %v (expected — no instance configured)", err)
	} else {
		statusJSON, _ := json.MarshalIndent(status, "", "  ")
		t.Logf("agentctl status: %s", statusJSON)
	}

	// --- Agentctl operation subtests ---
	client := instance.Client

	t.Run("FileOps", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		// Use "." — paths are relative to instance workspace (/workspace)
		tree, err := client.RequestFileTree(ctx, ".", 2)
		require.NoError(t, err)
		require.NotNil(t, tree.Root)
		t.Logf("file tree root: %s, children: %d", tree.Root.Name, len(tree.Root.Children))

		results, err := client.SearchFiles(ctx, "gin.go", 10)
		require.NoError(t, err)
		require.NotEmpty(t, results.Files)
		t.Logf("found %d files matching 'gin.go'", len(results.Files))

		content, err := client.RequestFileContent(ctx, "go.mod")
		require.NoError(t, err)
		require.Contains(t, content.Content, "github.com/gin-gonic/gin")
		t.Logf("go.mod: %d bytes", len(content.Content))

		_, err = client.CreateFile(ctx, "e2e-test.txt")
		require.NoError(t, err)
		_, err = client.DeleteFile(ctx, "e2e-test.txt")
		require.NoError(t, err)
		t.Log("file create+delete roundtrip OK")
	})

	t.Run("ConcurrentProcesses", func(t *testing.T) {
		// Completed processes are immediately removed from tracking,
		// so we use a workspace stream to capture output in real-time.
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		procClient := agentctl.NewClient("127.0.0.1", localPort, log)
		defer procClient.Close()

		var procOutput strings.Builder
		var procMu sync.Mutex
		connected := make(chan struct{})

		stream, err := procClient.StreamWorkspace(ctx, agentctl.WorkspaceStreamCallbacks{
			OnConnected: func() { close(connected) },
			OnProcessOutput: func(output *agentctl.ProcessOutput) {
				procMu.Lock()
				procOutput.WriteString(output.Data)
				procMu.Unlock()
			},
		})
		require.NoError(t, err)
		defer stream.Close()

		select {
		case <-connected:
		case <-ctx.Done():
			t.Fatal("workspace stream connection timed out")
		}

		markers := []string{
			"PROC_A_" + randomSuffix(),
			"PROC_B_" + randomSuffix(),
			"PROC_C_" + randomSuffix(),
		}

		// Start all 3 concurrently
		var wg sync.WaitGroup
		var mu sync.Mutex
		var startErrs []error

		for _, m := range markers {
			wg.Add(1)
			go func(marker string) {
				defer wg.Done()
				_, err := client.StartProcess(ctx, agentctl.StartProcessRequest{
					SessionID: "e2e-session",
					Command:   fmt.Sprintf("echo %s", marker),
				})
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					startErrs = append(startErrs, err)
				}
			}(m)
		}
		wg.Wait()
		require.Empty(t, startErrs, "all processes should start without error")

		// Verify all markers appear in process output stream
		for _, marker := range markers {
			m := marker
			require.Eventually(t, func() bool {
				procMu.Lock()
				defer procMu.Unlock()
				return strings.Contains(procOutput.String(), m)
			}, 10*time.Second, 200*time.Millisecond,
				"process output should contain marker %s", m)
		}
		t.Logf("all %d concurrent processes produced output via stream", len(markers))
	})

	t.Run("WorkspaceStreamShellIO", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		// Ensure shell is started before connecting stream
		require.NoError(t, client.StartShell(ctx))

		var shellOutput strings.Builder
		var shellMu sync.Mutex
		connected := make(chan struct{})

		shellClient := agentctl.NewClient("127.0.0.1", localPort, log)
		defer shellClient.Close()

		stream, err := shellClient.StreamWorkspace(ctx, agentctl.WorkspaceStreamCallbacks{
			OnConnected: func() { close(connected) },
			OnShellOutput: func(data string) {
				shellMu.Lock()
				shellOutput.WriteString(data)
				shellMu.Unlock()
			},
		})
		require.NoError(t, err)
		defer stream.Close()

		select {
		case <-connected:
		case <-ctx.Done():
			t.Fatal("workspace stream connection timed out")
		}

		marker := "E2E_SHELL_MARKER_" + randomSuffix()
		err = stream.WriteShellInput(fmt.Sprintf("echo %s\n", marker))
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			shellMu.Lock()
			defer shellMu.Unlock()
			return strings.Contains(shellOutput.String(), marker)
		}, 10*time.Second, 200*time.Millisecond, "shell output should contain marker")
		t.Logf("shell PTY roundtrip confirmed with marker: %s", marker)
	})

	t.Run("MultipleWebSocketStreams", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		localPort := instance.Metadata["local_port"].(int)

		client2 := agentctl.NewClient("127.0.0.1", localPort, log)
		defer client2.Close()

		require.NoError(t, client2.Health(ctx))
		t.Log("second client health: OK")

		var output2 strings.Builder
		var mu2 sync.Mutex
		connected2 := make(chan struct{})

		stream2, err := client2.StreamWorkspace(ctx, agentctl.WorkspaceStreamCallbacks{
			OnConnected: func() { close(connected2) },
			OnProcessOutput: func(output *agentctl.ProcessOutput) {
				mu2.Lock()
				output2.WriteString(output.Data)
				mu2.Unlock()
			},
		})
		require.NoError(t, err)
		defer stream2.Close()

		select {
		case <-connected2:
		case <-ctx.Done():
			t.Fatal("second workspace stream connection timed out")
		}

		marker := "MULTI_WS_" + randomSuffix()
		proc, err := client.StartProcess(ctx, agentctl.StartProcessRequest{
			SessionID: "e2e-session",
			Command:   fmt.Sprintf("echo %s", marker),
		})
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			mu2.Lock()
			defer mu2.Unlock()
			return strings.Contains(output2.String(), marker)
		}, 10*time.Second, 200*time.Millisecond,
			"second workspace stream should receive process output")
		t.Logf("multi-stream process %s: output confirmed on stream 2", proc.ID)
	})

	t.Run("GitOps", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		// Configure git user (fire-and-forget — process completes instantly)
		_, err := client.StartProcess(ctx, agentctl.StartProcessRequest{
			SessionID: "e2e-session",
			Command:   "cd /workspace && git config user.email 'e2e@test.com' && git config user.name 'E2E Test'",
		})
		require.NoError(t, err)
		time.Sleep(2 * time.Second) // let config complete

		// Create a file, stage, commit via git API
		_, err = client.CreateFile(ctx, "e2e-change.txt")
		require.NoError(t, err)

		result, err := client.GitStage(ctx, []string{"e2e-change.txt"})
		require.NoError(t, err)
		require.True(t, result.Success, "git stage should succeed: %s", result.Error)

		result, err = client.GitCommit(ctx, "e2e test commit", false)
		require.NoError(t, err)
		require.True(t, result.Success, "git commit should succeed: %s", result.Error)
		t.Logf("git commit: %s", result.Output)
	})

	t.Run("VscodeStartAndReachable", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
		defer cancel()

		resp, err := client.StartVscode(ctx, "dark")
		require.NoError(t, err)
		require.True(t, resp.Success, "vscode start should succeed: %s", resp.Error)
		t.Log("vscode start requested")

		var vscodePort int
		require.Eventually(t, func() bool {
			st, err := client.VscodeStatus(ctx)
			if err != nil {
				return false
			}
			t.Logf("vscode status: %s port=%d", st.Status, st.Port)
			if st.Status == "running" && st.Port > 0 {
				vscodePort = st.Port
				return true
			}
			return false
		}, 90*time.Second, 3*time.Second, "vscode should reach running status")
		t.Logf("vscode running on port %d inside sprite", vscodePort)

		// Verify VS Code is serving by curling from inside the sprite.
		// Use sleep to keep process alive (completed processes are removed
		// from tracking immediately, so GetProcess needs an alive process).
		curlCmd := fmt.Sprintf(
			"curl -sL http://localhost:%d/ > /tmp/vscode-curl.out 2>&1; sleep 60", vscodePort)
		proc, err := client.StartProcess(ctx, agentctl.StartProcessRequest{
			SessionID: "e2e-session",
			Command:   curlCmd,
		})
		require.NoError(t, err)

		// Wait for curl to write output, then read via a second process
		time.Sleep(5 * time.Second)
		catProc, err := client.StartProcess(ctx, agentctl.StartProcessRequest{
			SessionID: "e2e-session",
			Command:   "cat /tmp/vscode-curl.out; sleep 60",
		})
		require.NoError(t, err)
		time.Sleep(2 * time.Second)

		info, err := client.GetProcess(ctx, catProc.ID, true)
		require.NoError(t, err)
		var htmlOutput string
		for _, chunk := range info.Output {
			htmlOutput += chunk.Data
		}
		t.Logf("vscode HTML preview: %.200s", htmlOutput)
		require.True(t,
			strings.Contains(strings.ToLower(htmlOutput), "html") ||
				strings.Contains(strings.ToLower(htmlOutput), "doctype"),
			"vscode should serve HTML content")

		// Clean up long-running processes
		_ = client.StopProcess(ctx, proc.ID)
		_ = client.StopProcess(ctx, catProc.ID)

		err = client.StopVscode(ctx)
		require.NoError(t, err)
		t.Log("vscode stopped")
	})

	// Interactive mode: block so user can "sprites attach <name>"
	if *spritesInteractive {
		spriteName, _ := instance.Metadata["sprite_name"].(string)
		t.Logf("\n  Sprite ready! Attach with:  sprites attach %s", spriteName)
		t.Log("  Press Ctrl+C to destroy and exit.")
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		<-sig
		t.Log("Interrupted, cleaning up...")
	}
}

// TestSpritesE2E_CredentialsAndAuth verifies credential copying and agent auth on a Sprites sandbox.
// It copies all locally available credentials (SSH, gh, auggie, codex, gemini) to the sprite,
// then runs subtests for gh auth, SSH git clone, and one-off agent invocations.
func TestSpritesE2E_CredentialsAndAuth(t *testing.T) {
	token := os.Getenv("SPRITES_API_TOKEN")
	if token == "" {
		t.Skip("SPRITES_API_TOKEN not set, skipping sprites credentials e2e test")
	}
	gitUserName, gitUserEmail, ok := hostGitIdentity(t)
	if !ok {
		t.Skip("local git user.name/user.email not configured; skipping")
	}

	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:      "debug",
		Format:     "console",
		OutputPath: "stdout",
	})
	require.NoError(t, err)

	agentRegistry := registry.NewRegistry(log)
	agentRegistry.LoadDefaults()
	catalog := remoteauth.BuildCatalog(agentRegistry.ListEnabled())
	credIDs := make([]string, 0, len(catalog.Specs)+1)
	credIDs = append(credIDs, "gh_cli_token") // auto-detect GH CLI token
	for _, spec := range catalog.Specs {
		for _, method := range spec.Methods {
			if method.Type != "files" || !method.HasLocalFiles {
				continue
			}
			credIDs = append(credIDs, method.MethodID)
			t.Logf("credential available: %s (%s)", method.MethodID, spec.DisplayName)
		}
	}

	credIDsJSON, err := json.Marshal(credIDs)
	require.NoError(t, err)

	env := map[string]string{
		"SPRITES_API_TOKEN": token,
	}
	if oauthToken := os.Getenv("CLAUDE_CODE_OAUTH_TOKEN"); oauthToken != "" {
		env["CLAUDE_CODE_OAUTH_TOKEN"] = oauthToken
	}

	resolver := NewAgentctlResolver(log)
	spritesExec := NewSpritesExecutor(&noopSecretStore{}, agentRegistry, resolver, spritesAgentctlPort, log)

	req := &ExecutorCreateRequest{
		InstanceID: "e2e-creds-" + randomSuffix(),
		TaskID:     "e2e-creds-task",
		SessionID:  "e2e-creds-session",
		Env:        env,
		Metadata: map[string]interface{}{
			"repository_clone_url": "https://github.com/kdlbs/agents-protocol-debug.git",
			"repository_branch":    "main",
			"remote_credentials":   string(credIDsJSON),
			"git_user_name":        gitUserName,
			"git_user_email":       gitUserEmail,
		},
		OnProgress: func(step PrepareStep, stepIndex int, totalSteps int) {
			t.Logf("[%d/%d] %s — %s", stepIndex+1, totalSteps, step.Name, step.Status)
			if step.Output != "" {
				t.Logf("  output: %s", step.Output)
			}
			if step.Error != "" {
				t.Logf("  ERROR: %s", step.Error)
			}
		},
	}

	ctx := context.Background()
	instance, err := spritesExec.CreateInstance(ctx, req)
	require.NoError(t, err)
	defer func() {
		t.Log("Cleaning up sprite instance...")
		_ = spritesExec.StopInstance(context.Background(), instance, true)
	}()

	// Wait for agentctl health
	localPort := instance.Metadata["local_port"].(int)
	var healthErr error
	for i := 0; i < 5; i++ {
		healthCtx, healthCancel := context.WithTimeout(ctx, 5*time.Second)
		healthErr = instance.Client.Health(healthCtx)
		healthCancel()
		if healthErr == nil {
			break
		}
		t.Logf("  health attempt %d failed: %v", i+1, healthErr)
		time.Sleep(1 * time.Second)
	}
	require.NoError(t, healthErr, "agentctl health check failed")
	t.Log("agentctl health: OK")

	client := instance.Client

	t.Run("GitIdentityCopiedFromHost", func(t *testing.T) {
		res := runSpriteCmd(t, client, localPort, log,
			"git config --global --get user.name 2>&1", 30*time.Second)
		require.Equal(t, 0, res.ExitCode, "git config user.name should succeed")
		require.Equal(t, gitUserName, strings.TrimSpace(res.Output), "sprite git user.name mismatch")

		res = runSpriteCmd(t, client, localPort, log,
			"git config --global --get user.email 2>&1", 30*time.Second)
		require.Equal(t, 0, res.ExitCode, "git config user.email should succeed")
		require.Equal(t, gitUserEmail, strings.TrimSpace(res.Output), "sprite git user.email mismatch")
	})

	t.Run("GhCliAuth", func(t *testing.T) {
		if !hasCredential(credIDs, "gh_cli_token") {
			t.Skip("gh_cli_token not selected")
		}
		// GITHUB_TOKEN is injected via gh_cli_token auto-detect — verify gh can use it.
		// gh auth status makes a network call that can be slow on sprites; retry with short timeout.
		var res spriteCmdResult
		for attempt := 1; attempt <= 2; attempt++ {
			res = runSpriteCmdNoFail(t, client, localPort, log, "gh auth status 2>&1", 10*time.Second)
			if res.Output != "" {
				break
			}
			t.Logf("gh auth status attempt %d timed out, retrying...", attempt)
		}
		t.Logf("gh auth status output:\n%s", res.Output)
		require.NotEmpty(t, res.Output, "gh auth status should produce output")
		lower := strings.ToLower(res.Output)
		require.True(t,
			strings.Contains(lower, "logged in") || strings.Contains(lower, "github_token"),
			"gh auth status should indicate logged in or GITHUB_TOKEN active, got: %s", res.Output)
	})

	t.Run("AuggieAuth", func(t *testing.T) {
		if !hasCredentialPrefix(credIDs, "agent:auggie:files:") {
			t.Skip("auggie file auth credential not available on host")
		}
		res := runSpriteCmd(t, client, localPort, log,
			"npx -y @augmentcode/auggie -p hello 2>&1", 180*time.Second)
		t.Logf("auggie output:\n%s", res.Output)
		require.Equal(t, 0, res.ExitCode, "auggie should exit with code 0")
		require.NotContains(t, strings.ToLower(res.Output), "authentication",
			"auggie should not show auth errors")
	})

	t.Run("CodexAuth", func(t *testing.T) {
		if !hasAgentFileCredential(credIDs, "codex") {
			t.Skip("codex file auth credential not available on host")
		}
		res := runSpriteCmd(t, client, localPort, log,
			"npx -y @openai/codex exec \"hello\" 2>&1", 180*time.Second)
		t.Logf("codex output:\n%s", res.Output)
		require.Equal(t, 0, res.ExitCode, "codex should exit with code 0")
		require.NotContains(t, strings.ToLower(res.Output), "unauthorized",
			"codex should not show auth errors")
	})

	t.Run("GeminiAuth", func(t *testing.T) {
		if !hasAgentFileCredential(credIDs, "gemini") {
			t.Skip("gemini file auth credential not available on host")
		}
		res := runSpriteCmd(t, client, localPort, log,
			"npx -y @google/gemini-cli --model flash -p hello 2>&1", 180*time.Second)
		t.Logf("gemini output:\n%s", res.Output)
		// require.Equal(t, 0, res.ExitCode, "gemini should exit with code 0")
		require.NotContains(t, strings.ToLower(res.Output), "unauthenticated",
			"gemini should not show auth errors")
	})

	t.Run("ClaudeCodeAuth", func(t *testing.T) {
		if os.Getenv("CLAUDE_CODE_OAUTH_TOKEN") == "" {
			t.Skip("CLAUDE_CODE_OAUTH_TOKEN not set, skipping Claude Code auth test")
		}

		// Verify credential files were created by the auth setup script
		res := runSpriteCmd(t, client, localPort, log,
			`test -f ~/.claude/.credentials.json && echo CREDS_OK || echo CREDS_MISSING`, 30*time.Second)
		require.Contains(t, res.Output, "CREDS_OK", "credential file should exist")

		res = runSpriteCmd(t, client, localPort, log,
			`test -f ~/.claude.json && echo ONBOARD_OK || echo ONBOARD_MISSING`, 30*time.Second)
		require.Contains(t, res.Output, "ONBOARD_OK", "onboarding file should exist")

		// Token is injected via ExecutorCreateRequest.Env and inherited by sprite processes.
		// Keep command text free of secrets so logs do not expose token values.
		res = runSpriteCmd(t, client, localPort, log,
			"npx -y @anthropic-ai/claude-code --verbose -p hello 2>&1",
			180*time.Second)
		t.Logf("claude-code output:\n%s", res.Output)
		require.Equal(t, 0, res.ExitCode, "claude-code should exit with code 0")
		lower := strings.ToLower(res.Output)
		for _, errPattern := range []string{
			"authentication_error",
			"invalid_api_key",
			"invalid bearer token",
			"failed to authenticate",
		} {
			require.NotContains(t, lower, errPattern,
				"claude-code should not show auth errors")
		}
	})

	// Interactive mode
	if *spritesInteractive {
		spriteName, _ := instance.Metadata["sprite_name"].(string)
		t.Logf("\n  Sprite ready! Attach with:  sprites attach %s", spriteName)
		t.Log("  Press Ctrl+C to destroy and exit.")
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		<-sig
		t.Log("Interrupted, cleaning up...")
	}
}

// spriteCmdResult holds the output and exit code of a command run on a sprite.
type spriteCmdResult struct {
	Output   string
	ExitCode int
}

// runSpriteCmd runs a command on the sprite and returns its combined output and exit code.
// It uses a workspace stream with an end marker to reliably capture all output.
func runSpriteCmd(
	t *testing.T,
	client *agentctl.Client,
	localPort int,
	log *logger.Logger,
	cmd string,
	timeout time.Duration,
) spriteCmdResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	marker := "END_MARKER_" + randomSuffix()

	var output strings.Builder
	var mu sync.Mutex
	connected := make(chan struct{})

	streamClient := agentctl.NewClient("127.0.0.1", localPort, log)
	defer streamClient.Close()

	stream, err := streamClient.StreamWorkspace(ctx, agentctl.WorkspaceStreamCallbacks{
		OnConnected: func() { close(connected) },
		OnProcessOutput: func(o *agentctl.ProcessOutput) {
			mu.Lock()
			output.WriteString(o.Data)
			mu.Unlock()
		},
	})
	require.NoError(t, err)
	defer stream.Close()

	select {
	case <-connected:
	case <-ctx.Done():
		t.Fatal("workspace stream connection timed out")
	}

	// Wrap command to capture exit code and echo end marker when done
	wrappedCmd := fmt.Sprintf("(%s); _ec=$?; echo \"%s EXIT_CODE=$_ec\"", cmd, marker)
	_, err = client.StartProcess(ctx, agentctl.StartProcessRequest{
		SessionID: "e2e-creds-session",
		Command:   wrappedCmd,
	})
	require.NoError(t, err)

	// Wait for end marker to appear in output
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return strings.Contains(output.String(), marker)
	}, timeout, 500*time.Millisecond, "command did not complete within timeout: %s", cmd)

	mu.Lock()
	fullOutput := output.String()
	mu.Unlock()

	// Parse exit code and strip the marker line from output
	exitCode := -1
	idx := strings.Index(fullOutput, marker)
	if idx >= 0 {
		markerLine := fullOutput[idx:]
		if ecIdx := strings.Index(markerLine, "EXIT_CODE="); ecIdx >= 0 {
			ecStr := strings.TrimSpace(markerLine[ecIdx+len("EXIT_CODE="):])
			if n, err := strconv.Atoi(strings.Split(ecStr, "\n")[0]); err == nil {
				exitCode = n
			}
		}
		return spriteCmdResult{
			Output:   strings.TrimSpace(fullOutput[:idx]),
			ExitCode: exitCode,
		}
	}
	return spriteCmdResult{
		Output:   strings.TrimSpace(fullOutput),
		ExitCode: exitCode,
	}
}

// runSpriteCmdNoFail is like runSpriteCmd but returns an empty result on timeout instead of failing.
func runSpriteCmdNoFail(
	t *testing.T,
	client *agentctl.Client,
	localPort int,
	log *logger.Logger,
	cmd string,
	timeout time.Duration,
) spriteCmdResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	marker := "END_MARKER_" + randomSuffix()

	var output strings.Builder
	var mu sync.Mutex
	connected := make(chan struct{})

	streamClient := agentctl.NewClient("127.0.0.1", localPort, log)
	defer streamClient.Close()

	stream, err := streamClient.StreamWorkspace(ctx, agentctl.WorkspaceStreamCallbacks{
		OnConnected: func() { close(connected) },
		OnProcessOutput: func(o *agentctl.ProcessOutput) {
			mu.Lock()
			output.WriteString(o.Data)
			mu.Unlock()
		},
	})
	if err != nil {
		return spriteCmdResult{ExitCode: -1}
	}
	defer stream.Close()

	select {
	case <-connected:
	case <-ctx.Done():
		return spriteCmdResult{ExitCode: -1}
	}

	wrappedCmd := fmt.Sprintf("(%s); _ec=$?; echo \"%s EXIT_CODE=$_ec\"", cmd, marker)
	if _, err = client.StartProcess(ctx, agentctl.StartProcessRequest{
		SessionID: "e2e-creds-session",
		Command:   wrappedCmd,
	}); err != nil {
		return spriteCmdResult{ExitCode: -1}
	}

	// Wait for marker — return empty on timeout
	deadline := time.After(timeout)
	for {
		select {
		case <-deadline:
			return spriteCmdResult{ExitCode: -1}
		case <-time.After(500 * time.Millisecond):
			mu.Lock()
			if strings.Contains(output.String(), marker) {
				fullOutput := output.String()
				mu.Unlock()

				exitCode := -1
				idx := strings.Index(fullOutput, marker)
				if idx >= 0 {
					markerLine := fullOutput[idx:]
					if ecIdx := strings.Index(markerLine, "EXIT_CODE="); ecIdx >= 0 {
						ecStr := strings.TrimSpace(markerLine[ecIdx+len("EXIT_CODE="):])
						if n, parseErr := strconv.Atoi(strings.Split(ecStr, "\n")[0]); parseErr == nil {
							exitCode = n
						}
					}
					return spriteCmdResult{Output: strings.TrimSpace(fullOutput[:idx]), ExitCode: exitCode}
				}
				return spriteCmdResult{Output: strings.TrimSpace(fullOutput), ExitCode: exitCode}
			}
			mu.Unlock()
		}
	}
}

func hasCredentialPrefix(credIDs []string, prefix string) bool {
	for _, c := range credIDs {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

// hasCredential checks for an exact credential ID match.
// Kept for compatibility with optional e2e subtests that may be toggled on/off.
func hasCredential(credIDs []string, target string) bool {
	for _, c := range credIDs {
		if c == target {
			return true
		}
	}
	return false
}

func hasAgentFileCredential(credIDs []string, agentID string) bool {
	return hasCredentialPrefix(credIDs, "agent:"+agentID+":files:")
}

func hostGitIdentity(t *testing.T) (string, string, bool) {
	t.Helper()
	name := readGitConfigValue("user.name")
	email := readGitConfigValue("user.email")
	if name == "" || email == "" {
		t.Log("local git identity not fully configured")
		return "", "", false
	}
	return name, email, true
}

func readGitConfigValue(key string) string {
	if value := runGitConfig("config", "--get", key); value != "" {
		return value
	}
	return runGitConfig("config", "--global", "--get", key)
}

func runGitConfig(args ...string) string {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// noopSecretStore is a minimal stub — sprites executor doesn't use it directly.
type noopSecretStore struct{}

func (n *noopSecretStore) Create(_ context.Context, _ *secrets.SecretWithValue) error { return nil }
func (n *noopSecretStore) Get(_ context.Context, _ string) (*secrets.Secret, error) {
	return nil, nil
}
func (n *noopSecretStore) Reveal(_ context.Context, _ string) (string, error) { return "", nil }
func (n *noopSecretStore) Update(_ context.Context, _ string, _ *secrets.UpdateSecretRequest) error {
	return nil
}
func (n *noopSecretStore) Delete(_ context.Context, _ string) error { return nil }
func (n *noopSecretStore) List(_ context.Context) ([]*secrets.SecretListItem, error) {
	return nil, nil
}
func (n *noopSecretStore) Close() error { return nil }

func randomSuffix() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
