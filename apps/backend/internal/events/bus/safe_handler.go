package bus

import (
	"context"
	"expvar"
	"runtime/debug"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// subscriberPanicTotal counts subscriber panics recovered during event
// delivery, labelled by subscription pattern and delivery mode (regular,
// queue, nats). The pattern is the string a subscriber registered with
// (Subscribe/QueueSubscribe), so its cardinality is bounded by the number of
// distinct subscription call sites in the codebase — unlike the concrete
// delivery subject, which embeds per-session/per-run identifiers (see
// events.BuildGitWSEventSubject and friends) and would otherwise grow this
// never-evicted expvar.Map by one key per session for the process lifetime.
var subscriberPanicTotal = expvar.NewMap("events_subscriber_panic_total")

// invokeHandler calls handler and recovers a panic so that one bad
// subscriber cannot truncate delivery to the remaining subscribers on the
// same Publish call, nor escape into the Publish caller. A handler-returned
// error is passed through unchanged; a recovered panic is logged (with the
// subject, subscription pattern, event identity, and a stack trace — the
// stack is load-bearing because handler is frequently an anonymous closure
// with no other identity) and counted, and invokeHandler returns nil for it
// so callers don't double-log the same failure via their existing
// error-handling path.
func invokeHandler(ctx context.Context, log *logger.Logger, mode, subject, pattern string, event *Event, handler EventHandler) (err error) {
	defer func() {
		if r := recover(); r != nil {
			subscriberPanicTotal.Add(metricLabel("pattern", pattern, "mode", mode), 1)
			var eventID, eventType string
			if event != nil {
				eventID, eventType = event.ID, event.Type
			}
			log.Error("Event subscriber panicked",
				zap.String("subject", subject),
				zap.String("pattern", pattern),
				zap.String("mode", mode),
				zap.String("event_id", eventID),
				zap.String("event_type", eventType),
				zap.Any("recovered", r),
				zap.String("stack", string(debug.Stack())),
			)
			err = nil
		}
	}()
	return handler(ctx, event)
}

// metricLabel builds a "k1=v1;k2=v2;..." label string for an expvar map key,
// matching the convention in internal/workflow/signalmetrics/metrics_vars.go
// so a downstream Prometheus translation layer can split on the same
// delimiters. Keys are intentionally NOT escaped — callers must only pass
// bounded-cardinality values (subscription patterns, the fixed mode
// constants), never a concrete per-session/per-run subject.
func metricLabel(pairs ...string) string {
	label := ""
	for i := 0; i+1 < len(pairs); i += 2 {
		if label != "" {
			label += ";"
		}
		label += pairs[i] + "=" + pairs[i+1]
	}
	return label
}
