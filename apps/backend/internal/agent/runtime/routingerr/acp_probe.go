package routingerr

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// CommandResolver returns the argv to spawn for a given provider id. The
// returned argv MUST be non-empty when ok is true. Implementations are
// expected to consult the agent registry's BuildCommand for the matching
// agent type. Kept as a function seam so this package does not import
// the heavyweight registry/agents packages — the wire-up site (see
// registry.Provide) injects a concrete resolver at boot.
type CommandResolver func(providerID string) (argv []string, env map[string]string, ok bool)

// ACPProbe is a cheap availability check that spawns the agent's ACP
// binary, performs a single JSON-RPC `initialize` round-trip, and tears
// the subprocess down. It never starts a session, so the probe avoids
// the full launch path's worktree/MCP/credential overhead.
//
// Implements ProviderProber. Wired in at boot via RegisterProber for
// each routable provider id (see internal/agent/registry/provider.go).
//
// Behaviour:
//   - On a clean initialize response within DefaultProbeTimeout → returns
//     nil so the caller flips the provider back to healthy.
//   - On any failure (binary missing, non-zero exit, malformed reply,
//     timeout) → returns a *Error produced by Classify, with
//     Phase=PhaseAuthCheck because this is the auth-check stage.
//
// The probe is concurrency-safe — each call spawns its own subprocess.
type ACPProbe struct {
	resolver CommandResolver
	logger   *logger.Logger
	timeout  time.Duration
}

// DefaultProbeTimeout bounds a single probe round-trip. Real ACP CLIs
// reliably respond to initialize in under one second when authed; five
// seconds gives generous headroom for cold-start npm-cache fetches
// without blocking the retry endpoint for too long.
const DefaultProbeTimeout = 5 * time.Second

// NewACPProbe builds an ACPProbe with the supplied command resolver and
// the default 5s timeout. Pass a nil logger to fall back to the default.
func NewACPProbe(resolver CommandResolver, log *logger.Logger) *ACPProbe {
	if log == nil {
		log = logger.Default()
	}
	return &ACPProbe{
		resolver: resolver,
		logger:   log.WithFields(zap.String("component", "acp-probe")),
		timeout:  DefaultProbeTimeout,
	}
}

// Probe implements ProviderProber. See ACPProbe doc for behaviour.
func (p *ACPProbe) Probe(ctx context.Context, in ProbeInput) *Error {
	if p.resolver == nil {
		return Classify(Input{
			Phase:      PhaseAuthCheck,
			ProviderID: in.ProviderID,
			Stderr:     "acp-probe: no command resolver wired",
		})
	}
	argv, env, ok := p.resolver(in.ProviderID)
	if !ok || len(argv) == 0 {
		return Classify(Input{
			Phase:      PhaseAuthCheck,
			ProviderID: in.ProviderID,
			Stderr:     "acp-probe: provider has no resolvable command",
		})
	}

	pCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	cmd := exec.CommandContext(pCtx, argv[0], argv[1:]...)
	if len(env) > 0 {
		cmd.Env = mergeEnv(env)
	}
	return p.runProbe(pCtx, cmd, in)
}

// runProbe drives the subprocess lifecycle: spawn → send initialize →
// read one frame → kill. Splitting it out keeps Probe under the
// complexity budget. Returns nil on a clean initialize response.
func (p *ACPProbe) runProbe(ctx context.Context, cmd *exec.Cmd, in ProbeInput) *Error {
	stdin, stdout, stderrBuf, err := setupPipes(cmd)
	if err != nil {
		return Classify(Input{
			Phase:      PhaseAuthCheck,
			ProviderID: in.ProviderID,
			Stderr:     fmt.Sprintf("acp-probe: pipe setup: %v", err),
		})
	}
	if err := cmd.Start(); err != nil {
		return Classify(Input{
			Phase:      PhaseAuthCheck,
			ProviderID: in.ProviderID,
			Stderr:     fmt.Sprintf("acp-probe: spawn: %v", err),
		})
	}

	// Always reap the subprocess; the deferred Kill is a safety net
	// for paths that exit before the explicit teardown below.
	var probeErr atomic.Pointer[Error]
	defer func() { _ = cmd.Process.Kill() }()

	if err := writeInitialize(stdin); err != nil {
		probeErr.Store(Classify(Input{
			Phase:      PhaseAuthCheck,
			ProviderID: in.ProviderID,
			Stderr:     fmt.Sprintf("acp-probe: write initialize: %v", err),
		}))
		_ = stdin.Close()
		return probeErr.Load()
	}
	_ = stdin.Close()

	resp, readErr := readOneFrame(ctx, stdout)
	exitCode := waitExitCode(cmd)
	stderrStr := stderrBuf.String()

	if readErr == nil && hasInitializeResult(resp) {
		return nil
	}

	stderr := stderrStr
	if readErr != nil {
		stderr = appendErr(stderr, readErr.Error())
	}
	return Classify(Input{
		Phase:      PhaseAuthCheck,
		ProviderID: in.ProviderID,
		ExitCode:   exitCode,
		Stderr:     stderr,
		Stdout:     string(resp),
	})
}

// setupPipes wires stdin, stdout, and a captured stderr buffer onto
// cmd. Stderr is captured into a bytes.Buffer so the classifier sees
// it even if the process dies before stdout reads any frames.
func setupPipes(cmd *exec.Cmd) (io.WriteCloser, io.ReadCloser, *bytes.Buffer, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("stdout: %w", err)
	}
	stderrBuf := &bytes.Buffer{}
	cmd.Stderr = stderrBuf
	return stdin, stdout, stderrBuf, nil
}

// writeInitialize writes a JSON-RPC initialize request. The protocol
// version is hardcoded — production probers do not negotiate; either
// the binary speaks ACP v1 or we treat it as unavailable.
func writeInitialize(w io.Writer) error {
	frame := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": 1,
			"clientInfo": map[string]any{
				"name":    "kandev-routing-probe",
				"version": "1.0.0",
			},
			"clientCapabilities": map[string]any{},
		},
	}
	b, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	if _, err := w.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

// readOneFrame reads one newline-delimited JSON frame from stdout. The
// caller cancels via context; on cancel we return the most recent bytes
// (typically empty) so the classifier can see whatever the process
// emitted before timing out.
func readOneFrame(ctx context.Context, r io.Reader) ([]byte, error) {
	type result struct {
		line []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		scanner := bufio.NewScanner(r)
		// Allow large initialize responses; real Claude responses can
		// pack ~32 KiB of capability metadata.
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 256*1024)
		if scanner.Scan() {
			done <- result{line: scanner.Bytes(), err: nil}
			return
		}
		done <- result{err: scanner.Err()}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-done:
		if r.err != nil {
			return r.line, r.err
		}
		if len(r.line) == 0 {
			return nil, errors.New("acp-probe: empty stdout")
		}
		return r.line, nil
	}
}

// hasInitializeResult parses one JSON-RPC frame and reports whether it
// is a successful initialize response. The schema check is intentionally
// loose — any frame with an `id` and a non-error `result` field is
// treated as a successful handshake.
func hasInitializeResult(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	var frame struct {
		ID     any             `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(b, &frame); err != nil {
		return false
	}
	if len(frame.Error) > 0 && string(frame.Error) != "null" {
		return false
	}
	return frame.ID != nil && len(frame.Result) > 0
}

// waitExitCode reaps the subprocess and returns its exit code as a
// pointer. Returns nil when the process is still running or the exit
// code cannot be determined (e.g. signal).
func waitExitCode(cmd *exec.Cmd) *int {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code := ee.ExitCode()
			return &code
		}
		return nil
	case <-time.After(100 * time.Millisecond):
		// Process is still running; the deferred Kill in runProbe
		// will tear it down once Probe returns.
		return nil
	}
}

// mergeEnv combines the supplied env overrides with os.Environ-style
// semantics: caller-supplied entries take precedence; os.Environ entries
// fill the gaps so the subprocess inherits the host PATH and creds.
func mergeEnv(overrides map[string]string) []string {
	envMap := map[string]string{}
	for _, e := range osEnviron() {
		k, v := splitEnv(e)
		envMap[k] = v
	}
	for k, v := range overrides {
		envMap[k] = v
	}
	out := make([]string, 0, len(envMap))
	for k, v := range envMap {
		out = append(out, k+"="+v)
	}
	return out
}

// splitEnv splits a "KEY=value" entry, falling back to (entry, "") when
// no '=' is present so malformed entries do not vanish.
func splitEnv(entry string) (string, string) {
	idx := strings.IndexByte(entry, '=')
	if idx < 0 {
		return entry, ""
	}
	return entry[:idx], entry[idx+1:]
}

// osEnviron is exposed as a package-level variable so tests can stub
// the host env without touching os.Environ.
var osEnviron = defaultOsEnviron

// appendErr joins existing stderr text with a new error string,
// inserting a newline only when both halves are non-empty.
func appendErr(existing, add string) string {
	if existing == "" {
		return add
	}
	if add == "" {
		return existing
	}
	return existing + "\n" + add
}
