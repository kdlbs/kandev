package launcher

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
)

func TestChildLogLevelJSONRecords(t *testing.T) {
	const timestamp = "2026-10-05T12:44:56.631+0100"
	tests := []struct {
		name string
		line string
		want string
	}{
		{name: "recognized root level", line: `{"level":"error","timestamp":"` + timestamp + `","caller":"server.go:8","msg":"request failed"}`, want: "ERROR"},
		{name: "utc timestamp", line: `{"level":"info","timestamp":"2026-10-05T11:44:56.631Z","caller":"server.go:8","msg":"started"}`, want: "INFO"},
		{name: "extra structured fields are ignored", line: `{"level":"warn","timestamp":"` + timestamp + `","caller":"server.go:8","msg":"slow","component":"acp","duration_ms":12}`, want: "WARN"},
		{name: "payload text does not change root severity", line: `{"level":"info","timestamp":"` + timestamp + `","caller":"server.go:8","msg":"payload says ERROR","payload":{"level":"ERROR"}}`, want: "INFO"},
		{name: "nested level without root level", line: `{"timestamp":"` + timestamp + `","caller":"server.go:8","msg":"record","payload":{"level":"ERROR"}}`},
		{name: "payload-only object", line: `{"payload":{"level":"ERROR"}}`},
		{name: "unknown level", line: `{"level":"TRACE","timestamp":"` + timestamp + `","caller":"server.go:8","msg":"record"}`},
		{name: "timestamp is not an agentctl timestamp", line: `{"level":"error","timestamp":"yesterday","caller":"server.go:8","msg":"record"}`},
		{name: "caller must not be empty", line: `{"level":"error","timestamp":"` + timestamp + `","caller":"  ","msg":"record"}`},
		{name: "required values must be strings", line: `{"level":"error","timestamp":"` + timestamp + `","caller":"server.go:8","msg":5}`},
		{name: "duplicate level field", line: `{"level":"error","level":"info","timestamp":"` + timestamp + `","caller":"server.go:8","msg":"record"}`},
		{name: "duplicate timestamp field", line: `{"level":"error","timestamp":"` + timestamp + `","timestamp":"` + timestamp + `","caller":"server.go:8","msg":"record"}`},
		{name: "duplicate caller field", line: `{"level":"error","timestamp":"` + timestamp + `","caller":"server.go:8","caller":"other.go:1","msg":"record"}`},
		{name: "duplicate msg field", line: `{"level":"error","timestamp":"` + timestamp + `","caller":"server.go:8","msg":"record","msg":"other"}`},
		{name: "duplicate additional field is ignored", line: `{"level":"warn","timestamp":"` + timestamp + `","caller":"server.go:8","msg":"record","tag":"one","tag":"two"}`, want: "WARN"},
		{name: "trailing json value", line: `{"level":"error","timestamp":"` + timestamp + `","caller":"server.go:8","msg":"record"} {}`},
		{name: "trailing text", line: `{"level":"error","timestamp":"` + timestamp + `","caller":"server.go:8","msg":"record"} trailing`},
		{name: "not an object", line: `[{"level":"error"}]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, childLogLevel(tt.line))
		})
	}
}

func TestPipeOutputPreservesSeverityWithNestedDuplicateComponentFields(t *testing.T) {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "timestamp"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	var output bytes.Buffer
	childCore := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.AddSync(&output),
		zapcore.DebugLevel,
	)
	child := zap.New(childCore, zap.AddCaller()).
		With(zap.String("component", "process-manager")).
		With(zap.String("component", "workspace-tracker"))
	child.Warn("workspace tracking is delayed")
	child.Error("workspace tracking failed")

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	require.Len(t, lines, 2)
	for index, line := range lines {
		want := []string{"WARN", "ERROR"}[index]
		require.Equal(t, want, childJSONLogLevel(line), "the real zap encoder timestamp and envelope must be valid for the child parser: "+line)
		require.Equal(t, 2, strings.Count(line, `"component"`))
	}

	parentCore, observed := observer.New(zapcore.InfoLevel)
	parentLog, err := logger.NewFromZap(zap.New(parentCore))
	require.NoError(t, err)
	(&Launcher{logger: parentLog}).pipeOutput("stdout", bufio.NewScanner(strings.NewReader(strings.Join(lines, "\n"))))

	entries := observed.All()
	require.Len(t, entries, 2)
	require.Equal(t, []zapcore.Level{zapcore.WarnLevel, zapcore.ErrorLevel}, []zapcore.Level{
		entries[0].Level, entries[1].Level,
	})
	require.Contains(t, entries[0].Message, `"msg":"workspace tracking is delayed"`)
	require.Contains(t, entries[1].Message, `"msg":"workspace tracking failed"`)
}

func TestPipeOutputPreservesJSONSeverityAtInfoThreshold(t *testing.T) {
	lines := []string{
		`{"level":"info","timestamp":"2026-10-05T12:44:56.631+0100","caller":"main.go:1","msg":"started"}`,
		`{"level":"warn","timestamp":"2026-10-05T12:44:56.631+0100","caller":"main.go:2","msg":"slow"}`,
		`{"level":"error","timestamp":"2026-10-05T12:44:56.631+0100","caller":"main.go:3","msg":"failed"}`,
		`{"level":"fatal","timestamp":"2026-10-05T12:44:56.631+0100","caller":"main.go:4","msg":"child fatal record"}`,
		`{"level":"debug","timestamp":"2026-10-05T12:44:56.631+0100","caller":"main.go:5","msg":"below threshold"}`,
	}
	core, observed := observer.New(zapcore.InfoLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	launcher := &Launcher{logger: log}
	launcher.pipeOutput("stdout", bufio.NewScanner(strings.NewReader(strings.Join(lines, "\n"))))

	entries := observed.All()
	require.Len(t, entries, 4)
	require.Equal(t, []zapcore.Level{zapcore.InfoLevel, zapcore.WarnLevel, zapcore.ErrorLevel, zapcore.ErrorLevel}, []zapcore.Level{
		entries[0].Level, entries[1].Level, entries[2].Level, entries[3].Level,
	})
	for index, entry := range entries {
		require.Equal(t, lines[index], entry.Message, "forward original child record")
		require.Equal(t, "stdout", entry.ContextMap()["stream"])
	}
}

func TestPipeOutputKeepsMalformedJSONStreamFallback(t *testing.T) {
	for _, tt := range []struct {
		stream string
		want   zapcore.Level
	}{
		{stream: "stdout", want: zapcore.DebugLevel},
		{stream: "stderr", want: zapcore.WarnLevel},
	} {
		t.Run(tt.stream, func(t *testing.T) {
			line := `{"level":"ERROR","timestamp":"not-a-time","caller":"main.go:1","msg":"failed"}`
			core, observed := observer.New(zapcore.DebugLevel)
			log, err := logger.NewFromZap(zap.New(core))
			require.NoError(t, err)
			(&Launcher{logger: log}).pipeOutput(tt.stream, bufio.NewScanner(strings.NewReader(line)))

			entries := observed.All()
			require.Len(t, entries, 1)
			require.Equal(t, tt.want, entries[0].Level)
			require.Equal(t, line, entries[0].Message)
		})
	}
}
