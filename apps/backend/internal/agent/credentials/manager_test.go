package credentials

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

func newTestLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	return log
}

func TestNewManager(t *testing.T) {
	log := newTestLogger()
	mgr := NewManager(log)

	if mgr == nil {
		t.Fatal("expected non-nil manager")
	} else if len(mgr.providers) != 0 {
		t.Errorf("expected no providers, got %d", len(mgr.providers))
	}
}

func TestManager_AddProvider(t *testing.T) {
	log := newTestLogger()
	mgr := NewManager(log)

	provider := NewEnvProvider("")
	mgr.AddProvider(provider)

	if len(mgr.providers) != 1 {
		t.Errorf("expected 1 provider, got %d", len(mgr.providers))
	}
}

func TestManager_GetCredential_FromEnv(t *testing.T) {
	// Set up test environment variable
	testKey := "TEST_CREDENTIAL_KEY_12345"
	testValue := "test-secret-value"
	t.Setenv(testKey, testValue)

	log := newTestLogger()
	mgr := NewManager(log)
	mgr.AddProvider(NewEnvProvider(""))

	ctx := context.Background()
	cred, err := mgr.GetCredential(ctx, testKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cred.Key != testKey {
		t.Errorf("expected key %q, got %q", testKey, cred.Key)
	}
	if cred.Value != testValue {
		t.Errorf("expected value %q, got %q", testValue, cred.Value)
	}
	if cred.Source != "environment" {
		t.Errorf("expected source 'environment', got %q", cred.Source)
	}
}

func TestManager_GetCredential_Cached(t *testing.T) {
	testKey := "TEST_CACHED_KEY"
	testValue := "cached-value"
	t.Setenv(testKey, testValue)

	log := newTestLogger()
	mgr := NewManager(log)
	mgr.AddProvider(NewEnvProvider(""))

	ctx := context.Background()

	// First call should fetch and cache
	cred1, err := mgr.GetCredential(ctx, testKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Remove from env - should still get cached value
	if err := os.Unsetenv(testKey); err != nil {
		t.Fatalf("failed to unset env var: %v", err)
	}

	cred2, err := mgr.GetCredential(ctx, testKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cred1.Value != cred2.Value {
		t.Error("expected cached value to be returned")
	}
}

func TestManager_GetCredential_NotFound(t *testing.T) {
	log := newTestLogger()
	mgr := NewManager(log)
	mgr.AddProvider(NewEnvProvider(""))

	ctx := context.Background()
	_, err := mgr.GetCredential(ctx, "NON_EXISTENT_KEY_999999")
	if err == nil {
		t.Error("expected error for non-existent credential")
	}
}

func TestManager_GetCredentials(t *testing.T) {
	testKey1 := "TEST_MULTI_KEY_1"
	testKey2 := "TEST_MULTI_KEY_2"
	t.Setenv(testKey1, "value1")
	t.Setenv(testKey2, "value2")

	log := newTestLogger()
	mgr := NewManager(log)
	mgr.AddProvider(NewEnvProvider(""))

	ctx := context.Background()
	creds, err := mgr.GetCredentials(ctx, []string{testKey1, testKey2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(creds) != 2 {
		t.Errorf("expected 2 credentials, got %d", len(creds))
	}
	if creds[testKey1].Value != "value1" {
		t.Errorf("expected value1, got %q", creds[testKey1].Value)
	}
	if creds[testKey2].Value != "value2" {
		t.Errorf("expected value2, got %q", creds[testKey2].Value)
	}
}

func TestManager_GetCredentials_PartialFailure(t *testing.T) {
	testKey := "TEST_PARTIAL_KEY"
	t.Setenv(testKey, "value")

	log := newTestLogger()
	mgr := NewManager(log)
	mgr.AddProvider(NewEnvProvider(""))

	ctx := context.Background()
	creds, err := mgr.GetCredentials(ctx, []string{testKey, "NON_EXISTENT_999"})

	// Should return partial results with error
	if err == nil {
		t.Error("expected error for missing credentials")
	}
	if len(creds) != 1 {
		t.Errorf("expected 1 credential, got %d", len(creds))
	}
}

func TestManager_BuildEnvVars(t *testing.T) {
	testKey := "TEST_BUILD_ENV_KEY"
	testValue := "build-env-value"
	t.Setenv(testKey, testValue)

	log := newTestLogger()
	mgr := NewManager(log)
	mgr.AddProvider(NewEnvProvider(""))

	ctx := context.Background()
	additional := map[string]string{
		"EXTRA_KEY": "extra-value",
	}

	envVars, err := mgr.BuildEnvVars(ctx, []string{testKey}, additional)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(envVars) != 2 {
		t.Errorf("expected 2 env vars, got %d", len(envVars))
	}

	// Sort for consistent comparison
	sort.Strings(envVars)
	expectedVars := []string{
		"EXTRA_KEY=extra-value",
		testKey + "=" + testValue,
	}
	sort.Strings(expectedVars)

	for i, expected := range expectedVars {
		if envVars[i] != expected {
			t.Errorf("expected %q, got %q", expected, envVars[i])
		}
	}
}

func TestManager_BuildEnvVars_MissingRequired(t *testing.T) {
	log := newTestLogger()
	mgr := NewManager(log)
	mgr.AddProvider(NewEnvProvider(""))

	ctx := context.Background()
	_, err := mgr.BuildEnvVars(ctx, []string{"NON_EXISTENT_REQUIRED"}, nil)
	if err == nil {
		t.Error("expected error for missing required credential")
	}
}

func TestManager_HasCredential(t *testing.T) {
	testKey := "TEST_HAS_CREDENTIAL"
	t.Setenv(testKey, "value")

	log := newTestLogger()
	mgr := NewManager(log)
	mgr.AddProvider(NewEnvProvider(""))

	ctx := context.Background()

	if !mgr.HasCredential(ctx, testKey) {
		t.Error("expected credential to exist")
	}
	if mgr.HasCredential(ctx, "NON_EXISTENT_99999") {
		t.Error("expected credential to not exist")
	}
}

func TestManager_ListAvailable(t *testing.T) {
	testKey := "TEST_LIST_AVAILABLE_API_KEY"
	t.Setenv(testKey, "value")

	log := newTestLogger()
	mgr := NewManager(log)
	mgr.AddProvider(NewEnvProvider(""))

	ctx := context.Background()
	available := mgr.ListAvailable(ctx)

	// Should include our test key (contains "api_key" pattern)
	found := false
	for _, key := range available {
		if key == testKey {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected %q in available list, got %v", testKey, available)
	}
}

func TestManager_ClearCache(t *testing.T) {
	testKey := "TEST_CLEAR_CACHE"
	t.Setenv(testKey, "original-value")

	log := newTestLogger()
	mgr := NewManager(log)
	mgr.AddProvider(NewEnvProvider(""))

	ctx := context.Background()

	// First call - cache the value
	cred1, _ := mgr.GetCredential(ctx, testKey)
	if cred1.Value != "original-value" {
		t.Fatalf("expected original-value, got %q", cred1.Value)
	}

	// Update the env and clear cache
	if err := os.Setenv(testKey, "new-value"); err != nil {
		t.Fatalf("failed to update env var: %v", err)
	}
	mgr.ClearCache()

	// Should get new value
	cred2, _ := mgr.GetCredential(ctx, testKey)
	if cred2.Value != "new-value" {
		t.Errorf("expected new-value after cache clear, got %q", cred2.Value)
	}
}

// EnvProvider tests

func TestEnvProvider_Name(t *testing.T) {
	provider := NewEnvProvider("")
	if provider.Name() != "environment" {
		t.Errorf("expected name 'environment', got %q", provider.Name())
	}
}

func TestEnvProvider_GetCredential(t *testing.T) {
	testKey := "TEST_ENV_PROVIDER_KEY"
	testValue := "env-provider-value"
	t.Setenv(testKey, testValue)

	provider := NewEnvProvider("")
	ctx := context.Background()

	cred, err := provider.GetCredential(ctx, testKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred.Key != testKey {
		t.Errorf("expected key %q, got %q", testKey, cred.Key)
	}
	if cred.Value != testValue {
		t.Errorf("expected value %q, got %q", testValue, cred.Value)
	}
}

func TestEnvProvider_GetCredential_WithPrefix(t *testing.T) {
	prefix := "KANDEV_"
	testKey := "MY_SECRET"
	testValue := "prefixed-value"
	t.Setenv(prefix+testKey, testValue)

	provider := NewEnvProvider(prefix)
	ctx := context.Background()

	cred, err := provider.GetCredential(ctx, testKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred.Key != testKey {
		t.Errorf("expected key %q, got %q", testKey, cred.Key)
	}
	if cred.Value != testValue {
		t.Errorf("expected value %q, got %q", testValue, cred.Value)
	}
}

func TestEnvProvider_ListAvailable(t *testing.T) {
	testKey := "ANTHROPIC_API_KEY"
	t.Setenv(testKey, "test-value")

	provider := NewEnvProvider("")
	ctx := context.Background()

	available, err := provider.ListAvailable(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, key := range available {
		if key == testKey {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected %q in available list", testKey)
	}
}

// FileProvider tests

func TestFileProvider_Name(t *testing.T) {
	provider := NewFileProvider("/nonexistent")
	if provider.Name() != "file" {
		t.Errorf("expected name 'file', got %q", provider.Name())
	}
}

func TestFileProvider_GetCredential(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "creds.json")

	creds := map[string]string{
		"SECRET_KEY": "secret-value",
		"API_KEY":    "api-value",
	}
	data, _ := json.Marshal(creds)
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	provider := NewFileProvider(configPath)
	ctx := context.Background()

	cred, err := provider.GetCredential(ctx, "SECRET_KEY")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred.Key != "SECRET_KEY" {
		t.Errorf("expected key 'SECRET_KEY', got %q", cred.Key)
	}
	if cred.Value != "secret-value" {
		t.Errorf("expected value 'secret-value', got %q", cred.Value)
	}
	if cred.Source != "file" {
		t.Errorf("expected source 'file', got %q", cred.Source)
	}
}

func TestFileProvider_GetCredential_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "creds.json")

	creds := map[string]string{"EXISTING": "value"}
	data, _ := json.Marshal(creds)
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	provider := NewFileProvider(configPath)
	ctx := context.Background()

	_, err := provider.GetCredential(ctx, "NON_EXISTENT")
	if err == nil {
		t.Error("expected error for non-existent key")
	}
}

func TestFileProvider_NonExistentFile(t *testing.T) {
	provider := NewFileProvider("/path/does/not/exist.json")
	ctx := context.Background()

	// Should not error - just returns not found for keys
	_, err := provider.GetCredential(ctx, "ANY_KEY")
	if err == nil {
		t.Error("expected error for non-existent key")
	}
}

func TestFileProvider_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.json")

	if err := os.WriteFile(configPath, []byte("invalid json"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	provider := NewFileProvider(configPath)
	ctx := context.Background()

	_, err := provider.GetCredential(ctx, "ANY_KEY")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestFileProvider_ListAvailable(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "creds.json")

	creds := map[string]string{
		"KEY_1": "value1",
		"KEY_2": "value2",
	}
	data, _ := json.Marshal(creds)
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	provider := NewFileProvider(configPath)
	ctx := context.Background()

	available, err := provider.ListAvailable(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(available) != 2 {
		t.Errorf("expected 2 keys, got %d", len(available))
	}
}

func TestFileProvider_Reload(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "creds.json")

	// Initial content
	creds := map[string]string{"KEY": "original"}
	data, _ := json.Marshal(creds)
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	provider := NewFileProvider(configPath)
	ctx := context.Background()

	// Load initial
	cred, _ := provider.GetCredential(ctx, "KEY")
	if cred.Value != "original" {
		t.Fatalf("expected original, got %q", cred.Value)
	}

	// Update file
	creds["KEY"] = "updated"
	data, _ = json.Marshal(creds)
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Reload
	err := provider.Reload()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should get new value
	cred, _ = provider.GetCredential(ctx, "KEY")
	if cred.Value != "updated" {
		t.Errorf("expected updated, got %q", cred.Value)
	}
}
