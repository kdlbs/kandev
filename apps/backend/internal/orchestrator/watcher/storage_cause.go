package watcher

import "github.com/kandev/kandev/internal/task/models"

// withDeliveryStorageCause projects validated terminal storage evidence into
// the existing bounded cause list. It never classifies the error message.
func withDeliveryStorageCause(data AgentEventData) AgentEventData {
	if data.Storage == nil {
		return data
	}
	cause := models.AgentErrorCause{
		Operation: models.AgentErrorCauseOperationDelivery,
		Code:      models.AgentErrorCauseCodeDeliveryStorage,
		Storage:   data.Storage,
	}
	data.Causes = models.NormalizeAgentErrorCauses(append([]models.AgentErrorCause{cause}, data.Causes...))
	return data
}
