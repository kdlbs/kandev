package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agent/remoteauth"
)

type recordingCredentialUploader struct {
	path string
	data []byte
	mode os.FileMode
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
