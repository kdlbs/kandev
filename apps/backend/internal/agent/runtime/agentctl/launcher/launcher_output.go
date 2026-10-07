package launcher

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"

	"go.uber.org/zap"
)

const (
	pipeOutputReadBufferBytes  = 4 * 1024
	pipeOutputRecordLimitBytes = 64 * 1024
)

// pipeOutput drains one child stream with fixed buffers and forwards only
// complete records that fit within the record limit.
func (l *Launcher) pipeOutput(name string, input io.Reader) {
	reader := bufio.NewReaderSize(input, pipeOutputReadBufferBytes)
	record := make([]byte, 0, pipeOutputRecordLimitBytes)
	discarding := false
	warnedOversized := false

	for {
		fragment, readErr := reader.ReadSlice('\n')
		terminated := readErr == nil
		content := fragment
		if terminated {
			content = content[:len(content)-1]
		}

		if !discarding && len(record)+len(content) > pipeOutputRecordLimitBytes {
			discarding = true
			record = record[:0]
			if !warnedOversized {
				l.logger.Warn(
					"discarded oversized agentctl log record",
					zap.String("stream", name),
					zap.Int("limit_bytes", pipeOutputRecordLimitBytes),
				)
				warnedOversized = true
			}
		}
		if !discarding {
			record = append(record, content...)
		}

		if terminated {
			if !discarding {
				l.forwardChildLogRecord(name, record)
			}
			discarding = false
			record = record[:0]
			continue
		}

		switch {
		case errors.Is(readErr, bufio.ErrBufferFull):
			continue
		case errors.Is(readErr, io.EOF):
			if !discarding && len(record) > 0 {
				l.forwardChildLogRecord(name, record)
			}
			return
		case isClosedPipeError(readErr):
			return
		default:
			l.logger.Warn(
				"agentctl log stream read failed",
				zap.String("stream", name),
				zap.String("error_class", pipeReadErrorClass(readErr)),
			)
			return
		}
	}
}

func (l *Launcher) forwardChildLogRecord(name string, record []byte) {
	line := bytes.TrimSuffix(record, []byte{'\r'})
	level, message := childLogRecord(string(line))
	if level == "" {
		if name == "stderr" {
			l.logger.Warn(string(line), zap.String("stream", name))
		} else {
			l.logger.Debug(string(line), zap.String("stream", name))
		}
		return
	}
	switch level {
	case "DEBUG":
		l.logger.Debug(message, zap.String("stream", name))
	case "INFO":
		l.logger.Info(message, zap.String("stream", name))
	case "ERROR", "FATAL", "PANIC", "DPANIC":
		l.logger.Error(message, zap.String("stream", name))
	default:
		l.logger.Warn(message, zap.String("stream", name))
	}
}

func isClosedPipeError(err error) bool {
	return errors.Is(err, os.ErrClosed) || errors.Is(err, io.ErrClosedPipe)
}

func pipeReadErrorClass(err error) string {
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return "unexpected_eof"
	}
	return "read_failure"
}
