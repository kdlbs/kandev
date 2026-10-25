package configsync

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// filenameStem returns a file's base name without its extension. It plays no
// part in identity (AC-OFFICE-CONFIG-SYNC-003.1 excludes the path); it is
// only compared against a declared name to decide whether AC-003.2's
// mismatch warning fires.
func filenameStem(filePath string) string {
	base := path.Base(filePath)
	if i := strings.LastIndex(base, "."); i > 0 {
		return base[:i]
	}
	return base
}

// keyedPath is one fetched file's resolved (kind-scoped) key and full
// repository path — the input to collision resolution.
type keyedPath struct {
	Key  string
	Path string
}

// resolveKeyCollisions picks, for each key, the file whose full repository
// path sorts first byte-wise (AC-OFFICE-CONFIG-SYNC-003.3), independent of
// the order the provider listed files in. Every losing path is named in a
// single warning per key.
func resolveKeyCollisions(kind string, items []keyedPath) (winners map[string]string, warnings []string) {
	byKey := make(map[string][]string, len(items))
	for _, it := range items {
		byKey[it.Key] = append(byKey[it.Key], it.Path)
	}
	winners = make(map[string]string, len(byKey))
	keys := make([]string, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		paths := append([]string(nil), byKey[k]...)
		sort.Strings(paths)
		winners[k] = paths[0]
		if len(paths) > 1 {
			warnings = append(warnings, fmt.Sprintf(
				"%s %q: multiple files resolve to this key; %q wins, ignoring %s",
				kind, k, paths[0], strings.Join(paths[1:], ", ")))
		}
	}
	return winners, warnings
}

// stemMismatchWarning returns a non-empty warning when a declared name
// differs from its file's stem (AC-OFFICE-CONFIG-SYNC-003.2). The declared
// name always wins as the key; this only documents why.
func stemMismatchWarning(kind, filePath, declaredName string) string {
	stem := filenameStem(filePath)
	if stem == declaredName {
		return ""
	}
	return fmt.Sprintf(
		"%s %q: declared name %q differs from filename %q; using the declared name",
		kind, filePath, declaredName, stem)
}
