package process

// literalGitPathspec selects a named file or directory without wildcard or magic expansion.
func literalGitPathspec(path string) string {
	if path == "" {
		return ""
	}
	return ":(literal)" + path
}
