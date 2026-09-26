// service_interactions.go assembles the durable agent-interaction projection
// (ADR 0052) from the persisted permission_request / clarification_request
// message rows. It is the single place that knows how those rows encode an
// interaction, so the plugin Host interaction API and any future consumer read
// the same shape rather than re-deriving it from message metadata.
package service

import (
	"context"
	"sort"

	"github.com/kandev/kandev/internal/task/models"
)

// ListPendingInteractions returns every interaction still owed a user response
// under filter, newest last. Clarification bundles are collapsed from their
// per-question rows into a single Interaction carrying every question.
func (s *Service) ListPendingInteractions(
	ctx context.Context,
	filter models.PendingInteractionFilter,
) ([]*models.Interaction, error) {
	rows, err := s.messages.ListPendingInteractions(ctx, filter)
	if err != nil {
		return nil, err
	}
	return assembleInteractions(rows), nil
}

// GetInteraction returns one interaction by its pending id whatever its
// current status — including terminal ones. That is what makes an
// event-driven plugin cache reconcilable: a replayed or missed event resolves
// to the current durable result rather than to "not found". Returns
// (nil, nil) when no message carries the id.
func (s *Service) GetInteraction(ctx context.Context, pendingID string) (*models.Interaction, error) {
	if pendingID == "" {
		return nil, nil
	}
	rows, err := s.messages.FindMessagesByPendingID(ctx, pendingID)
	if err != nil {
		return nil, err
	}
	interactions := assembleInteractions(rows)
	if len(interactions) == 0 {
		return nil, nil
	}
	return interactions[0], nil
}

// assembleInteractions groups request messages by pending id and converts each
// group into one Interaction. Rows whose metadata carries no pending_id are
// dropped: without it there is no id a response could ever be keyed on.
func assembleInteractions(rows []*models.Message) []*models.Interaction {
	grouped := make(map[string][]*models.Message)
	order := make([]string, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		pendingID, _ := row.Metadata["pending_id"].(string)
		if pendingID == "" {
			continue
		}
		if _, seen := grouped[pendingID]; !seen {
			order = append(order, pendingID)
		}
		grouped[pendingID] = append(grouped[pendingID], row)
	}
	out := make([]*models.Interaction, 0, len(order))
	for _, pendingID := range order {
		if interaction := buildInteraction(pendingID, grouped[pendingID]); interaction != nil {
			out = append(out, interaction)
		}
	}
	return out
}

func buildInteraction(pendingID string, rows []*models.Message) *models.Interaction {
	if len(rows) == 0 {
		return nil
	}
	switch rows[0].Type {
	case models.MessageTypePermissionRequest:
		return buildPermissionInteraction(pendingID, rows[0])
	case models.MessageTypeClarificationRequest:
		return buildClarificationInteraction(pendingID, rows)
	default:
		return nil
	}
}

func buildPermissionInteraction(pendingID string, row *models.Message) *models.Interaction {
	toolCallID, _ := row.Metadata["tool_call_id"].(string)
	actionType, _ := row.Metadata["action_type"].(string)
	requestID, _ := row.Metadata["request_id"].(string)
	return &models.Interaction{
		ID:         pendingID,
		Kind:       models.InteractionKindPermission,
		TaskID:     row.TaskID,
		SessionID:  row.TaskSessionID,
		TurnID:     row.TurnID,
		Status:     models.InteractionStatusFromMetadata(row.Metadata),
		RequestID:  requestID,
		Title:      row.Content,
		ToolCallID: toolCallID,
		ActionType: actionType,
		Options:    permissionOptionsFromMetadata(row.Metadata),
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}
}

// buildClarificationInteraction collapses a bundle's per-question rows. The
// bundle's status is the status of its LEAST resolved question: a bundle is
// only terminal once every question is, because the agent stays blocked until
// the whole bundle is answered. Its timestamps span the bundle (earliest
// created, latest updated) so a cache can order it against sibling events.
func buildClarificationInteraction(pendingID string, rows []*models.Message) *models.Interaction {
	ordered := make([]*models.Message, len(rows))
	copy(ordered, rows)
	sort.SliceStable(ordered, func(i, j int) bool {
		return questionIndex(ordered[i].Metadata) < questionIndex(ordered[j].Metadata)
	})

	first := ordered[0]
	sharedContext, _ := first.Metadata["context"].(string)
	interaction := &models.Interaction{
		ID:        pendingID,
		Kind:      models.InteractionKindClarification,
		TaskID:    first.TaskID,
		SessionID: first.TaskSessionID,
		TurnID:    first.TurnID,
		Context:   sharedContext,
		CreatedAt: first.CreatedAt,
		UpdatedAt: first.UpdatedAt,
	}

	status := models.InteractionStatus("")
	for _, row := range ordered {
		interaction.Questions = append(interaction.Questions, questionFromMetadata(row.Metadata))
		if disconnected, _ := row.Metadata["agent_disconnected"].(bool); disconnected {
			interaction.AgentDisconnected = true
		}
		rowStatus := models.InteractionStatusFromMetadata(row.Metadata)
		if status == "" || (status.IsTerminal() && !rowStatus.IsTerminal()) {
			status = rowStatus
		}
		if row.CreatedAt.Before(interaction.CreatedAt) {
			interaction.CreatedAt = row.CreatedAt
		}
		if row.UpdatedAt.After(interaction.UpdatedAt) {
			interaction.UpdatedAt = row.UpdatedAt
		}
	}
	interaction.Status = status
	if len(interaction.Questions) > 0 {
		interaction.Title = interaction.Questions[0].Prompt
	}
	return interaction
}

func questionIndex(metadata map[string]interface{}) int {
	switch v := metadata["question_index"].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}

func questionFromMetadata(metadata map[string]interface{}) models.InteractionQuestion {
	questionID, _ := metadata["question_id"].(string)
	question := models.InteractionQuestion{ID: questionID}
	data, ok := metadata["question"].(map[string]interface{})
	if !ok {
		return question
	}
	if v, ok := data["prompt"].(string); ok {
		question.Prompt = v
	}
	if v, ok := data["title"].(string); ok {
		question.Title = v
	}
	if question.ID == "" {
		if v, ok := data["id"].(string); ok {
			question.ID = v
		}
	}
	question.Options = clarificationOptionsFromMetadata(data["options"])
	return question
}

func permissionOptionsFromMetadata(metadata map[string]interface{}) []models.InteractionOption {
	raw, ok := metadata["options"].([]interface{})
	if !ok {
		return nil
	}
	out := make([]models.InteractionOption, 0, len(raw))
	for _, entry := range raw {
		data, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		id, _ := data["option_id"].(string)
		if id == "" {
			continue
		}
		// The persisted permission option uses ACP's "name" for its human
		// label; the interaction contract calls every selectable label "label".
		label, _ := data["name"].(string)
		kind, _ := data["kind"].(string)
		out = append(out, models.InteractionOption{ID: id, Label: label, Kind: kind})
	}
	return out
}

func clarificationOptionsFromMetadata(value interface{}) []models.InteractionOption {
	raw, ok := value.([]interface{})
	if !ok {
		return nil
	}
	out := make([]models.InteractionOption, 0, len(raw))
	for _, entry := range raw {
		data, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		id, _ := data["option_id"].(string)
		if id == "" {
			continue
		}
		label, _ := data["label"].(string)
		description, _ := data["description"].(string)
		out = append(out, models.InteractionOption{ID: id, Label: label, Description: description})
	}
	return out
}
