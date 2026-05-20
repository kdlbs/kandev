package routingerr

import (
	"os"
	"strings"
	"sync"
)

const injectEnvVar = "KANDEV_PROVIDER_FAILURES"

var (
	injectOnce sync.Once
	injectMap  map[string]Code
)

// LoadInjectionFromEnv parses KANDEV_PROVIDER_FAILURES into a provider→code
// map. Malformed entries are skipped silently so tests can use safe syntax.
func LoadInjectionFromEnv() map[string]Code {
	raw := os.Getenv(injectEnvVar)
	if raw == "" {
		return map[string]Code{}
	}
	out := map[string]Code{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		idx := strings.Index(part, ":")
		if idx <= 0 || idx == len(part)-1 {
			continue
		}
		providerID := strings.TrimSpace(part[:idx])
		code := strings.TrimSpace(part[idx+1:])
		if providerID == "" || code == "" {
			continue
		}
		out[providerID] = Code(code)
	}
	return out
}

func getInjection() map[string]Code {
	injectOnce.Do(func() {
		injectMap = LoadInjectionFromEnv()
	})
	return injectMap
}

// InjectedCode returns the test-injected error code for providerID,
// or ("", false) when no injection is configured for that provider.
// Callers (the scheduler dispatch loop) consult this pre-launch so
// KANDEV_PROVIDER_FAILURES drives deterministic E2E behaviour without
// requiring the agent binary to fail on its own.
func InjectedCode(providerID string) (Code, bool) {
	inj := getInjection()
	if inj == nil {
		return "", false
	}
	code, ok := inj[providerID]
	return code, ok
}
