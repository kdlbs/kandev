package process

import (
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func (m *Manager) UpdateStreamStage() streams.UpdateStreamStageHeartbeat {
	stage := streams.UpdateStreamStageHeartbeat{
		AdapterSeen: m.adapterStageSeen.Load(), AdapterAgeMillis: -1, WriterAgeMillis: -1,
	}
	if at := m.adapterStageAt.Load(); at != 0 {
		stage.AdapterAgeMillis = max(0, time.Since(time.Unix(0, at)).Milliseconds())
	}
	return stage
}
