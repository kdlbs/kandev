// Package gitconfigenv composes Git's indexed environment configuration.
package gitconfigenv

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	countKey    = "GIT_CONFIG_COUNT"
	keyPrefix   = "GIT_CONFIG_KEY_"
	valuePrefix = "GIT_CONFIG_VALUE_"
	maxEntries  = 256
)

// Entry is one Git configuration key/value pair supplied through the
// GIT_CONFIG_COUNT environment protocol.
type Entry struct {
	Key   string
	Value string
}

// Merge combines ordinary environment variables with Git's indexed configuration block.
// Overlay values take precedence for ordinary variables; indexed Git entries retain source order.
func Merge(base, overlay map[string]string) (map[string]string, error) {
	baseEntries, err := entriesFrom(base)
	if err != nil {
		return nil, fmt.Errorf("base Git config: %w", err)
	}
	overlayEntries, err := entriesFrom(overlay)
	if err != nil {
		return nil, fmt.Errorf("overlay Git config: %w", err)
	}
	result := make(map[string]string)
	for key, value := range base {
		if !isIndexedKey(key) {
			result[key] = value
		}
	}
	for key, value := range overlay {
		if !isIndexedKey(key) {
			result[key] = value
		}
	}
	entries := removeBoundaryOverlap(baseEntries, overlayEntries)
	writeEntries(result, entries)
	return result, nil
}

// CopyIndexed replaces target's indexed Git configuration with source's validated block.
func CopyIndexed(target, source map[string]string) {
	for key := range target {
		if isIndexedKey(key) {
			delete(target, key)
		}
	}
	for key, value := range source {
		if isIndexedKey(key) {
			target[key] = value
		}
	}
}

// EnvironmentEntries serializes a validated indexed Git configuration block.
func EnvironmentEntries(env map[string]string) ([]string, error) {
	entries, err := entriesFrom(env)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}
	result := make([]string, 0, len(entries)*2+1)
	result = append(result, fmt.Sprintf("%s=%d", countKey, len(entries)))
	for index, entry := range entries {
		result = append(result,
			fmt.Sprintf("%s%d=%s", keyPrefix, index, entry.Key),
			fmt.Sprintf("%s%d=%s", valuePrefix, index, entry.Value),
		)
	}
	return result, nil
}

// Filter returns a copy of env with indexed Git configuration entries that do
// not satisfy keep removed. The complete ordered block is provided so callers
// can remove a related pair without disturbing unrelated host configuration.
// Ordinary environment variables are preserved.
func Filter(env map[string]string, keep func(index int, entries []Entry) bool) (map[string]string, error) {
	entries, err := entriesFrom(env)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(env))
	for key, value := range env {
		if !isIndexedKey(key) {
			result[key] = value
		}
	}
	filtered := make([]Entry, 0, len(entries))
	for index, entry := range entries {
		if keep(index, entries) {
			filtered = append(filtered, entry)
		}
	}
	writeEntries(result, filtered)
	return result, nil
}

func isIndexedKey(key string) bool {
	return key == countKey || strings.HasPrefix(key, keyPrefix) || strings.HasPrefix(key, valuePrefix)
}

// IsIndexedKey reports whether key belongs to Git's indexed environment
// configuration block.
func IsIndexedKey(key string) bool {
	return isIndexedKey(key)
}

func entriesFrom(env map[string]string) ([]Entry, error) {
	if len(env) == 0 {
		return nil, nil
	}
	countValue, hasCount := env[countKey]
	if !hasCount {
		if hasIndexedEntry(env) {
			return nil, fmt.Errorf("%s is required when indexed entries are set", countKey)
		}
		return nil, nil
	}
	count, err := strconv.Atoi(countValue)
	if err != nil || count < 0 || count > maxEntries {
		return nil, fmt.Errorf("%s must be between 0 and %d", countKey, maxEntries)
	}
	entries := make([]Entry, 0, count)
	for index := 0; index < count; index++ {
		key, keyOK := env[fmt.Sprintf("%s%d", keyPrefix, index)]
		value, valueOK := env[fmt.Sprintf("%s%d", valuePrefix, index)]
		if !keyOK || !valueOK || key == "" {
			return nil, fmt.Errorf("entry %d must include a key and value", index)
		}
		entries = append(entries, Entry{Key: key, Value: value})
	}
	if hasOutOfRangeIndexedEntry(env, count) {
		return nil, fmt.Errorf("indexed entry exceeds %s", countKey)
	}
	return entries, nil
}

func hasIndexedEntry(env map[string]string) bool {
	for key := range env {
		if strings.HasPrefix(key, keyPrefix) || strings.HasPrefix(key, valuePrefix) {
			return true
		}
	}
	return false
}

func hasOutOfRangeIndexedEntry(env map[string]string, count int) bool {
	for key := range env {
		index, ok := indexedEntryIndex(key)
		if ok && index >= count {
			return true
		}
	}
	return false
}

func indexedEntryIndex(key string) (int, bool) {
	prefix := keyPrefix
	if strings.HasPrefix(key, valuePrefix) {
		prefix = valuePrefix
	} else if !strings.HasPrefix(key, keyPrefix) {
		return 0, false
	}
	index, err := strconv.Atoi(strings.TrimPrefix(key, prefix))
	return index, err == nil && index >= 0
}

func removeBoundaryOverlap(base, overlay []Entry) []Entry {
	maxOverlap := min(len(base), len(overlay))
	for overlap := maxOverlap; overlap > 0; overlap-- {
		if entriesEqual(base[len(base)-overlap:], overlay[:overlap]) {
			return append(append([]Entry{}, base...), overlay[overlap:]...)
		}
	}
	return append(append([]Entry{}, base...), overlay...)
}

func entriesEqual(left, right []Entry) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func writeEntries(env map[string]string, entries []Entry) {
	if len(entries) == 0 {
		return
	}
	env[countKey] = strconv.Itoa(len(entries))
	for index, entry := range entries {
		env[fmt.Sprintf("%s%d", keyPrefix, index)] = entry.Key
		env[fmt.Sprintf("%s%d", valuePrefix, index)] = entry.Value
	}
}
