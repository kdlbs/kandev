package lifecycle

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/remoteauth"
)

type recordingCredentialUploader struct {
	path string
	data []byte
	mode os.FileMode
}

type memoryCredentialUploader struct {
	files map[string][]byte
}

func (u *memoryCredentialUploader) ReadFile(_ context.Context, path string) ([]byte, error) {
	data, ok := u.files[path]
	if !ok {
		return nil, &os.PathError{Op: "read", Path: path, Err: fs.ErrNotExist}
	}
	return append([]byte(nil), data...), nil
}

func (u *memoryCredentialUploader) WriteFile(
	_ context.Context,
	path string,
	data []byte,
	_ os.FileMode,
) error {
	u.files[path] = append([]byte(nil), data...)
	return nil
}

func (u *recordingCredentialUploader) WriteFile(_ context.Context, path string, data []byte, mode os.FileMode) error {
	u.path = path
	u.data = append([]byte(nil), data...)
	u.mode = mode
	return nil
}

func TestUploadCredentialFilesWritesSecretFilesPrivate(t *testing.T) {
	hostHome := seedTestHostHome(t)
	writeFile(t, hostHome, ".local/share/devin/credentials.toml", []byte("windsurf_api_key = \"secret\"\n"))

	uploader := &recordingCredentialUploader{}
	methods := []remoteauth.Method{{
		MethodID:     "agent:devin-acp:files:0",
		Type:         "files",
		SourceFiles:  []string{".local/share/devin/credentials.toml"},
		TargetRelDir: ".local/share/devin",
	}}

	targetHome := filepath.Join(t.TempDir(), "remote-home")
	if err := UploadCredentialFiles(context.Background(), uploader, methods, targetHome, newSeederTestLogger(t)); err != nil {
		t.Fatalf("UploadCredentialFiles: %v", err)
	}

	wantPath := filepath.Join(targetHome, ".local/share/devin", "credentials.toml")
	if uploader.path != wantPath {
		t.Fatalf("uploaded path = %q, want %q", uploader.path, wantPath)
	}
	if string(uploader.data) != "windsurf_api_key = \"secret\"\n" {
		t.Fatalf("uploaded data = %q", string(uploader.data))
	}
	if uploader.mode != credentialFileMode {
		t.Fatalf("uploaded mode = %o, want %o", uploader.mode, credentialFileMode)
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.11
func TestUploadCredentialFilesMergesOpenCodeProviderMap(t *testing.T) {
	hostHome := seedTestHostHome(t)
	writeFile(t, hostHome, ".local/share/opencode/auth.json", []byte(`{
		"openai":{"type":"oauth","access":"host-openai"},
		"anthropic":{"type":"oauth","access":"host-anthropic"}
	}`))

	targetHome := filepath.Join(t.TempDir(), "remote-home")
	targetPath := filepath.Join(targetHome, ".local/share/opencode/auth.json")
	uploader := &memoryCredentialUploader{files: map[string][]byte{
		targetPath: []byte(`{
			"openai":{"type":"oauth","access":"remote-openai"},
			"custom":{"type":"api","key":"remote-custom"}
		}`),
	}}
	methods := []remoteauth.Method{{
		MethodID:           "agent:opencode-acp:files:0",
		Type:               "files",
		SourceFiles:        []string{".local/share/opencode/auth.json"},
		TargetRelDir:       ".local/share/opencode",
		FileConflictPolicy: agents.RemoteAuthFileConflictPolicyMergeJSONObject,
	}}

	if err := UploadCredentialFiles(context.Background(), uploader, methods, targetHome, newSeederTestLogger(t)); err != nil {
		t.Fatalf("UploadCredentialFiles: %v", err)
	}

	var providers map[string]map[string]string
	if err := json.Unmarshal(uploader.files[targetPath], &providers); err != nil {
		t.Fatalf("merged auth file is not JSON: %v", err)
	}
	if providers["custom"]["key"] != "remote-custom" {
		t.Fatalf("target-only provider was replaced: %s", uploader.files[targetPath])
	}
	if providers["anthropic"]["access"] != "host-anthropic" {
		t.Fatalf("source-only provider is missing: %s", uploader.files[targetPath])
	}
	if providers["openai"]["access"] != "host-openai" {
		t.Fatalf("source provider did not win collision: %s", uploader.files[targetPath])
	}
}

func TestUploadCredentialFilesWritesMergeSourceToIsolatedTarget(t *testing.T) {
	hostHome := seedTestHostHome(t)
	writeFile(t, hostHome, ".local/share/opencode/auth.json", []byte(`{"openai":{"type":"oauth"}}`))

	uploader := &recordingCredentialUploader{}
	methods := []remoteauth.Method{{
		Type:               "files",
		SourceFiles:        []string{".local/share/opencode/auth.json"},
		TargetRelDir:       ".local/share/opencode",
		FileConflictPolicy: agents.RemoteAuthFileConflictPolicyMergeJSONObject,
	}}

	err := UploadCredentialFiles(context.Background(), uploader, methods, t.TempDir(), newSeederTestLogger(t))
	if err != nil {
		t.Fatalf("UploadCredentialFiles: %v", err)
	}
	if string(uploader.data) != `{"openai":{"type":"oauth"}}` {
		t.Fatalf("uploaded auth = %s", uploader.data)
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.12
func TestUploadCredentialFilesLeavesMalformedTargetUnchanged(t *testing.T) {
	hostHome := seedTestHostHome(t)
	writeFile(t, hostHome, ".local/share/opencode/auth.json", []byte(`{"openai":{"type":"oauth"}}`))

	targetHome := filepath.Join(t.TempDir(), "remote-home")
	targetPath := filepath.Join(targetHome, ".local/share/opencode/auth.json")
	const malformed = `{"custom":`
	uploader := &memoryCredentialUploader{files: map[string][]byte{targetPath: []byte(malformed)}}
	methods := []remoteauth.Method{{
		Type:               "files",
		SourceFiles:        []string{".local/share/opencode/auth.json"},
		TargetRelDir:       ".local/share/opencode",
		FileConflictPolicy: agents.RemoteAuthFileConflictPolicyMergeJSONObject,
	}}

	err := UploadCredentialFiles(context.Background(), uploader, methods, targetHome, newSeederTestLogger(t))
	if err == nil || !strings.Contains(err.Error(), "existing target is not a JSON object") {
		t.Fatalf("UploadCredentialFiles error = %v", err)
	}
	if string(uploader.files[targetPath]) != malformed {
		t.Fatalf("malformed target changed to %q", uploader.files[targetPath])
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.12
func TestUploadCredentialFilesLeavesTargetUnchangedForMalformedSource(t *testing.T) {
	hostHome := seedTestHostHome(t)
	writeFile(t, hostHome, ".local/share/opencode/auth.json", []byte(`{"openai":`))

	targetHome := filepath.Join(t.TempDir(), "remote-home")
	targetPath := filepath.Join(targetHome, ".local/share/opencode/auth.json")
	const target = `{"custom":{"type":"api"}}`
	uploader := &memoryCredentialUploader{files: map[string][]byte{targetPath: []byte(target)}}
	methods := []remoteauth.Method{{
		Type:               "files",
		SourceFiles:        []string{".local/share/opencode/auth.json"},
		TargetRelDir:       ".local/share/opencode",
		FileConflictPolicy: agents.RemoteAuthFileConflictPolicyMergeJSONObject,
	}}

	err := UploadCredentialFiles(context.Background(), uploader, methods, targetHome, newSeederTestLogger(t))
	if err == nil || !strings.Contains(err.Error(), "source is not a JSON object") {
		t.Fatalf("UploadCredentialFiles error = %v", err)
	}
	if string(uploader.files[targetPath]) != target {
		t.Fatalf("target changed to %q", uploader.files[targetPath])
	}
}
