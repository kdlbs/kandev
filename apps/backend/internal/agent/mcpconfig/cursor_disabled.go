package mcpconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	cursorMCPDisabledFilename = "mcp-disabled.json"
	cursorMCPDisabledMaxBytes = 1024 * 1024
)

// ReadCursorMCPDisabledServers returns the exact disabled MCP identifiers from
// the selected source repository and task workspace.
func ReadCursorMCPDisabledServers(cursorHome, sourceRepository, taskWorkspace string) (map[string]struct{}, error) {
	disabled := make(map[string]struct{})
	if sourceRepository == "" && taskWorkspace == "" {
		return disabled, nil
	}
	projectsPath, exists, err := cursorMCPProjectsDirectory(cursorHome)
	if err != nil {
		return nil, errors.New("cursor MCP disabled preferences unavailable")
	}
	if !exists {
		return disabled, nil
	}
	seenSlugs := make(map[string]struct{}, 2)
	for _, root := range []string{sourceRepository, taskWorkspace} {
		entries, include, err := readSelectedCursorMCPDisabledStore(projectsPath, root, seenSlugs)
		if err != nil {
			return nil, errors.New("cursor MCP disabled preferences unavailable")
		}
		if include {
			for name := range entries {
				disabled[name] = struct{}{}
			}
		}
	}
	return disabled, nil
}

func cursorMCPProjectsDirectory(cursorHome string) (string, bool, error) {
	cursorInfo, err := os.Lstat(cursorHome)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil || cursorInfo.Mode()&os.ModeSymlink != 0 || !cursorInfo.IsDir() {
		return "", false, errors.New("unsafe Cursor home")
	}
	projectsPath := filepath.Join(cursorHome, "projects")
	projectsInfo, err := os.Lstat(projectsPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil || projectsInfo.Mode()&os.ModeSymlink != 0 || !projectsInfo.IsDir() {
		return "", false, errors.New("unsafe Cursor projects directory")
	}
	return projectsPath, true, nil
}

func readSelectedCursorMCPDisabledStore(projectsPath, root string, seenSlugs map[string]struct{}) (map[string]struct{}, bool, error) {
	if root == "" {
		return nil, false, nil
	}
	canonicalRoot, err := canonicalPathIncludingMissingSuffix(root)
	if err != nil {
		return nil, false, err
	}
	slug := DeriveCursorProjectSlug(canonicalRoot)
	if slug == "" {
		return nil, false, errors.New("invalid Cursor project slug")
	}
	if _, exists := seenSlugs[slug]; exists {
		return nil, false, nil
	}
	seenSlugs[slug] = struct{}{}
	projectPath := filepath.Join(projectsPath, slug)
	projectInfo, err := os.Lstat(projectPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil || projectInfo.Mode()&os.ModeSymlink != 0 || !projectInfo.IsDir() {
		return nil, false, errors.New("unsafe Cursor project directory")
	}
	entries, err := readCursorMCPDisabledStore(filepath.Join(projectPath, cursorMCPDisabledFilename))
	if err != nil {
		return nil, false, err
	}
	return entries, true, nil
}

func readCursorMCPDisabledStore(path string) (map[string]struct{}, error) {
	file, err := openCursorMCPDisabledStore(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, cursorMCPDisabledMaxBytes+1))
	if err != nil || len(data) > cursorMCPDisabledMaxBytes {
		return nil, errors.New("invalid Cursor MCP disabled file size")
	}
	return parseCursorMCPDisabledNames(data)
}

func openCursorMCPDisabledStore(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, os.ErrNotExist
	}
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("unsafe Cursor MCP disabled file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("unreadable Cursor MCP disabled file")
	}
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		_ = file.Close()
		return nil, errors.New("changed Cursor MCP disabled file")
	}
	return file, nil
}

func parseCursorMCPDisabledNames(data []byte) (map[string]struct{}, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return nil, errors.New("invalid Cursor MCP disabled list")
	}
	var names []string
	if err := json.Unmarshal(trimmed, &names); err != nil || names == nil {
		return nil, errors.New("invalid Cursor MCP disabled list")
	}
	entries := make(map[string]struct{}, len(names))
	for _, name := range names {
		if name == "" || strings.TrimSpace(name) != name {
			return nil, errors.New("invalid Cursor MCP disabled identifier")
		}
		entries[name] = struct{}{}
	}
	return entries, nil
}
