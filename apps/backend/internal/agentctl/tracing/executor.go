package tracing

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const executorTracerName = "kandev-executor"

func executorTracer() trace.Tracer {
	return Tracer(executorTracerName)
}

// TraceExecutorPrepare creates a span for environment preparation.
func TraceExecutorPrepare(ctx context.Context, taskID, sessionID, executorType string) (context.Context, trace.Span) {
	ctx, span := executorTracer().Start(ctx, "executor.prepare",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	span.SetAttributes(
		attribute.String("task_id", taskID),
		attribute.String("session_id", sessionID),
		attribute.String("executor_type", executorType),
	)
	return ctx, span
}

// TraceExecutorPrepareStep creates a child span for a single preparation step.
func TraceExecutorPrepareStep(ctx context.Context, stepName string, stepIndex, totalSteps int) (context.Context, trace.Span) {
	ctx, span := executorTracer().Start(ctx, "executor.prepare.step",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	span.SetAttributes(
		attribute.String("step_name", stepName),
		attribute.Int("step_index", stepIndex),
		attribute.Int("total_steps", totalSteps),
	)
	return ctx, span
}

// TraceExecutorPrepareStepResult records the result of a preparation step on its span.
func TraceExecutorPrepareStepResult(span trace.Span, status string, err error) {
	span.SetAttributes(attribute.String("status", status))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// TraceExecutorCreateInstance creates a span for executor instance creation.
func TraceExecutorCreateInstance(ctx context.Context, executionID, taskID, executorType string) (context.Context, trace.Span) {
	ctx, span := executorTracer().Start(ctx, "executor.create_instance",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	span.SetAttributes(
		attribute.String("execution_id", executionID),
		attribute.String("task_id", taskID),
		attribute.String("executor_type", executorType),
	)
	return ctx, span
}

// TraceExecutorHealthCheck creates a span for an executor health check.
func TraceExecutorHealthCheck(ctx context.Context, executorName string) (context.Context, trace.Span) {
	ctx, span := executorTracer().Start(ctx, "executor.health_check",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	span.SetAttributes(
		attribute.String("executor_name", executorName),
	)
	return ctx, span
}

// TraceExecutorStop creates a span for executor instance stop.
func TraceExecutorStop(ctx context.Context, executionID, taskID string, force bool) (context.Context, trace.Span) {
	ctx, span := executorTracer().Start(ctx, "executor.stop",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	span.SetAttributes(
		attribute.String("execution_id", executionID),
		attribute.String("task_id", taskID),
		attribute.Bool("force", force),
	)
	return ctx, span
}
