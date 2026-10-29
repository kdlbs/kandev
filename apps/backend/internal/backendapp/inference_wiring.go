package backendapp

import (
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/system/sessioncapacity"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
)

type sharedInferenceEndpoint interface {
	inferenceClientSource
	SetInferenceConsumer(agentruntime.InferenceConsumer)
}

func wireSharedInferenceCapacity(capacity *orchestrator.Service, settings *systemsettings.Store, endpoints ...sharedInferenceEndpoint) {
	consumer := &sharedInferenceConsumer{capacity: capacity}
	for _, endpoint := range endpoints {
		consumer.sources = append(consumer.sources, endpoint)
		endpoint.SetInferenceConsumer(consumer)
	}
	capacity.SetInferenceCapacityOwner(sessioncapacity.NewStore(settings), consumer.Refresh)
}
