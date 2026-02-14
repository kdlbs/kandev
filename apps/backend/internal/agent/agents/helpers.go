package agents

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// CmdBuilder constructs CLI command slices using a fluent API.
type CmdBuilder struct {
	args []string
}

// Cmd starts building a command from a base command and arguments.
func Cmd(base ...string) *CmdBuilder {
	return &CmdBuilder{args: append([]string{}, base...)}
}

// Model appends a model flag if model is non-empty.
// flag args support {model} placeholder, e.g. NewParam("--model", "{model}").
func (b *CmdBuilder) Model(flag Param, model string) *CmdBuilder {
	if flag.IsEmpty() || model == "" {
		return b
	}
	for _, arg := range flag.args {
		b.args = append(b.args, strings.ReplaceAll(arg, "{model}", model))
	}
	return b
}

// Resume appends a resume flag with sessionID if applicable.
// Skipped when sessionID is empty, nativeResume is true, or flag is empty.
func (b *CmdBuilder) Resume(flag Param, sessionID string, nativeResume bool) *CmdBuilder {
	if sessionID == "" || nativeResume || flag.IsEmpty() {
		return b
	}
	b.args = append(b.args, flag.args...)
	b.args = append(b.args, sessionID)
	return b
}

// Permissions appends per-tool permission flags when not auto-approving.
// Each tool gets: flag tool:ask-user (e.g. --permission launch-process:ask-user).
func (b *CmdBuilder) Permissions(flag string, tools []string, opts CommandOptions) *CmdBuilder {
	if opts.AutoApprove || flag == "" || len(tools) == 0 {
		return b
	}
	for _, tool := range tools {
		b.args = append(b.args, flag, tool+":ask-user")
	}
	return b
}

// Settings appends CLI flags for enabled permission settings.
func (b *CmdBuilder) Settings(settings map[string]PermissionSetting, values map[string]bool) *CmdBuilder {
	if settings == nil || values == nil {
		return b
	}
	for settingName, setting := range settings {
		if !setting.Supported || setting.ApplyMethod != "cli_flag" || setting.CLIFlag == "" {
			continue
		}
		value, exists := values[settingName]
		if !exists || !value {
			continue
		}
		if setting.CLIFlagValue != "" {
			b.args = append(b.args, setting.CLIFlag, setting.CLIFlagValue)
		} else {
			b.args = append(b.args, strings.Fields(setting.CLIFlag)...)
		}
	}
	return b
}

// Prompt appends a prompt flag if prompt is non-empty.
// flag args support {prompt} placeholder, e.g. NewParam("--prompt", "{prompt}").
// If flag is empty, the prompt is appended as a positional argument.
func (b *CmdBuilder) Prompt(flag Param, prompt string) *CmdBuilder {
	if prompt == "" {
		return b
	}
	if flag.IsEmpty() {
		b.args = append(b.args, prompt)
		return b
	}
	for _, arg := range flag.args {
		b.args = append(b.args, strings.ReplaceAll(arg, "{prompt}", prompt))
	}
	return b
}

// Flag appends arbitrary flag parts to the command.
func (b *CmdBuilder) Flag(parts ...string) *CmdBuilder {
	b.args = append(b.args, parts...)
	return b
}

// Build returns the final Command value.
func (b *CmdBuilder) Build() Command {
	return Command{args: b.args}
}

// execAndParse runs a command with a timeout and parses stdout with the given parser function.
func execAndParse(ctx context.Context, timeout time.Duration, parse func(string) ([]Model, error), name string, args ...string) ([]Model, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("command timed out after %v", timeout)
		}
		return nil, fmt.Errorf("command failed: %w (stderr: %s)", err, stderr.String())
	}

	return parse(stdout.String())
}

