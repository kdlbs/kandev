package lifecycle

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kandev/kandev/internal/agent/agents"
	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	"github.com/kandev/kandev/internal/agent/remoteauth"
)

const (
	testKubernetesFileMissingMarker = "\x00KANDEV_FILE_V1:missing\x00"
	testKubernetesFilePresentMarker = "\x00KANDEV_FILE_V1:present\x00"
)

func TestKubernetesCredentialUploaderMergesRemoteAuthFile(t *testing.T) {
	tests := []struct {
		name       string
		remoteFile []byte
		readErr    error
		wantErr    string
		wantFile   string
	}{
		{
			name:       "keeps remote-only providers and replaces matching credentials",
			remoteFile: []byte(`{"remote":{"key":"keep"},"anthropic":{"key":"old"}}`),
			wantFile:   `{"anthropic":{"key":"host"},"remote":{"key":"keep"}}`,
		},
		{
			name:     "uses host file when remote file is missing",
			wantFile: `{"anthropic":{"key":"host"}}`,
		},
		{
			name:       "leaves remote file unchanged when read fails",
			remoteFile: []byte(`{"remote":{"key":"keep"}}`),
			readErr:    errors.New("permission denied"),
			wantErr:    "read existing target",
			wantFile:   `{"remote":{"key":"keep"}}`,
		},
		{
			name:       "leaves malformed remote file unchanged",
			remoteFile: []byte(`not json`),
			wantErr:    "existing target is not a JSON object",
			wantFile:   `not json`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			sourcePath := filepath.Join(home, ".pi", "agent", "auth.json")
			require.NoError(t, os.MkdirAll(filepath.Dir(sourcePath), 0o700))
			require.NoError(t, os.WriteFile(sourcePath, []byte(`{"anthropic":{"key":"host"}}`), 0o600))

			remotePath := "/run/kandev/sessions/session/home/.pi/agent/auth.json"
			podExec := &kubernetesFilePodExecutor{
				files:    map[string][]byte{},
				readErrs: map[string]error{},
			}
			if test.remoteFile != nil {
				podExec.files[remotePath] = append([]byte(nil), test.remoteFile...)
			}
			if test.readErr != nil {
				podExec.readErrs[remotePath] = test.readErr
			}
			uploader := newTestKubernetesPodFileUploader(podExec)
			catalog := remoteauth.BuildCatalogForHost([]agents.Agent{agents.NewPiACP()}, runtime.GOOS, home)
			var method remoteauth.Method
			for _, spec := range catalog.Specs {
				if spec.ID == "pi-acp" {
					method = spec.Methods[0]
					break
				}
			}
			require.NotEmpty(t, method.MethodID)

			err := UploadCredentialFiles(
				context.Background(), uploader, []remoteauth.Method{method},
				"/run/kandev/sessions/session/home", newTestLogger(),
			)

			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, test.wantFile, string(podExec.files[remotePath]))
		})
	}
}

func TestKubernetesPodFileUploaderReadFileDistinguishesMissingAndReadErrors(t *testing.T) {
	remotePath := "/run/kandev/sessions/session/home/.pi/agent/auth.json"
	podExec := &kubernetesFilePodExecutor{
		files:    map[string][]byte{},
		readErrs: map[string]error{},
	}
	uploader := newTestKubernetesPodFileUploader(podExec)

	_, err := uploader.ReadFile(context.Background(), remotePath)
	require.ErrorIs(t, err, fs.ErrNotExist)

	podExec.readErrs[remotePath] = errors.New("permission denied")
	_, err = uploader.ReadFile(context.Background(), remotePath)
	require.Error(t, err)
	require.NotErrorIs(t, err, fs.ErrNotExist)
}

func TestKubernetesReadFileCommandReportsMissingAndExistingFiles(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.ToSlash(filepath.Join(root, "nested", "auth.json"))
	var output bytes.Buffer
	command := exec.Command("sh", "-c", kubernetesReadFileCommand(filePath))
	command.Stdout = &output
	require.NoError(t, command.Run())
	require.Equal(t, testKubernetesFileMissingMarker, output.String())

	require.NoError(t, os.MkdirAll(filepath.Dir(filePath), 0o700))
	require.NoError(t, os.WriteFile(filePath, []byte(`{"provider":"remote"}`), 0o600))
	output.Reset()
	command = exec.Command("sh", "-c", kubernetesReadFileCommand(filePath))
	command.Stdout = &output
	require.NoError(t, command.Run())
	require.Equal(t, testKubernetesFilePresentMarker+`{"provider":"remote"}`, output.String())

	linkPath := filepath.ToSlash(filepath.Join(root, "linked", "auth.json"))
	require.NoError(t, os.MkdirAll(filepath.Dir(linkPath), 0o700))
	require.NoError(t, os.Symlink(filePath, linkPath))
	command = exec.Command("sh", "-c", kubernetesReadFileCommand(linkPath))
	require.Error(t, command.Run())
}

func TestKubernetesFreshProvisionCopiesPortableConfigAndReconnectPreservesRemoteEdits(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for name, content := range map[string]string{
		"settings.json": `{"theme":"host"}`,
		"models.json":   `{"models":["host-model"]}`,
	} {
		filePath := filepath.Join(home, ".pi", "agent", name)
		require.NoError(t, os.MkdirAll(filepath.Dir(filePath), 0o700))
		require.NoError(t, os.WriteFile(filePath, []byte(content), 0o600))
	}

	files := &kubernetesFilePodExecutor{files: map[string][]byte{}, readErrs: map[string]error{}}
	streams := kubeexecutor.NewStreamOperations(files, nil)
	executor := &KubernetesExecutor{logger: newTestLogger()}
	runtime := &kubernetesRuntimeClient{streams: streams}
	req := validKubernetesCreateRequest()
	req.SessionID = "session"
	req.AgentConfig = agents.NewPiACP()
	req.Metadata[metadataKubernetesTaskOwned] = true
	req.Metadata[MetadataKeyAgentConfigBundles] = `["pi.config"]`
	req.Env = map[string]string{"OPENROUTER_API_KEY": "current-secret"}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "kandev-agents", Name: "agent-pod"}}
	settingsPath := filepath.Join(kubernetesSessionHome(req), ".pi", "agent", "settings.json")
	modelsPath := filepath.Join(kubernetesSessionHome(req), ".pi", "agent", "models.json")

	require.NoError(t, executor.materializeKubernetesRemoteFiles(context.Background(), runtime, req, pod, "kandev-agent"))
	require.Equal(t, `{"theme":"host"}`, string(files.files[settingsPath]))
	require.Equal(t, `{"models":["host-model"]}`, string(files.files[modelsPath]))

	files.files[settingsPath] = []byte(`{"theme":"remote-edit"}`)
	files.files[modelsPath] = []byte(`{"models":["remote-model"]}`)

	require.NoError(t, executor.prepareSharedKubernetesCredentials(context.Background(), runtime, req, pod, "kandev-agent"))
	require.Equal(t, `{"theme":"remote-edit"}`, string(files.files[settingsPath]))
	require.Equal(t, `{"models":["remote-model"]}`, string(files.files[modelsPath]))
	require.Contains(t, string(files.files[kubernetesSessionAuthPath(req)]), "OPENROUTER_API_KEY='current-secret'")
}

func newTestKubernetesPodFileUploader(executor kubeexecutor.PodExecutor) kubernetesPodFileUploader {
	return kubernetesPodFileUploader{
		streams: kubeexecutor.NewStreamOperations(executor, nil),
		pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Namespace: "kandev-agents", Name: "agent-pod",
		}},
		container: "kandev-agent",
	}
}

type kubernetesFilePodExecutor struct {
	mu       sync.Mutex
	files    map[string][]byte
	readErrs map[string]error
}

func (e *kubernetesFilePodExecutor) Exec(_ context.Context, request kubeexecutor.ExecRequest) error {
	command := strings.Join(request.Command, " ")
	destination := shellAssignmentValue(command, "destination")
	if destination == "" {
		destination = shellAssignmentValue(command, "source")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if strings.Contains(command, "KANDEV_FILE_V1:missing") {
		if err := e.readErrs[destination]; err != nil {
			return err
		}
		marker := testKubernetesFileMissingMarker
		if content, ok := e.files[destination]; ok {
			marker = testKubernetesFilePresentMarker
			marker += string(content)
		}
		_, err := io.WriteString(request.Stdout, marker)
		return err
	}
	if strings.Contains(command, `cat > "$temporary"`) {
		data, err := io.ReadAll(request.Stdin)
		if err != nil {
			return err
		}
		e.files[destination] = data
	}
	return nil
}

func shellAssignmentValue(command, name string) string {
	prefix := name + "="
	for _, line := range strings.Split(command, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.Trim(strings.TrimPrefix(line, prefix), "'")
		}
	}
	return ""
}
