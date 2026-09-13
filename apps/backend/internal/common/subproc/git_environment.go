package subproc

import (
	"os/exec"
	"runtime"
	"strings"
)

const (
	gitTerminalPromptAssignment = "GIT_TERMINAL_PROMPT=0"
	gcmInteractiveAssignment    = "GCM_INTERACTIVE=Never"
	gcmGUIPromptAssignment      = "GCM_GUI_PROMPT=0"
	gitAskpassAssignment        = "GIT_ASKPASS=exit 1"
	sshAskpassAssignment        = "SSH_ASKPASS=exit 1"
	sshAskpassRequireAssignment = "SSH_ASKPASS_REQUIRE=never"
	defaultGitSSHCommand        = "ssh -oBatchMode=yes"
)

// PrepareGitCommand applies the final managed Git policy after a caller has
// assembled its command environment. It preserves the caller's selected
// credential and configuration scope while overriding prompt controls.
func PrepareGitCommand(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.Env = prepareGitEnvironment(cmd.Environ())
}

// PrepareGitEnvironment applies the final managed Git policy to an explicit
// environment. A nil environment is treated as empty; command callers that
// need inherited values should pass os.Environ or use PrepareGitCommand.
func PrepareGitEnvironment(env []string) []string {
	return prepareGitEnvironment(env)
}

func prepareGitEnvironment(env []string) []string {
	prepared := collapseEnvironmentAssignments(env)
	prepared = replaceEnvironmentAssignment(prepared, gitTerminalPromptAssignment)
	prepared = replaceEnvironmentAssignment(prepared, gcmInteractiveAssignment)
	prepared = replaceEnvironmentAssignment(prepared, gcmGUIPromptAssignment)
	prepared = replaceEnvironmentAssignment(prepared, gitAskpassAssignment)
	prepared = replaceEnvironmentAssignment(prepared, sshAskpassAssignment)
	prepared = replaceEnvironmentAssignment(prepared, sshAskpassRequireAssignment)
	prepared = replaceEnvironmentAssignment(prepared, "GIT_SSH_COMMAND="+gitSSHCommand(prepared))
	return prepared
}

func collapseEnvironmentAssignments(env []string) []string {
	last := make(map[string]int, len(env))
	for index, entry := range env {
		key, ok := environmentAssignmentKey(entry)
		if ok {
			last[normalizeEnvironmentKey(key)] = index
		}
	}
	result := make([]string, 0, len(env))
	for index, entry := range env {
		key, ok := environmentAssignmentKey(entry)
		if ok && last[normalizeEnvironmentKey(key)] != index {
			continue
		}
		result = append(result, entry)
	}
	return result
}

func replaceEnvironmentAssignment(env []string, assignment string) []string {
	key, _, ok := strings.Cut(assignment, "=")
	if !ok {
		return env
	}
	result := make([]string, 0, len(env)+1)
	for _, entry := range env {
		entryKey, hasValue := environmentAssignmentKey(entry)
		if hasValue && environmentKeysEqual(entryKey, key) {
			continue
		}
		result = append(result, entry)
	}
	return append(result, assignment)
}

func environmentAssignmentKey(entry string) (string, bool) {
	key, _, ok := strings.Cut(entry, "=")
	return key, ok && key != ""
}

func normalizeEnvironmentKey(key string) string {
	if runtime.GOOS == "windows" {
		return strings.ToUpper(key)
	}
	return key
}

func environmentKeysEqual(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func environmentValue(env []string, key string) (string, bool) {
	for index := len(env) - 1; index >= 0; index-- {
		entryKey, ok := environmentAssignmentKey(env[index])
		if !ok || !environmentKeysEqual(entryKey, key) {
			continue
		}
		return strings.TrimPrefix(env[index], entryKey+"="), true
	}
	return "", false
}

func gitSSHCommand(env []string) string {
	if command, ok := environmentValue(env, "GIT_SSH_COMMAND"); ok && strings.TrimSpace(command) != "" {
		return ForceGitSSHBatchMode(command)
	}
	if executable, ok := environmentValue(env, "GIT_SSH"); ok && strings.TrimSpace(executable) != "" {
		return ForceGitSSHBatchMode(quoteSSHExecutable(executable))
	}
	return defaultGitSSHCommand
}

func quoteSSHExecutable(executable string) string {
	executable = strings.TrimSpace(executable)
	if executable == "" || (executable[0] == '\'' && executable[len(executable)-1] == '\'') ||
		(executable[0] == '"' && executable[len(executable)-1] == '"') {
		return executable
	}
	if strings.ContainsAny(executable, " \t\n\r") {
		return "'" + strings.ReplaceAll(executable, "'", "'\\''") + "'"
	}
	return executable
}

// ForceGitSSHBatchMode places BatchMode=yes immediately after a direct
// OpenSSH executable. Unsupported wrappers use the safe default rather than
// receiving an option in an unknown position.
func ForceGitSSHBatchMode(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return defaultGitSSHCommand
	}
	commandEnd := shellWordEnd(command)
	if commandEnd == 0 {
		return defaultGitSSHCommand
	}
	executable, ok := shellWordValue(command[:commandEnd])
	if !ok || !isOpenSSHExecutable(executable) {
		return defaultGitSSHCommand
	}
	command = removeBatchModeYes(command, commandEnd)
	return command[:commandEnd] + " -oBatchMode=yes" + command[commandEnd:]
}

func removeBatchModeYes(command string, commandEnd int) string {
	suffix := command[commandEnd:]
	for _, option := range []string{" -oBatchMode=yes", " -o BatchMode=yes"} {
		suffix = strings.ReplaceAll(suffix, option, "")
	}
	return command[:commandEnd] + suffix
}

func shellWordValue(word string) (string, bool) {
	if word == "" {
		return "", false
	}
	if word[0] == '\'' || word[0] == '"' {
		if len(word) < 2 || word[len(word)-1] != word[0] {
			return "", false
		}
		return word[1 : len(word)-1], true
	}
	if strings.ContainsAny(word, "'\"") {
		return "", false
	}
	return word, true
}

func isOpenSSHExecutable(executable string) bool {
	lastSeparator := strings.LastIndexAny(executable, `/\\`)
	base := executable[lastSeparator+1:]
	return base == "ssh" || base == "ssh.exe"
}

func shellWordEnd(command string) int {
	var quote byte
	escaped := false
	for index := 0; index < len(command); index++ {
		character := command[index]
		if escaped {
			escaped = false
			continue
		}
		if quote != 0 {
			if quote == '"' && character == '\\' {
				escaped = true
				continue
			}
			if character == quote {
				quote = 0
			}
			continue
		}
		switch character {
		case '\\':
			escaped = true
		case '\'', '"':
			quote = character
		case ' ', '\t', '\n', '\r':
			return index
		}
	}
	return len(command)
}
