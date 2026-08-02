package logger

import (
	"context"
	"io"
	"sync"
	"sync/atomic"

	"go.uber.org/zap/zapcore"
)

const (
	lossCapacity = iota
	lossReservedCapacity
	lossEntryTooLarge
	lossWriteError
	lossShutdownTimeout
	lossReasonCount
)

var lossReasonNames = [lossReasonCount]string{
	"capacity", "reserved_capacity", "entry_too_large", "write_error", "shutdown_timeout",
}

type asyncSinkConfig struct {
	Name            string
	MaxEntries      int
	MaxBytes        int
	ReservedEntries int
	ReservedBytes   int
	MaxEntryBytes   int
}

type SinkStatistics struct {
	Name         string                       `json:"name"`
	Accepted     map[string]uint64            `json:"accepted"`
	Lost         map[string]map[string]uint64 `json:"lost"`
	QueueEntries int                          `json:"queue_entries"`
	QueueBytes   int                          `json:"queue_bytes"`
}

type sinkEntry struct {
	level zapcore.Level
	data  []byte
}

type asyncSink struct {
	writer io.Writer
	cfg    asyncSinkConfig

	mu              sync.Mutex
	cond            *sync.Cond
	queue           []sinkEntry
	inFlight        *sinkEntry
	pendingEntries  int
	pendingBytes    int
	closing         bool
	timeoutRecorded bool
	done            chan struct{}
	accepted        [7]atomic.Uint64
	lost            [7][lossReasonCount]atomic.Uint64
}

func newAsyncSink(writer io.Writer, cfg asyncSinkConfig) *asyncSink {
	sink := &asyncSink{writer: writer, cfg: cfg, done: make(chan struct{})}
	sink.cond = sync.NewCond(&sink.mu)
	go sink.run()
	return sink
}

func (s *asyncSink) Enqueue(level zapcore.Level, data []byte) bool {
	levelIndex := sinkLevelIndex(level)
	if len(data) > s.cfg.MaxEntryBytes {
		s.lost[levelIndex][lossEntryTooLarge].Add(1)
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing {
		s.lost[levelIndex][lossShutdownTimeout].Add(1)
		return false
	}
	if s.exceedsTotalCapacity(len(data)) {
		s.lost[levelIndex][lossCapacity].Add(1)
		return false
	}
	if level < zapcore.WarnLevel && s.exceedsUnreservedCapacity(len(data)) {
		s.lost[levelIndex][lossReservedCapacity].Add(1)
		return false
	}
	copied := append([]byte(nil), data...)
	s.queue = append(s.queue, sinkEntry{level: level, data: copied})
	s.pendingEntries++
	s.pendingBytes += len(copied)
	s.accepted[levelIndex].Add(1)
	s.cond.Signal()
	return true
}

func (s *asyncSink) exceedsTotalCapacity(size int) bool {
	return s.pendingEntries+1 > s.cfg.MaxEntries || s.pendingBytes+size > s.cfg.MaxBytes
}

func (s *asyncSink) exceedsUnreservedCapacity(size int) bool {
	entryLimit := s.cfg.MaxEntries - s.cfg.ReservedEntries
	byteLimit := s.cfg.MaxBytes - s.cfg.ReservedBytes
	return s.pendingEntries+1 > entryLimit || s.pendingBytes+size > byteLimit
}

func (s *asyncSink) run() {
	defer close(s.done)
	for {
		entry, ok := s.next()
		if !ok {
			return
		}
		written, err := s.writer.Write(entry.data)
		if err != nil || written != len(entry.data) {
			s.lost[sinkLevelIndex(entry.level)][lossWriteError].Add(1)
		}
		s.complete(entry)
	}
}

func (s *asyncSink) next() (sinkEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for len(s.queue) == 0 && !s.closing {
		s.cond.Wait()
	}
	if len(s.queue) == 0 {
		return sinkEntry{}, false
	}
	entry := s.queue[0]
	s.queue[0] = sinkEntry{}
	s.queue = s.queue[1:]
	s.inFlight = &entry
	return entry, true
}

func (s *asyncSink) complete(entry sinkEntry) {
	s.mu.Lock()
	s.pendingEntries--
	s.pendingBytes -= len(entry.data)
	s.inFlight = nil
	s.mu.Unlock()
}

func (s *asyncSink) Stats() SinkStatistics {
	stats := SinkStatistics{
		Name: s.cfg.Name, Accepted: make(map[string]uint64), Lost: make(map[string]map[string]uint64),
	}
	for index, name := range sinkLevelNames {
		stats.Accepted[name] = s.accepted[index].Load()
		stats.Lost[name] = make(map[string]uint64)
		for reason := range lossReasonCount {
			stats.Lost[name][lossReasonNames[reason]] = s.lost[index][reason].Load()
		}
	}
	s.mu.Lock()
	stats.QueueEntries = s.pendingEntries
	stats.QueueBytes = s.pendingBytes
	s.mu.Unlock()
	return stats
}

func (s *asyncSink) Close(ctx context.Context) error {
	s.mu.Lock()
	if !s.closing {
		s.closing = true
		s.cond.Broadcast()
	}
	s.mu.Unlock()
	select {
	case <-s.done:
		if syncer, ok := s.writer.(interface{ Sync() error }); ok {
			return syncer.Sync()
		}
		return nil
	case <-ctx.Done():
		s.recordShutdownLoss()
		return ctx.Err()
	}
}

func (s *asyncSink) recordShutdownLoss() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timeoutRecorded {
		return
	}
	s.timeoutRecorded = true
	for _, entry := range s.queue {
		s.lost[sinkLevelIndex(entry.level)][lossShutdownTimeout].Add(1)
	}
	if s.inFlight != nil {
		s.lost[sinkLevelIndex(s.inFlight.level)][lossShutdownTimeout].Add(1)
	}
}

var sinkLevelNames = [7]string{"debug", "info", "warn", "error", "dpanic", "panic", "fatal"}

func sinkLevelIndex(level zapcore.Level) int {
	switch {
	case level <= zapcore.DebugLevel:
		return 0
	case level == zapcore.InfoLevel:
		return 1
	case level == zapcore.WarnLevel:
		return 2
	case level == zapcore.ErrorLevel:
		return 3
	case level == zapcore.DPanicLevel:
		return 4
	case level == zapcore.PanicLevel:
		return 5
	default:
		return 6
	}
}
