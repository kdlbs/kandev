package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
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
