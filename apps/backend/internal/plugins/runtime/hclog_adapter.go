package runtime

import (
	"strings"
	"sync/atomic"

	hclog "github.com/hashicorp/go-hclog"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// pluginExitedMsg is the message go-plugin's monitor goroutine logs when the
// subprocess exits. On a deliberate shutdown kill the wait error is an
// expected "signal: terminated" / "signal: killed", which go-plugin emits at
// ERROR — misleading noise during graceful teardown. See client.go's
// "plugin process exited" site in hashicorp/go-plugin.
const pluginExitedMsg = "plugin process exited"

// hclogZapAdapter routes hashicorp/go-plugin's internal hclog output through
// Kandev's zap logger so plugin diagnostics land in the backend log stream.
// It embeds a base hclog.Logger purely to satisfy the large hclog.Logger
// interface's boilerplate (Is*, StandardLogger, level bookkeeping); the
// level-emitting methods are overridden to forward to zap.
//
// The stopping flag (shared per client) lets the deliberate-kill path mark a
// process as intentionally terminated: while set, the expected
// "plugin process exited" ERROR is downgraded to DEBUG so a graceful shutdown
// does not print a stack-trace-bearing error. Genuine crashes (stopping unset)
// still surface at ERROR.
type hclogZapAdapter struct {
	hclog.Logger
	log      *logger.Logger
	stopping *atomic.Bool
}

// newHCLogAdapter builds an adapter forwarding to log, sharing stopping with
// the owning process so Kill can flag an expected exit.
func newHCLogAdapter(log *logger.Logger, stopping *atomic.Bool) *hclogZapAdapter {
	return &hclogZapAdapter{
		Logger:   hclog.NewNullLogger(),
		log:      log,
		stopping: stopping,
	}
}

func (a *hclogZapAdapter) fields(args []interface{}) []zap.Field {
	fields := make([]zap.Field, 0, len(args)/2+1)
	for i := 0; i+1 < len(args); i += 2 {
		key, ok := args[i].(string)
		if !ok {
			continue
		}
		fields = append(fields, zap.Any(key, args[i+1]))
	}
	return fields
}

func (a *hclogZapAdapter) Trace(msg string, args ...interface{}) { a.log.Debug(msg, a.fields(args)...) }
func (a *hclogZapAdapter) Debug(msg string, args ...interface{}) { a.log.Debug(msg, a.fields(args)...) }
func (a *hclogZapAdapter) Info(msg string, args ...interface{})  { a.log.Info(msg, a.fields(args)...) }
func (a *hclogZapAdapter) Warn(msg string, args ...interface{})  { a.log.Warn(msg, a.fields(args)...) }

func (a *hclogZapAdapter) Error(msg string, args ...interface{}) {
	// An expected exit during a deliberate kill is teardown noise, not a fault.
	if a.stopping != nil && a.stopping.Load() && strings.Contains(msg, pluginExitedMsg) {
		a.log.Debug(msg, a.fields(args)...)
		return
	}
	a.log.Error(msg, a.fields(args)...)
}

func (a *hclogZapAdapter) Log(level hclog.Level, msg string, args ...interface{}) {
	switch level {
	case hclog.Trace, hclog.Debug:
		a.Debug(msg, args...)
	case hclog.Info:
		a.Info(msg, args...)
	case hclog.Warn:
		a.Warn(msg, args...)
	case hclog.Error:
		a.Error(msg, args...)
	default:
		a.Info(msg, args...)
	}
}

// With/Named/ResetNamed return the same adapter so the stopping flag and zap
// sink survive go-plugin's sublogger creation. go-plugin decorates its client
// logger with fields/names; a base-logger passthrough would drop the override.
func (a *hclogZapAdapter) With(args ...interface{}) hclog.Logger {
	return &hclogZapAdapter{Logger: a.Logger.With(args...), log: a.log.WithFields(a.fields(args)...), stopping: a.stopping}
}

func (a *hclogZapAdapter) Named(name string) hclog.Logger {
	return &hclogZapAdapter{Logger: a.Logger.Named(name), log: a.log, stopping: a.stopping}
}

func (a *hclogZapAdapter) ResetNamed(name string) hclog.Logger {
	return &hclogZapAdapter{Logger: a.Logger.ResetNamed(name), log: a.log, stopping: a.stopping}
}

// IsError/IsWarn/IsInfo/IsDebug report true so go-plugin does not elide the
// messages the adapter forwards to zap; zap applies its own level filtering.
func (a *hclogZapAdapter) IsTrace() bool { return true }
func (a *hclogZapAdapter) IsDebug() bool { return true }
func (a *hclogZapAdapter) IsInfo() bool  { return true }
func (a *hclogZapAdapter) IsWarn() bool  { return true }
func (a *hclogZapAdapter) IsError() bool { return true }

var _ hclog.Logger = (*hclogZapAdapter)(nil)
