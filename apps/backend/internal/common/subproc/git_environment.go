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
	words, ok := parseShellWords(command, commandEnd)
	if !ok {
		return command
	}

	removals := make([]shellRange, 0, len(words))
	for index := 0; index < len(words); index++ {
		word := words[index]
		end := word.end
		remove := false
		switch word.value {
		case "-oBatchMode=yes":
			// Match the decoded value so shell quoting around the option is
			// removed together with the option itself.
			remove = true
		case "-o":
			if index+1 < len(words) && words[index+1].value == "BatchMode=yes" {
				end = words[index+1].end
				index++
				remove = true
			}
		}
		if !remove {
			continue
		}

		start := word.start
		for start > commandEnd && isShellSpace(command[start-1]) {
			start--
		}
		removals = append(removals, shellRange{start: start, end: end})
	}

	if len(removals) == 0 {
		return command
	}
	var result strings.Builder
	result.Grow(len(command))
	cursor := 0
	for _, removal := range removals {
		if removal.start < cursor {
			continue
		}
		result.WriteString(command[cursor:removal.start])
		cursor = removal.end
	}
	result.WriteString(command[cursor:])
	return result.String()
}

type shellRange struct {
	start int
	end   int
}

type shellWord struct {
	start int
	end   int
	value string
}

func parseShellWords(command string, offset int) ([]shellWord, bool) {
	words := make([]shellWord, 0)
	for offset < len(command) {
		for offset < len(command) && isShellSpace(command[offset]) {
			offset++
		}
		if offset == len(command) {
			break
		}

		word, next, ok := parseShellWord(command, offset)
		if !ok {
			return nil, false
		}
		words = append(words, word)
		offset = next
	}
	return words, true
}

func parseShellWord(command string, start int) (shellWord, int, bool) {
	offset := start
	var value strings.Builder
	for offset < len(command) && !isShellSpace(command[offset]) {
		next, ok := parseShellWordPart(command, offset, &value)
		if !ok {
			return shellWord{}, 0, false
		}
		offset = next
	}
	return shellWord{start: start, end: offset, value: value.String()}, offset, true
}

func parseShellWordPart(command string, offset int, value *strings.Builder) (int, bool) {
	switch command[offset] {
	case '\'':
		return parseSingleQuotedShellWordPart(command, offset, value)
	case '"':
		return parseDoubleQuotedShellWordPart(command, offset, value)
	case '\\':
		return parseEscapedShellWordPart(command, offset, value)
	default:
		value.WriteByte(command[offset])
		return offset + 1, true
	}
}

func parseSingleQuotedShellWordPart(command string, offset int, value *strings.Builder) (int, bool) {
	offset++
	for offset < len(command) && command[offset] != '\'' {
		value.WriteByte(command[offset])
		offset++
	}
	if offset == len(command) {
		return 0, false
	}
	return offset + 1, true
}

func parseDoubleQuotedShellWordPart(command string, offset int, value *strings.Builder) (int, bool) {
	offset++
	for offset < len(command) {
		if command[offset] == '"' {
			return offset + 1, true
		}
		if command[offset] == '\\' {
			next, ok := parseDoubleQuotedEscape(command, offset, value)
			if !ok {
				return 0, false
			}
			offset = next
			continue
		}
		value.WriteByte(command[offset])
		offset++
	}
	return 0, false
}

func parseDoubleQuotedEscape(command string, offset int, value *strings.Builder) (int, bool) {
	if offset+1 >= len(command) {
		return 0, false
	}
	if command[offset+1] != '\n' {
		value.WriteByte(command[offset+1])
	}
	return offset + 2, true
}

func parseEscapedShellWordPart(command string, offset int, value *strings.Builder) (int, bool) {
	if offset+1 >= len(command) {
		return 0, false
	}
	if command[offset+1] != '\n' {
		value.WriteByte(command[offset+1])
	}
	return offset + 2, true
}

func isShellSpace(character byte) bool {
	switch character {
	case ' ', '\t', '\n', '\r':
		return true
	default:
		return false
	}
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
