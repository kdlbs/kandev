package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

type RuntimeArtifact struct {
	Name     string
	Content  string
	Bindings []ArtifactBinding
}

type RuntimeFiles struct {
	Root        string
	Executable  string
	Directories RuntimeDirectories
	Artifacts   map[string]string
	Digests     map[string]string
	mu          sync.Mutex
	ownedRoot   string
	identity    os.FileInfo
	closed      bool
}

func MaterializeRuntime(ctx context.Context, base string, executable []byte, digest string, artifacts []RuntimeArtifact, values map[string]string) (*RuntimeFiles, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validRuntimeDirectory(base) || len(executable) == 0 || len(executable) > 128<<20 || runtimeDigest(executable) != digest {
		return nil, ErrInvalidPolicy
	}
	rendered, err := renderRuntimeArtifacts(artifacts, values)
	if err != nil {
		return nil, err
	}
	root, err := os.MkdirTemp(base, "attempt-")
	if err != nil {
		return nil, ErrInvalidPolicy
	}
	files := &RuntimeFiles{Root: root, ownedRoot: root, Executable: filepath.Join(root, "runtime"), Artifacts: map[string]string{}, Digests: map[string]string{}}
	files.identity, err = os.Stat(root)
	if err != nil {
		_ = os.RemoveAll(root)
		return nil, ErrInvalidPolicy
	}
	if err = files.write(ctx, executable, rendered); err != nil {
		_ = files.Close()
		return nil, err
	}
	return files, nil
}

func (f *RuntimeFiles) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil
	}
	current, err := os.Lstat(f.ownedRoot)
	if errors.Is(err, os.ErrNotExist) {
		f.closed = true
		return nil
	}
	if err != nil || f.identity == nil || !os.SameFile(current, f.identity) || !current.IsDir() {
		return ErrInvalidPolicy
	}
	if err = os.RemoveAll(f.ownedRoot); err != nil {
		return err
	}
	f.closed = true
	return nil
}

func renderRuntimeArtifacts(artifacts []RuntimeArtifact, values map[string]string) (map[string][]byte, error) {
	if len(artifacts) > 32 {
		return nil, ErrInvalidPolicy
	}
	rendered := make(map[string][]byte, len(artifacts))
	total := 0
	for _, artifact := range artifacts {
		if !validRuntimeArtifactName(artifact.Name) || rendered[artifact.Name] != nil || !utf8.ValidString(artifact.Content) || strings.ContainsRune(artifact.Content, 0) {
			return nil, ErrInvalidPolicy
		}
		content := []byte(artifact.Content)
		if len(artifact.Bindings) > 0 {
			var err error
			content, err = RenderJSONArtifact(artifact.Content, artifact.Bindings, values)
			if err != nil {
				return nil, err
			}
		}
		total += len(content)
		if total > maxArtifactBytes {
			return nil, ErrInvalidPolicy
		}
		rendered[artifact.Name] = content
	}
	return rendered, nil
}

func validRuntimeArtifactName(name string) bool {
	first, _, _ := strings.Cut(name, "/")
	return name != "" && name != "." && name != ".." && len(name) <= 255 && path.Clean(name) == name && !path.IsAbs(name) && !strings.HasPrefix(name, "../") &&
		!strings.ContainsAny(name, "\\:") && !strings.ContainsFunc(name, unicode.IsControl) && utf8.ValidString(name) &&
		!slices.Contains([]string{"runtime", "home", "config", "state", "tmp"}, first)
}

func (f *RuntimeFiles) write(ctx context.Context, executable []byte, artifacts map[string][]byte) error {
	f.Directories = RuntimeDirectories{Home: filepath.Join(f.Root, "home"), Config: filepath.Join(f.Root, "config"), State: filepath.Join(f.Root, "state"), Temp: filepath.Join(f.Root, "tmp")}
	for _, dir := range []string{f.Directories.Home, f.Directories.Config, f.Directories.State, f.Directories.Temp} {
		if err := os.Mkdir(dir, 0700); err != nil {
			return ErrInvalidPolicy
		}
	}
	if err := writeRuntimeFile(f.Executable, executable, 0500); err != nil {
		return err
	}
	f.Digests["runtime"] = runtimeDigest(executable)
	for name, content := range artifacts {
		if err := ctx.Err(); err != nil {
			return err
		}
		destination := filepath.Join(f.Root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
			return ErrInvalidPolicy
		}
		if err := writeRuntimeFile(destination, content, 0400); err != nil {
			return err
		}
		f.Artifacts[name] = destination
		f.Digests[name] = runtimeDigest(content)
	}
	return ctx.Err()
}

func writeRuntimeFile(name string, content []byte, mode os.FileMode) error {
	file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return ErrInvalidPolicy
	}
	_, writeErr := file.Write(content)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return ErrInvalidPolicy
	}
	return nil
}

func runtimeDigest(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}
