package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// agyBinary is the Antigravity CLI executable the shim drives. It is resolved
// from PATH at exec time.
const agyBinary = "agy"

// trustValue is the value `agy` writes in trustedFolders.json to mark a
// directory trusted, skipping the interactive first-run trust prompt. The shim
// pre-seeds it because `agy -p` (non-interactive) cannot answer the prompt.
const trustValue = "TRUST_FOLDER"

// buildPromptArgs assembles the `agy` argv for a single non-interactive prompt.
// continueSession appends --continue so the CLI threads the turn onto the prior
// conversation in the same workspace; model selects the session model when set.
func buildPromptArgs(prompt, model string, continueSession bool) []string {
	args := []string{"--print", prompt}
	if model != "" {
		args = append(args, "--model", model)
	}
	if continueSession {
		args = append(args, "--continue")
	}
	return args
}

// parseModels turns the line-per-model output of `agy models` into id/name
// pairs. The CLI prints no separate identifier, so the printed name is also the
// value passed to --model. Blank lines are skipped.
func parseModels(out string) []modelEntry {
	var models []modelEntry
	for _, line := range strings.Split(out, "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		models = append(models, modelEntry{ID: name, Name: name})
	}
	return models
}

// modelEntry is a single advertised model.
type modelEntry struct {
	ID   string
	Name string
}

// trustedFoldersPath returns the absolute path to `agy`'s trusted-folders
// registry, or ok=false when the home directory cannot be resolved.
func trustedFoldersPath() (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", false
	}
	return filepath.Join(home, ".gemini", "trustedFolders.json"), true
}

// seedTrustedFolder adds workspace→TRUST_FOLDER to the flat JSON map at path,
// creating the file if missing and preserving existing entries. It is a no-op
// when the folder is already present, and never writes through a symlink. Any
// error is returned so the caller can log and continue (trust seeding is a
// convenience, not a hard requirement).
func seedTrustedFolder(path, workspace string) error {
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	folders := map[string]string{}
	switch data, err := os.ReadFile(path); {
	case err == nil:
		if uerr := json.Unmarshal(data, &folders); uerr != nil {
			return uerr
		}
	case !os.IsNotExist(err):
		return err
	}
	clean := filepath.Clean(workspace)
	if _, present := folders[clean]; present {
		return nil
	}
	folders[clean] = trustValue
	content, err := json.MarshalIndent(folders, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o600)
}

// trustWorkspace seeds trust for an absolute workspace path, ignoring failures
// (the prompt may still appear, but launch is never blocked).
func trustWorkspace(workspace string) {
	if workspace == "" {
		return
	}
	path, ok := trustedFoldersPath()
	if !ok {
		return
	}
	_ = seedTrustedFolder(path, workspace)
}
