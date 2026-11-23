package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agentruntime"
)

// @covers AC-EXECUTORS-OPENCODE-AUTH-001.7
func TestOpenCodeCredentialRestartWithoutSource(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity":
			_, _ = io.WriteString(w, `{"capabilities":["opencode-credential-bootstrap.v1"]}`)
		case "/api/v1/agent/opencode/credentials":
			if r.Method != http.MethodGet {
				t.Error("authenticated destination must not be imported again")
			}
			_, _ = io.WriteString(w, `{"version":1,"state":"present"}`)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	execution := &AgentExecution{AgentID: openCodeCredentialAgentID, RuntimeName: agentruntime.RuntimeDocker,
		OpenCodeRuntime: managedruntime.OpenCodeRuntime{Family: managedruntime.OpenCodeFamilyV2, Source: managedruntime.OpenCodeSourceManaged, Version: "2.1.2"}}
	client := newTestAgentctlClient(t, server.URL, newTestLogger())
	if err := newTestManager(t).provisionOpenCodeCredentials(context.Background(), execution, client); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveredOpenCodeRuntimeReachesKubernetesRefresh(t *testing.T) {
	want := managedruntime.OpenCodeRuntime{Family: managedruntime.OpenCodeFamilyV2, Source: managedruntime.OpenCodeSourceManaged, Version: "2.1.2"}
	var configured managedruntime.OpenCodeRuntime
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agent/configure":
			var request struct {
				Runtime managedruntime.OpenCodeRuntime `json:"opencode_runtime"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				return
			}
			configured = request.Runtime
			_, _ = io.WriteString(w, `{"success":true}`)
		case "/identity":
			_, _ = io.WriteString(w, `{"capabilities":["opencode-credential-bootstrap.v1"]}`)
		case "/api/v1/agent/opencode/credentials":
			if r.Method != http.MethodGet {
				t.Errorf("unexpected credential request method: %s", r.Method)
			}
			_, _ = io.WriteString(w, `{"version":1,"state":"present"}`)
		default:
			_, _ = io.WriteString(w, `{"success":true}`)
		}
	}))
	defer server.Close()

	manager := newTestManager(t)
	manager.profileResolver = &restartProfileResolver{profile: &AgentProfileInfo{
		ProfileID: "profile-opencode", AgentName: "opencode-acp",
	}}
	manager.SetManagedRuntimeSelectionStore(managedRuntimeOpenCodeStore{openCodeSelection: managedruntime.OpenCodeSelection{
		SchemaVersion: 1, Family: want.Family, Source: want.Source, Package: "@opencode/cli",
		SelectedVersion: want.Version, AppliedDefaultVersion: want.Version, Revision: 1,
	}})
	execution := &AgentExecution{
		ID: "recovered-opencode", AgentProfileID: "profile-opencode", RuntimeName: agentruntime.RuntimeKubernetes,
		WorkspacePath: t.TempDir(), metadata: map[string]interface{}{"remote_credentials": ` ["agent:opencode-acp:files:0"] `},
	}
	if err := manager.reDeriveRecoveredAgentIdentity(context.Background(), execution); err != nil {
		t.Fatalf("rederive recovered agent identity: %v", err)
	}
	if execution.OpenCodeRuntime != want {
		t.Fatalf("recovered runtime identity = %+v, want %+v", execution.OpenCodeRuntime, want)
	}
	client := newTestAgentctlClient(t, server.URL, newTestLogger())
	refresh := &RemoteInstanceRefresh{Instance: &ExecutorInstance{Client: client}}
	if _, err := manager.prepareRestartedKubernetesAgentctl(context.Background(), execution, refresh); err != nil {
		t.Fatalf("prepare restarted Kubernetes agentctl: %v", err)
	}
	if configured != want {
		t.Fatalf("Kubernetes refresh configured runtime = %+v, want %+v", configured, want)
	}
}

const syntheticOpenCodeCredentials = `[{"id":"cred_test","integrationID":"openai","label":"test","active":true,"value":{"type":"key","key":"synthetic-secret"}}]`

func TestOpenCodeCredentialProvisioningLeavesV1Unchanged(t *testing.T) {
	execution := &AgentExecution{
		AgentID: "opencode-acp", RuntimeName: agentruntime.RuntimeKubernetes,
		AgentArgs: []string{"npx", "--yes", "--prefer-offline", "--prefix", managedruntime.NPMProjectPrefix, "opencode-ai@1.18.32", "acp"},
		metadata:  map[string]interface{}{"remote_credentials": "invalid legacy metadata"},
	}
	if err := newTestManager(t).provisionOpenCodeCredentials(context.Background(), execution, nil); err != nil {
		t.Fatalf("V2 provisioning changed V1 launch behavior: %v", err)
	}
}

// @covers AC-EXECUTORS-OPENCODE-AUTH-001.9
func TestOpenCodeCredentialProvisioningRejectsNativeMetadata(t *testing.T) {
	execution := &AgentExecution{
		AgentID: "opencode-acp", RuntimeName: agentruntime.RuntimeDocker,
		AgentArgs:       []string{"custom-native-launcher", "acp"},
		OpenCodeRuntime: managedruntime.OpenCodeRuntime{Family: managedruntime.OpenCodeFamilyV2, Source: managedruntime.OpenCodeSourceNative, Version: "2.2.1"},
	}
	if err := newTestManager(t).provisionOpenCodeCredentials(context.Background(), execution, nil); err == nil {
		t.Fatal("selected native V2 provisioning silently bypassed")
	}
}

func runOpenCodeExportFixture() {
	if expected := os.Getenv("KANDEV_TEST_SOURCE_HOME"); expected != "" && (os.Getenv("HOME") != expected || os.Getenv("XDG_DATA_HOME") != expected+"/data") {
		os.Exit(3)
	}
	if endpoint := os.Getenv("KANDEV_TEST_EXPORT_CHILD"); endpoint != "" {
		runOpenCodeExportChild(endpoint)
		os.Exit(0)
	}
	if len(os.Args) < 4 || !slices.Equal(os.Args[len(os.Args)-3:], []string{"auth", "export", "--standalone"}) {
		os.Exit(2)
	}
	fmt.Print(syntheticOpenCodeCredentials)
	os.Exit(0)
}

func runOpenCodeExportChild(endpoint string) {
	if os.Getenv("KANDEV_TEST_EXPORT_GRANDCHILD") != "1" {
		child := exec.Command(os.Args[0], os.Args[1:]...)
		child.Env = append(os.Environ(), "KANDEV_TEST_EXPORT_GRANDCHILD=1")
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		_ = child.Run()
		return
	}
	if os.Getenv("KANDEV_TEST_EXPORT_OVERSIZE") == "1" {
		fmt.Print(strings.Repeat("x", (1<<20)+1))
	}
	resp, err := http.Get(endpoint)
	if err == nil {
		_ = resp.Body.Close()
	}
}

// @covers AC-EXECUTORS-OPENCODE-AUTH-001.10
func TestOpenCodeCredentialSourceEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("host CLI fixture uses an executable symlink")
	}
	bin := t.TempDir()
	if err := os.Symlink(os.Args[0], filepath.Join(bin, "npx")); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("PATH", bin)
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", home+"/data")
	t.Setenv("KANDEV_TEST_SOURCE_HOME", home)
	t.Setenv("KANDEV_TEST_OPENCODE_EXPORT", "1")
	source := captureOpenCodeCredentialSource()
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	runtime := managedruntime.OpenCodeRuntime{Family: managedruntime.OpenCodeFamilyV2, Source: managedruntime.OpenCodeSourceManaged, Version: "2.1.2"}
	data, err := source.export(context.Background(), runtime)
	if err != nil || string(data) != syntheticOpenCodeCredentials {
		t.Fatalf("snapshot export failed: %v", err)
	}
}

func TestOpenCodeCredentialExportBoundsOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("host CLI fixture uses an executable symlink")
	}
	release := make(chan struct{})
	barrier := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer barrier.Close()
	defer close(release)
	bin := t.TempDir()
	if err := os.Symlink(os.Args[0], filepath.Join(bin, "npx")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KANDEV_TEST_OPENCODE_EXPORT", "1")
	t.Setenv("KANDEV_TEST_EXPORT_CHILD", barrier.URL)
	t.Setenv("KANDEV_TEST_EXPORT_OVERSIZE", "1")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	source := captureOpenCodeCredentialSource()
	go func() {
		_, err := source.export(ctx, managedruntime.OpenCodeRuntime{Family: managedruntime.OpenCodeFamilyV2, Source: managedruntime.OpenCodeSourceManaged, Version: "2.1.2"})
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("oversized export succeeded")
		}
	case <-time.After(5 * time.Second):
		cancel()
		<-done //nolint:errcheck // Drain the failed export before asserting teardown.
		t.Fatal("oversized export was not stopped at the output limit")
	}
}

func TestOpenCodeCredentialExportCancellationStopsDescendants(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("host CLI fixture uses an executable symlink")
	}
	arrived, disconnected, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	barrier := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(arrived)
		select {
		case <-r.Context().Done():
			close(disconnected)
		case <-release:
		}
	}))
	defer barrier.Close()
	defer close(release)
	bin := t.TempDir()
	if err := os.Symlink(os.Args[0], filepath.Join(bin, "npx")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KANDEV_TEST_OPENCODE_EXPORT", "1")
	t.Setenv("KANDEV_TEST_EXPORT_CHILD", barrier.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	done := make(chan error, 1)
	source := captureOpenCodeCredentialSource()
	go func() {
		_, err := source.export(ctx, managedruntime.OpenCodeRuntime{Family: managedruntime.OpenCodeFamilyV2, Source: managedruntime.OpenCodeSourceManaged, Version: "2.1.2"})
		done <- err
	}()
	select {
	case <-arrived:
	case <-ctx.Done():
		t.Fatal("export child did not start")
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("cancelled export succeeded")
	}
	select {
	case <-disconnected:
	case <-time.After(time.Second):
		t.Fatal("export descendant survived cancellation")
	}
}

func TestConfigureAndStartAgentProvisionsOpenCodeBeforeStart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("host CLI fixture uses an executable symlink")
	}
	bin := t.TempDir()
	if err := os.Symlink(os.Args[0], filepath.Join(bin, "npx")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KANDEV_TEST_OPENCODE_EXPORT", "1")
	var calls []string
	var imported string
	var rejectImport bool
	var capability = true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		if r.URL.Path == "/api/v1/agent/configure" {
			var configured struct {
				Runtime managedruntime.OpenCodeRuntime `json:"opencode_runtime"`
			}
			if err := json.NewDecoder(r.Body).Decode(&configured); err != nil {
				t.Error(err)
			}
			if configured.Runtime.Version != "2.1.2" || configured.Runtime.Source != managedruntime.OpenCodeSourceManaged {
				t.Errorf("missing resolved runtime: %+v", configured.Runtime)
			}
		}
		if r.URL.Path == "/identity" {
			if capability {
				_, _ = io.WriteString(w, `{"capabilities":["opencode-credential-bootstrap.v1"]}`)
			} else {
				_, _ = io.WriteString(w, `{}`)
			}
			return
		}
		if r.URL.Path == "/api/v1/agent/opencode/credentials" {
			if r.Method == http.MethodGet {
				_, _ = io.WriteString(w, `{"version":1,"state":"empty"}`)
				return
			}
			data, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
			}
			imported = string(data)
			if rejectImport {
				http.Error(w, "synthetic-secret", http.StatusInternalServerError)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"success":true}`)
	}))
	t.Cleanup(srv.Close)
	client := newTestAgentctlClient(t, srv.URL, newTestLogger())
	execution := &AgentExecution{
		ID: "exec-opencode", AgentID: "opencode-acp", RuntimeName: agentruntime.RuntimeKubernetes,
		OpenCodeRuntime: managedruntime.OpenCodeRuntime{Family: managedruntime.OpenCodeFamilyV2, Source: managedruntime.OpenCodeSourceManaged, Version: "2.1.2"},
		AgentArgs:       []string{"npx", "--yes", "--prefer-offline", "--prefix", managedruntime.NPMProjectPrefix, "@opencode/cli@2.1.2", "acp"},
		WorkspacePath:   t.TempDir(), agentctl: client,
		metadata: map[string]interface{}{"remote_credentials": `["agent:opencode-acp:files:0"]`},
	}
	if _, err := newTestManager(t).configureAndStartAgent(context.Background(), execution); err != nil {
		t.Fatal(err)
	}
	if imported != syntheticOpenCodeCredentials || !slices.Equal(calls, []string{"/api/v1/agent/configure", "/identity", "/api/v1/agent/opencode/credentials", "/identity", "/api/v1/agent/opencode/credentials", "/api/v1/start"}) {
		t.Fatalf("credential transfer or ordering incorrect: calls=%v", calls)
	}
	calls = nil
	imported = ""
	execution.AgentCommand = "npx"
	refresh := &RemoteInstanceRefresh{Instance: &ExecutorInstance{Client: client}}
	if _, err := newTestManager(t).prepareRestartedKubernetesAgentctl(context.Background(), execution, refresh); err != nil {
		t.Fatal(err)
	}
	if imported != syntheticOpenCodeCredentials || !slices.Equal(calls, []string{"/api/v1/agent/configure", "/identity", "/api/v1/agent/opencode/credentials", "/identity", "/api/v1/agent/opencode/credentials", "/api/v1/start"}) {
		t.Fatalf("restart credential transfer or ordering incorrect: calls=%v", calls)
	}
	calls = nil
	rejectImport = true
	if _, err := newTestManager(t).configureAndStartAgent(context.Background(), execution); err == nil || strings.Contains(err.Error(), "synthetic-secret") {
		t.Fatalf("import failure was not sanitized and propagated: %v", err)
	}
	if !slices.Equal(calls, []string{"/api/v1/agent/configure", "/identity", "/api/v1/agent/opencode/credentials", "/identity", "/api/v1/agent/opencode/credentials"}) {
		t.Fatalf("failed credential import admitted ACP: %v", calls)
	}
	capability = false
	t.Setenv("KANDEV_TEST_EXPORT_CHILD", "invalid-export-endpoint")
	if _, err := newTestManager(t).configureAndStartAgent(context.Background(), execution); err == nil || !strings.Contains(err.Error(), "update agentctl") {
		t.Fatalf("unsupported worker must fail before attempting source export: %v", err)
	}
	for _, selection := range []string{"", `[]`, `["agent:other:files:0"]`} {
		calls = nil
		execution.metadata["remote_credentials"] = selection
		if _, err := newTestManager(t).configureAndStartAgent(context.Background(), execution); err != nil {
			t.Fatalf("unselected provisioning failed launch: %v", err)
		}
		if !slices.Equal(calls, []string{"/api/v1/agent/configure", "/api/v1/start"}) {
			t.Fatalf("unselected credentials were transferred: %v", calls)
		}
	}
}
