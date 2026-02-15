package process

import "strings"

// escapeArg rewrites a command-line argument following the MSDN
// CommandLineToArgvW parsing rules (same algorithm as Go's
// syscall.EscapeArg on Windows):
//
//   - every backslash (\) is doubled, but only if immediately
//     followed by a double quote (");
//   - every double quote (") is escaped with a backslash (\);
//   - finally, the argument is wrapped in double quotes only if
//     it contains spaces or tabs.
//   - an empty string becomes "" (two double-quote characters).
func escapeArg(s string) string {
	if len(s) == 0 {
		return `""`
	}

	needsBackslash := false
	hasSpace := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"', '\\':
			needsBackslash = true
		case ' ', '\t':
			hasSpace = true
		}
	}

	if !needsBackslash && !hasSpace {
		return s
	}
	if !needsBackslash {
		return `"` + s + `"`
	}

	var b []byte
	if hasSpace {
		b = append(b, '"')
	}
	slashes := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		default:
			slashes = 0
		case '\\':
			slashes++
		case '"':
			for ; slashes > 0; slashes-- {
				b = append(b, '\\')
			}
			b = append(b, '\\')
		}
		b = append(b, c)
	}
	if hasSpace {
		for ; slashes > 0; slashes-- {
			b = append(b, '\\')
		}
		b = append(b, '"')
	}
	return string(b)
}

// buildCmdLine joins arguments into a single command-line string
// with proper quoting for Windows CreateProcess.
func buildCmdLine(args []string) string {
	escaped := make([]string, len(args))
	for i, arg := range args {
		escaped[i] = escapeArg(arg)
	}
	return strings.Join(escaped, " ")
}
