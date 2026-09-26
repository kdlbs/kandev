// Package review turns a task's working diff into anchored, advisory code-review
// findings. See docs/specs/agents/requirements/native-code-review.md.
//
// The package owns run orchestration only: persistence and event publication
// live in internal/task/service (ReviewService), and the inference call is made
// through the utility-agent substrate.
package review

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/kandev/kandev/internal/utility/hash"
)

// fileKeySep matches the NUL separator the frontend's `reviewFileKey` uses to
// compose <repository>\x00<path>. It cannot occur in a real path or repository
// name, so the composite key is always uniquely splittable.
const fileKeySep = "\x00"

// stagedDiffSeparator is the marker agentctl uses when it concatenates unstaged
// and staged diffs for one file. The frontend's `normalizeDiffContent` keeps the
// staged half; we mirror that so both sides hash identical text.
const stagedDiffSeparator = "--- Staged changes ---"

// ChangedFile is one file in the task's current change set, already normalized
// and hashed the same way the Review panel does.
type ChangedFile struct {
	RepositoryID   string
	RepositoryName string
	Path           string
	Status         string
	Diff           string
	DiffHash       string
	Additions      int
	Deletions      int
	// Source is "uncommitted" or "committed", matching the Review panel's
	// per-file source label.
	Source string
}

// Key returns the composite <repository>\x00<path> identity, matching
// `reviewFileKey` in apps/web/components/review/types.ts.
func (f ChangedFile) Key() string {
	// A finding from a legacy single-repository task has neither repository
	// identity field and keeps the historical bare-path key. An explicit empty
	// repository name with an ID is the real workspace-root scope in a
	// multi-repository task and must remain distinct from that legacy key.
	if f.RepositoryName == "" && f.RepositoryID == "" {
		return f.Path
	}
	return f.RepositoryName + fileKeySep + f.Path
}

// ChangeSource reads a session's changed-file payloads.
//
// It deliberately speaks in the wire's untyped per-file maps rather than
// agentctl's result structs: `internal/agent/runtime/agentctl` is a runtime-tier
// package that must not be imported outside `internal/agent/runtime/` (see
// apps/backend/AGENTS.md), so the agentctl calls live in an adapter and this
// package stays testable with a plain fake.
type ChangeSource interface {
	// UncommittedFiles returns the working-tree per-file payloads.
	UncommittedFiles(ctx context.Context, sessionID string) (map[string]any, error)
	// CommittedFiles returns the cumulative committed-on-branch per-file
	// payloads. A nil map with a nil error means "nothing committed yet".
	CommittedFiles(ctx context.Context, sessionID string) (map[string]any, error)
}

// rawFileEntry is the per-file payload shape both git endpoints embed in their
// untyped `Files` maps. Decoded via a JSON round-trip so this package does not
// depend on agentctl's internal process types.
type rawFileEntry struct {
	Path           string `json:"path"`
	Diff           string `json:"diff"`
	Status         string `json:"status"`
	Additions      int    `json:"additions"`
	Deletions      int    `json:"deletions"`
	RepositoryName string `json:"repository_name"`
	RepositoryID   string `json:"repository_id"`
}

// CollectChanges builds the task's review file set for one session.
//
// Priority and dedup mirror `buildAllFiles` in
// apps/web/components/review/review-dialog.tsx: uncommitted files win over the
// polled cumulative diff, because the working tree is always the fresher
// content for a file that appears in both. When repositoryID is non-empty the
// result is scoped to that repository. Files with no textual diff are dropped —
// there is nothing for a reviewer to read.
func CollectChanges(ctx context.Context, src ChangeSource, sessionID, repositoryID string) ([]ChangedFile, error) {
	if src == nil {
		return nil, fmt.Errorf("%w: no change source configured", ErrWorkspaceUnavailable)
	}
	if sessionID == "" {
		return nil, fmt.Errorf("%w: session id is required", ErrWorkspaceUnavailable)
	}

	byKey := make(map[string]ChangedFile)

	uncommitted, err := src.UncommittedFiles(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("%w: read working tree: %v", ErrWorkspaceUnavailable, err)
	}
	addRawFiles(byKey, uncommitted, sourceUncommitted)

	// A missing or failed cumulative diff is not fatal: uncommitted work alone
	// is still reviewable, and a repo-less or freshly-branched task legitimately
	// has no committed range.
	if committed, cumErr := src.CommittedFiles(ctx, sessionID); cumErr == nil {
		addRawFiles(byKey, committed, sourceCommitted)
	}

	files := make([]ChangedFile, 0, len(byKey))
	for _, f := range byKey {
		if repositoryID != "" && f.RepositoryID != "" && f.RepositoryID != repositoryID {
			continue
		}
		files = append(files, f)
	}
	sortChangedFiles(files)
	return files, nil
}

const (
	sourceUncommitted = "uncommitted"
	sourceCommitted   = "committed"
)

// addRawFiles decodes an untyped per-file map and inserts each entry, keeping
// the first writer for a key so the caller controls precedence by call order.
func addRawFiles(byKey map[string]ChangedFile, raw map[string]any, source string) {
	for mapKey, value := range raw {
		entry, ok := decodeRawFileEntry(value)
		if !ok {
			continue
		}
		path := entry.Path
		if path == "" {
			// Single-repo payloads carry the bare path only on the map key.
			_, path = splitFileKey(mapKey)
		}
		if path == "" {
			continue
		}
		diff := NormalizeDiff(entry.Diff)
		if diff == "" {
			continue
		}
		file := ChangedFile{
			RepositoryID:   entry.RepositoryID,
			RepositoryName: entry.RepositoryName,
			Path:           path,
			Status:         entry.Status,
			Diff:           diff,
			DiffHash:       hash.DJB2(diff),
			Additions:      entry.Additions,
			Deletions:      entry.Deletions,
			Source:         source,
		}
		key := file.Key()
		// A repo-unaware entry already recorded under the bare path is the same
		// file; do not add a second row for it once a repo name shows up.
		if _, exists := byKey[key]; exists {
			continue
		}
		if key != path {
			if _, exists := byKey[path]; exists {
				continue
			}
		}
		byKey[key] = file
	}
}

func decodeRawFileEntry(value any) (rawFileEntry, bool) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return rawFileEntry{}, false
	}
	var entry rawFileEntry
	if err := json.Unmarshal(encoded, &entry); err != nil {
		return rawFileEntry{}, false
	}
	return entry, true
}

// NormalizeDiff mirrors `normalizeDiffContent` in
// apps/web/components/review/types.ts. Both halves must normalize identically
// before hashing, or every finding would look stale the moment it is stored.
func NormalizeDiff(diff string) string {
	trimmed := strings.TrimSpace(diff)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, stagedDiffSeparator) {
		parts := strings.Split(trimmed, stagedDiffSeparator)
		staged := parts[0]
		if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" {
			staged = parts[1]
		}
		trimmed = strings.TrimSpace(staged)
	}
	return trimmed
}

func splitFileKey(key string) (repositoryName, path string) {
	if idx := strings.Index(key, fileKeySep); idx >= 0 {
		return key[:idx], key[idx+len(fileKeySep):]
	}
	return "", key
}

// sortChangedFiles orders by repository then path, matching the Review panel so
// prompt batches and the resulting findings read in the same order the user
// sees.
func sortChangedFiles(files []ChangedFile) {
	sort.Slice(files, func(i, j int) bool {
		if files[i].RepositoryName != files[j].RepositoryName {
			return files[i].RepositoryName < files[j].RepositoryName
		}
		return files[i].Path < files[j].Path
	})
}

// RepositoryCount returns the number of distinct repositories in the set. A
// single-repository task reports 1 even though its files carry no repository
// name.
func RepositoryCount(files []ChangedFile) int {
	if len(files) == 0 {
		return 0
	}
	names := make(map[string]struct{}, len(files))
	for _, f := range files {
		names[f.RepositoryName] = struct{}{}
	}
	return len(names)
}

// FileByKey indexes a change set by composite key so the runner can attach the
// authoritative diff hash and anchor text to a parsed finding.
func FileByKey(files []ChangedFile) map[string]ChangedFile {
	index := make(map[string]ChangedFile, len(files))
	for _, f := range files {
		index[f.Key()] = f
	}
	return index
}
