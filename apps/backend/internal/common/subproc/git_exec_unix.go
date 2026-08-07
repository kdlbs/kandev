//go:build linux || darwin

package subproc

import "golang.org/x/sys/unix"

// ExecGit replaces the current helper process with Git. Callers must hold a
// classified Git admission slot for the lifetime of that process; this seam
// only owns the platform-specific exec operation.
func ExecGit(path string, args, env []string) error {
	argv := make([]string, 0, len(args)+1)
	argv = append(argv, "git")
	argv = append(argv, args...)
	return unix.Exec(path, argv, env)
}
