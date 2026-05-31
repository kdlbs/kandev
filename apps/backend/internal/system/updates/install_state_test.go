package updates

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

func TestService_GetManagedUserServiceSupportsApply(t *testing.T) {
	homeDir := t.TempDir()
	metadataPath, _ := writeServiceInstallForTest(t, homeDir, serviceInstallMetadata{
		Manager:     "systemd",
		Mode:        "user",
		Kind:        "npm",
		HomeDir:     homeDir,
		LogDir:      filepath.Join(homeDir, "logs"),
		ServicePath: filepath.Join(homeDir, "kandev.service"),
		NodePath:    "/usr/bin/node",
		CLIEntry:    "/usr/lib/node_modules/kandev/bin/cli.js",
	})
	t.Setenv(envRunningAsService, "true")
	t.Setenv(envServiceMode, "user")
	t.Setenv(envServiceManager, "systemd")
	t.Setenv(envInstallKind, "npm")
	t.Setenv(envServiceMetadata, metadataPath)

	svc := NewService(newTestPool(t), "v1.0.0", nil, logger.Default(), WithHomeDir(homeDir))
	resp, err := svc.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !resp.Install.RunningAsService || !resp.Install.ManagedService {
		t.Fatalf("install state = %+v, want managed service", resp.Install)
	}
	if !resp.ApplySupported {
		t.Fatalf("ApplySupported=false reason=%q", resp.ApplyUnsupportedReason)
	}
}

func TestService_GetSystemServiceDisablesApply(t *testing.T) {
	homeDir := t.TempDir()
	metadataPath, _ := writeServiceInstallForTest(t, homeDir, serviceInstallMetadata{
		Manager:     "systemd",
		Mode:        "system",
		Kind:        "homebrew",
		HomeDir:     homeDir,
		LogDir:      filepath.Join(homeDir, "logs"),
		ServicePath: filepath.Join(homeDir, "kandev.service"),
		NodePath:    "/opt/homebrew/bin/node",
		CLIEntry:    "/opt/homebrew/opt/kandev/libexec/cli/bin/cli.js",
	})
	t.Setenv(envRunningAsService, "true")
	t.Setenv(envServiceMode, "system")
	t.Setenv(envServiceManager, "systemd")
	t.Setenv(envInstallKind, "homebrew")
	t.Setenv(envServiceMetadata, metadataPath)

	svc := NewService(newTestPool(t), "v1.0.0", nil, logger.Default(), WithHomeDir(homeDir))
	resp, err := svc.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !resp.Install.ManagedService {
		t.Fatalf("expected service to be recognised as managed")
	}
	if resp.ApplySupported {
		t.Fatalf("ApplySupported=true for system service")
	}
	if resp.ApplyUnsupportedReason == "" {
		t.Fatalf("expected unsupported reason")
	}
	if !hasString(resp.ManualCommands, "kandev service install --system") {
		t.Fatalf("manual commands = %v, want system install command", resp.ManualCommands)
	}
	if !hasString(resp.ManualCommands, "kandev service restart --system") {
		t.Fatalf("manual commands = %v, want system restart command", resp.ManualCommands)
	}
}

func TestService_GetForeignServiceDisablesApply(t *testing.T) {
	homeDir := t.TempDir()
	metadataPath, servicePath := writeServiceInstallForTest(t, homeDir, serviceInstallMetadata{
		Manager:     "launchd",
		Mode:        "user",
		Kind:        "npx",
		HomeDir:     homeDir,
		LogDir:      filepath.Join(homeDir, "logs"),
		ServicePath: filepath.Join(homeDir, "com.kdlbs.kandev.plist"),
		NodePath:    "/usr/local/bin/node",
		CLIEntry:    "/Users/alice/.npm/_npx/cache/node_modules/kandev/bin/cli.js",
	})
	if err := os.WriteFile(servicePath, []byte("not managed\n"), 0o644); err != nil {
		t.Fatalf("write foreign service: %v", err)
	}
	t.Setenv(envRunningAsService, "true")
	t.Setenv(envServiceMode, "user")
	t.Setenv(envServiceManager, "launchd")
	t.Setenv(envInstallKind, "npx")
	t.Setenv(envServiceMetadata, metadataPath)

	svc := NewService(newTestPool(t), "v1.0.0", nil, logger.Default(), WithHomeDir(homeDir))
	resp, err := svc.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.Install.ManagedService {
		t.Fatalf("foreign service was treated as managed: %+v", resp.Install)
	}
	if resp.ApplySupported {
		t.Fatalf("ApplySupported=true for foreign service")
	}
}

func TestManualCommandsNPXHasNoDuplicateBinaryName(t *testing.T) {
	cmds := manualCommands(InstallStateResponse{Kind: installKindNPX, Mode: installModeUser}, "v1.2.3")
	if !hasString(cmds, "npx -y kandev@1.2.3 service install") {
		t.Fatalf("manual commands = %v, want non-duplicated npx install command", cmds)
	}
	if hasString(cmds, "npx -y kandev@1.2.3 kandev service install") {
		t.Fatalf("npx manual command duplicates the binary name: %v", cmds)
	}
}

func writeServiceInstallForTest(t *testing.T, homeDir string, metadata serviceInstallMetadata) (string, string) {
	t.Helper()
	metadata.Version = serviceMetadataVersion
	if metadata.InstalledAt == "" {
		metadata.InstalledAt = "2026-05-29T00:00:00Z"
	}
	metadataPath := filepath.Join(homeDir, "service", "install.json")
	metadata.ServicePath = filepath.Clean(metadata.ServicePath)
	if err := os.MkdirAll(filepath.Dir(metadataPath), 0o700); err != nil {
		t.Fatalf("mkdir metadata dir: %v", err)
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	if err := os.WriteFile(metadataPath, data, 0o600); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	serviceContent := managedMarkerText + "\n" + envRunningAsService + "\n" + envServiceMetadata + "=" + metadataPath + "\n"
	if err := os.WriteFile(metadata.ServicePath, []byte(serviceContent), 0o644); err != nil {
		t.Fatalf("write service: %v", err)
	}
	return metadataPath, metadata.ServicePath
}

func hasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
