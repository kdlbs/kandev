package lifecycle

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
)

// mockPassthroughProfileResolver is a mock for testing passthrough verification
type mockPassthroughProfileResolver struct {
	cliPassthrough bool
	err            error
}

func (m *mockPassthroughProfileResolver) ResolveProfile(ctx context.Context, profileID string) (*AgentProfileInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &AgentProfileInfo{
		ProfileID:      profileID,
		CLIPassthrough: m.cliPassthrough,
	}, nil
}

func TestBuildPassthroughCommand(t *testing.T) {
	tests := []struct {
		name    string
		agent   agents.PassthroughAgent
		opts    agents.PassthroughOptions
		wantCmd []string
	}{
		{
			name: "basic command without options",
			agent: &testAgent{
				id: "test-agent",
				StandardPassthrough: agents.StandardPassthrough{
					Cfg: agents.PassthroughConfig{Supported: true, PassthroughCmd: agents.NewCommand("test-cli", "--verbose")},
				},
			},
			opts:    agents.PassthroughOptions{Resume: true},
			wantCmd: []string{"test-cli", "--verbose"},
		},
		{
			name: "command with model",
			agent: &testAgent{
				id: "test-agent",
				StandardPassthrough: agents.StandardPassthrough{
					Cfg: agents.PassthroughConfig{Supported: true, PassthroughCmd: agents.NewCommand("test-cli"), ModelFlag: agents.NewParam("--model", "{model}")},
				},
			},
			opts:    agents.PassthroughOptions{Model: "gpt-4"},
			wantCmd: []string{"test-cli", "--model", "gpt-4"},
		},
		{
			name: "resume with single-word flag",
			agent: &testAgent{
				id: "test-agent",
				StandardPassthrough: agents.StandardPassthrough{
					Cfg: agents.PassthroughConfig{Supported: true, PassthroughCmd: agents.NewCommand("test-cli"), ResumeFlag: agents.NewParam("-c")},
				},
			},
			opts:    agents.PassthroughOptions{Resume: true},
			wantCmd: []string{"test-cli", "-c"},
		},
		{
			name: "resume with multi-word flag",
			agent: &testAgent{
				id: "gemini-agent",
				StandardPassthrough: agents.StandardPassthrough{
					Cfg: agents.PassthroughConfig{Supported: true, PassthroughCmd: agents.NewCommand("gemini"), ResumeFlag: agents.NewParam("--resume", "latest")},
				},
			},
			opts:    agents.PassthroughOptions{Resume: true},
			wantCmd: []string{"gemini", "--resume", "latest"},
		},
		{
			name: "permission settings as CLI flags",
			agent: &testAgent{
				id: "test-agent",
				StandardPassthrough: agents.StandardPassthrough{
					Cfg: agents.PassthroughConfig{Supported: true, PassthroughCmd: agents.NewCommand("test-cli")},
					PermSettings: map[string]agents.PermissionSetting{
						"auto_approve": {Supported: true, ApplyMethod: "cli_flag", CLIFlag: "--yes"},
					},
				},
			},
			opts: agents.PassthroughOptions{
				PermissionValues: map[string]bool{"auto_approve": true},
			},
			wantCmd: []string{"test-cli", "--yes"},
		},
		{
			name: "full resume with model + settings + resume flag",
			agent: &testAgent{
				id: "claude-code",
				StandardPassthrough: agents.StandardPassthrough{
					Cfg: agents.PassthroughConfig{
						Supported:      true,
						PassthroughCmd: agents.NewCommand("npx", "-y", "@anthropic-ai/claude-code"),
						ModelFlag:      agents.NewParam("--model", "{model}"),
						ResumeFlag:     agents.NewParam("-c"),
					},
					PermSettings: map[string]agents.PermissionSetting{
						"dangerously_skip_permissions": {Supported: true, ApplyMethod: "cli_flag", CLIFlag: "--dangerously-skip-permissions"},
					},
				},
			},
			opts: agents.PassthroughOptions{
				Model:            "claude-sonnet-4",
				Resume:           true,
				PermissionValues: map[string]bool{"dangerously_skip_permissions": true},
			},
			wantCmd: []string{"npx", "-y", "@anthropic-ai/claude-code", "--model", "claude-sonnet-4", "--dangerously-skip-permissions", "-c"},
		},
		{
			name: "permission setting with cli_flag_value",
			agent: &testAgent{
				id: "test-agent",
				StandardPassthrough: agents.StandardPassthrough{
					Cfg: agents.PassthroughConfig{Supported: true, PassthroughCmd: agents.NewCommand("test-cli")},
					PermSettings: map[string]agents.PermissionSetting{
						"auto_approve": {Supported: true, ApplyMethod: "cli_flag", CLIFlag: "--approve-level", CLIFlagValue: "all"},
					},
				},
			},
			opts: agents.PassthroughOptions{
				PermissionValues: map[string]bool{"auto_approve": true},
			},
			wantCmd: []string{"test-cli", "--approve-level", "all"},
		},
		{
			name: "new session with prompt (positional)",
			agent: &testAgent{
				id: "test-agent",
				StandardPassthrough: agents.StandardPassthrough{
					Cfg: agents.PassthroughConfig{Supported: true, PassthroughCmd: agents.NewCommand("test-cli")},
				},
			},
			opts:    agents.PassthroughOptions{Prompt: "fix the bug"},
			wantCmd: []string{"test-cli", "fix the bug"},
		},
		{
			name: "new session with prompt flag",
			agent: &testAgent{
				id: "test-agent",
				StandardPassthrough: agents.StandardPassthrough{
					Cfg: agents.PassthroughConfig{Supported: true, PassthroughCmd: agents.NewCommand("test-cli"), PromptFlag: agents.NewParam("--prompt", "{prompt}")},
				},
			},
			opts:    agents.PassthroughOptions{Prompt: "fix the bug"},
			wantCmd: []string{"test-cli", "--prompt", "fix the bug"},
		},
		{
			name: "resume with session ID",
			agent: &testAgent{
				id: "test-agent",
				StandardPassthrough: agents.StandardPassthrough{
					Cfg: agents.PassthroughConfig{
						Supported:         true,
						PassthroughCmd:    agents.NewCommand("test-cli"),
						SessionResumeFlag: agents.NewParam("--resume"),
					},
				},
			},
			opts:    agents.PassthroughOptions{SessionID: "sess-123"},
			wantCmd: []string{"test-cli", "--resume", "sess-123"},
		},
		{
			name: "mock agent resume with -c flag",
			agent: &testAgent{
				id: "mock-agent",
				StandardPassthrough: agents.StandardPassthrough{
					Cfg: agents.PassthroughConfig{
						Supported:      true,
						PassthroughCmd: agents.NewCommand("mock-agent", "--tui"),
						ModelFlag:      agents.NewParam("--model", "{model}"),
						ResumeFlag:     agents.NewParam("-c"),
					},
				},
			},
			opts:    agents.PassthroughOptions{Model: "mock-fast", Resume: true},
			wantCmd: []string{"mock-agent", "--tui", "--model", "mock-fast", "-c"},
		},
		{
			name: "mock agent session resume with --resume flag",
			agent: &testAgent{
				id: "mock-agent",
				StandardPassthrough: agents.StandardPassthrough{
					Cfg: agents.PassthroughConfig{
						Supported:         true,
						PassthroughCmd:    agents.NewCommand("mock-agent", "--tui"),
						ModelFlag:         agents.NewParam("--model", "{model}"),
						SessionResumeFlag: agents.NewParam("--resume"),
					},
				},
			},
			opts:    agents.PassthroughOptions{Model: "mock-fast", SessionID: "sess-123"},
			wantCmd: []string{"mock-agent", "--tui", "--model", "mock-fast", "--resume", "sess-123"},
		},
		{
			name: "user cli flag tokens appended after model + settings",
			agent: &testAgent{
				id: "test-agent",
				StandardPassthrough: agents.StandardPassthrough{
					Cfg: agents.PassthroughConfig{
						Supported:      true,
						PassthroughCmd: agents.NewCommand("test-cli"),
						ModelFlag:      agents.NewParam("--model", "{model}"),
						ResumeFlag:     agents.NewParam("-c"),
					},
					PermSettings: map[string]agents.PermissionSetting{
						"auto_approve": {Supported: true, ApplyMethod: "cli_flag", CLIFlag: "--yes"},
					},
				},
			},
			opts: agents.PassthroughOptions{
				Model:            "gpt-4",
				Resume:           true,
				PermissionValues: map[string]bool{"auto_approve": true},
				CLIFlagTokens:    []string{"--debug", "--log-level", "trace"},
			},
			wantCmd: []string{"test-cli", "--model", "gpt-4", "--yes", "--debug", "--log-level", "trace", "-c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.agent.BuildPassthroughCommand(tt.opts).Args()

			if len(got) != len(tt.wantCmd) {
				t.Errorf("BuildPassthroughCommand() = %v, want %v", got, tt.wantCmd)
				return
			}

			for i, arg := range got {
				if arg != tt.wantCmd[i] {
					t.Errorf("BuildPassthroughCommand()[%d] = %q, want %q", i, arg, tt.wantCmd[i])
				}
			}
		})
	}
}

// TestManager_HandlePassthroughExit_SkipsDuringShutdown verifies that the
// detached goroutine spawned when a passthrough child exits bails out
// immediately once graceful shutdown has begun, instead of racing the
// teardown and logging a spurious "failed to auto-restart passthrough
// session" error. Regression test for the Ctrl+C-in-terminal shutdown
// noise.
//
// Uses testing/synctest so the assertion is "the function returned without
// any time advancing" — i.e. it short-circuited before the cleanupDelay
// sleep. Under fake time, a non-short-circuit path would advance by
// cleanupDelay (and then take the nil-runner branch in the test rig).
func TestManager_HandlePassthroughExit_SkipsDuringShutdown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mgr := newTestManager()

		if mgr.IsShuttingDown() {
			t.Fatal("fresh manager reports IsShuttingDown() == true")
		}

		if err := mgr.StopAllAgents(context.Background()); err != nil {
			t.Fatalf("StopAllAgents returned error: %v", err)
		}
		if !mgr.IsShuttingDown() {
			t.Fatal("StopAllAgents did not set IsShuttingDown() = true")
		}

		execution := &AgentExecution{ID: "exec-1", SessionID: "sess-1"}
		status := &agentctltypes.ProcessStatusUpdate{SessionID: "sess-1"}

		start := time.Now()
		mgr.handlePassthroughExit(execution, status, start)
		if elapsed := time.Since(start); elapsed != 0 {
			t.Errorf("handlePassthroughExit advanced fake time by %v — did not short-circuit during shutdown", elapsed)
		}
	})
}

// TestIsFastFailExit covers the predicate that decides whether a passthrough
// exit looks like a launch failure (bad CLI flag, missing binary, auth
// rejection) and should bypass the auto-restart loop.
func TestIsFastFailExit(t *testing.T) {
	const window = 2 * time.Second
	now := time.Now()

	tests := []struct {
		name      string
		startedAt time.Time
		exitCode  int
		want      bool
	}{
		{
			name:      "fast exit with non-zero code → fast-fail",
			startedAt: now.Add(-100 * time.Millisecond),
			exitCode:  1,
			want:      true,
		},
		{
			name:      "slow exit with non-zero code → restart",
			startedAt: now.Add(-5 * time.Second),
			exitCode:  1,
			want:      false,
		},
		{
			name:      "fast exit with zero code → not fast-fail (clean exit)",
			startedAt: now.Add(-100 * time.Millisecond),
			exitCode:  0,
			want:      false,
		},
		{
			name:      "zero start time → check disabled (recovered execution)",
			startedAt: time.Time{},
			exitCode:  1,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isFastFailExit(tt.startedAt, tt.exitCode, window); got != tt.want {
				t.Errorf("isFastFailExit() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestManager_ProfileCLIFlagTokens confirms profile-configured cli_flags
// reach the passthrough launch path (regression for issue #718, where the
// passthrough builder silently dropped them).
func TestManager_ProfileCLIFlagTokens(t *testing.T) {
	mgr := newTestManager()

	t.Run("nil profile returns nil", func(t *testing.T) {
		if got := mgr.profileCLIFlagTokens(nil); got != nil {
			t.Errorf("profileCLIFlagTokens(nil) = %v, want nil", got)
		}
	})

	t.Run("enabled flags tokenised, disabled skipped", func(t *testing.T) {
		profile := &AgentProfileInfo{
			ProfileID: "p1",
			CLIFlags: []settingsmodels.CLIFlag{
				{Flag: "--allow-all-tools", Enabled: true},
				{Flag: "--skip-me", Enabled: false},
				{Flag: "--add-dir /shared", Enabled: true},
			},
		}
		got := mgr.profileCLIFlagTokens(profile)
		want := []string{"--allow-all-tools", "--add-dir", "/shared"}
		if len(got) != len(want) {
			t.Fatalf("profileCLIFlagTokens() = %v, want %v", got, want)
		}
		for i, tok := range want {
			if got[i] != tok {
				t.Errorf("profileCLIFlagTokens()[%d] = %q, want %q", i, got[i], tok)
			}
		}
	})

	t.Run("malformed flag does not abort — returns nil and warns", func(t *testing.T) {
		profile := &AgentProfileInfo{
			ProfileID: "p2",
			CLIFlags: []settingsmodels.CLIFlag{
				{Flag: `--broken "unterminated`, Enabled: true},
			},
		}
		if got := mgr.profileCLIFlagTokens(profile); got != nil {
			t.Errorf("profileCLIFlagTokens(malformed) = %v, want nil", got)
		}
	})
}

func TestManager_VerifyPassthroughEnabled(t *testing.T) {
	tests := []struct {
		name      string
		profileID string
		wantErr   bool
	}{
		{
			name:      "valid profile with passthrough enabled",
			profileID: "test-profile",
			wantErr:   false,
		},
		{
			name:      "empty profile ID",
			profileID: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newTestManager()

			// Override profile resolver for this test
			if tt.profileID != "" {
				mgr.profileResolver = &mockPassthroughProfileResolver{
					cliPassthrough: true,
				}
			}

			err := mgr.verifyPassthroughEnabled(context.Background(), "test-session", tt.profileID)
			if (err != nil) != tt.wantErr {
				t.Errorf("verifyPassthroughEnabled() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
