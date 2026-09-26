//go:build !windows

package repoclone

import (
	"fmt"
	"os"
	"sync"
)

func gitCredentialHelperCommand(
	auth *cloneAuth,
) (string, []*os.File, []string, func(), error) {
	helper, err := os.CreateTemp("", "kandev-git-credential-helper-*")
	if err != nil {
		return "", nil, nil, nil, fmt.Errorf("create Git credential helper: %w", err)
	}
	cleanupHelper := func() { _ = os.Remove(helper.Name()) }
	const script = "#!/bin/sh\n" +
		"if [ \"$1\" = get ] && IFS= read -r username <&4 && IFS= read -r password <&3; then\n" +
		"  printf 'username=%s\\npassword=%s\\n' \"$username\" \"$password\"\n" +
		"fi\n"
	if _, err := helper.WriteString(script); err != nil {
		_ = helper.Close()
		cleanupHelper()
		return "", nil, nil, nil, fmt.Errorf("write Git credential helper: %w", err)
	}
	if err := helper.Close(); err != nil {
		cleanupHelper()
		return "", nil, nil, nil, fmt.Errorf("close Git credential helper: %w", err)
	}
	if err := os.Chmod(helper.Name(), 0o700); err != nil {
		cleanupHelper()
		return "", nil, nil, nil, fmt.Errorf("set Git credential helper permissions: %w", err)
	}
	passwordReader, stopPassword, err := repeatingCredentialPipe(auth.password)
	if err != nil {
		cleanupHelper()
		return "", nil, nil, nil, err
	}
	usernameReader, stopUsername, err := repeatingCredentialPipe(auth.username)
	if err != nil {
		stopPassword()
		cleanupHelper()
		return "", nil, nil, nil, err
	}
	return helper.Name(), []*os.File{passwordReader, usernameReader}, nil, func() {
		stopPassword()
		stopUsername()
		cleanupHelper()
	}, nil
}

func repeatingCredentialPipe(value string) (*os.File, func(), error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, nil, fmt.Errorf("create Git credential pipe: %w", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		line := []byte(value + "\n")
		for {
			if _, writeErr := writer.Write(line); writeErr != nil {
				return
			}
		}
	}()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			_ = reader.Close()
			_ = writer.Close()
			<-done
		})
	}
	return reader, stop, nil
}
