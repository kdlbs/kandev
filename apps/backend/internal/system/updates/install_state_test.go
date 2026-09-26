package updates

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

	svc := NewService(
		newTestPool(t),
		"v1.0.0",
		nil,
		logger.Default(),
		WithHomeDir(homeDir),
		WithSettingsStore(&memorySettingsStore{}),
	)
	resp, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !resp.Install.RunningAsService || !resp.Install.ManagedService {
		t.Fatalf("install state = %+v, want managed service", resp.Install)
	}
	if !resp.ApplySupported {
		t.Fatalf("ApplySupported=false reason=%q", resp.ApplyUnsupportedReason)
	}
	if !resp.ChannelEditable || resp.ChannelUnsupportedReason != "" {
		t.Fatalf("nightly capability editable=%v reason=%q", resp.ChannelEditable, resp.ChannelUnsupportedReason)
	}
}

func TestService_GetManagedNativeServiceSupportsApply(t *testing.T) {
	homeDir := t.TempDir()
	servicePath := filepath.Join(homeDir, "kandev.service")
	metadataPath := filepath.Join(homeDir, "service", "install.json")
	if err := os.MkdirAll(filepath.Dir(metadataPath), 0o700); err != nil {
		t.Fatal(err)
	}
	metadata := map[string]interface{}{
		"version":          serviceMetadataVersion,
		"manager":          serviceManagerSystemd,
		"mode":             installModeUser,
		"kind":             installKindHomebrew,
		"home_dir":         homeDir,
		"log_dir":          filepath.Join(homeDir, "logs"),
		"service_path":     servicePath,
		"launcher_path":    "/opt/homebrew/Cellar/kandev/1.2.3/libexec/bin/kandev",
		"bundle_dir":       "/opt/homebrew/Cellar/kandev/1.2.3/libexec",
		"launcher_version": "1.2.3",
		"installed_at":     "2026-07-25T00:00:00Z",
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metadataPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	serviceContent := managedMarkerText + "\n" + envRunningAsService + "\n" +
		envServiceMetadata + "=" + metadataPath + "\n"
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envRunningAsService, "true")
	t.Setenv(envServiceMode, installModeUser)
	t.Setenv(envServiceManager, serviceManagerSystemd)
	t.Setenv(envInstallKind, installKindHomebrew)
	t.Setenv(envServiceMetadata, metadataPath)

	svc := NewService(newTestPool(t), "v1.0.0", nil, logger.Default(), WithHomeDir(homeDir))
	resp, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !resp.Install.ManagedService || !resp.ApplySupported {
		t.Fatalf("native install state = %+v apply_supported=%v reason=%q", resp.Install, resp.ApplySupported, resp.ApplyUnsupportedReason)
	}
	if resp.ChannelEditable || resp.ChannelUnsupportedReason == "" {
		t.Fatalf("Homebrew nightly capability editable=%v reason=%q", resp.ChannelEditable, resp.ChannelUnsupportedReason)
	}
}

func TestService_GetManagedSystemdServiceSupportsApplyWithPercentInMetadataPath(t *testing.T) {
	homeDir := filepath.Join(t.TempDir(), "home%dir")
	metadataPath, servicePath := writeServiceInstallForTest(t, homeDir, serviceInstallMetadata{
		Manager:      serviceManagerSystemd,
		Mode:         installModeUser,
		Kind:         installKindHomebrew,
		HomeDir:      homeDir,
		LogDir:       filepath.Join(homeDir, "logs"),
		ServicePath:  filepath.Join(homeDir, "kandev.service"),
		LauncherPath: "/opt/homebrew/bin/kandev",
	})
	serviceContent := managedMarkerText + "\n" + envRunningAsService + "\n" +
		envServiceMetadata + "=" + strings.ReplaceAll(metadataPath, "%", "%%") + "\n"
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envRunningAsService, "true")
	t.Setenv(envServiceMode, installModeUser)
	t.Setenv(envServiceManager, serviceManagerSystemd)
	t.Setenv(envInstallKind, installKindHomebrew)
	t.Setenv(envServiceMetadata, metadataPath)

	svc := NewService(newTestPool(t), "v1.0.0", nil, logger.Default(), WithHomeDir(homeDir))
	resp, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !resp.Install.ManagedService || !resp.ApplySupported {
		t.Fatalf("percent-escaped install state = %+v apply_supported=%v reason=%q",
			resp.Install, resp.ApplySupported, resp.ApplyUnsupportedReason)
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
	resp, err := svc.Get(context.Background())
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
	resp, err := svc.Get(context.Background())
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

func TestService_GetLocalBundleServiceDisablesApply(t *testing.T) {
	homeDir := t.TempDir()
	metadataPath, _ := writeServiceInstallForTest(t, homeDir, serviceInstallMetadata{
		Manager:     "systemd",
		Mode:        "user",
		Kind:        installKindLocal,
		HomeDir:     homeDir,
		LogDir:      filepath.Join(homeDir, "logs"),
		ServicePath: filepath.Join(homeDir, "kandev.service"),
		NodePath:    "/usr/local/bin/node",
		CLIEntry:    "/Users/alice/src/kandev/dist/kandev/cli/bin/cli.js",
		BundleDir:   "/Users/alice/src/kandev/dist/kandev",
	})
	t.Setenv(envRunningAsService, "true")
	t.Setenv(envServiceMode, "user")
	t.Setenv(envServiceManager, "systemd")
	t.Setenv(envInstallKind, installKindLocal)
	t.Setenv(envServiceMetadata, metadataPath)

	svc := NewService(newTestPool(t), "v1.0.0", nil, logger.Default(), WithHomeDir(homeDir))
	resp, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !resp.Install.ManagedService {
		t.Fatalf("expected service to be recognised as managed")
	}
	if resp.ApplySupported {
		t.Fatalf("ApplySupported=true for local bundle service")
	}
	if resp.ApplyUnsupportedReason == "" {
		t.Fatalf("expected unsupported reason for local bundle service")
	}
	if !hasString(resp.ManualCommands, "kandev service install") {
		t.Fatalf("manual commands = %v, want service install command", resp.ManualCommands)
	}
	if !hasString(resp.ManualCommands, "kandev service restart") {
		t.Fatalf("manual commands = %v, want service restart command", resp.ManualCommands)
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

func TestManagedUserServiceCapabilitiesShareBaseEligibility(t *testing.T) {
	for name, state := range map[string]InstallStateResponse{
		"not running": {
			ManagedService: true,
			Mode:           installModeUser,
			Manager:        serviceManagerSystemd,
			Kind:           installKindNPM,
		},
		"unmanaged": {
			RunningAsService: true,
			Mode:             installModeUser,
			Manager:          serviceManagerSystemd,
			Kind:             installKindNPM,
		},
		"system mode": {
			RunningAsService: true,
			ManagedService:   true,
			Mode:             installModeSystem,
			Manager:          serviceManagerSystemd,
			Kind:             installKindNPM,
		},
		"unsupported manager": {
			RunningAsService: true,
			ManagedService:   true,
			Mode:             installModeUser,
			Manager:          "other",
			Kind:             installKindNPM,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if supported, _ := state.applySupport(); supported {
				t.Fatal("apply unexpectedly supported")
			}
			if supported, _ := state.nightlySupport(); supported {
				t.Fatal("Nightly unexpectedly supported")
			}
		})
	}

	homebrew := InstallStateResponse{
		RunningAsService: true,
		ManagedService:   true,
		Mode:             installModeUser,
		Manager:          serviceManagerLaunchd,
		Kind:             installKindHomebrew,
	}
	if supported, _ := homebrew.applySupport(); !supported {
		t.Fatal("managed Homebrew user service should support Stable apply")
	}
	if supported, _ := homebrew.nightlySupport(); supported {
		t.Fatal("managed Homebrew user service should not support Nightly")
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
