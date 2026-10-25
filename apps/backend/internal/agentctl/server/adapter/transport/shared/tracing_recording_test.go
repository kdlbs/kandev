package shared

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

type protocolMarshalProbe struct{ calls int }

type recordOnlyProtocolSampler struct{}

func (recordOnlyProtocolSampler) ShouldSample(sdktrace.SamplingParameters) sdktrace.SamplingResult {
	return sdktrace.SamplingResult{Decision: sdktrace.RecordOnly}
}

func (recordOnlyProtocolSampler) Description() string { return "record only" }

func (probe *protocolMarshalProbe) MarshalJSON() ([]byte, error) {
	probe.calls++
	return []byte(`"serialized"`), nil
}

func useProtocolTestTracer(t *testing.T, tracer trace.Tracer) {
	t.Helper()
	previousDebug, previousTracer := debugMode, noopTracer
	debugMode, noopTracer = false, tracer
	t.Cleanup(func() { debugMode, noopTracer = previousDebug, previousTracer })
}

func TestTraceProtocolEventNonRecordingSkipsJSON(t *testing.T) {
	unsampled := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.NeverSample()))
	t.Cleanup(func() { require.NoError(t, unsampled.Shutdown(context.Background())) })
	for name, tracer := range map[string]trace.Tracer{
		"disabled":    noop.NewTracerProvider().Tracer("protocol-test"),
		"sampled out": unsampled.Tracer("protocol-test"),
	} {
		t.Run(name, func(t *testing.T) {
			useProtocolTestTracer(t, tracer)
			probe := &protocolMarshalProbe{}
			event := &streams.AgentEvent{Type: "tool_update", SessionID: "session",
				Data: map[string]any{"probe": probe}}
			TraceProtocolEvent(context.Background(), "acp", "agent", event.Type, nil, event)
			require.Zero(t, probe.calls, "discarded tracing must not serialize the event")
		})
	}
}

func TestTraceProtocolEventRecordingPreservesPayloads(t *testing.T) {
	for name, sampler := range map[string]sdktrace.Sampler{
		"sampled":     sdktrace.AlwaysSample(),
		"record only": recordOnlyProtocolSampler{},
	} {
		t.Run(name, func(t *testing.T) { assertProtocolRecordingPayloads(t, sampler) })
	}
}

func assertProtocolRecordingPayloads(t *testing.T, sampler sdktrace.Sampler) {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sampler),
		sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	useProtocolTestTracer(t, provider.Tracer("protocol-test"))
	probe := &protocolMarshalProbe{}
	event := &streams.AgentEvent{Type: "tool_update", SessionID: "session",
		Text: strings.Repeat("n", maxAttrValueLen+1), Data: map[string]any{"probe": probe}}
	raw := json.RawMessage(`{"text":"` + strings.Repeat("r", maxAttrValueLen+1) + `"}`)
	TraceProtocolEvent(context.Background(), "acp", "agent", event.Type, raw, event)
	require.Equal(t, 1, probe.calls)
	spans := recorder.Ended()
	require.Len(t, spans, 1)
	require.Equal(t, "acp.tool_update", spans[0].Name())
	attributes := make(map[string]string)
	for _, field := range spans[0].Attributes() {
		attributes[string(field.Key)] = field.Value.AsString()
	}
	require.Equal(t, map[string]string{"protocol": "acp", "agent_id": "agent",
		"event_type": "tool_update", "session_id": "session"}, attributes)
	events := spans[0].Events()
	require.Len(t, events, 2)
	require.Equal(t, "raw", events[0].Name)
	require.Equal(t, truncate(string(raw), maxAttrValueLen), events[0].Attributes[0].Value.AsString())
	want, err := json.Marshal(event)
	require.NoError(t, err)
	require.Equal(t, "normalized", events[1].Name)
	require.Equal(t, truncate(string(want), maxAttrValueLen), events[1].Attributes[0].Value.AsString())
}
