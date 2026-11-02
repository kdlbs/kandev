package lifecycle

import (
	"os"
	"time"

	"github.com/kandev/kandev/internal/profiles"
)

func promptStallTiming() (interval, threshold time.Duration) {
	if profiles.DetectEnvironment() == profiles.EnvE2E {
		if duration, err := time.ParseDuration(os.Getenv("KANDEV_E2E_PROMPT_STALL_TIMEOUT")); err == nil &&
			duration >= time.Second && duration < 5*time.Minute {
			return duration / 5, duration
		}
	}
	return time.Minute, 5 * time.Minute
}
