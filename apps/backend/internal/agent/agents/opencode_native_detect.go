package agents

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/kandev/kandev/internal/agent/managedruntime"
)

// opencodeNativeVersionTimeout bounds one `opencode --version` run.
const opencodeNativeVersionTimeout = 10 * time.Second

// opencodeNativeDetectionTTL is how long a successful detection may stand in
// for a later run of the same executable that fails transiently.
const opencodeNativeDetectionTTL = 10 * time.Minute

// opencodeNativeOutputExcerptRunes bounds the version output quoted in an error.
const opencodeNativeOutputExcerptRunes = 200

var opencodeNativeVersionPattern = regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?`)

// opencodeNativeRetryBackoff is the wait before each retry of a version run
// that failed or printed no readable version.
var opencodeNativeRetryBackoff = []time.Duration{300 * time.Millisecond, time.Second}

// openCodeNativeDetector reads the native OpenCode version. Its seams are the
// executable lookup, the version run, the retry wait and the clock.
type openCodeNativeDetector struct {
	lookPath       func(string) (string, error)
	runVersion     func(ctx context.Context, path string) ([]byte, error)
	sleep          func(ctx context.Context, wait time.Duration) error
	now            func() time.Time
	versionTimeout time.Duration
	backoff        []time.Duration

	mu    sync.Mutex
	cache map[string]openCodeNativeDetection
}

type openCodeNativeDetection struct {
	runtime    OpenCodeNativeRuntime
	detectedAt time.Time
}

// unsupportedOpenCodeMajorError is a definite answer, not a transient
// failure: the executable ran and reported a major Kandev cannot drive.
type unsupportedOpenCodeMajorError struct{ major uint64 }

func (e *unsupportedOpenCodeMajorError) Error() string {
	return fmt.Sprintf("native OpenCode major %d is not supported", e.major)
}

var defaultOpenCodeNativeDetector = newOpenCodeNativeDetector()

func newOpenCodeNativeDetector() *openCodeNativeDetector {
	return &openCodeNativeDetector{
		lookPath:       exec.LookPath,
		runVersion:     runOpenCodeVersion,
		sleep:          sleepContext,
		now:            time.Now,
		versionTimeout: opencodeNativeVersionTimeout,
		backoff:        opencodeNativeRetryBackoff,
		cache:          make(map[string]openCodeNativeDetection),
	}
}

// DetectOpenCodeNativeRuntime reads the bounded version output of the native
// OpenCode CLI and rejects unsupported majors before a launch can guess flags.
// A failed or unreadable run is retried; when retries still fail and the same
// executable was detected recently, that detection is returned instead of
// failing the launch.
func DetectOpenCodeNativeRuntime(ctx context.Context) (OpenCodeNativeRuntime, bool, error) {
	return defaultOpenCodeNativeDetector.detect(ctx)
}

func (d *openCodeNativeDetector) detect(ctx context.Context) (OpenCodeNativeRuntime, bool, error) {
	path, err := d.lookPath(opencodeNativeBinary)
	if err != nil {
		if isExecutableNotFound(err) {
			return OpenCodeNativeRuntime{}, false, nil
		}
		return OpenCodeNativeRuntime{}, false, fmt.Errorf("locate native OpenCode executable: %w", err)
	}
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return OpenCodeNativeRuntime{}, true, err
		}
		detected, err := d.detectOnce(ctx, path)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return OpenCodeNativeRuntime{}, true, ctxErr
		}
		if err == nil {
			d.remember(path, detected)
			return detected, true, nil
		}
		var unsupported *unsupportedOpenCodeMajorError
		if errors.As(err, &unsupported) {
			d.forget(path)
			return OpenCodeNativeRuntime{}, true, err
		}
		if attempt >= len(d.backoff) {
			if cached, ok := d.cached(path); ok {
				return cached, true, nil
			}
			return OpenCodeNativeRuntime{}, true, err
		}
		if err := d.sleep(ctx, d.backoff[attempt]); err != nil {
			return OpenCodeNativeRuntime{}, true, err
		}
	}
}

func (d *openCodeNativeDetector) detectOnce(ctx context.Context, path string) (OpenCodeNativeRuntime, error) {
	versionCtx, cancel := context.WithTimeout(ctx, d.versionTimeout)
	defer cancel()
	output, err := d.runVersion(versionCtx, path)
	if err != nil {
		if errors.Is(versionCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
			err = fmt.Errorf("%w (timed out after %s)", err, d.versionTimeout)
		}
		return OpenCodeNativeRuntime{}, fmt.Errorf("read native OpenCode version: %w%s", err, openCodeOutputSuffix(output))
	}
	match := opencodeNativeVersionPattern.FindString(string(output))
	parsed, err := managedruntime.ParseStableVersion(strings.TrimSpace(match))
	if err != nil {
		return OpenCodeNativeRuntime{}, fmt.Errorf("read native OpenCode version: unsupported output%s", openCodeOutputSuffix(output))
	}
	switch parsed.Major() {
	case 1:
		return OpenCodeNativeRuntime{Family: managedruntime.OpenCodeFamilyV1, Version: parsed.Original()}, nil
	case 2:
		return OpenCodeNativeRuntime{Family: managedruntime.OpenCodeFamilyV2, Version: parsed.Original()}, nil
	default:
		return OpenCodeNativeRuntime{}, &unsupportedOpenCodeMajorError{major: parsed.Major()}
	}
}

func (d *openCodeNativeDetector) cached(path string) (OpenCodeNativeRuntime, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	entry, ok := d.cache[path]
	if !ok || d.now().Sub(entry.detectedAt) > opencodeNativeDetectionTTL {
		return OpenCodeNativeRuntime{}, false
	}
	return entry.runtime, true
}

func (d *openCodeNativeDetector) remember(path string, detected OpenCodeNativeRuntime) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cache[path] = openCodeNativeDetection{runtime: detected, detectedAt: d.now()}
}

func (d *openCodeNativeDetector) forget(path string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.cache, path)
}

func isExecutableNotFound(err error) bool {
	var execErr *exec.Error
	if errors.As(err, &execErr) && errors.Is(execErr.Err, exec.ErrNotFound) {
		return true
	}
	return errors.Is(err, exec.ErrNotFound)
}

func runOpenCodeVersion(ctx context.Context, path string) ([]byte, error) {
	return exec.CommandContext(ctx, path, "--version").CombinedOutput()
}

func sleepContext(ctx context.Context, wait time.Duration) error {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// openCodeOutputSuffix quotes a bounded, single-line excerpt of the version
// output with the user's home directory replaced by "~".
func openCodeOutputSuffix(output []byte) string {
	text := strings.ToValidUTF8(string(output), "?")
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		text = strings.ReplaceAll(text, home, "~")
	}
	text = strings.Join(strings.FieldsFunc(text, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}), " ")
	if text == "" {
		return ""
	}
	if runes := []rune(text); len(runes) > opencodeNativeOutputExcerptRunes {
		text = string(runes[:opencodeNativeOutputExcerptRunes]) + "..."
	}
	return fmt.Sprintf("; output: %q", text)
}
