package agenthistory

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// codexMetaLineLimit bounds how much of a rollout's first line is read.
const codexMetaLineLimit = 1 << 20

type codexSessionMeta struct {
	Type    string `json:"type"`
	Payload struct {
		ID  string `json:"id"`
		Cwd string `json:"cwd"`
	} `json:"payload"`
}

// removeCodex removes <root>/sessions/YYYY/MM/DD/rollout-*-<token>.jsonl files
// whose session_meta records the same thread ID and working directory. Codex's
// shared thread indexes are left to Codex, which skips threads without a rollout.
func removeCodex(ctx context.Context, id Identity) error {
	root := configRoot(id, "CODEX_HOME", ".codex")
	if root == "" {
		return nil
	}
	history, err := openHistoryRoot(root)
	if err != nil || history == nil {
		return err
	}
	defer func() { _ = history.Close() }()
	sessions, err := openRealSubdirectory(history, "sessions")
	if err != nil || sessions == nil {
		return err
	}
	defer func() { _ = sessions.Close() }()
	return removeCodexPartitions(ctx, sessions, 3, id)
}

func removeCodexPartitions(ctx context.Context, root *os.Root, depth int, id Identity) error {
	if depth == 0 {
		return removeCodexDay(ctx, root, id)
	}
	entries, err := historyEntries(root)
	if err != nil {
		return err
	}
	var errs []error
	for _, entry := range entries {
		if err := context.Cause(ctx); err != nil {
			return errors.Join(append(errs, err)...)
		}
		if !entry.IsDir() {
			continue
		}
		child, err := openRealSubdirectory(root, entry.Name())
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if child != nil {
			errs = append(errs, removeCodexPartitions(ctx, child, depth-1, id))
			_ = child.Close()
		}
	}
	return errors.Join(errs...)
}

func removeCodexDay(ctx context.Context, day *os.Root, id Identity) error {
	entries, err := historyEntries(day)
	if err != nil {
		return err
	}
	var errs []error
	for _, entry := range entries {
		if err := context.Cause(ctx); err != nil {
			return errors.Join(append(errs, err)...)
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "rollout-") || !strings.HasSuffix(name, "-"+id.ResumeToken+".jsonl") {
			continue
		}
		f, info, err := openRegularFile(day, name)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if f == nil {
			continue
		}
		owned, err := codexRolloutBelongsTo(f, id.ResumeToken, filepath.Clean(id.WorkDir))
		_ = f.Close()
		if err == nil && owned {
			_, err = removeObservedEntry(day, name, info)
		}
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func codexRolloutBelongsTo(f *os.File, thread, workDir string) (bool, error) {
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), codexMetaLineLimit)
	if !scanner.Scan() {
		return false, scanner.Err()
	}
	var meta codexSessionMeta
	if json.Unmarshal(scanner.Bytes(), &meta) != nil {
		return false, nil
	}
	return meta.Type == "session_meta" &&
		meta.Payload.ID == thread &&
		meta.Payload.Cwd != "" &&
		filepath.Clean(meta.Payload.Cwd) == workDir, nil
}
