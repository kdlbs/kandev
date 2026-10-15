package agenthistory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// claudeMaxProjectNameLength is the longest encoded project name Claude Code
// stores verbatim; longer names are truncated and hashed by Claude.
const claudeMaxProjectNameLength = 200

const claudeMemoryDir = "memory"

// encodeClaudeProjectName mirrors Claude Code's project directory naming:
// every character outside [A-Za-z0-9] becomes '-'.
func encodeClaudeProjectName(workDir string) string {
	var b strings.Builder
	b.Grow(len(workDir))
	for _, r := range workDir {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('-')
	}
	return b.String()
}

// removeClaude removes <root>/projects/<encoded cwd>/<token>.jsonl, its
// <token>/ sidecar directory, and the project directory when nothing else
// remains in it.
func removeClaude(ctx context.Context, id Identity) error {
	root := configRoot(id, "CLAUDE_CONFIG_DIR", ".claude")
	if root == "" {
		return nil
	}
	name := encodeClaudeProjectName(filepath.Clean(id.WorkDir))
	if len(name) > claudeMaxProjectNameLength {
		return nil
	}
	projects, project, err := openClaudeProject(root, name)
	if err != nil || project == nil {
		return err
	}
	defer func() { _ = projects.Close() }()
	defer func() { _ = project.Close() }()
	if err := context.Cause(ctx); err != nil {
		return err
	}
	var errs []error
	removed, err := removeRegularFile(project, id.ResumeToken+".jsonl")
	if err != nil {
		errs = append(errs, err)
	}
	sidecar, err := openRealSubdirectory(project, id.ResumeToken)
	if err != nil {
		errs = append(errs, err)
	} else if sidecar != nil {
		sidecarRemoved, removeErr := removePinnedDirectory(ctx, project, id.ResumeToken, sidecar)
		_ = sidecar.Close()
		removed = removed || sidecarRemoved
		errs = append(errs, removeErr)
	}
	if removed && errors.Join(errs...) == nil {
		errs = append(errs, removeEmptyClaudeProject(projects, name, project))
	}
	return errors.Join(errs...)
}

func openClaudeProject(configDir, name string) (*os.Root, *os.Root, error) {
	history, err := openHistoryRoot(configDir)
	if err != nil || history == nil {
		return nil, nil, err
	}
	defer func() { _ = history.Close() }()
	projects, err := openRealSubdirectory(history, "projects")
	if err != nil || projects == nil {
		return nil, nil, err
	}
	project, err := openRealSubdirectory(projects, name)
	if err != nil || project == nil {
		_ = projects.Close()
		return nil, nil, err
	}
	return projects, project, nil
}

// removeEmptyClaudeProject removes a project directory that holds nothing, or
// only an empty memory directory.
func removeEmptyClaudeProject(projects *os.Root, name string, project *os.Root) error {
	entries, err := historyEntries(project)
	if err != nil {
		return err
	}
	if len(entries) > 1 || (len(entries) == 1 && entries[0].Name() != claudeMemoryDir) {
		return nil
	}
	if len(entries) == 1 {
		memory, err := openRealSubdirectory(project, claudeMemoryDir)
		if err != nil || memory == nil {
			return err
		}
		defer func() { _ = memory.Close() }()
		info, err := memory.Stat(".")
		if err != nil {
			return err
		}
		contents, err := historyEntries(memory)
		if err != nil || len(contents) > 0 {
			return err
		}
		if _, err := removeObservedEntry(project, claudeMemoryDir, info); err != nil {
			return err
		}
	}
	info, err := project.Stat(".")
	if err != nil {
		return err
	}
	_, err = removeObservedEntry(projects, name, info)
	return err
}
