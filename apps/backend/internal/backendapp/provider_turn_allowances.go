package backendapp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type providerTurnAllowanceStore interface {
	GetProviderTurnAllowance(context.Context, string) (*agentusage.TurnAllowanceRecord, error)
	PatchProviderTurnAllowance(context.Context, string, string, json.RawMessage) (*agentusage.TurnAllowanceRecord, error)
	HasProviderTurnAllowance(context.Context, string) (bool, error)
	AcquireProviderTurn(context.Context, string, tasksqlite.ProviderTurnClaim, ...[]tasksqlite.ProviderTurnClaim) (bool, error)
	ProviderTurnClaimExists(context.Context, string, tasksqlite.ProviderTurnClaim) (bool, error)
	FinishProviderTurn(context.Context, string, uint64, bool) error
	CompleteProviderExecution(context.Context, string) error
}

type providerTurnAllowances struct {
	*profileUsageSnapshot
	claims      providerTurnAllowanceStore
	admissionMu sync.Mutex
}

func (s *providerTurnAllowances) profileBinding(ctx context.Context, id, model string, actual ...map[string]string) (snapshotBinding, error) {
	profile, err := s.store.GetAgentProfile(ctx, id)
	if err != nil {
		return snapshotBinding{}, err
	}
	if profile == nil || profile.DeletedAt != nil {
		return snapshotBinding{}, agentusage.ErrTurnAllowanceUnknown
	}
	agents, err := s.store.ListAgents(ctx)
	if err != nil {
		return snapshotBinding{}, err
	}
	for _, agent := range agents {
		if agent != nil && agent.ID == profile.AgentID {
			copy := *profile
			if model != "" {
				copy.Model = model
			}
			if len(actual) > 0 {
				return s.bindWithProviderSelection(&copy, agent.Name, snapshotClaudeProviderUncertain(&copy, actual[0])), nil
			}
			return s.bind(&copy, agent.Name), nil
		}
	}
	return snapshotBinding{}, agentusage.ErrTurnAllowanceUnknown
}

func allowanceBindingKnown(b snapshotBinding, home string) bool {
	if b.key == "" {
		return false
	}
	if b.entry.Provider == "opencode-go" {
		return agentusage.NewOpenCodeGoUsageClient(agentusage.OpenCodeAuthPath(home)).HasCredentials()
	}
	if b.client == nil {
		return false
	}
	return b.entry.BindingScope == snapshotHostDefault || b.entry.BindingScope == "local_proxy"
}

func (s *providerTurnAllowances) GetProfileTurnAllowance(ctx context.Context, id string) (*dto.ProfileTurnAllowanceDTO, error) {
	b, err := s.profileBinding(ctx, id, "")
	if err != nil {
		return nil, err
	}
	if !allowanceBindingKnown(b, s.home) {
		return nil, agentusage.ErrTurnAllowanceUnknown
	}
	row, err := s.claims.GetProviderTurnAllowance(ctx, b.key)
	if err != nil {
		return nil, err
	}
	return profileTurnAllowanceDTO(id, b, row), nil
}

func (s *providerTurnAllowances) PatchProfileTurnAllowance(ctx context.Context, id string, patch json.RawMessage) (*dto.ProfileTurnAllowanceDTO, error) {
	s.admissionMu.Lock()
	defer s.admissionMu.Unlock()
	b, err := s.profileBinding(ctx, id, "")
	if err != nil {
		return nil, err
	}
	if !allowanceBindingKnown(b, s.home) {
		return nil, agentusage.ErrTurnAllowanceUnknown
	}
	row, err := s.claims.PatchProviderTurnAllowance(ctx, b.key, b.entry.Provider, patch)
	if err != nil {
		return nil, err
	}
	return profileTurnAllowanceDTO(id, b, row), nil
}

func profileTurnAllowanceDTO(id string, b snapshotBinding, row *agentusage.TurnAllowanceRecord) *dto.ProfileTurnAllowanceDTO {
	result := &dto.ProfileTurnAllowanceDTO{ProfileID: id, Provider: b.entry.Provider, BindingScope: b.entry.BindingScope,
		Unit: agentusage.KandevTurnUnit, ProviderSpendState: "unknown", Owner: json.RawMessage(`{}`), Learned: json.RawMessage(`{}`)}
	if row != nil {
		result.Owner, result.Learned, result.UpdatedAt = row.Owner, row.Learned, &row.UpdatedAt
	}
	return result
}

func (s *providerTurnAllowances) AdmitProviderTurn(ctx context.Context, req agentruntime.ProviderTurnRequest) (bool, error) {
	s.admissionMu.Lock()
	defer s.admissionMu.Unlock()
	b, err := s.profileBinding(ctx, req.ProfileID, req.Model, req.Environment)
	if err != nil {
		return false, s.unresolvedProviderTurn(ctx, req, b)
	}
	if !s.turnRequestBindingKnown(req, b) {
		return false, s.unresolvedProviderTurn(ctx, req, b)
	}
	population, err := s.incumbentTurns(ctx, b.key, req)
	if err != nil {
		return false, err
	}
	claim := providerTurnClaim(req)
	refund, err := s.claims.AcquireProviderTurn(ctx, b.key, claim, population)
	if err != nil {
		return false, err
	}
	tracked, err := s.claims.ProviderTurnClaimExists(ctx, b.key, claim)
	if err != nil {
		return false, err
	}
	if !tracked {
		recordUnmeteredTurn(req)
	}
	return refund, nil
}

func recordUnmeteredTurn(req agentruntime.ProviderTurnRequest) {
	if req.RecordUnmetered != nil {
		req.RecordUnmetered()
	}
}

func providerTurnClaim(req agentruntime.ProviderTurnRequest) tasksqlite.ProviderTurnClaim {
	id := fmt.Sprintf("execution:%s:%d", req.ExecutionID, req.Generation)
	if req.SubmissionID != "" {
		id = "submission:" + req.SubmissionID
	}
	return tasksqlite.ProviderTurnClaim{ID: id, ExecutionID: req.ExecutionID, SessionID: req.SessionID, Generation: req.Generation, SubmissionID: req.SubmissionID}
}

func (s *providerTurnAllowances) incumbentTurns(ctx context.Context, key string, req agentruntime.ProviderTurnRequest) ([]tasksqlite.ProviderTurnClaim, error) {
	if req.Population == nil {
		return nil, nil
	}
	row, err := s.claims.GetProviderTurnAllowance(ctx, key)
	if err != nil || row == nil {
		return nil, err
	}
	policy, err := agentusage.DecodeTurnAllowance(row.Owner)
	if err != nil {
		return nil, err
	}
	if policy.ConcurrentTurns == nil || *policy.ConcurrentTurns == 0 {
		return nil, nil
	}
	var result []tasksqlite.ProviderTurnClaim
	for _, active := range req.Population() {
		bound, err := s.profileBinding(ctx, active.ProfileID, active.Model, active.Environment)
		if !allowanceMayUseProvider(active, bound, row.Provider) {
			continue
		}
		if err != nil || !s.turnRequestBindingKnown(active, bound) {
			return nil, agentusage.ErrTurnAllowanceUnknown
		}
		if bound.key != key {
			continue
		}
		if active.Generation == 0 {
			return nil, agentusage.ErrTurnAllowanceUnknown
		}
		result = append(result, providerTurnClaim(active))
	}
	return result, nil
}

func (s *providerTurnAllowances) turnRequestBindingKnown(req agentruntime.ProviderTurnRequest, b snapshotBinding) bool {
	if req.AgentID == "opencode-acp" && !strings.HasPrefix(req.Model, "opencode-go/") && !strings.HasPrefix(req.Model, "opencode/") {
		return false
	}
	return req.AgentID == b.agentType && localAllowanceExecution(req.ExecutorType) && allowanceBindingKnown(b, s.home) && allowanceEnvironmentMatches(req.Environment, b.profile)
}

func (s *providerTurnAllowances) unresolvedProviderTurn(ctx context.Context, req agentruntime.ProviderTurnRequest, b snapshotBinding) error {
	families := allowanceProviderFamilies(req, b)
	if len(families) == 0 {
		families = []string{""}
	}
	for _, provider := range families {
		configured, err := s.claims.HasProviderTurnAllowance(ctx, provider)
		if err != nil {
			return err
		}
		if configured {
			return agentusage.ErrTurnAllowanceUnknown
		}
	}
	recordUnmeteredTurn(req)
	return nil
}

func allowanceMayUseProvider(req agentruntime.ProviderTurnRequest, b snapshotBinding, provider string) bool {
	families := allowanceProviderFamilies(req, b)
	if len(families) == 0 {
		return true
	}
	for _, family := range families {
		if family == provider {
			return true
		}
	}
	return false
}

// Only native CLI families establish that an unresolved account is unrelated.
func allowanceProviderFamilies(req agentruntime.ProviderTurnRequest, b snapshotBinding) []string {
	if b.profile == nil || req.AgentID != b.agentType || b.profile.ProviderKind != "" || b.profile.CommandPrefix != "" {
		return nil
	}
	if req.AgentID == claudeACPAgentID && snapshotClaudeProviderUncertain(b.profile, req.Environment) {
		return nil
	}
	switch req.AgentID {
	case claudeACPAgentID:
		return []string{"anthropic", "teamclaude"}
	case snapshotCodexACPAgentID, snapshotCodexAppServerAgentID:
		return []string{"openai"}
	case "opencode-acp":
		if strings.HasPrefix(req.Model, "opencode-go/") {
			return []string{"opencode-go"}
		}
		if strings.HasPrefix(req.Model, "opencode/") {
			return []string{"opencode-zen"}
		}
	}
	return nil
}

func localAllowanceExecution(executor string) bool {
	switch executor {
	case hostSourceLocal, legacyLocalPCExecutor, "worktree", "standalone":
		return true
	}
	return false
}

// Compare only credential-affecting values; no value leaves this process.
func allowanceEnvironmentMatches(actual map[string]string, profile *settingsmodels.AgentProfile) bool {
	if actual == nil || profile == nil {
		return false
	}
	expected := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			expected[snapshotEnvKey(key)] = value
		}
	}
	for _, entry := range profile.EnvVars {
		key := snapshotEnvKey(entry.Key)
		if isClaudeProviderSelector(key) {
			continue
		}
		if !allowanceCredentialEnvKey(key) {
			continue
		}
		if entry.SecretID != "" {
			return false
		}
		expected[key] = entry.Value
	}
	for key, value := range actual {
		key = snapshotEnvKey(key)
		if isClaudeProviderSelector(key) {
			continue
		}
		if allowanceCredentialEnvKey(key) && value != expected[key] {
			return false
		}
	}
	return true
}

func allowanceCredentialEnvKey(key string) bool {
	if isClaudeProviderSelector(key) {
		return true
	}
	switch key {
	case "HOME", "USERPROFILE", "XDG_DATA_HOME", "CODEX_HOME", "CLAUDE_CONFIG_DIR", "OPENCODE_CONFIG", "OPENCODE_CONFIG_DIR", "OPENCODE_CONFIG_CONTENT",
		"ANTHROPIC_API_KEY", "CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_BASE_URL", "OPENAI_API_KEY", "OPENAI_BASE_URL", "USAGE_PROXY_VENDOR":
		return true
	}
	return false
}

func (s *providerTurnAllowances) FinishProviderTurn(ctx context.Context, execution string, generation uint64, refund bool) error {
	return s.claims.FinishProviderTurn(ctx, execution, generation, refund)
}

func (s *providerTurnAllowances) CompleteProviderExecution(ctx context.Context, execution string) error {
	return s.claims.CompleteProviderExecution(ctx, execution)
}
