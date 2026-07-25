package launcher

import (
	"fmt"
	"time"
)

const (
	launchdBootoutPollInterval = 100 * time.Millisecond
	launchdBootoutPollAttempts = 50
	launchdBootstrapAttempts   = 5
	launchdBootstrapRetryBase  = 300 * time.Millisecond
)

var (
	launchctlCommand = func(args ...string) error {
		return runCommand("launchctl", args...)
	}
	launchctlSleep = time.Sleep
)

func reloadLaunchdService(target, domain, plistPath string) error {
	if err := bootoutLaunchdAndWait(target); err != nil {
		return err
	}
	var lastErr error
	for attempt := 1; attempt <= launchdBootstrapAttempts; attempt++ {
		lastErr = launchctlCommand("bootstrap", domain, plistPath)
		if lastErr == nil {
			return nil
		}
		if attempt < launchdBootstrapAttempts {
			launchctlSleep(launchdBootstrapRetryBase * time.Duration(attempt))
		}
	}
	return fmt.Errorf("launchctl bootstrap %s %s: %w", domain, plistPath, lastErr)
}

func bootoutLaunchdAndWait(target string) error {
	_ = launchctlCommand("bootout", target)
	for attempt := 0; attempt < launchdBootoutPollAttempts; attempt++ {
		if err := launchctlCommand("print", target); err != nil {
			return nil
		}
		launchctlSleep(launchdBootoutPollInterval)
	}
	return fmt.Errorf("timed out waiting for launchctl bootout %s", target)
}
