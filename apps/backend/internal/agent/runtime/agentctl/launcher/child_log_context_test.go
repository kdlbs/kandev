package launcher

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
)

const childLogTestTaskID = "11111111-1111-4111-8111-111111111111"
const childLogTestSessionID = "22222222-2222-4222-8222-22222222222a"

func encodeChildLogLine(t *testing.T, level zapcore.Level, message string, fields ...zap.Field) string {
	t.Helper()
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "timestamp"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	var output bytes.Buffer
	core := zapcore.NewCore(zapcore.NewJSONEncoder(encoderConfig), zapcore.AddSync(&output), zapcore.DebugLevel)
	child := zap.New(core, zap.AddCaller())
	child.Check(level, message).Write(fields...)
	return strings.TrimSpace(output.String())
}

func forwardChildLogLine(t *testing.T, line string) (*observer.ObservedLogs, string) {
	t.Helper()
	return forwardChildLogLineAt(t, line, zapcore.DebugLevel)
}

func forwardChildLogLineAt(t *testing.T, line string, level zapcore.Level) (*observer.ObservedLogs, string) {
	t.Helper()
	core, observed := observer.New(level)
	parentLog, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	launcher := &Launcher{logger: parentLog.WithFields(zap.String("component", "agentctl-launcher"))}
	launcher.pipeOutput("stdout", strings.NewReader(line))
	return observed, line
}

func decodedChildMessage(t *testing.T, message string) map[string]any {
	t.Helper()
	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(message), &decoded))
	return decoded
}

// @covers AC-PLATFORM-DIAGNOSTIC-SIGNAL-001.4
// @covers AC-PLATFORM-DIAGNOSTIC-SIGNAL-001.5
func TestPipeOutputPreservesSafeDiagnosticContext(t *testing.T) {
	line := encodeChildLogLine(t, zapcore.WarnLevel, "durable delivery pressure changed",
		zap.String("component", "workspace-tracker"),
		zap.String("task_id", childLogTestTaskID),
		zap.String("session_id", childLogTestSessionID),
		zap.String("state", "pressure"),
		zap.String("error_class", "deadline_exceeded"),
		zap.String("operation", "git_inspection"),
		zap.String("trigger", "turn_complete"),
		zap.String("runtime", "server"),
		zap.String("poll_mode", "slow"),
		zap.Int("exit_code", 128),
		zap.Int64("retained_bytes", 4096),
		zap.Int64("limit_bytes", 8192),
		zap.String("error", "raw error must not cross"),
		zap.String("path", "/private/workspace"),
		zap.String("token", "secret-token"),
	)

	observed, _ := forwardChildLogLine(t, line)
	entries := observed.All()
	require.Len(t, entries, 1)
	require.Equal(t, zapcore.WarnLevel, entries[0].Level)
	require.Equal(t, "agentctl-launcher", entries[0].ContextMap()["component"])
	require.Equal(t, "stdout", entries[0].ContextMap()["stream"])

	decoded := decodedChildMessage(t, entries[0].Message)
	for key, want := range map[string]any{
		"component":      "workspace-tracker",
		"task_id":        childLogTestTaskID,
		"session_id":     childLogTestSessionID,
		"state":          "pressure",
		"error_class":    "deadline_exceeded",
		"operation":      "git_inspection",
		"trigger":        "turn_complete",
		"runtime":        "server",
		"poll_mode":      "slow",
		"exit_code":      float64(128),
		"retained_bytes": float64(4096),
		"limit_bytes":    float64(8192),
	} {
		require.Equal(t, want, decoded[key], "safe field %q", key)
	}
	for _, sensitive := range []string{"raw error must not cross", "/private/workspace", "secret-token"} {
		require.NotContains(t, entries[0].Message, sensitive)
	}
}

// @covers AC-PLATFORM-DIAGNOSTIC-SIGNAL-001.5
func TestPipeOutputRejectsUnsafeOptionalContext(t *testing.T) {
	tests := []struct {
		name       string
		fields     []zap.Field
		absentKeys []string
		message    string
	}{
		{
			name: "invalid uuid",
			fields: []zap.Field{
				zap.String("task_id", "private-session"),
				zap.String("state", "pressure"),
			},
			absentKeys: []string{"task_id"},
			message:    "private-session",
		},
		{
			name: "uppercase uuid is not canonical",
			fields: []zap.Field{
				zap.String("session_id", strings.ToUpper(childLogTestSessionID)),
				zap.String("state", "pressure"),
			},
			absentKeys: []string{"session_id"},
			message:    strings.ToUpper(childLogTestSessionID),
		},
		{
			name: "duplicate optional field",
			fields: []zap.Field{
				zap.String("task_id", childLogTestTaskID),
				zap.String("task_id", childLogTestTaskID),
				zap.String("state", "pressure"),
			},
			absentKeys: []string{"task_id"},
			message:    "duplicate-state",
		},
		{
			name: "numeric overflow",
			fields: []zap.Field{
				zap.Uint64("retained_bytes", math.MaxUint64),
				zap.String("state", "pressure"),
			},
			absentKeys: []string{"retained_bytes"},
			message:    "overflow",
		},
		{
			name: "null retained bytes",
			fields: []zap.Field{
				zap.Any("retained_bytes", nil),
				zap.String("state", "pressure"),
			},
			absentKeys: []string{"retained_bytes"},
			message:    "null-retained",
		},
		{
			name: "null limit bytes",
			fields: []zap.Field{
				zap.Any("limit_bytes", nil),
				zap.String("state", "pressure"),
			},
			absentKeys: []string{"limit_bytes"},
			message:    "null-limit",
		},
		{
			name: "null exit code",
			fields: []zap.Field{
				zap.Any("exit_code", nil),
				zap.String("state", "pressure"),
			},
			absentKeys: []string{"exit_code"},
			message:    "null-exit",
		},
		{
			name: "unsupported enum value",
			fields: []zap.Field{
				zap.String("error_class", "raw provider error"),
				zap.String("state", "pressure"),
			},
			absentKeys: []string{"error_class"},
			message:    "raw provider error",
		},
		{
			name: "unknown sensitive keys",
			fields: []zap.Field{
				zap.String("error", "provider stderr private"),
				zap.String("target", "/private/repo"),
				zap.String("path", "/private/repo/file"),
				zap.String("secret", "token-value"),
				zap.String("state", "pressure"),
			},
			absentKeys: []string{"error", "target", "path", "secret"},
			message:    "record",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := encodeChildLogLine(t, zapcore.WarnLevel, tt.message, tt.fields...)
			observed, _ := forwardChildLogLine(t, line)
			entries := observed.All()
			require.Len(t, entries, 1)
			require.Equal(t, zapcore.WarnLevel, entries[0].Level)
			decoded := decodedChildMessage(t, entries[0].Message)
			for _, key := range tt.absentKeys {
				require.NotContains(t, decoded, key)
			}
			require.Equal(t, "pressure", decoded["state"])
		})
	}
}

// @covers AC-PLATFORM-DIAGNOSTIC-SIGNAL-001.5
func TestPipeOutputHandlesRepeatedComponentSafely(t *testing.T) {
	t.Run("last valid component is retained", func(t *testing.T) {
		line := encodeChildLogLine(t, zapcore.InfoLevel, "started",
			zap.String("component", "process-manager"),
			zap.String("component", "workspace-tracker"),
			zap.String("state", "healthy"),
		)
		observed, _ := forwardChildLogLine(t, line)
		decoded := decodedChildMessage(t, observed.All()[0].Message)
		require.Equal(t, "workspace-tracker", decoded["component"])
		require.Equal(t, "healthy", decoded["state"])
	})

	t.Run("one unsupported component omits the field", func(t *testing.T) {
		line := encodeChildLogLine(t, zapcore.InfoLevel, "started",
			zap.String("component", "process-manager"),
			zap.String("component", "not-a-component"),
			zap.String("state", "healthy"),
		)
		observed, _ := forwardChildLogLine(t, line)
		decoded := decodedChildMessage(t, observed.All()[0].Message)
		require.NotContains(t, decoded, "component")
		require.Equal(t, "healthy", decoded["state"])
		require.NotContains(t, observed.All()[0].Message, "not-a-component")
	})
}

// @covers AC-PLATFORM-DIAGNOSTIC-SIGNAL-001.5
func TestChildLogIntegerBounds(t *testing.T) {
	valid := []struct {
		key   string
		value any
	}{
		{"exit_code", 4294967295},
		{"exit_code", -1},
		{"retained_bytes", 0},
		{"limit_bytes", int64(1 << 62)},
	}
	for _, tt := range valid {
		raw, err := json.Marshal(tt.value)
		require.NoError(t, err)
		require.True(t, childLogContextValidators[tt.key](raw), "%s=%v", tt.key, tt.value)
	}
	invalid := []struct {
		key   string
		value any
	}{
		{"exit_code", nil},
		{"retained_bytes", nil},
		{"limit_bytes", nil},
		{"exit_code", 4294967296},
		{"exit_code", -2},
		{"retained_bytes", -1},
		{"retained_bytes", 1.5},
		{"limit_bytes", "8192"},
	}
	for _, tt := range invalid {
		raw, err := json.Marshal(tt.value)
		require.NoError(t, err)
		require.False(t, childLogContextValidators[tt.key](raw), "%s=%v", tt.key, tt.value)
	}
}

// @covers AC-PLATFORM-DIAGNOSTIC-SIGNAL-001.5
func TestChildLogContextAcceptsPollTrigger(t *testing.T) {
	if !childLogContextValidators["trigger"](json.RawMessage(`"poll"`)) {
		t.Fatal("periodic poll trigger must remain forwardable")
	}
}

// @covers AC-PLATFORM-DIAGNOSTIC-SIGNAL-001.5
// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.9
func TestPipeOutputForwardsAgentFailureContext(t *testing.T) {
	const instanceID = "33333333-3333-4333-8333-333333333333"
	for _, tt := range []struct {
		operation string
		class     string
	}{
		{operation: "agent_configure", class: "agent_running"},
		{operation: "agent_configure", class: "unknown"},
		{operation: "agent_probe", class: "managed_runtime_npm_resolution"},
		{operation: "agent_probe", class: "managed_runtime_npm_policy"},
		{operation: "agent_probe", class: "unsupported_profile_context"},
		{operation: "agent_probe", class: "authentication_required"},
		{operation: "git_inspection", class: "evidence_changed"},
	} {
		t.Run(tt.operation+"-"+tt.class, func(t *testing.T) {
			line := encodeChildLogLine(t, zapcore.WarnLevel, "agent operation failed",
				zap.String("task_id", childLogTestTaskID),
				zap.String("session_id", childLogTestSessionID),
				zap.String("instance_id", instanceID),
				zap.String("operation", tt.operation),
				zap.String("error_class", tt.class),
				zap.String("error", "raw failure must not cross"),
			)
			observed, _ := forwardChildLogLineAt(t, line, zapcore.InfoLevel)
			entries := observed.All()
			require.Len(t, entries, 1)
			require.Equal(t, zapcore.WarnLevel, entries[0].Level)
			decoded := decodedChildMessage(t, entries[0].Message)
			require.Equal(t, tt.operation, decoded["operation"])
			require.Equal(t, tt.class, decoded["error_class"])
			require.Equal(t, childLogTestTaskID, decoded["task_id"])
			require.Equal(t, childLogTestSessionID, decoded["session_id"])
			require.Equal(t, instanceID, decoded["instance_id"])
			require.NotContains(t, entries[0].Message, "raw failure must not cross")
		})
	}
}

// @covers AC-PLATFORM-DIAGNOSTIC-SIGNAL-001.5
func TestPipeOutputRejectsUnsupportedAgentFailureContext(t *testing.T) {
	for _, tt := range []struct {
		name string
		key  string
		raw  string
	}{
		{name: "unknown operation", key: "operation", raw: `"agent_restart"`},
		{name: "unclassified configure failure", key: "error_class", raw: `"configure_failed"`},
		{name: "free-form probe code", key: "error_class", raw: `"npm Etarget private"`},
		{name: "operation with path", key: "operation", raw: `"/private/workspace"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.False(t, childLogContextValidators[tt.key](json.RawMessage(tt.raw)))
		})
	}
}
