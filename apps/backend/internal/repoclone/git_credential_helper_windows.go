//go:build windows

package repoclone

import (
	"fmt"
	"os"
	"path/filepath"
)

// Windows does not support exec.Cmd.ExtraFiles. Keep credentials out of the
// git command line and environment values by placing them in a private,
// one-command temporary directory read by a credential-helper batch file.
func gitCredentialHelperCommand(
	auth *cloneAuth,
) (string, []*os.File, []string, func(), error) {
	dir, err := os.MkdirTemp("", "kandev-git-credential-*")
	if err != nil {
		return "", nil, nil, nil, fmt.Errorf("create Git credential directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	usernamePath := filepath.Join(dir, "username")
	passwordPath := filepath.Join(dir, "password")
	helperPath := filepath.Join(dir, "credential-helper.cmd")
	for path, value := range map[string]string{
		usernamePath: auth.username + "\r\n",
		passwordPath: auth.password + "\r\n",
	} {
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			cleanup()
			return "", nil, nil, nil, fmt.Errorf("write Git credential input: %w", err)
		}
	}
	const script = "@echo off\r\n" +
		"if /I not \"%~1\"==\"get\" exit /b 0\r\n" +
		"<nul set /p \"=username=\"\r\n" +
		"type \"%KANDEV_GIT_USERNAME_FILE%\"\r\n" +
		"<nul set /p \"=password=\"\r\n" +
		"type \"%KANDEV_GIT_PASSWORD_FILE%\"\r\n"
	if err := os.WriteFile(helperPath, []byte(script), 0o700); err != nil {
		cleanup()
		return "", nil, nil, nil, fmt.Errorf("write Git credential helper: %w", err)
	}
	quotedHelper := `"` + filepath.ToSlash(helperPath) + `"`
	return quotedHelper, nil, []string{
		gitCredentialUsernameFileEnv + "=" + usernamePath,
		gitCredentialPasswordFileEnv + "=" + passwordPath,
	}, cleanup, nil
}
