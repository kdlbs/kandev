package agents

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/usage"
)

// TestProjectSkillDir_AllAgentTypes verifies that every registered agent type
// sets a non-empty ProjectSkillDir in its RuntimeConfig, and that the fallback
// function ProjectSkillDirFromRuntime returns a sensible default when the field
// is absent.
func TestProjectSkillDir_AllAgentTypes(t *testing.T) {
	allAgents := []Agent{
		NewClaudeACP(),
		NewCodexACP(),
		NewOpenCodeACP(),
		NewGemini(),
		NewCopilotACP(),
		NewAuggie(),
		NewAmpACP(),
		NewMockAgent(),
	}

	for _, a := range allAgents {
		t.Run(a.ID(), func(t *testing.T) {
			rt := a.Runtime()
			if rt == nil {
				t.Fatal("Runtime() returned nil")
			}
			dir := ProjectSkillDirFromRuntime(a)
			if dir == "" {
				t.Errorf("ProjectSkillDirFromRuntime(%q) returned empty string", a.ID())
			}
		})
	}
}

// TestProjectSkillDir_ClaudeUsesClaudeDir verifies that Claude uses .claude/skills
// (not .agents/skills) as its project skill directory.
func TestProjectSkillDir_ClaudeUsesClaudeDir(t *testing.T) {
	a := NewClaudeACP()
	dir := ProjectSkillDirFromRuntime(a)
	if dir != ".claude/skills" {
		t.Errorf("claude-acp ProjectSkillDir = %q, want %q", dir, ".claude/skills")
	}
}

// TestProjectSkillDir_OthersUseAgentsDir verifies that non-Claude agents use
// .agents/skills as their project skill directory.
func TestProjectSkillDir_OthersUseAgentsDir(t *testing.T) {
	others := []Agent{
		NewCodexACP(),
		NewOpenCodeACP(),
		NewGemini(),
		NewCopilotACP(),
		NewAuggie(),
		NewAmpACP(),
	}
	for _, a := range others {
		t.Run(a.ID(), func(t *testing.T) {
			dir := ProjectSkillDirFromRuntime(a)
			if dir != DefaultProjectSkillDir {
				t.Errorf("%s ProjectSkillDir = %q, want %q", a.ID(), dir, DefaultProjectSkillDir)
			}
		})
	}
}

// TestProjectSkillDirFromRuntime_Fallback verifies that an agent with empty
// ProjectSkillDir in RuntimeConfig falls back to DefaultProjectSkillDir.
func TestProjectSkillDirFromRuntime_Fallback(t *testing.T) {
	// Create an agent that returns a RuntimeConfig with no ProjectSkillDir set.
	a := &emptySkillDirAgent{}
	dir := ProjectSkillDirFromRuntime(a)
	if dir != DefaultProjectSkillDir {
		t.Errorf("fallback ProjectSkillDir = %q, want %q", dir, DefaultProjectSkillDir)
	}
}

func TestUserSkillDir_KnownProviders(t *testing.T) {
	tests := []struct {
		name string
		a    Agent
		want string
	}{
		{"claude", NewClaudeACP(), ".claude/skills"},
		{"codex", NewCodexACP(), ".codex/skills"},
		{"opencode", NewOpenCodeACP(), ".config/opencode/skills"},
		{"copilot", NewCopilotACP(), ".copilot/skills"},
		{"mock-agent", NewMockAgent(), ".mock-agent/skills"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UserSkillDirFromRuntime(tt.a)
			if got != tt.want {
				t.Errorf("UserSkillDirFromRuntime(%q) = %q, want %q", tt.a.ID(), got, tt.want)
			}
		})
	}
}

func TestUserSkillDirFromRuntime_EmptyWhenUnset(t *testing.T) {
	a := &emptySkillDirAgent{}
	if got := UserSkillDirFromRuntime(a); got != "" {
		t.Errorf("fallback UserSkillDir = %q, want empty", got)
	}
}

// emptySkillDirAgent is a minimal Agent implementation used only in tests to
// verify the fallback behaviour of ProjectSkillDirFromRuntime.
type emptySkillDirAgent struct{}

func (e *emptySkillDirAgent) ID() string                { return "test-empty" }
func (e *emptySkillDirAgent) Name() string              { return "Test Empty" }
func (e *emptySkillDirAgent) DisplayName() string       { return "Test" }
func (e *emptySkillDirAgent) Description() string       { return "" }
func (e *emptySkillDirAgent) Enabled() bool             { return true }
func (e *emptySkillDirAgent) DisplayOrder() int         { return 0 }
func (e *emptySkillDirAgent) Logo(_ LogoVariant) []byte { return nil }
func (e *emptySkillDirAgent) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	return &DiscoveryResult{}, nil
}
func (e *emptySkillDirAgent) BuildCommand(_ CommandOptions) Command {
	return Command{}
}
func (e *emptySkillDirAgent) PermissionSettings() map[string]PermissionSetting { return nil }
func (e *emptySkillDirAgent) Runtime() *RuntimeConfig {
	return &RuntimeConfig{} // ProjectSkillDir intentionally left empty
}
func (e *emptySkillDirAgent) BillingType() usage.BillingType { return usage.BillingTypeAPIKey }
func (e *emptySkillDirAgent) RemoteAuth() *RemoteAuth        { return nil }
func (e *emptySkillDirAgent) InstallScript() string          { return "" }
