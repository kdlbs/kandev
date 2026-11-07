package api

import (
	"encoding/json"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

func (s *Server) writeUpdateStreamStageHeartbeat(writeMessage func([]byte) error, sent uint64, lastSent time.Time) error {
	stage := streams.UpdateStreamStageHeartbeat{AdapterAgeMillis: -1, WriterAgeMillis: -1}
	if s.procMgr != nil {
		stage = s.procMgr.UpdateStreamStage()
	}
	stage.WriterSent = sent
	if !lastSent.IsZero() {
		stage.WriterAgeMillis = max(0, time.Since(lastSent).Milliseconds())
	}
	msg, err := ws.NewNotification(streams.UpdateStreamStageAction, stage)
	if err != nil {
		return err
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return writeMessage(data)
}

// streamStageWriter belongs to one stream writer goroutine. Event and
// heartbeat writes share that goroutine and never change replay ordering.
type streamStageWriter struct {
	server   *Server
	write    func([]byte) error
	ticker   *time.Ticker
	sent     uint64
	lastSent time.Time
}

func streamStageInputs(write func([]byte) error, stages []*streamStageWriter) (*streamStageWriter, <-chan time.Time, func([]byte) error) {
	if len(stages) == 0 || stages[0] == nil {
		return nil, nil, write
	}
	stage := stages[0]
	return stage, stage.ticks(), stage.writeEvent
}

func (s *Server) newStreamStageWriter(write func([]byte) error, enabled bool) *streamStageWriter {
	w := &streamStageWriter{server: s, write: write}
	if enabled {
		w.ticker = time.NewTicker(10 * time.Second)
	}
	return w
}

func (w *streamStageWriter) ticks() <-chan time.Time {
	if w == nil || w.ticker == nil {
		return nil
	}
	return w.ticker.C
}

func (w *streamStageWriter) stop() {
	if w.ticker != nil {
		w.ticker.Stop()
	}
}

func (w *streamStageWriter) writeEvent(data []byte) error {
	if err := w.write(data); err != nil {
		return err
	}
	w.sent++
	w.lastSent = time.Now()
	return nil
}

func (w *streamStageWriter) heartbeat() bool {
	if err := w.server.writeUpdateStreamStageHeartbeat(w.write, w.sent, w.lastSent); err != nil {
		w.server.logger.Debug("failed to write update stream stage heartbeat", zap.Error(err))
		return false
	}
	return true
}
