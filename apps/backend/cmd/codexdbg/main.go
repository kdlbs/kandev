// Command codexdbg runs bounded native Codex app-server probes and inspects
// their local JSONL captures. It never attaches to a running Kandev session.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/kandev/kandev/internal/agent/codexdbg"
	"github.com/kandev/kandev/pkg/codexappserver"
)

const (
	defaultTimeout      = 2 * time.Minute
	commandPrompt       = "prompt"
	commandInterrupt    = "interrupt"
	commandThreadResume = "thread-resume"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}
	if args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		printUsage(stdout)
		return 0
	}
	command := args[0]
	if command == "inspect" {
		if err := runInspect(args[1:], stdout, stderr); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return 0
			}
			_, _ = fmt.Fprintf(stderr, "codexdbg: %v\n", err)
			return 1
		}
		return 0
	}
	opts, err := parseCommand(command, args[1:], stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		_, _ = fmt.Fprintf(stderr, "codexdbg: %v\n", err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := execute(ctx, opts, stdout); err != nil {
		_, _ = fmt.Fprintf(stderr, "codexdbg: %v\n", err)
		return 1
	}
	return 0
}

func printUsage(writer io.Writer) {
	_, _ = fmt.Fprintln(writer, `codexdbg: native Codex app-server diagnostic tool

Usage:
  codexdbg probe [flags]
  codexdbg prompt --prompt TEXT [flags]
  codexdbg thread-read --thread-id ID [flags]
  codexdbg thread-resume --thread-id ID --prompt TEXT [flags]
  codexdbg thread-fork --thread-id ID --through-turn ID [flags]
  codexdbg interrupt --prompt TEXT [--after 2s] [flags]
  codexdbg mcp-probe [flags]
  codexdbg inspect --file CAPTURE.jsonl [--thread-id ID] [--turn-id ID]

Operation flags:
  --executable PATH       Codex CLI executable (default codex)
  --arg ARG               app-server argument; repeat to pass multiple arguments (default app-server)
  --timeout DURATION      operation deadline (default 2m)
  --out DIR               capture directory (default ./codex-app-server-debug)
  --file PATH             exact capture output path; it must not already exist
  --workdir DIR           use this existing working directory (default: private temporary directory)
  --capture-stderr        include bounded child stderr in the capture
  --answer-file PATH      explicit responses for supported server requests
  --prompt TEXT           prompt text for prompt, thread-resume, and interrupt
  --thread-id ID          existing native thread for read, resume, and fork
  --through-turn ID       last source turn to include in a fork
  --after DURATION        interrupt delay (default 2s)
  --linger DURATION       observe background events after root-turn completion (default 0)

Probe initializes the app-server and reads models and experimental features. It does not create
a thread or start a model turn. inspect is offline and does not start Codex. Server approvals
default to decline; unknown server requests are rejected.`)
}

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

type commandOptions struct {
	command     string
	executable  string
	args        []string
	outDir      string
	file        string
	workdir     string
	timeout     time.Duration
	linger      time.Duration
	after       time.Duration
	stderr      bool
	answerFile  string
	prompt      string
	threadID    string
	throughTurn string
}

func parseCommand(command string, args []string, stderr io.Writer) (commandOptions, error) {
	allowed := map[string]bool{"probe": true, "prompt": true, "thread-read": true, "thread-resume": true, "thread-fork": true, "interrupt": true, "mcp-probe": true}
	if !allowed[command] {
		return commandOptions{}, fmt.Errorf("unknown subcommand %q; run codexdbg --help", command)
	}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(stderr)
	opts := commandOptions{command: command, executable: "codex", outDir: "./codex-app-server-debug", timeout: defaultTimeout, after: 2 * time.Second}
	var explicitArgs stringList
	fs.StringVar(&opts.executable, "executable", opts.executable, "Codex CLI executable")
	fs.Var(&explicitArgs, "arg", "app-server argument; repeat for multiple arguments")
	fs.StringVar(&opts.outDir, "out", opts.outDir, "capture output directory")
	fs.StringVar(&opts.file, "file", "", "exact capture output path")
	fs.StringVar(&opts.workdir, "workdir", "", "existing working directory")
	fs.DurationVar(&opts.timeout, "timeout", opts.timeout, "operation deadline")
	fs.DurationVar(&opts.linger, "linger", 0, "post-turn observation interval")
	fs.DurationVar(&opts.after, "after", opts.after, "interrupt delay")
	fs.BoolVar(&opts.stderr, "capture-stderr", false, "include bounded child stderr in capture")
	fs.StringVar(&opts.answerFile, "answer-file", "", "explicit method-specific server request responses")
	fs.StringVar(&opts.prompt, "prompt", "", "prompt text")
	fs.StringVar(&opts.threadID, "thread-id", "", "native thread id")
	fs.StringVar(&opts.throughTurn, "through-turn", "", "last source turn to include in a fork")
	if err := fs.Parse(args); err != nil {
		return commandOptions{}, err
	}
	if len(fs.Args()) != 0 {
		return commandOptions{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	if len(explicitArgs) == 0 {
		opts.args = []string{"app-server"}
	} else {
		opts.args = append([]string(nil), explicitArgs...)
	}
	if err := validateCommandOptions(command, opts); err != nil {
		return commandOptions{}, err
	}
	return opts, nil
}

func validateCommandOptions(command string, opts commandOptions) error {
	if opts.timeout <= 0 || opts.linger < 0 || opts.after < 0 {
		return errors.New("timeout must be positive and linger and interrupt delay cannot be negative")
	}
	if opts.executable == "" {
		return errors.New("--executable cannot be empty")
	}
	switch command {
	case commandPrompt, commandInterrupt:
		if strings.TrimSpace(opts.prompt) == "" {
			return errors.New("--prompt is required")
		}
	case "thread-read":
		if strings.TrimSpace(opts.threadID) == "" {
			return errors.New("--thread-id is required")
		}
	case commandThreadResume:
		if strings.TrimSpace(opts.threadID) == "" || strings.TrimSpace(opts.prompt) == "" {
			return errors.New("--thread-id and --prompt are required")
		}
	case "thread-fork":
		if strings.TrimSpace(opts.threadID) == "" || strings.TrimSpace(opts.throughTurn) == "" {
			return errors.New("--thread-id and --through-turn are required")
		}
	}
	return nil
}

func execute(parent context.Context, opts commandOptions, stdout io.Writer) (retErr error) {
	ctx, cancel, err := operationContext(parent, opts)
	if err != nil {
		return err
	}
	defer cancel()
	runner, sentinel, answers, err := prepareDebugRunner(ctx, opts)
	if err != nil {
		return err
	}
	if sentinel != nil {
		defer sentinel.Close()
	}
	defer func() {
		reason := "completed"
		if retErr != nil {
			reason = "error"
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			reason = "timeout"
		}
		retErr = errors.Join(retErr, runner.Close(reason))
	}()
	runner.Client().SetRequestHandler(codexdbg.NewRequestPolicy(answers).Handle)
	return executeOperation(ctx, opts, runner, sentinel, stdout)
}

func operationContext(parent context.Context, opts commandOptions) (context.Context, context.CancelFunc, error) {
	if opts.command == commandInterrupt && opts.after > opts.timeout {
		return nil, nil, errors.New("--after must be shorter than --timeout")
	}
	deadline := opts.timeout
	if opts.command == commandPrompt || opts.command == commandThreadResume || opts.command == commandInterrupt {
		if opts.linger > time.Duration(1<<63-1)-opts.timeout {
			return nil, nil, errors.New("--timeout plus --linger is too large")
		}
		deadline += opts.linger
	}
	ctx, cancel := context.WithTimeout(parent, deadline)
	return ctx, cancel, nil
}

func prepareDebugRunner(ctx context.Context, opts commandOptions) (*codexdbg.Runner, *codexdbg.MCPSentinel, map[string]json.RawMessage, error) {
	var answers map[string]json.RawMessage
	if opts.answerFile != "" {
		loaded, err := codexdbg.LoadAnswerFile(opts.answerFile)
		if err != nil {
			return nil, nil, nil, err
		}
		answers = loaded
	}
	args := append([]string(nil), opts.args...)
	var sentinel *codexdbg.MCPSentinel
	if opts.command == "mcp-probe" {
		sentinel = codexdbg.NewMCPSentinel()
		args = append(args, sentinel.ConfigArgs()...)
	}
	runner, err := startDebugRunner(ctx, opts, args)
	if err != nil && sentinel != nil {
		sentinel.Close()
	}
	return runner, sentinel, answers, err
}

func startDebugRunner(ctx context.Context, opts commandOptions, args []string) (*codexdbg.Runner, error) {
	version, err := codexdbg.ResolveExecutable(opts.executable)
	if err != nil {
		return nil, err
	}
	executableVersion, err := codexdbg.ReadExecutableVersion(ctx, version)
	if err != nil {
		return nil, err
	}
	capturePath, err := resolveCapturePath(opts)
	if err != nil {
		return nil, err
	}
	return codexdbg.StartRunner(ctx, codexdbg.RunnerConfig{
		Executable: version, Args: args, Workdir: opts.workdir, CapturePath: capturePath,
		ExecutableVersion: executableVersion, Operation: opts.command, CaptureStderr: opts.stderr,
	})
}

func executeOperation(ctx context.Context, opts commandOptions, runner *codexdbg.Runner, sentinel *codexdbg.MCPSentinel, stdout io.Writer) error {
	client := runner.Client()
	operations := map[string]func() error{
		"probe":       func() error { return executeProbe(ctx, client, stdout, runner.CapturePath()) },
		commandPrompt: func() error { return executePrompt(ctx, client, opts, runner, stdout) },
		"thread-read": func() error { return executeThreadRead(ctx, client, opts.threadID, stdout, runner.CapturePath()) },
		commandThreadResume: func() error {
			return executeThreadResume(ctx, client, opts, stdout, runner.CapturePath())
		},
		"thread-fork": func() error {
			return executeThreadFork(ctx, client, opts.threadID, opts.throughTurn, stdout, runner.CapturePath())
		},
		commandInterrupt: func() error { return executeInterrupt(ctx, client, opts, runner, stdout) },
		"mcp-probe":      func() error { return executeMCPProbe(ctx, client, sentinel, runner, stdout) },
	}
	operation := operations[opts.command]
	if operation == nil {
		return fmt.Errorf("unsupported operation %q", opts.command)
	}
	if err := operation(); err != nil {
		return err
	}
	_, err := fmt.Fprintf(stdout, "capture: %s\n", runner.CapturePath())
	return err
}

func executeProbe(ctx context.Context, client *codexappserver.Client, stdout io.Writer, capturePath string) error {
	result, err := codexdbg.Probe(ctx, client)
	if err != nil {
		return withCapture(err, capturePath)
	}
	printProbe(stdout, result)
	return nil
}

func executePrompt(ctx context.Context, client *codexappserver.Client, opts commandOptions, runner *codexdbg.Runner, stdout io.Writer) error {
	result, err := codexdbg.RunPrompt(ctx, client, opts.prompt, runner.Workdir(), opts.linger)
	if err != nil {
		return withCapture(err, runner.CapturePath())
	}
	printTurn(stdout, result)
	return nil
}

func executeThreadRead(ctx context.Context, client *codexappserver.Client, threadID string, stdout io.Writer, capturePath string) error {
	result, err := codexdbg.ReadThread(ctx, client, threadID)
	if err != nil {
		return withCapture(err, capturePath)
	}
	printThread(stdout, result)
	return nil
}

func executeThreadResume(ctx context.Context, client *codexappserver.Client, opts commandOptions, stdout io.Writer, capturePath string) error {
	result, err := codexdbg.ResumePrompt(ctx, client, opts.threadID, opts.prompt, opts.workdir, opts.linger)
	if err != nil {
		return withCapture(err, capturePath)
	}
	printTurn(stdout, result)
	return nil
}

func executeThreadFork(ctx context.Context, client *codexappserver.Client, threadID, throughTurn string, stdout io.Writer, capturePath string) error {
	result, err := codexdbg.ForkThread(ctx, client, threadID, throughTurn)
	if err != nil {
		return withCapture(err, capturePath)
	}
	printThread(stdout, result)
	return nil
}

func executeInterrupt(ctx context.Context, client *codexappserver.Client, opts commandOptions, runner *codexdbg.Runner, stdout io.Writer) error {
	result, err := codexdbg.InterruptPrompt(ctx, client, opts.prompt, runner.Workdir(), opts.after, opts.linger)
	if err != nil {
		return withCapture(err, runner.CapturePath())
	}
	printTurn(stdout, result)
	return nil
}

func executeMCPProbe(ctx context.Context, client *codexappserver.Client, sentinel *codexdbg.MCPSentinel, runner *codexdbg.Runner, stdout io.Writer) error {
	result, err := codexdbg.MCPProbe(ctx, client, sentinel, runner.Workdir())
	if err != nil {
		return withCapture(err, runner.CapturePath())
	}
	printMCP(stdout, result)
	return nil
}

func resolveCapturePath(opts commandOptions) (string, error) {
	if opts.file != "" {
		return filepath.Clean(opts.file), nil
	}
	name := fmt.Sprintf("%s-%s.jsonl", opts.command, time.Now().UTC().Format("20060102T150405.000000000Z"))
	return filepath.Join(opts.outDir, name), nil
}

func withCapture(err error, capturePath string) error {
	return fmt.Errorf("%w (capture: %s)", err, capturePath)
}

func printProbe(writer io.Writer, result codexdbg.ProbeResult) {
	_, _ = fmt.Fprintf(writer, "status: initialized\nversion: %s\nprotocol_schema: %s\nmodels: %d\nexperimental_features: %d\n", result.Version, result.SchemaHash, len(result.Models), len(result.Features))
	for _, model := range result.Models {
		id := model.Model
		if id == "" {
			id = model.ID
		}
		_, _ = fmt.Fprintf(writer, "model: %s (%s)\n", id, model.DisplayName)
	}
}

func printTurn(writer io.Writer, result codexdbg.TurnResult) {
	_, _ = fmt.Fprintf(writer, "thread: %s\nturn: %s\nstatus: %s\ncompletion_observed: %t\ninterrupt_requested: %t\nnotifications_seen: %d\n", result.ThreadID, result.TurnID, result.Status, result.CompletionObserved, result.InterruptRequested, result.NotificationsSeen)
}

func printThread(writer io.Writer, thread codexappserver.Thread) {
	_, _ = fmt.Fprintf(writer, "thread: %s\nturns: %d\n", thread.ID, len(thread.Turns))
	if thread.ParentThreadID != nil {
		_, _ = fmt.Fprintf(writer, "parent_thread: %s\n", *thread.ParentThreadID)
	}
	if thread.ForkedFromID != nil {
		_, _ = fmt.Fprintf(writer, "forked_from: %s\n", *thread.ForkedFromID)
	}
	if len(thread.Status) != 0 {
		_, _ = fmt.Fprintf(writer, "thread_status: %s\n", thread.Status)
	}
	for _, turn := range thread.Turns {
		_, _ = fmt.Fprintf(writer, "turn: %s status=%s items=%d\n", turn.ID, turn.Status, len(turn.Items))
	}
}

func printMCP(writer io.Writer, result codexdbg.MCPProbeResult) {
	_, _ = fmt.Fprintf(writer, "thread: %s\nsentinel_configured: %t\nsentinel_connected: %t\ninitialize_observed: %t\ntools_list_observed: %t (tools: %d)\ntool_call_observed: %t\ntool_call_succeeded: %t\n", result.ThreadID, result.Configured, result.Connected, result.InitializeObserved, result.ToolsListObserved, result.ToolCount, result.ToolCallObserved, result.ToolCallSucceeded)
	if !result.InitializeObserved {
		_, _ = fmt.Fprintln(writer, "sentinel traffic: unobserved; this does not prove that the app-server rejected the configuration")
	}
}

type inspectFlags struct {
	file, threadID, turnID string
}

func runInspect(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var opts inspectFlags
	fs.StringVar(&opts.file, "file", "", "JSONL capture to inspect")
	fs.StringVar(&opts.threadID, "thread-id", "", "only show entries for this thread")
	fs.StringVar(&opts.turnID, "turn-id", "", "only show entries for this turn")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	if opts.file == "" {
		return errors.New("--file is required")
	}
	entries, err := codexdbg.ReadCapture(opts.file)
	if err != nil {
		return err
	}
	items := inspectEntries(entries, opts.threadID, opts.turnID)
	encoded, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, string(encoded))
	return err
}

type inspectedEntry struct {
	Sequence          uint64          `json:"sequence"`
	Kind              string          `json:"kind"`
	Direction         string          `json:"direction,omitempty"`
	Method            string          `json:"method,omitempty"`
	RequestID         json.RawMessage `json:"request_id,omitempty"`
	ResolvesRequestID json.RawMessage `json:"resolves_request_id,omitempty"`
	ResponseTo        string          `json:"response_to,omitempty"`
	ThreadID          string          `json:"thread_id,omitempty"`
	TurnID            string          `json:"turn_id,omitempty"`
	ResponseID        string          `json:"response_id,omitempty"`
	ItemType          string          `json:"item_type,omitempty"`
	Scope             string          `json:"usage_scope,omitempty"`
	Usage             map[string]any  `json:"usage,omitempty"`
	Event             string          `json:"event,omitempty"`
	Meta              map[string]any  `json:"meta,omitempty"`
}

func inspectEntries(entries []codexdbg.CaptureEntry, threadID, turnID string) []inspectedEntry {
	requestMethods, requestParams, serverRequestMethods, serverRequestParams := indexCapturedRequests(entries)
	out := make([]inspectedEntry, 0, len(entries))
	for _, entry := range entries {
		item := inspectCaptureEntry(entry, requestMethods, requestParams, serverRequestMethods, serverRequestParams)
		if matchesInspectFilter(item, threadID, turnID) {
			out = append(out, item)
		}
	}
	return out
}

type capturedFrame struct {
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
}

func indexCapturedRequests(entries []codexdbg.CaptureEntry) (map[string]string, map[string]json.RawMessage, map[string]string, map[string]json.RawMessage) {
	methods := make(map[string]string)
	params := make(map[string]json.RawMessage)
	serverMethods := make(map[string]string)
	serverParams := make(map[string]json.RawMessage)
	for _, entry := range entries {
		if entry.Kind != "frame" || entry.Method == "" || len(entry.RequestID) == 0 {
			continue
		}
		key := jsonIDKey(entry.RequestID)
		var frame capturedFrame
		if json.Unmarshal(entry.Frame, &frame) == nil {
			switch entry.Direction {
			case codexappserver.FrameSent:
				methods[key] = entry.Method
				params[key] = frame.Params
			case codexappserver.FrameReceived:
				serverMethods[key] = entry.Method
				serverParams[key] = frame.Params
			}
		}
	}
	return methods, params, serverMethods, serverParams
}

func inspectCaptureEntry(entry codexdbg.CaptureEntry, methods map[string]string, requestParams map[string]json.RawMessage, serverMethods map[string]string, serverRequestParams map[string]json.RawMessage) inspectedEntry {
	item := inspectedEntry{Sequence: entry.Sequence, Kind: entry.Kind, Direction: string(entry.Direction), Method: entry.Method, RequestID: entry.RequestID, ResponseTo: entry.ResponseTo, Event: entry.Event, Meta: entry.Meta}
	if entry.Kind != "frame" {
		return item
	}
	var frame capturedFrame
	if json.Unmarshal(entry.Frame, &frame) != nil {
		return item
	}
	target := frame.Params
	if entry.Method == "" {
		target = frame.Result
	}
	item.ThreadID, item.TurnID = captureIDs(target)
	setInspectNativeIdentity(&item, entry, frame, serverMethods)
	setInspectRequestContext(&item, entry, requestParams, serverRequestParams)
	setInspectUsage(&item, entry, frame, methods)
	if item.Method == "" {
		item.Method = entry.Method
	}
	return item
}

func setInspectNativeIdentity(item *inspectedEntry, entry codexdbg.CaptureEntry, frame capturedFrame, serverMethods map[string]string) {
	if entry.Method == codexappserver.NotificationRawResponse {
		var response struct {
			ResponseID string `json:"responseId"`
		}
		if json.Unmarshal(frame.Params, &response) == nil {
			item.ResponseID = response.ResponseID
		}
	}
	if entry.Method == "item/started" || entry.Method == "item/completed" {
		var notification struct {
			Item struct {
				Type string `json:"type"`
			} `json:"item"`
		}
		if json.Unmarshal(frame.Params, &notification) == nil {
			item.ItemType = notification.Item.Type
		}
	}
	if entry.Method == codexappserver.NotificationServerRequestResolved {
		var notification struct {
			RequestID json.RawMessage `json:"requestId"`
		}
		if json.Unmarshal(frame.Params, &notification) == nil {
			item.ResolvesRequestID = notification.RequestID
			item.ResponseTo = serverMethods[jsonIDKey(notification.RequestID)]
		}
	}
	if item.Method == "" && entry.ResponseTo != "" {
		item.Method = entry.ResponseTo
	}
}

func setInspectRequestContext(item *inspectedEntry, entry codexdbg.CaptureEntry, requestParams, serverRequestParams map[string]json.RawMessage) {
	if entry.Method == "" && entry.ResponseTo == codexappserver.MethodAccountUsageRead {
		if params := requestParams[jsonIDKey(entry.RequestID)]; len(params) != 0 {
			item.ThreadID, _ = captureIDs(params)
		}
	} else if entry.Method == "" && entry.Direction == codexappserver.FrameSent && entry.ResponseTo != "" {
		if params := serverRequestParams[jsonIDKey(entry.RequestID)]; len(params) != 0 {
			item.ThreadID, item.TurnID = captureIDs(params)
		}
	}
}

func setInspectUsage(item *inspectedEntry, entry codexdbg.CaptureEntry, frame capturedFrame, methods map[string]string) {
	switch entry.Method {
	case codexappserver.NotificationRawResponse:
		item.Scope = "response"
		item.Usage = usageFields(frame.Params)
	case codexappserver.NotificationTokenUsage:
		item.Scope = "thread"
		item.Usage = usageFields(frame.Params)
	case codexappserver.NotificationTurnComplete:
		item.Scope = "turn"
		item.Usage = usageFields(frame.Params)
	case "":
		if entry.ResponseTo == codexappserver.MethodAccountUsageRead {
			item.Scope = "provider_thread_estimate"
			item.Usage = usageFields(frame.Result)
		} else if entry.ResponseTo != "" {
			item.Method = entry.ResponseTo
		} else if method := methods[jsonIDKey(entry.RequestID)]; method != "" {
			item.Method = method
		}
	}
}

func matchesInspectFilter(item inspectedEntry, threadID, turnID string) bool {
	return (threadID == "" || item.ThreadID == threadID) && (turnID == "" || item.TurnID == turnID)
}

func jsonIDKey(raw json.RawMessage) string {
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return ""
	}
	switch id := value.(type) {
	case string:
		return "s:" + id
	case json.Number:
		return "n:" + id.String()
	default:
		return ""
	}
}

func captureIDs(raw json.RawMessage) (string, string) {
	var root any
	if json.Unmarshal(raw, &root) != nil {
		return "", ""
	}
	var threadID, turnID string
	var visit func(any)
	visit = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			for key, child := range typed {
				if text, ok := child.(string); ok {
					switch key {
					case "threadId":
						threadID = text
					case "turnId":
						turnID = text
					}
				}
				visit(child)
			}
		case []any:
			for _, child := range typed {
				visit(child)
			}
		}
	}
	visit(root)
	return threadID, turnID
}

func usageFields(raw json.RawMessage) map[string]any {
	var root any
	if json.Unmarshal(raw, &root) != nil {
		return nil
	}
	usage := make(map[string]any)
	var visit func(any)
	visit = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			for key, child := range typed {
				if (key == "usage" || key == "tokenUsage" || key == "estimatedUsageUsdMicros" || key == "credits") && child != nil {
					usage[key] = child
				}
			}
			for _, child := range typed {
				visit(child)
			}
		case []any:
			for _, child := range typed {
				visit(child)
			}
		}
	}
	visit(root)
	if len(usage) == 0 {
		return nil
	}
	return usage
}
