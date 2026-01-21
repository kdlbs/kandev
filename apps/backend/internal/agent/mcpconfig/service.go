package mcpconfig

import (
	"context"
	"database/sql"
	"errors"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

// Repository defines the storage requirements for MCP config access.
type Repository interface {
	GetAgent(ctx context.Context, id string) (*models.Agent, error)
	GetAgentProfile(ctx context.Context, id string) (*models.AgentProfile, error)
	GetAgentProfileMcpConfig(ctx context.Context, profileID string) (*models.AgentProfileMcpConfig, error)
	UpsertAgentProfileMcpConfig(ctx context.Context, config *models.AgentProfileMcpConfig) error
}

var (
	ErrAgentNotFound        = errors.New("agent not found")
	ErrAgentProfileNotFound = errors.New("agent profile not found")
	ErrAgentMcpUnsupported  = errors.New("mcp not supported by agent")
)

// Service provides MCP configuration access and validation.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) GetConfigByProfileID(ctx context.Context, profileID string) (*ProfileConfig, error) {
	profile, err := s.repo.GetAgentProfile(ctx, profileID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAgentProfileNotFound
		}
		return nil, err
	}
	if profile == nil {
		return nil, ErrAgentProfileNotFound
	}

	agent, err := s.repo.GetAgent(ctx, profile.AgentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}
	if agent == nil {
		return nil, ErrAgentNotFound
	}
	if !agent.SupportsMCP {
		return nil, ErrAgentMcpUnsupported
	}

	config, err := s.repo.GetAgentProfileMcpConfig(ctx, profile.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &ProfileConfig{
				ProfileID:   profile.ID,
				ProfileName: profile.Name,
				AgentID:     agent.ID,
				AgentName:   agent.Name,
				Enabled:     false,
				Servers:     map[string]ServerDef{},
				Meta:        map[string]any{},
			}, nil
		}
		return nil, err
	}

	return &ProfileConfig{
		ProfileID:   profile.ID,
		ProfileName: profile.Name,
		AgentID:     agent.ID,
		AgentName:   agent.Name,
		Enabled:     config.Enabled,
		Servers:     castServerDefs(config.Servers),
		Meta:        config.Meta,
	}, nil
}

func (s *Service) UpsertConfigByProfileID(ctx context.Context, profileID string, config *ProfileConfig) (*ProfileConfig, error) {
	profile, err := s.repo.GetAgentProfile(ctx, profileID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAgentProfileNotFound
		}
		return nil, err
	}
	if profile == nil {
		return nil, ErrAgentProfileNotFound
	}

	agent, err := s.repo.GetAgent(ctx, profile.AgentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}
	if agent == nil {
		return nil, ErrAgentNotFound
	}
	if !agent.SupportsMCP {
		return nil, ErrAgentMcpUnsupported
	}
	if config == nil {
		config = &ProfileConfig{}
	}

	record := &models.AgentProfileMcpConfig{
		ProfileID: profile.ID,
		Enabled:   config.Enabled,
		Servers:   castServerInterfaces(config.Servers),
		Meta:      castMetaInterfaces(config.Meta),
	}
	if err := s.repo.UpsertAgentProfileMcpConfig(ctx, record); err != nil {
		return nil, err
	}

	return &ProfileConfig{
		ProfileID:   profile.ID,
		ProfileName: profile.Name,
		AgentID:     agent.ID,
		AgentName:   agent.Name,
		Enabled:     record.Enabled,
		Servers:     config.Servers,
		Meta:        config.Meta,
	}, nil
}

func castServerDefs(raw map[string]interface{}) map[string]ServerDef {
	if raw == nil {
		return map[string]ServerDef{}
	}
	servers := make(map[string]ServerDef, len(raw))
	for key, value := range raw {
		switch v := value.(type) {
		case ServerDef:
			servers[key] = v
		case map[string]interface{}:
			servers[key] = mapToServerDef(v)
		default:
			// ignore invalid types
		}
	}
	return servers
}

func castServerInterfaces(raw map[string]ServerDef) map[string]interface{} {
	if raw == nil {
		return map[string]interface{}{}
	}
	servers := make(map[string]interface{}, len(raw))
	for key, value := range raw {
		servers[key] = serverDefToMap(value)
	}
	return servers
}

func castMetaInterfaces(raw map[string]any) map[string]interface{} {
	if raw == nil {
		return map[string]interface{}{}
	}
	meta := make(map[string]interface{}, len(raw))
	for k, v := range raw {
		meta[k] = v
	}
	return meta
}

func mapToServerDef(value map[string]interface{}) ServerDef {
	def := ServerDef{}
	if v, ok := value["type"].(string); ok {
		def.Type = ServerType(v)
	}
	if v, ok := value["command"].(string); ok {
		def.Command = v
	}
	if v, ok := value["args"].([]interface{}); ok {
		def.Args = interfaceSliceToStrings(v)
	}
	if v, ok := value["args"].([]string); ok {
		def.Args = append([]string{}, v...)
	}
	if v, ok := value["url"].(string); ok {
		def.URL = v
	}
	if v, ok := value["mode"].(string); ok {
		def.Mode = ServerMode(v)
	}
	if v, ok := value["env"].(map[string]interface{}); ok {
		def.Env = interfaceMapToStringMap(v)
	}
	if v, ok := value["headers"].(map[string]interface{}); ok {
		def.Headers = interfaceMapToStringMap(v)
	}
	if v, ok := value["meta"].(map[string]interface{}); ok {
		def.Meta = v
	}
	if v, ok := value["extra"].(map[string]interface{}); ok {
		def.Extra = v
	}
	return def
}

func serverDefToMap(def ServerDef) map[string]interface{} {
	out := map[string]interface{}{}
	if def.Type != "" {
		out["type"] = string(def.Type)
	}
	if def.Command != "" {
		out["command"] = def.Command
	}
	if len(def.Args) > 0 {
		args := make([]interface{}, 0, len(def.Args))
		for _, arg := range def.Args {
			args = append(args, arg)
		}
		out["args"] = args
	}
	if def.URL != "" {
		out["url"] = def.URL
	}
	if def.Mode != "" {
		out["mode"] = string(def.Mode)
	}
	if len(def.Env) > 0 {
		out["env"] = stringMapToInterfaceMap(def.Env)
	}
	if len(def.Headers) > 0 {
		out["headers"] = stringMapToInterfaceMap(def.Headers)
	}
	if len(def.Meta) > 0 {
		out["meta"] = def.Meta
	}
	if len(def.Extra) > 0 {
		out["extra"] = def.Extra
	}
	return out
}

func interfaceSliceToStrings(raw []interface{}) []string {
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

func interfaceMapToStringMap(raw map[string]interface{}) map[string]string {
	result := make(map[string]string, len(raw))
	for k, v := range raw {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}
	return result
}

func stringMapToInterfaceMap(raw map[string]string) map[string]interface{} {
	result := make(map[string]interface{}, len(raw))
	for k, v := range raw {
		result[k] = v
	}
	return result
}
