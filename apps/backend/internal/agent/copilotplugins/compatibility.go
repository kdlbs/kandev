package copilotplugins

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/common/managednpm"
)

func pluginLoadingHelpCommand(launch LaunchContext) ([]string, error) {
	configDir, _, err := configDirForLaunch(launch)
	if err != nil {
		return nil, err
	}
	command := append([]string(nil), launch.RuntimeCommand...)
	if err := managednpm.PrepareNPMProjectPrefix(command); err != nil {
		return nil, err
	}
	command = append(command, "--help")
	if configDir != "" {
		command = append(command, "--config-dir="+configDir)
	}
	return command, nil
}

func observePluginLoading(ctx context.Context, runner Runner, launch LaunchContext) (bool, ReasonCode) {
	command, err := pluginLoadingHelpCommand(launch)
	if err != nil {
		if errors.Is(err, errUnsupportedLaunchContext) {
			return false, reasonForContextError(err)
		}
		return false, ReasonCLIUnavailable
	}
	result, runErr := runner.Run(ctx, command, launch.Env, launch.WorkDir)
	if reason := commandResultReason(ctx, result, runErr); reason != "" {
		return false, reason
	}
	if !helpAdvertisesPluginDir(result.Stdout) {
		return false, ReasonPluginLoadingUnsupported
	}
	return true, ""
}

func helpAdvertisesPluginDir(output []byte) bool {
	if !utf8.Valid(output) {
		return false
	}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if value, found := strings.CutPrefix(fields[0], "--plugin-dir="); found && isPluginDirArgument(value) {
			return true
		}
		if fields[0] == "--plugin-dir" && len(fields) > 1 && isPluginDirArgument(fields[1]) {
			return true
		}
	}
	return false
}

func isPluginDirArgument(value string) bool {
	value = strings.Trim(value, "<>()[]")
	if value == "" || strings.HasPrefix(value, "--") {
		return false
	}
	switch strings.ToLower(value) {
	case "path", "dir", "directory", "plugin-dir", "plugin_dir", "plugin-directory", "plugin_directory":
		return true
	}
	if value != strings.ToUpper(value) {
		return false
	}
	for _, r := range value {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func commandResultReason(ctx context.Context, result CommandResult, runErr error) ReasonCode {
	if ctxErr := ctx.Err(); ctxErr != nil {
		if errors.Is(ctxErr, context.DeadlineExceeded) {
			return ReasonTimeout
		}
		return ReasonCancelled
	}
	if result.Truncated || len(result.Stdout) > inventoryStdoutLimit || len(result.Stderr) > inventoryStderrLimit {
		return ReasonOutputLimit
	}
	if runErr != nil || result.ExitCode != 0 {
		return ReasonCLIUnavailable
	}
	return ""
}
