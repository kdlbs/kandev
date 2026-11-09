// Package profileenv validates the shape of agent and executor profile
// environment declarations shared by settings writes and runtime admission.
package profileenv

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

const (
	// MaxEntries caps one profile's environment declaration list.
	MaxEntries = 100
	// MaxKeyLen caps an environment key's length.
	MaxKeyLen = 256
	// MaxValueLen caps a literal environment value's length.
	MaxValueLen = 8 * 1024
	// ReservedKey is the task-description placeholder owned by the runtime.
	ReservedKey = "TASK_DESCRIPTION"
	// ReservedPrefix identifies runtime-owned environment names.
	ReservedPrefix = "KANDEV_"
)

var posixEnvIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Validate checks key shape, duplicates, reserved names, and value/secret
// exclusivity. It never resolves or reveals a secret.
func Validate(entries []models.ProfileEnvVar) error {
	if len(entries) > MaxEntries {
		return fmt.Errorf("at most %d entries allowed", MaxEntries)
	}
	seen := make(map[string]int, len(entries))
	for i, entry := range entries {
		key := strings.TrimSpace(entry.Key)
		if key != entry.Key {
			return fmt.Errorf("env_vars[%d].key must be a POSIX environment identifier", i)
		}
		if err := validateKey(key, i, seen); err != nil {
			return err
		}
		seen[key] = i
		if err := validateValue(entry, i); err != nil {
			return err
		}
	}
	return nil
}

func validateKey(key string, i int, seen map[string]int) error {
	if key == "" {
		return fmt.Errorf("env_vars[%d].key is required", i)
	}
	if len(key) > MaxKeyLen {
		return fmt.Errorf("env_vars[%d].key exceeds %d characters", i, MaxKeyLen)
	}
	if !posixEnvIdentifier.MatchString(key) {
		return fmt.Errorf("env_vars[%d].key must be a POSIX environment identifier", i)
	}
	if strings.HasPrefix(key, ReservedPrefix) || key == ReservedKey {
		return fmt.Errorf("env_vars[%d].key %q is reserved", i, key)
	}
	if first, exists := seen[key]; exists {
		return fmt.Errorf("env_vars[%d].key duplicates env_vars[%d].key", i, first)
	}
	return nil
}

func validateValue(entry models.ProfileEnvVar, i int) error {
	if entry.SecretID != "" && entry.Value != "" {
		return fmt.Errorf("env_vars[%d]: set value or secret_id, not both", i)
	}
	if entry.SecretID == "" && entry.Value == "" {
		return fmt.Errorf("env_vars[%d]: must set either value or secret_id", i)
	}
	if entry.Value != "" {
		if len(entry.Value) > MaxValueLen {
			return fmt.Errorf("env_vars[%d].value exceeds %d characters", i, MaxValueLen)
		}
		if strings.Contains(entry.Value, "\x00") {
			return fmt.Errorf("env_vars[%d].value must not contain null bytes", i)
		}
	}
	return nil
}
