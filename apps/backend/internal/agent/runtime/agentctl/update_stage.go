package client

import (
	"sync/atomic"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type UpdateStreamStageSnapshot struct {
	ReadCount            uint64
	HandledCount         uint64
	Pending              uint64
	LastReadAgeMillis    int64
	LastHandledAgeMillis int64
	HeartbeatAgeMillis   int64
	Connected            bool
	AgentctlStage        streams.UpdateStreamStageHeartbeat
}

type updateStreamStage struct {
	readCount    atomic.Uint64
	handledCount atomic.Uint64
	readAt       atomic.Int64
	handledAt    atomic.Int64
	heartbeatAt  atomic.Int64
	heartbeat    atomic.Pointer[streams.UpdateStreamStageHeartbeat]
	connected    atomic.Bool
}

func newUpdateStreamStage() *updateStreamStage { return &updateStreamStage{} }
func (s *updateStreamStage) recordRead() {
	s.readAt.Store(time.Now().UnixNano())
	s.readCount.Add(1)
}
func (s *updateStreamStage) recordHandled() {
	s.handledAt.Store(time.Now().UnixNano())
	s.handledCount.Add(1)
}
func (s *updateStreamStage) snapshot() UpdateStreamStageSnapshot {
	read := s.readCount.Load()
	handled := s.handledCount.Load()
	snapshot := UpdateStreamStageSnapshot{
		ReadCount: read, HandledCount: handled, Pending: read - min(read, handled),
		LastReadAgeMillis:    stageAgeMillis(s.readAt.Load()),
		LastHandledAgeMillis: stageAgeMillis(s.handledAt.Load()),
		HeartbeatAgeMillis:   stageAgeMillis(s.heartbeatAt.Load()),
		Connected:            s.connected.Load(),
		AgentctlStage:        streams.UpdateStreamStageHeartbeat{AdapterAgeMillis: -1, WriterAgeMillis: -1},
	}
	if heartbeat := s.heartbeat.Load(); heartbeat != nil {
		snapshot.AgentctlStage = *heartbeat
	}
	return snapshot
}

func stageAgeMillis(unixNano int64) int64 {
	if unixNano == 0 {
		return -1
	}
	return max(0, time.Since(time.Unix(0, unixNano)).Milliseconds())
}

func (*Client) consumeUpdateStreamMetadata(stage *updateStreamStage, msg ws.Message) bool {
	if msg.Type != ws.MessageTypeNotification || msg.Action != streams.UpdateStreamStageAction {
		return false
	}
	var heartbeat streams.UpdateStreamStageHeartbeat
	if err := msg.ParsePayload(&heartbeat); err == nil {
		stage.heartbeat.Store(&heartbeat)
		stage.heartbeatAt.Store(time.Now().UnixNano())
	}
	return true
}

// UpdateStreamStage reports only the currently installed stream generation.
func (c *Client) UpdateStreamStage() (UpdateStreamStageSnapshot, bool) {
	stage := c.updateStage.Load()
	if stage == nil {
		return UpdateStreamStageSnapshot{}, false
	}
	return stage.snapshot(), true
}
