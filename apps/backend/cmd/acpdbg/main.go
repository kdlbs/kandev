// Command acpdbg speaks raw ACP JSON-RPC to an agent CLI and captures every
// frame to a JSONL file. See .agents/skills/acp-debug/SKILL.md for usage.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kandev/kandev/internal/agent/acpdbg"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/logger"
)

const defaultTimeout = 30 * time.Second

func main() { os.Exit(run()) }

func run() int {
	if len(os.Args) < 2 {
		usage()
		return 2
	}
	sub := os.Args[1]
	args := os.Args[2:]

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var err error
	switch sub {
	case "list":
		err = runList(args)
	case "probe":
		err = runProbe(ctx, args)
	case "prompt":
		err = runPrompt(ctx, args)
	case "session-load":
		err = runSessionLoad(ctx, args)
	case "matrix":
		err = runMatrix(ctx, args)
	case "-h", "--help", "help":
		usage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "acpdbg: unknown subcommand %q\n\n", sub)
		usage()
		return 2
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "acpdbg: %v\n", err)
		return 1
	}
	return 0
}

func usage() {
	fmt.Fprintln(os.Stderr, `acpdbg — raw ACP JSON-RPC debugger

Usage:
  acpdbg list
  acpdbg probe <agent> [flags]
  acpdbg probe --exec "<cmd> [args...]" [flags]
  acpdbg prompt <agent> --prompt TEXT [--model M] [--mode M] [flags]
  acpdbg session-load <agent> --session-id ID [flags]
  acpdbg matrix [flags]

Shared flags:
  --out DIR        output directory for JSONL (default ./acp-debug/)
  --file PATH      exact JSONL output path (overrides --out)
  --timeout DUR    per-run timeout (default 30s)
  --workdir PATH   child process cwd (default: fresh temp dir)
  --verbose        mirror frames to stderr
  --stderr         capture child stderr into the JSONL
  --exec "CMD"     spawn an arbitrary command instead of a registered agent

The JSONL file records every request, response, notification, and meta
event in chronological order. See .claude/skills/acp-debug/SKILL.md for the
schema and examples.`)
}

type sharedFlags struct {
	out     string
	file    string
	timeout time.Duration
	workdir string
	verbose bool
	stderr  bool
	exec    string
}

func registerShared(fs *flag.FlagSet) *sharedFlags {
	f := &sharedFlags{}
	fs.StringVar(&f.out, "out", "./acp-debug", "output directory for JSONL files")
	fs.StringVar(&f.file, "file", "", "exact JSONL output path (overrides --out)")
	fs.DurationVar(&f.timeout, "timeout", defaultTimeout, "overall run timeout")
	fs.StringVar(&f.workdir, "workdir", "", "child process cwd (default: fresh temp dir)")
	fs.BoolVar(&f.verbose, "verbose", false, "mirror frames to stderr")
	fs.BoolVar(&f.stderr, "stderr", false, "capture child stderr into the JSONL")
	fs.StringVar(&f.exec, "exec", "", "spawn an arbitrary command instead of a registered agent")
	return f
}

func buildLogger() (*logger.Logger, error) {
	return logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
}

func loadRegistry() (*registry.Registry, error) {
	log, err := buildLogger()
	if err != nil {
		return nil, err
	}
	reg, _, err := registry.Provide(log)
	if err != nil {
		return nil, err
	}
	return reg, nil
}

// --- list ---

func runList(_ []string) error {
	reg, err := loadRegistry()
	if err != nil {
		return err
	}
	specs := acpdbg.ListACPAgents(reg)
	if len(specs) == 0 {
		fmt.Println("no ACP-capable agents found")
		return nil
	}
	fmt.Printf("%-16s  %-16s  %s\n", "ID", "DISPLAY", "COMMAND")
	for _, s := range specs {
		fmt.Printf("%-16s  %-16s  %s\n", s.ID, s.DisplayName, strings.Join(s.Command, " "))
	}
	return nil
}

// --- probe ---

func runProbe(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("probe", flag.ExitOnError)
	shared := registerShared(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := resolveRunConfig(fs, shared, "probe")
	if err != nil {
		return err
	}

	jsonlPath := resolveJSONLPath(shared, cfg.AgentID, "probe")
	runCtx, cancel := context.WithTimeout(ctx, shared.timeout)
	defer cancel()

	runner, err := acpdbg.NewRunner(runCtx, jsonlPath, cfg)
	if err != nil {
		return err
	}
	defer runner.Close("completed")

	res, err := acpdbg.Probe(runCtx, runner)
	if err != nil {
		if acpdbg.IsAuthErrorMessage(err.Error()) {
			fmt.Printf("agent:            %s\n", cfg.AgentID)
			fmt.Printf("jsonl:            %s\n", runner.Path())
			fmt.Printf("status:           auth_required\n")
			fmt.Printf("error:            %v\n", err)
			return nil
		}
		fmt.Fprintf(os.Stderr, "probe failed: %v\n", err)
		fmt.Fprintf(os.Stderr, "jsonl: %s\n", runner.Path())
		return err
	}
	printProbeSummary(cfg.AgentID, runner.Path(), res)
	return nil
}

// --- prompt ---

func runPrompt(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("prompt", flag.ExitOnError)
	shared := registerShared(fs)
	model := fs.String("model", "", "model to set before prompting")
	mode := fs.String("mode", "", "session mode to set before prompting")
	prompt := fs.String("prompt", "", "prompt text (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *prompt == "" {
		return errors.New("--prompt is required")
	}

	cfg, err := resolveRunConfig(fs, shared, "prompt")
	if err != nil {
		return err
	}

	jsonlPath := resolveJSONLPath(shared, cfg.AgentID, "prompt")
	runCtx, cancel := context.WithTimeout(ctx, shared.timeout)
	defer cancel()

	runner, err := acpdbg.NewRunner(runCtx, jsonlPath, cfg)
	if err != nil {
		return err
	}
	defer runner.Close("completed")

	res, err := acpdbg.Prompt(runCtx, runner, acpdbg.PromptOptions{
		Model:  *model,
		Mode:   *mode,
		Prompt: *prompt,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "prompt failed: %v\n", err)
		fmt.Fprintf(os.Stderr, "jsonl: %s\n", runner.Path())
		return err
	}
	printProbeSummary(cfg.AgentID, runner.Path(), &res.ProbeResult)
	fmt.Println()
	fmt.Println("--- response ---")
	fmt.Println(res.Text)
	return nil
}

// --- session-load ---

func runSessionLoad(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("session-load", flag.ExitOnError)
	shared := registerShared(fs)
	sessionID := fs.String("session-id", "", "ACP session id to load (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *sessionID == "" {
		return errors.New("--session-id is required")
	}

	cfg, err := resolveRunConfig(fs, shared, "session-load")
	if err != nil {
		return err
	}

	jsonlPath := resolveJSONLPath(shared, cfg.AgentID, "session-load")
	runCtx, cancel := context.WithTimeout(ctx, shared.timeout)
	defer cancel()

	runner, err := acpdbg.NewRunner(runCtx, jsonlPath, cfg)
	if err != nil {
		return err
	}
	defer runner.Close("completed")

	res, err := acpdbg.SessionLoad(runCtx, runner, *sessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "session-load failed: %v\n", err)
		fmt.Fprintf(os.Stderr, "jsonl: %s\n", runner.Path())
		return err
	}
	printProbeSummary(cfg.AgentID, runner.Path(), res)
	return nil
}

// --- matrix ---

type matrixEntry struct {
	AgentID          string   `json:"agent_id"`
	Status           string   `json:"status"`
	JSONL            string   `json:"jsonl"`
	AgentName        string   `json:"agent_name,omitempty"`
	AgentVersion     string   `json:"agent_version,omitempty"`
	ProtocolVersion  int      `json:"protocol_version,omitempty"`
	ModelsCount      int      `json:"models_count"`
	CurrentModelID   string   `json:"current_model_id,omitempty"`
	ModesCount       int      `json:"modes_count"`
	CurrentModeID    string   `json:"current_mode_id,omitempty"`
	AuthMethodsCount int      `json:"auth_methods_count"`
	AuthMethods      []string `json:"auth_methods,omitempty"`
	Error            string   `json:"error,omitempty"`
	DurationMs       int      `json:"duration_ms"`
}

type matrixSummary struct {
	TS     string        `json:"ts"`
	Agents []matrixEntry `json:"agents"`
}

func runMatrix(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("matrix", flag.ExitOnError)
	shared := registerShared(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if shared.exec != "" {
		return errors.New("--exec is not supported with matrix; use `probe --exec` instead")
	}

	reg, err := loadRegistry()
	if err != nil {
		return err
	}
	specs := acpdbg.ListACPAgents(reg)
	if len(specs) == 0 {
		return errors.New("no ACP-capable agents found")
	}

	if err := os.MkdirAll(shared.out, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", shared.out, err)
	}

	results := make([]matrixEntry, len(specs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4) // limit parallel npx resolutions
	for i, s := range specs {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, s acpdbg.AgentSpec) {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = probeOne(ctx, shared, s)
		}(i, s)
	}
	wg.Wait()

	printMatrixTable(results)

	summary := matrixSummary{
		TS:     time.Now().UTC().Format(time.RFC3339),
		Agents: results,
	}
	summaryPath := filepath.Join(shared.out, "matrix-summary.json")
	f, err := os.Create(summaryPath)
	if err != nil {
		return fmt.Errorf("create summary: %w", err)
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(summary); err != nil {
		_ = f.Close()
		return fmt.Errorf("encode summary: %w", err)
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Printf("\nsummary: %s\n", summaryPath)
	return nil
}

func probeOne(ctx context.Context, shared *sharedFlags, spec acpdbg.AgentSpec) matrixEntry {
	start := time.Now()
	entry := matrixEntry{
		AgentID: spec.ID,
		Status:  "failed",
	}
	jsonlPath := resolveJSONLPath(shared, spec.ID, "probe")
	entry.JSONL = jsonlPath

	runCtx, cancel := context.WithTimeout(ctx, shared.timeout)
	defer cancel()

	runner, err := acpdbg.NewRunner(runCtx, jsonlPath, acpdbg.RunConfig{
		AgentID:       spec.ID,
		Command:       spec.Command,
		Workdir:       shared.workdir,
		CaptureStderr: shared.stderr,
	})
	if err != nil {
		entry.Error = err.Error()
		entry.Status = classifyError(err.Error())
		entry.DurationMs = int(time.Since(start).Milliseconds())
		return entry
	}
	defer runner.Close("completed")

	res, err := acpdbg.Probe(runCtx, runner)
	entry.DurationMs = int(time.Since(start).Milliseconds())
	if err != nil {
		entry.Error = err.Error()
		entry.Status = classifyError(err.Error())
		return entry
	}
	entry.Status = "ok"
	entry.AgentName = res.AgentName
	entry.AgentVersion = res.AgentVersion
	entry.ProtocolVersion = res.ProtocolVersion
	entry.ModelsCount = len(res.Models)
	entry.CurrentModelID = res.CurrentModelID
	entry.ModesCount = len(res.Modes)
	entry.CurrentModeID = res.CurrentModeID
	entry.AuthMethods = res.AuthMethods
	entry.AuthMethodsCount = len(res.AuthMethods)
	return entry
}

// --- helpers ---

func resolveRunConfig(fs *flag.FlagSet, shared *sharedFlags, _ string) (acpdbg.RunConfig, error) {
	cfg := acpdbg.RunConfig{
		Workdir:       shared.workdir,
		CaptureStderr: shared.stderr,
	}
	if shared.exec != "" {
		parts := splitExec(shared.exec)
		if len(parts) == 0 {
			return cfg, errors.New("--exec is empty")
		}
		cfg.AgentID = "custom-" + acpdbg.SanitizeAgentID(filepath.Base(parts[0]))
		cfg.Command = parts
		return cfg, nil
	}
	if fs.NArg() == 0 {
		return cfg, errors.New("agent argument required (or use --exec)")
	}
	agentID := fs.Arg(0)
	reg, err := loadRegistry()
	if err != nil {
		return cfg, err
	}
	spec, err := acpdbg.LookupAgent(reg, agentID)
	if err != nil {
		return cfg, err
	}
	cfg.AgentID = spec.ID
	cfg.Command = spec.Command
	return cfg, nil
}

func resolveJSONLPath(shared *sharedFlags, agentID, op string) string {
	if shared.file != "" {
		return shared.file
	}
	return acpdbg.SuggestJSONLPath(shared.out, agentID, op)
}

func printProbeSummary(agentID, jsonlPath string, r *acpdbg.ProbeResult) {
	fmt.Printf("agent:            %s\n", agentID)
	fmt.Printf("jsonl:            %s\n", jsonlPath)
	fmt.Printf("agent_name:       %s\n", r.AgentName)
	fmt.Printf("agent_version:    %s\n", r.AgentVersion)
	fmt.Printf("protocol_version: %d\n", r.ProtocolVersion)
	fmt.Printf("session_id:       %s\n", r.SessionID)
	fmt.Printf("auth_methods:     %s\n", strings.Join(r.AuthMethods, ", "))
	fmt.Printf("models (%d):       %s (current: %s)\n", len(r.Models), strings.Join(r.Models, ", "), r.CurrentModelID)
	fmt.Printf("modes  (%d):       %s (current: %s)\n", len(r.Modes), strings.Join(r.Modes, ", "), r.CurrentModeID)
}

func printMatrixTable(rows []matrixEntry) {
	fmt.Printf("%-16s %-14s %-6s %-6s %-6s %s\n", "AGENT", "STATUS", "MODELS", "MODES", "AUTH", "FILE")
	for _, r := range rows {
		status := r.Status
		if r.Error != "" && len(status) < 14 {
			status += fmt.Sprintf(" (%s)", shortReason(r.Error))
		}
		if len(status) > 14 {
			status = status[:14]
		}
		fmt.Printf("%-16s %-14s %-6d %-6d %-6d %s\n",
			r.AgentID, status,
			r.ModelsCount, r.ModesCount, r.AuthMethodsCount,
			r.JSONL)
	}
}

func shortReason(s string) string {
	if len(s) > 30 {
		return s[:30] + "…"
	}
	return s
}

func classifyError(msg string) string {
	l := strings.ToLower(msg)
	switch {
	case strings.Contains(l, "not found") && strings.Contains(l, "exec"):
		return "not_installed"
	case strings.Contains(l, "executable file not found"):
		return "not_installed"
	case strings.Contains(l, "auth"), strings.Contains(l, "login"),
		strings.Contains(l, "credential"), strings.Contains(l, "api key"),
		strings.Contains(l, "unauthorized"):
		return "auth_required"
	case strings.Contains(l, "context deadline exceeded"):
		return "timeout"
	default:
		return "failed"
	}
}

// splitExec splits a command string on whitespace. No shell expansion —
// this is a simple splitter for development convenience. Quoted arguments
// are NOT supported; users needing that can pass a shell wrapper like
// `sh -c "…"` which ends up as three tokens and we exec sh.
func splitExec(s string) []string {
	return strings.Fields(s)
}
