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
	"github.com/kandev/kandev/internal/common/winproc"
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

// startupExit ends the process after a second shutdown signal. Tests replace it.
var startupExit = os.Exit

type processContextKey struct{}

type workerCancelContextKey struct{}

type startupSignalController struct {
	first   chan os.Signal
	stop    chan struct{}
	done    chan struct{}
	signals chan os.Signal
	// cooperative receives the parent's Windows stop request. It is kept apart
	// from signals so a request that arrives after shutdown started (for
	// example when Ctrl+C reached both the launcher and this backend through a
	// shared console) is ignored instead of being treated as a second signal.
	cooperative chan os.Signal
	once        sync.Once
	// stopLocal releases the cooperative shutdown listener used on Windows.
	stopLocal func()
}

func newStartupSignalController(ctx context.Context, cancel context.CancelFunc, log *logger.Logger) *startupSignalController {
	controller := &startupSignalController{
		first:       make(chan os.Signal, 1),
		stop:        make(chan struct{}),
		done:        make(chan struct{}),
		signals:     make(chan os.Signal, 2),
		cooperative: make(chan os.Signal, 1),
	}
	signal.Notify(controller.signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	stopLocal, err := winproc.ListenForShutdown(controller.cooperative)
	if err != nil {
		if log != nil {
			log.Warn("cooperative shutdown listener unavailable", zap.Error(err))
		}
		stopLocal = func() {}
	}
	controller.stopLocal = stopLocal
	if log != nil {
		log.Debug("shutdown signal handler armed",
			zap.Int("pid", os.Getpid()),
			zap.Int("ppid", os.Getppid()))
	}
	go func() {
		defer close(controller.done)
		var pairing winproc.ShutdownPairing
		var sig os.Signal
		select {
		case sig = <-controller.signals:
		case sig = <-controller.cooperative:
			sig = pairing.CooperativeFirst(sig, controller.signals)
		case <-ctx.Done():
			return
		case <-controller.stop:
			return
		}
		controller.first <- sig
		cancel()
		// Only a deliberate second OS signal forces exit. A later cooperative
		// request is left unread on its own channel, and the OS signal from the
		// same Ctrl+C as a cooperative first request is absorbed.
		for {
			select {
			case second := <-controller.signals:
				if !pairing.Forces(second) {
					if log != nil {
						log.Debug("ignored console signal paired with cooperative shutdown", zap.String("signal", second.String()))
					}
					continue
				}
				if log != nil {
					log.Warn("Received second shutdown signal, forcing exit", zap.String("signal", second.String()))
					_ = log.Close()
				}
				startupExit(1)
				return
			case <-controller.stop:
				return
			}
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
		if controller.stopLocal != nil {
			controller.stopLocal()
		}
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
