package process

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type discardRename struct {
	source, destination string
	staged              bool
}

type discardRenameStatus struct {
	renamed map[string]discardRename
	paths   map[string]bool
	uses    map[string]int
}

func (g *GitOperator) prepareDiscardRenames(ctx context.Context, paths []string) ([]discardRename, map[string]string, error) {
	for _, path := range paths {
		if path == "" {
			return nil, nil, fmt.Errorf("empty filename specified to discard")
		}
	}
	output, err := g.runGitCommandWithEnvironment(ctx, discardSelectionEnvironment(),
		"status", "--porcelain", "-z", "--untracked-files=no")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read Discard rename evidence: %w", err)
	}
	status, err := parseDiscardRenameStatus(output)
	if err != nil {
		return nil, nil, err
	}
	identities, err := g.discardRenamePathIdentities(ctx, paths, status)
	if err != nil {
		return nil, nil, err
	}
	selected := make([]discardRename, 0)
	seen := make(map[string]bool)
	for _, rawPath := range paths {
		path := identities[rawPath]
		pair, renamed := status.renamed[path]
		if !renamed || seen[path] {
			continue
		}
		seen[path] = true
		if !pair.staged || status.uses[path] != 1 || status.uses[pair.source] != 1 || status.paths[pair.source] {
			return nil, nil, fmt.Errorf("unsupported or ambiguous staged rename for %q", path)
		}
		if err := g.checkDiscardRenameTree(ctx, pair); err != nil {
			return nil, nil, err
		}
		selected = append(selected, pair)
	}
	if err := g.checkDiscardRenameSources(selected); err != nil {
		return nil, nil, err
	}
	return selected, identities, nil
}

func (g *GitOperator) discardRenamePathIdentities(ctx context.Context, paths []string, status discardRenameStatus) (map[string]string, error) {
	identities := make(map[string]string, len(paths))
	for _, rawPath := range paths {
		identity := filepath.ToSlash(filepath.Clean(rawPath))
		if identity == rawPath || !status.hasRenameEndpoint(identity) {
			identities[rawPath] = rawPath
			continue
		}
		// Cleaning only identifies a candidate; Git must accept the original argument.
		output, err := g.runGitCommandWithEnvironment(ctx, discardSelectionEnvironment(),
			"status", "--porcelain", "-z", "--untracked-files=no", "--", literalGitPathspec(rawPath))
		if err != nil {
			return nil, fmt.Errorf("failed to read Discard filename %q: %w", rawPath, err)
		}
		evidence, err := parseDiscardRenameStatus(output)
		if err != nil {
			return nil, err
		}
		if len(evidence.paths) != 1 || !evidence.paths[identity] {
			return nil, fmt.Errorf("unsupported Discard rename filename %q", rawPath)
		}
		identities[rawPath] = identity
	}
	return identities, nil
}

func (status discardRenameStatus) hasRenameEndpoint(path string) bool {
	for destination, pair := range status.renamed {
		if path == destination || path == pair.source {
			return true
		}
	}
	return false
}

func parseDiscardRenameStatus(output string) (discardRenameStatus, error) {
	status := discardRenameStatus{renamed: make(map[string]discardRename), paths: make(map[string]bool), uses: make(map[string]int)}
	for output != "" {
		record, remaining, terminated := strings.Cut(output, "\x00")
		if !terminated || len(record) < 4 || record[2] != ' ' || !validDiscardStatusColumns(record[:2]) {
			return status, fmt.Errorf("invalid Discard status framing")
		}
		path := record[3:]
		if status.paths[path] {
			return status, fmt.Errorf("duplicate Discard status path")
		}
		status.paths[path] = true
		output = remaining
		if !strings.ContainsAny(record[:2], "RC") {
			continue
		}
		source, remaining, terminated := strings.Cut(output, "\x00")
		if !terminated || source == "" || source == path {
			return status, fmt.Errorf("invalid Discard rename framing")
		}
		output = remaining
		status.uses[path]++
		status.uses[source]++
		if strings.ContainsRune(record[:2], 'R') {
			status.renamed[path] = discardRename{source: source, destination: path,
				staged: record[0] == 'R' && strings.ContainsRune(" MDT", rune(record[1]))}
		}
	}
	return status, nil
}

func validDiscardStatusColumns(columns string) bool {
	return strings.ContainsRune(" MADRCTU?!", rune(columns[0])) && strings.ContainsRune(" MADRCTU?!", rune(columns[1]))
}

func discardSelectionEnvironment() map[string]string {
	return map[string]string{gitLiteralPathspecEnv: "0", gitICasePathspecEnv: "0"}
}

func (g *GitOperator) checkDiscardRenameTree(ctx context.Context, pair discardRename) error {
	output, err := g.runGitCommandWithEnvironment(ctx, discardSelectionEnvironment(),
		"ls-tree", "-z", "HEAD", "--", literalGitPathspec(pair.source), literalGitPathspec(pair.destination))
	if err != nil {
		return fmt.Errorf("failed to read Discard committed paths: %w", err)
	}
	sourceFound := false
	for output != "" {
		record, remaining, terminated := strings.Cut(output, "\x00")
		metadata, path, separated := strings.Cut(record, "\t")
		fields := strings.Fields(metadata)
		if !terminated || !separated || len(fields) != 3 {
			return fmt.Errorf("invalid Discard committed-path framing")
		}
		if path != pair.source || fields[1] != "blob" || sourceFound {
			return fmt.Errorf("unsupported committed paths for staged rename %q", pair.destination)
		}
		sourceFound = true
		output = remaining
	}
	if !sourceFound {
		return fmt.Errorf("staged rename source %q is not a committed file", pair.source)
	}
	return nil
}

func (g *GitOperator) checkDiscardRenameSources(pairs []discardRename) error {
	for _, pair := range pairs {
		_, err := os.Lstat(filepath.Join(g.workDir, pair.source))
		if err == nil {
			return fmt.Errorf("cannot discard staged rename: source %q is occupied", pair.source)
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("cannot inspect staged rename source %q: %w", pair.source, err)
		}
	}
	return nil
}

func discardRenameRestorePaths(pairs []discardRename, identities map[string]string) ([]string, map[string]bool) {
	paths := make([]string, 0, len(pairs)*2)
	selected := make(map[string]bool)
	for _, pair := range pairs {
		paths = append(paths, pair.source, pair.destination)
		selected[pair.source], selected[pair.destination] = true, true
	}
	for rawPath, identity := range identities {
		if selected[identity] {
			selected[rawPath] = true
		}
	}
	return paths, selected
}

func (g *GitOperator) discardTrackedFilesWithRenames(ctx context.Context, paths []string, pairs []discardRename) ([]string, []string) {
	if err := g.checkDiscardRenameSources(pairs); err != nil {
		return nil, []string{err.Error()}
	}
	return g.discardTrackedFiles(ctx, paths)
}

func (g *GitOperator) discardFileCategories(ctx context.Context, paths []string, selected map[string]bool) ([]string, []string) {
	untrackedFiles := []string{}
	trackedFiles := []string{}
	// Get status for each file to determine how to discard it
	for _, path := range paths {
		if selected[path] {
			continue
		}
		selected[path] = true
		statusArgs := []string{"status", "--porcelain", "--", literalGitPathspec(path)}
		statusOutput, err := g.runGitCommandWithEnvironment(ctx,
			map[string]string{gitLiteralPathspecEnv: "0", gitICasePathspecEnv: "0"}, statusArgs...)
		if err != nil {
			// If we can't get status, assume it's tracked and try to restore it
			trackedFiles = append(trackedFiles, path)
			continue
		}

		statusLine := strings.TrimSpace(statusOutput)
		if len(statusLine) >= 2 {
			indexStatus := statusLine[0]
			workTreeStatus := statusLine[1]

			// Untracked files (??), or added files (A ) that don't exist in HEAD
			if (indexStatus == '?' && workTreeStatus == '?') || indexStatus == 'A' {
				untrackedFiles = append(untrackedFiles, path)
			} else {
				trackedFiles = append(trackedFiles, path)
			}
		} else if statusLine == "" {
			// Empty status means file is not modified - nothing to discard
			continue
		}
	}

	return untrackedFiles, trackedFiles
}
