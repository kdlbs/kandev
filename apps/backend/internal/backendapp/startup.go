package backendapp

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/startup"
	"go.uber.org/zap"
)

type bootstrapContextKey struct{}

type bootstrapRuntime struct {
	handler             *handlerSwitch
	server              *http.Server
	listeners           *serverListeners
	ready               atomic.Bool
	readinessMu         sync.Mutex
	readinessPublishing bool
	startupClosing      bool
}

type startupSignalContextKey struct{}

type processContextKey struct{}

type workerCancelContextKey struct{}

type startupSignalController struct {
	first   chan os.Signal
	stop    chan struct{}
	done    chan struct{}
	signals chan os.Signal
	once    sync.Once
}

func newStartupSignalController(ctx context.Context, cancel context.CancelFunc, log *logger.Logger) *startupSignalController {
	controller := &startupSignalController{
		first:   make(chan os.Signal, 1),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
		signals: make(chan os.Signal, 2),
	}
	signal.Notify(controller.signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	if log != nil {
		log.Debug("shutdown signal handler armed",
			zap.Int("pid", os.Getpid()),
			zap.Int("ppid", os.Getppid()))
	}
	go func() {
		defer close(controller.done)
		select {
		case sig := <-controller.signals:
			controller.first <- sig
			cancel()
			select {
			case second := <-controller.signals:
				if log != nil {
					log.Warn("Received second shutdown signal, forcing exit", zap.String("signal", second.String()))
					_ = log.Close()
				}
				os.Exit(1)
			case <-controller.stop:
			}
		case <-ctx.Done():
		case <-controller.stop:
		}
	}()
	return controller
}

func withStartupSignalController(ctx context.Context, controller *startupSignalController) context.Context {
	return context.WithValue(ctx, startupSignalContextKey{}, controller)
}

func withProcessContext(ctx, process context.Context) context.Context {
	return context.WithValue(ctx, processContextKey{}, process)
}

func processContextFromContext(ctx context.Context) context.Context {
	process, _ := ctx.Value(processContextKey{}).(context.Context)
	return process
}

// processRuntimeContext returns the process lifetime carried by startup
// workers. Restore cancels workers while the process remains alive to finish
// the restore and deliver its terminal job update.
func processRuntimeContext(ctx context.Context) context.Context {
	if process := processContextFromContext(ctx); process != nil {
		return process
	}
	return ctx
}

// beginReadinessPublication serializes the readiness handoff with the startup
// cancellation watcher. Once startup cancellation claims listener closure,
// initialization cannot publish a router into a closing server.
func (runtime *bootstrapRuntime) beginReadinessPublication(ctx context.Context) bool {
	runtime.readinessMu.Lock()
	defer runtime.readinessMu.Unlock()
	if runtime.startupClosing || ctx.Err() != nil {
		return false
	}
	runtime.readinessPublishing = true
	return true
}

func (runtime *bootstrapRuntime) claimStartupClose() bool {
	runtime.readinessMu.Lock()
	defer runtime.readinessMu.Unlock()
	if runtime.ready.Load() || runtime.readinessPublishing {
		return false
	}
	runtime.startupClosing = true
	return true
}

func withWorkerCancel(ctx context.Context, cancel context.CancelFunc) context.Context {
	return context.WithValue(ctx, workerCancelContextKey{}, cancel)
}

func workerCancelFromContext(ctx context.Context) context.CancelFunc {
	cancel, _ := ctx.Value(workerCancelContextKey{}).(context.CancelFunc)
	return cancel
}

func startupSignalControllerFromContext(ctx context.Context) *startupSignalController {
	controller, _ := ctx.Value(startupSignalContextKey{}).(*startupSignalController)
	return controller
}

func (controller *startupSignalController) wait(ctx context.Context) os.Signal {
	select {
	case sig := <-controller.first:
		return sig
	default:
	}
	select {
	case sig := <-controller.first:
		return sig
	case <-ctx.Done():
		select {
		case sig := <-controller.first:
			return sig
		default:
			return nil
		}
	}
}

func (controller *startupSignalController) stopAndWait() {
	if controller == nil {
		return
	}
	controller.once.Do(func() {
		signal.Stop(controller.signals)
		close(controller.stop)
		<-controller.done
	})
}

// runWithBootstrap owns the listener for both initialization and application use.
// Constructors run synchronously so resource cleanup cannot race initialization.
// The process context remains live while worker contexts are quiesced, which
// lets restore finish and deliver its job result before explicit shutdown.
func runWithBootstrap(ctx context.Context, cfg *config.Config, log *logger.Logger, initialize func(context.Context) bool) bool {
	startupCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	controller := newStartupSignalController(startupCtx, cancel, log)
	defer controller.stopAndWait()
	if err := startupCtx.Err(); err != nil {
		return false
	}

	progress := startup.New(log)
	startupCtx = startup.WithReporter(startupCtx, progress)
	startupCtx = withStartupSignalController(startupCtx, controller)
	workerCtx, cancelWorkers := context.WithCancel(startupCtx)
	defer cancelWorkers()
	workerCtx = withProcessContext(workerCtx, startupCtx)
	workerCtx = withWorkerCancel(workerCtx, cancelWorkers)
	ready.Store(false)
	handler, server, listeners, err := bindBootstrapListeners(cfg, log, Version, progress)
	if err != nil {
		return false
	}
	defer closeBoundListeners(server, listeners, log)
	runtime := &bootstrapRuntime{handler: handler, server: server, listeners: listeners}
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		progress.Report()
		for {
			select {
			case <-startupCtx.Done():
				if runtime.claimStartupClose() {
					closeBoundListeners(server, listeners, log)
				}
				return
			case <-done:
				return
			case <-ticker.C:
				progress.Report()
			}
		}
	}()
	defer func() { close(done); <-stopped }()
	ok := initialize(context.WithValue(workerCtx, bootstrapContextKey{}, runtime))
	return ok && (startupCtx.Err() == nil || runtime.ready.Load())
}
