package process

import "strings"

// scanArgChars inspects each byte of s and returns whether any character
// requires backslash-escaping (double-quote or backslash) and whether
// the argument contains whitespace (space or tab).
func scanArgChars(s string) (needsBackslash, hasSpace bool) {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"', '\\':
			needsBackslash = true
		case ' ', '\t':
			hasSpace = true
		}
	}
	return needsBackslash, hasSpace
}

// appendEscapedBytes appends the bytes of s to b applying MSDN
// CommandLineToArgvW backslash-doubling rules for double-quote characters.
// Returns the updated byte slice and the trailing slash count.
func appendEscapedBytes(b []byte, s string) ([]byte, int) {
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
	return b, slashes
}

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

	needsBackslash, hasSpace := scanArgChars(s)

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
	b, slashes := appendEscapedBytes(b, s)
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
