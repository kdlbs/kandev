package controller

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9]+`)

// slugify converts a display name into a URL-friendly slug.
func slugify(displayName string) string {
	s := strings.ToLower(strings.TrimSpace(displayName))
	s = nonAlphanumeric.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// CreateCustomTUIAgentRequest is the request to create a custom TUI agent.
type CreateCustomTUIAgentRequest struct {
	DisplayName string
	Model       string
	Command     string
	Description string
}

// CreateCustomTUIAgent registers a new custom TUI agent and persists it to the database.
func (c *Controller) CreateCustomTUIAgent(ctx context.Context, req CreateCustomTUIAgentRequest) (*dto.AgentDTO, error) {
	slug := slugify(req.DisplayName)
	if slug == "" {
		return nil, ErrInvalidSlug
	}
	if req.Command == "" {
		return nil, ErrCommandRequired
	}

	// Check for conflict with existing registry entry
	if c.agentRegistry.Exists(slug) {
		return nil, ErrAgentAlreadyExists
	}

	// Check for conflict with existing DB entry
	existing, err := c.repo.GetAgentByName(ctx, slug)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if existing != nil {
		return nil, ErrAgentAlreadyExists
	}

	// Register in the in-memory registry
	if regErr := c.agentRegistry.RegisterCustomTUIAgent(
		slug, req.DisplayName, req.Command, req.Description, req.Model, nil,
	); regErr != nil {
		return nil, fmt.Errorf("failed to register agent: %w", regErr)
	}

	// Persist to DB
	tuiConfig := &models.TUIConfigJSON{
		Command:         req.Command,
		DisplayName:     req.DisplayName,
		Model:           req.Model,
		Description:     req.Description,
		WaitForTerminal: true,
	}
	agent := &models.Agent{
		Name:      slug,
		TUIConfig: tuiConfig,
	}
	if err := c.repo.CreateAgent(ctx, agent); err != nil {
		// Rollback registry on DB failure
		_ = c.agentRegistry.Unregister(slug)
		return nil, err
	}

	// Create default passthrough profile — use model as profile name for distinct dropdown labels
	profileName := req.Model
	if profileName == "" {
		profileName = req.DisplayName
	}
	profile := &models.AgentProfile{
		AgentID:          agent.ID,
		Name:             profileName,
		AgentDisplayName: req.DisplayName,
		Model:            "passthrough",
		CLIPassthrough:   true,
	}
	if err := c.repo.CreateAgentProfile(ctx, profile); err != nil {
		return nil, err
	}

	profiles := []*models.AgentProfile{profile}
	result := toAgentDTO(agent, profiles)
	return &result, nil
}
