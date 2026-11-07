package backendapp

import (
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/system/sessioncapacity"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
	"go.uber.org/zap"
)

type sharedInferenceEndpoint interface {
	inferenceClientSource
	SetInferenceConsumer(agentruntime.InferenceConsumer)
}

func wireSharedInferenceCapacity(capacity *orchestrator.Service, settings *systemsettings.Store, log *zap.Logger, endpoints ...sharedInferenceEndpoint) *sharedInferenceConsumer {
	consumer := &sharedInferenceConsumer{capacity: capacity, populationLogger: newInferencePopulationLogger(log)}
	for _, endpoint := range endpoints {
		consumer.sources = append(consumer.sources, endpoint)
		endpoint.SetInferenceConsumer(consumer)
	}
	capacity.SetInferenceCapacityOwner(sessioncapacity.NewStore(settings), consumer.Refresh)
	return consumer
}
