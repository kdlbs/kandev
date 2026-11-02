package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/launchwrapper"
	"github.com/kandev/kandev/internal/common/opencode"
)

// @covers AC-EXECUTORS-OPENCODE-AUTH-001.9
func TestOpenCodeCredentialIdentityCapability(t *testing.T) {
	server := newTestServer(t)
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/identity", nil))
	var identity struct {
		Capabilities []string `json:"capabilities"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &identity); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(identity.Capabilities, "opencode-credential-bootstrap.v1") {
		t.Fatal("worker does not advertise bootstrap protocol")
	}
}

func configureCredentialFixture(server *Server, args []string, env map[string]string) error {
	env["PATH"] = os.Getenv("PATH")
	runtime := managedruntime.OpenCodeRuntime{Family: managedruntime.OpenCodeFamilyV2, Source: managedruntime.OpenCodeSourceManaged, Version: "2.1.2"}
	return server.procMgr.ConfigureRuntime("", args[3:], true, env, "", nil, false, false, runtime, launchwrapper.Profile(args[:3]))
}

func credentialFixtureAccountsPath(home, store string) string {
	if store == "" {
		store = filepath.Join(home, "opencode.db")
	}
	return store + ".accounts.json"
}

func writeCredentialFixtureAccounts(home, store string, data []byte) error {
	path := credentialFixtureAccountsPath(home, store)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// @covers AC-EXECUTORS-OPENCODE-AUTH-001.9
func TestOpenCodeCredentialRuntimeMetadataRejectsNative(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(func() { _ = server.procMgr.StopForTeardown(context.Background()) })
	args := []string{os.Args[0], "-test.run=^TestOpenCodeCredentialImportHelper$", "--", "npx", "--yes", "--prefer-offline", "--prefix", "~/.kandev/managed-npm-runtime", "@opencode/cli@2.1.2", "acp"}
	body, err := json.Marshal(map[string]any{
		"agent_args":       args,
		"env":              map[string]string{"HOME": t.TempDir(), "KANDEV_TEST_CREDENTIAL_IMPORT": "1"},
		"opencode_runtime": map[string]string{"family": "v2", "source": "native", "version": "2.1.2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/agent/configure", bytes.NewReader(body)))
	if response.Code != http.StatusOK {
		t.Fatalf("configure: %s", response.Body.String())
	}
	response = httptest.NewRecorder()
	server.router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/agent/opencode/credentials", strings.NewReader(`[{"id":"cred_test"}]`)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("native metadata bypassed by managed-looking ACP argv: status %d", response.Code)
	}
}

func TestOpenCodeCredentialImportUsesSessionHomeAndStdin(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(func() { _ = server.procMgr.StopForTeardown(context.Background()) })
	home := t.TempDir()
	args := []string{os.Args[0], "-test.run=^TestOpenCodeCredentialImportHelper$", "--", "npx", "--yes", "--prefer-offline", "--prefix", "~/.kandev/managed-npm-runtime", "@opencode/cli@2.1.2", "acp", "--print-logs"}
	if err := configureCredentialFixture(server, args, map[string]string{"HOME": home, "KANDEV_TEST_CREDENTIAL_IMPORT": "1"}); err != nil {
		t.Fatal(err)
	}
	const credentials = `[{"id":"cred_test","integrationID":"openai","label":"test","active":true,"value":{"type":"key","key":"synthetic-secret"}}]`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/opencode/credentials", strings.NewReader(credentials))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	server.router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("credential import status = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	got, err := os.ReadFile(credentialFixtureAccountsPath(home, ""))
	if err != nil || string(got) != credentials {
		t.Fatalf("session import did not receive exact credentials: %v", err)
	}
}

// @covers AC-EXECUTORS-OPENCODE-AUTH-001.11
func TestOpenCodeCredentialLostAcknowledgement(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(func() { _ = server.procMgr.StopForTeardown(context.Background()) })
	home := t.TempDir()
	args := []string{os.Args[0], "-test.run=^TestOpenCodeCredentialImportHelper$", "--", "npx", "--yes", "--prefer-offline", "--prefix", "~/.kandev/managed-npm-runtime", "@opencode/cli@2.1.2", "acp"}
	if err := configureCredentialFixture(server, args, map[string]string{"HOME": home, "KANDEV_TEST_CREDENTIAL_IMPORT": "1"}); err != nil {
		t.Fatal(err)
	}
	original := []byte(`[{"id":"credential","active":true,"value":{"type":"key","key":"original"}}]`)
	if err := server.procMgr.ImportOpenCodeCredentials(context.Background(), original); err != nil {
		t.Fatal(err)
	}
	refreshed := []byte(`[{"id":"credential","active":false,"value":{"type":"key","key":"refreshed"}}]`)
	if err := writeCredentialFixtureAccounts(home, "", refreshed); err != nil {
		t.Fatal(err)
	}
	if err := server.procMgr.ImportOpenCodeCredentials(context.Background(), original); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(credentialFixtureAccountsPath(home, ""))
	if err != nil || !bytes.Equal(got, refreshed) {
		t.Fatal("acknowledgement retry replaced destination-owned credentials")
	}
}

// @covers AC-EXECUTORS-OPENCODE-AUTH-001.11
func TestOpenCodeCredentialPendingRetry(t *testing.T) {
	home := t.TempDir()
	args := []string{os.Args[0], "-test.run=^TestOpenCodeCredentialImportHelper$", "--", "npx", "--yes", "--prefer-offline", "--prefix", "~/.kandev/managed-npm-runtime", "@opencode/cli@2.1.2", "acp"}
	first := newTestServer(t)
	t.Cleanup(func() { _ = first.procMgr.StopForTeardown(context.Background()) })
	if err := configureCredentialFixture(first, args, map[string]string{"HOME": home, "KANDEV_TEST_CREDENTIAL_IMPORT": "1", "KANDEV_TEST_IMPORT_PARTIAL": "1"}); err != nil {
		t.Fatal(err)
	}
	data := []byte(`[{"id":"first"},{"id":"second"}]`)
	if err := first.procMgr.ImportOpenCodeCredentials(context.Background(), data); err == nil {
		t.Fatal("partial import succeeded")
	}
	// Recreate the worker: readiness must derive from durable state, even though
	// the first account is now present in the credential store.
	second := newTestServer(t)
	t.Cleanup(func() { _ = second.procMgr.StopForTeardown(context.Background()) })
	if err := configureCredentialFixture(second, args, map[string]string{"HOME": home, "KANDEV_TEST_CREDENTIAL_IMPORT": "1"}); err != nil {
		t.Fatal(err)
	}
	state, err := second.procMgr.OpenCodeCredentialStatus(context.Background())
	if err != nil || state != opencode.CredentialStorePending {
		t.Fatalf("interrupted bootstrap state = %q, %v", state, err)
	}
	if err := second.procMgr.ImportOpenCodeCredentials(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(credentialFixtureAccountsPath(home, ""))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("partial bootstrap was not completed")
	}
	state, err = second.procMgr.OpenCodeCredentialStatus(context.Background())
	if err != nil || state != opencode.CredentialStorePresent {
		t.Fatalf("completed bootstrap state = %q, %v", state, err)
	}
}

// @covers AC-EXECUTORS-OPENCODE-AUTH-001.11
func TestOpenCodeCredentialInvalidMarker(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(func() { _ = server.procMgr.StopForTeardown(context.Background()) })
	home := t.TempDir()
	args := []string{os.Args[0], "-test.run=^TestOpenCodeCredentialImportHelper$", "--", "npx", "acp"}
	if err := configureCredentialFixture(server, args, map[string]string{"HOME": home, "KANDEV_TEST_CREDENTIAL_IMPORT": "1"}); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(home, "opencode.db.kandev-bootstrap-v1.pending")
	if err := os.Mkdir(marker, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := server.procMgr.OpenCodeCredentialStatus(context.Background()); err == nil {
		t.Fatal("invalid marker reported usable bootstrap state")
	}
	if err := server.procMgr.ImportOpenCodeCredentials(context.Background(), []byte(`[{"id":"test"}]`)); err == nil {
		t.Fatal("invalid marker allowed import")
	}
	if _, err := os.Stat(credentialFixtureAccountsPath(home, "")); !os.IsNotExist(err) {
		t.Fatal("invalid marker allowed credential store mutation")
	}
}

// @covers AC-EXECUTORS-OPENCODE-AUTH-001.10, AC-EXECUTORS-OPENCODE-AUTH-001.11
func TestOpenCodeCredentialDestinationEnvironment(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("OPENCODE_DB", filepath.Join(t.TempDir(), "ambient.db"))
	server := newTestServer(t)
	t.Cleanup(func() { _ = server.procMgr.StopForTeardown(context.Background()) })
	home := t.TempDir()
	store := filepath.Join(t.TempDir(), "session.db")
	env := map[string]string{"HOME": home, "XDG_DATA_HOME": filepath.Join(home, "data"), "OPENCODE_DB": store,
		"KANDEV_TEST_CREDENTIAL_IMPORT": "1", "KANDEV_TEST_DESTINATION_HOME": home, "KANDEV_TEST_IMPORT_PARTIAL": "1"}
	args := []string{os.Args[0], "-test.run=^TestOpenCodeCredentialImportHelper$", "--", "npx", "acp"}
	if err := configureCredentialFixture(server, args, env); err != nil {
		t.Fatal(err)
	}
	if err := server.procMgr.ImportOpenCodeCredentials(context.Background(), []byte(`[{"id":"test"}]`)); err == nil {
		t.Fatal("partial import unexpectedly completed")
	}
	if _, err := os.Stat(store + ".kandev-bootstrap-v1.pending"); err != nil {
		t.Fatalf("pending marker did not follow effective store: %v", err)
	}
	// A different effective store must not inherit the previous store's pending state.
	env["OPENCODE_DB"] = filepath.Join(t.TempDir(), "other.db")
	if err := configureCredentialFixture(server, args, env); err != nil {
		t.Fatal(err)
	}
	state, err := server.procMgr.OpenCodeCredentialStatus(context.Background())
	if err != nil || state != opencode.CredentialStoreEmpty {
		t.Fatalf("new store inherited old pending state: %q, %v", state, err)
	}
}

func TestOpenCodeCredentialImportHelper(t *testing.T) {
	if os.Getenv("KANDEV_TEST_CREDENTIAL_IMPORT") != "1" {
		return
	}
	if home := os.Getenv("KANDEV_TEST_DESTINATION_HOME"); home != "" &&
		(os.Getenv("HOME") != home || os.Getenv("XDG_DATA_HOME") != filepath.Join(home, "data")) {
		os.Exit(8)
	}
	if slices.Equal(os.Args[len(os.Args)-3:], []string{"debug", "paths", "db"}) {
		path := os.Getenv("OPENCODE_DB")
		if path == "" {
			path = filepath.Join(os.Getenv("HOME"), "opencode.db")
		}
		fmt.Println(path)
		os.Exit(0)
	}
	if slices.Equal(os.Args[len(os.Args)-3:], []string{"auth", "export", "--standalone"}) {
		if os.Getenv("KANDEV_TEST_EXPORT_FAIL") == "1" {
			fmt.Print("synthetic-secret")
			os.Exit(6)
		}
		data, err := os.ReadFile(credentialFixtureAccountsPath(os.Getenv("HOME"), os.Getenv("OPENCODE_DB")))
		if os.IsNotExist(err) {
			data, err = []byte(`[]`), nil
		}
		if err != nil {
			os.Exit(7)
		}
		_, _ = os.Stdout.Write(data)
		os.Exit(0)
	}
	if !slices.Equal(os.Args[len(os.Args)-3:], []string{"auth", "import", "--standalone"}) {
		os.Exit(2)
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil || !json.Valid(data) {
		os.Exit(3)
	}
	if os.Getenv("KANDEV_TEST_IMPORT_FAIL") == "1" {
		_, _ = os.Stdout.Write(data)
		_, _ = os.Stderr.Write(data)
		os.Exit(6)
	}
	if os.Getenv("KANDEV_TEST_IMPORT_PARTIAL") == "1" {
		if err := writeCredentialFixtureAccounts(os.Getenv("HOME"), os.Getenv("OPENCODE_DB"), []byte(`[{"id":"first"}]`)); err != nil {
			os.Exit(7)
		}
		os.Exit(6)
	}
	if barrier := os.Getenv("KANDEV_TEST_IMPORT_BARRIER"); barrier != "" {
		resp, err := http.Get(barrier)
		if err != nil {
			os.Exit(5)
		}
		_ = resp.Body.Close()
	}
	if err := writeCredentialFixtureAccounts(os.Getenv("HOME"), os.Getenv("OPENCODE_DB"), data); err != nil {
		os.Exit(4)
	}
	os.Exit(0)
}

// @covers AC-EXECUTORS-OPENCODE-AUTH-001.7
func TestOpenCodeCredentialStatus(t *testing.T) {
	for _, tc := range []struct {
		name, content, state string
		fail                 bool
	}{
		{"empty", "", opencode.CredentialStoreEmpty, false},
		{"present", `[{"id":"cred_test","value":{"key":"synthetic-secret"}}]`, opencode.CredentialStorePresent, false},
		{"invalid", `null`, "", false},
		{"invalid entry", `[null]`, "", false},
		{"missing account identity", `[{}]`, "", false},
		{"failed", "", "", true},
		{"oversized", `[]` + strings.Repeat(" ", 1<<20), "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := newTestServer(t)
			t.Cleanup(func() { _ = server.procMgr.StopForTeardown(context.Background()) })
			home := t.TempDir()
			if tc.content != "" {
				if err := writeCredentialFixtureAccounts(home, "", []byte(tc.content)); err != nil {
					t.Fatal(err)
				}
			}
			args := []string{os.Args[0], "-test.run=^TestOpenCodeCredentialImportHelper$", "--", "npx", "--yes", "--prefer-offline", "--prefix", "~/.kandev/managed-npm-runtime", "@opencode/cli@2.1.2", "acp"}
			env := map[string]string{"HOME": home, "KANDEV_TEST_CREDENTIAL_IMPORT": "1"}
			if tc.fail {
				env["KANDEV_TEST_EXPORT_FAIL"] = "1"
			}
			if err := configureCredentialFixture(server, args, env); err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			server.router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/agent/opencode/credentials", nil))
			if tc.state == "" {
				if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "synthetic-secret") {
					t.Fatalf("unsafe unknown status: %d %s", response.Code, response.Body.String())
				}
				return
			}
			var status struct {
				Version int    `json:"version"`
				State   string `json:"state"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusOK || status.Version != 1 || status.State != tc.state || strings.Contains(response.Body.String(), "synthetic-secret") {
				t.Fatalf("status: %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestOpenCodeCredentialImportSerializesConfigurationAndStart(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(func() { _ = server.procMgr.StopForTeardown(context.Background()) })
	arrived, release := make(chan struct{}), make(chan struct{})
	barrier := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(arrived)
		<-release
	}))
	defer barrier.Close()
	releaseImport := sync.OnceFunc(func() { close(release) })
	defer releaseImport()
	args := []string{os.Args[0], "-test.run=^TestOpenCodeCredentialImportHelper$", "--", "npx", "--yes", "--prefer-offline", "--prefix", "~/.kandev/managed-npm-runtime", "@opencode/cli@2.1.2", "acp"}
	env := map[string]string{"HOME": t.TempDir(), "KANDEV_TEST_CREDENTIAL_IMPORT": "1", "KANDEV_TEST_IMPORT_BARRIER": barrier.URL}
	if err := configureCredentialFixture(server, args, env); err != nil {
		t.Fatal(err)
	}
	// Readiness now starts two CLI children before the import child. Allow
	// instrumented process startup while keeping the teardown assertions bounded.
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- server.procMgr.ImportOpenCodeCredentials(ctx, []byte(`[]`)) }()
	select {
	case <-arrived:
	case err := <-done:
		t.Fatalf("import exited before barrier: %v", err)
	case <-ctx.Done():
		t.Fatal("import did not reach barrier")
	}
	if err := server.procMgr.Configure("", args, true, env, "", nil, false); err == nil {
		t.Fatal("configuration changed during credential import")
	}
	if _, err := server.procMgr.StartWithGeneration(ctx); err == nil || !strings.Contains(err.Error(), "credential import") {
		t.Fatalf("startup was not blocked by credential import: %v", err)
	}
	if err := server.procMgr.ImportOpenCodeCredentials(ctx, []byte(`[]`)); err == nil {
		t.Fatal("concurrent credential import was accepted")
	}
	if _, err := server.procMgr.Passthrough().Start(ctx, types.PassthroughStartRequest{
		SessionID: server.procMgr.GetSessionID(), OperationID: "during-import", Command: args,
	}); err == nil || !strings.Contains(err.Error(), "credential import") {
		t.Fatalf("passthrough startup was not blocked by credential import: %v", err)
	}
	releaseImport()
	if err := <-done; err != nil {
		t.Fatalf("credential import failed: %v", err)
	}
	if err := server.procMgr.Configure("", args, true, env, "", nil, false); err != nil {
		t.Fatalf("configuration remained blocked after import: %v", err)
	}
}

func TestOpenCodeCredentialImportsSerializeAcrossManagers(t *testing.T) {
	home := t.TempDir()
	store := filepath.Join(home, "data", "opencode.db")
	firstEntered := make(chan struct{})
	secondEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	var imports atomic.Int32
	barrier := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		switch imports.Add(1) {
		case 1:
			close(firstEntered)
			<-releaseFirst
		default:
			close(secondEntered)
			<-releaseFirst
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(barrier.Close)

	servers := []*Server{newTestServer(t), newTestServer(t)}
	for _, server := range servers {
		server := server
		t.Cleanup(func() { _ = server.procMgr.StopForTeardown(context.Background()) })
		args := []string{os.Args[0], "-test.run=^TestOpenCodeCredentialImportHelper$", "--", "npx", "--yes", "--prefer-offline", "--prefix", "~/.kandev/managed-npm-runtime", "@opencode/cli@2.1.2", "acp"}
		env := map[string]string{
			"HOME":                          home,
			"OPENCODE_DB":                   store,
			"KANDEV_TEST_CREDENTIAL_IMPORT": "1",
			"KANDEV_TEST_IMPORT_BARRIER":    barrier.URL,
		}
		if err := configureCredentialFixture(server, args, env); err != nil {
			t.Fatal(err)
		}
	}
	releaseImport := sync.OnceFunc(func() { close(releaseFirst) })
	t.Cleanup(releaseImport)

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- servers[0].procMgr.ImportOpenCodeCredentials(context.Background(), []byte(`[{"id":"first"}]`))
	}()
	select {
	case <-firstEntered:
	case err := <-firstDone:
		t.Fatalf("first import exited before its barrier: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("first import did not reach its barrier")
	}

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- servers[1].procMgr.ImportOpenCodeCredentials(context.Background(), []byte(`[{"id":"second"}]`))
	}()
	select {
	case <-secondEntered:
		t.Fatal("second manager entered the shared-store import while the first import was active")
	case err := <-secondDone:
		t.Fatalf("second import exited before the first store commit: %v", err)
	case <-time.After(500 * time.Millisecond):
	}
	releaseImport()
	for name, done := range map[string]<-chan error{"first": firstDone, "second": secondDone} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("%s import: %v", name, err)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("%s import did not finish", name)
		}
	}
	if got := imports.Load(); got != 1 {
		t.Fatalf("credential import commands = %d, want one after the shared store became present", got)
	}
	got, err := os.ReadFile(credentialFixtureAccountsPath(home, store))
	if _, err := os.Stat(credentialFixtureAccountsPath(home, "")); !os.IsNotExist(err) {
		t.Fatal("OpenCode test CLI wrote credentials under HOME instead of OPENCODE_DB")
	}
	if err != nil || string(got) != `[{"id":"first"}]` {
		t.Fatalf("shared destination credentials = %s, %v", got, err)
	}
}

func TestOpenCodeCredentialImportRejectsUnconfiguredAgent(t *testing.T) {
	server := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/opencode/credentials", strings.NewReader(`[{"id":"cred_test"}]`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	server.router.ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("credential import status = %d, want %d", resp.Code, http.StatusBadRequest)
	}
}

func TestOpenCodeCredentialImportFailureDoesNotExposeOutput(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(func() { _ = server.procMgr.StopForTeardown(context.Background()) })
	args := []string{os.Args[0], "-test.run=^TestOpenCodeCredentialImportHelper$", "--", "npx", "--yes", "--prefer-offline", "--prefix", "~/.kandev/managed-npm-runtime", "@opencode/cli@2.1.2", "acp"}
	if err := configureCredentialFixture(server, args, map[string]string{"HOME": t.TempDir(), "KANDEV_TEST_CREDENTIAL_IMPORT": "1", "KANDEV_TEST_IMPORT_FAIL": "1"}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/agent/opencode/credentials", strings.NewReader(`[{"secret":"synthetic-secret"}]`)))
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "synthetic-secret") {
		t.Fatalf("failed credential command exposed output or succeeded: status %d", response.Code)
	}
}

func TestOpenCodeCredentialImportCancellationReleasesSession(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(func() { _ = server.procMgr.StopForTeardown(context.Background()) })
	arrived, disconnected := make(chan struct{}), make(chan struct{})
	release := make(chan struct{})
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
	args := []string{os.Args[0], "-test.run=^TestOpenCodeCredentialImportHelper$", "--", "npx", "--yes", "--prefer-offline", "--prefix", "~/.kandev/managed-npm-runtime", "@opencode/cli@2.1.2", "acp"}
	env := map[string]string{"HOME": t.TempDir(), "KANDEV_TEST_CREDENTIAL_IMPORT": "1", "KANDEV_TEST_IMPORT_BARRIER": barrier.URL}
	if err := configureCredentialFixture(server, args, env); err != nil {
		t.Fatal(err)
	}
	// Readiness starts two CLI children before the import reaches this barrier.
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- server.procMgr.ImportOpenCodeCredentials(ctx, []byte(`[]`)) }()
	select {
	case <-arrived:
	case err := <-done:
		t.Fatalf("import exited before cancellation barrier: %v", err)
	case <-ctx.Done():
		t.Fatal("import did not reach barrier")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled import succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled import did not finish")
	}
	select {
	case <-disconnected:
	case <-time.After(time.Second):
		t.Fatal("import subprocess survived cancellation")
	}
	if err := server.procMgr.Configure("", args, true, env, "", nil, false); err != nil {
		t.Fatalf("canceled import left configuration blocked: %v", err)
	}
}

func TestOpenCodeCredentialImportRejectsInvalidPayloadBeforeLaunching(t *testing.T) {
	for _, payload := range []string{`null`, `{}`, `[]`, `{"secret":"synthetic-secret"}`, strings.Repeat("x", (1<<20)+1)} {
		t.Run(fmt.Sprintf("bytes-%d", len(payload)), func(t *testing.T) {
			server := newTestServer(t)
			t.Cleanup(func() { _ = server.procMgr.StopForTeardown(context.Background()) })
			home := t.TempDir()
			args := []string{os.Args[0], "-test.run=^TestOpenCodeCredentialImportHelper$", "--", "npx", "--yes", "--prefer-offline", "--prefix", "~/.kandev/managed-npm-runtime", "@opencode/cli@2.1.2", "acp"}
			if err := configureCredentialFixture(server, args, map[string]string{"HOME": home, "KANDEV_TEST_CREDENTIAL_IMPORT": "1"}); err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			server.router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/agent/opencode/credentials", strings.NewReader(payload)))
			if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), "synthetic-secret") {
				t.Fatalf("invalid credential payload response: %d", response.Code)
			}
			if _, err := os.Stat(credentialFixtureAccountsPath(home, "")); !os.IsNotExist(err) {
				t.Fatal("invalid payload reached import process")
			}
		})
	}
}
