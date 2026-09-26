package updates

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	envRunningAsService = "KANDEV_RUNNING_AS_SERVICE"
	envServiceMode      = "KANDEV_SERVICE_MODE"
	envServiceManager   = "KANDEV_SERVICE_MANAGER"
	envInstallKind      = "KANDEV_INSTALL_KIND"
	envServiceMetadata  = "KANDEV_SERVICE_METADATA"

	serviceMetadataVersion = 1
	managedMarkerText      = "managed by kandev"
	installModeUser        = "user"
	installModeSystem      = "system"
	serviceManagerSystemd  = "systemd"
	serviceManagerLaunchd  = "launchd"
	installKindHomebrew    = "homebrew"
	installKindNPM         = "npm"
	installKindNPX         = "npx"
	installKindLocal       = "local"
	manualServiceInstall   = "kandev service install"
	manualServiceRestart   = "kandev service restart"
)

type serviceInstallMetadata struct {
	Version         int    `json:"version"`
	Manager         string `json:"manager"`
	Mode            string `json:"mode"`
	Kind            string `json:"kind"`
	HomeDir         string `json:"home_dir"`
	LogDir          string `json:"log_dir"`
	ServicePath     string `json:"service_path"`
	LauncherPath    string `json:"launcher_path,omitempty"`
	NodePath        string `json:"node_path"`
	CLIEntry        string `json:"cli_entry"`
	BundleDir       string `json:"bundle_dir,omitempty"`
	LauncherVersion string `json:"launcher_version,omitempty"`
	Port            int    `json:"port,omitempty"`
	SystemUser      string `json:"system_user,omitempty"`
	NoBootStart     bool   `json:"no_boot_start,omitempty"`
	InstalledAt     string `json:"installed_at"`
}

type managedUserServiceProblem uint8

const (
	managedUserServiceSupported managedUserServiceProblem = iota
	managedUserServiceNotRunning
	managedUserServiceUnmanaged
	managedUserServiceWrongMode
	managedUserServiceUnsupportedManager
)

func (s *Service) detectInstallState() (InstallStateResponse, *serviceInstallMetadata) {
	if s.getenv(envRunningAsService) != "true" {
		return InstallStateResponse{}, nil
	}
	state := InstallStateResponse{
		RunningAsService: true,
		ManagedService:   false,
		Mode:             s.getenv(envServiceMode),
		Manager:          s.getenv(envServiceManager),
		Kind:             s.getenv(envInstallKind),
		MetadataPath:     s.serviceMetadataPath(),
	}
	metadata, err := readServiceMetadata(state.MetadataPath)
	if err != nil {
		return state, nil
	}
	state = mergeInstallMetadata(state, metadata)
	state.ManagedService = s.metadataMatchesService(state, metadata)
	return state, metadata
}

func (s *Service) serviceMetadataPath() string {
	if p := s.getenv(envServiceMetadata); p != "" {
		return p
	}
	if s.homeDir == "" {
		return ""
	}
	return filepath.Join(s.homeDir, "service", "install.json")
}

func readServiceMetadata(metadataPath string) (*serviceInstallMetadata, error) {
	if metadataPath == "" {
		return nil, errors.New("metadata path missing")
	}
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, err
	}
	var metadata serviceInstallMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}
	if metadata.Version != serviceMetadataVersion {
		return nil, fmt.Errorf("unsupported service metadata version %d", metadata.Version)
	}
	legacyLauncher := metadata.NodePath != "" && metadata.CLIEntry != ""
	if metadata.ServicePath == "" || (metadata.LauncherPath == "" && !legacyLauncher) {
		return nil, errors.New("service metadata missing required paths")
	}
	return &metadata, nil
}

func mergeInstallMetadata(state InstallStateResponse, metadata *serviceInstallMetadata) InstallStateResponse {
	if state.Mode == "" {
		state.Mode = metadata.Mode
	}
	if state.Manager == "" {
		state.Manager = metadata.Manager
	}
	if state.Kind == "" {
		state.Kind = metadata.Kind
	}
	return state
}

func (s *Service) metadataMatchesService(state InstallStateResponse, metadata *serviceInstallMetadata) bool {
	if metadata == nil {
		return false
	}
	if state.Mode != metadata.Mode || state.Manager != metadata.Manager || state.Kind != metadata.Kind {
		return false
	}
	content, err := os.ReadFile(metadata.ServicePath)
	if err != nil {
		return false
	}
	text := string(content)
	if !strings.Contains(text, managedMarkerText) {
		return false
	}
	if !strings.Contains(text, envRunningAsService) || !strings.Contains(text, envServiceMetadata) {
		return false
	}
	if strings.Contains(text, state.MetadataPath) || strings.Contains(text, escapeXML(state.MetadataPath)) {
		return true
	}
	return state.Manager == serviceManagerSystemd &&
		strings.Contains(text, strings.ReplaceAll(state.MetadataPath, "%", "%%"))
}

func (s InstallStateResponse) applySupport() (bool, string) {
	if problem := s.managedUserServiceProblem(); problem != managedUserServiceSupported {
		return false, problem.reason(false)
	}
	if s.Kind != installKindHomebrew && s.Kind != installKindNPM && s.Kind != installKindNPX {
		return false, "This installation method does not support UI self-update."
	}
	return true, ""
}

func (s InstallStateResponse) nightlySupport() (bool, string) {
	if problem := s.managedUserServiceProblem(); problem != managedUserServiceSupported {
		return false, problem.reason(true)
	}
	if s.Kind != installKindNPM && s.Kind != installKindNPX {
		return false, "Nightly updates are only available for npm or npx installations."
	}
	return true, ""
}

func (s InstallStateResponse) managedUserServiceProblem() managedUserServiceProblem {
	if !s.RunningAsService {
		return managedUserServiceNotRunning
	}
	if !s.ManagedService {
		return managedUserServiceUnmanaged
	}
	if s.Mode != installModeUser {
		return managedUserServiceWrongMode
	}
	if s.Manager != serviceManagerSystemd && s.Manager != serviceManagerLaunchd {
		return managedUserServiceUnsupportedManager
	}
	return managedUserServiceSupported
}

func (p managedUserServiceProblem) reason(nightly bool) string {
	if nightly {
		switch p {
		case managedUserServiceNotRunning:
			return "Nightly updates require a Kandev-managed npm or npx user service."
		case managedUserServiceUnmanaged:
			return "Nightly updates require valid Kandev service metadata."
		case managedUserServiceWrongMode:
			return "Nightly updates are only available for user services."
		case managedUserServiceUnsupportedManager:
			return "This service manager does not support nightly updates."
		}
	}
	switch p {
	case managedUserServiceNotRunning:
		return "Kandev is not running as a managed service."
	case managedUserServiceUnmanaged:
		return "Kandev service metadata is missing or invalid."
	case managedUserServiceWrongMode:
		return "Self-update is only available for user services."
	case managedUserServiceUnsupportedManager:
		return "This service manager does not support UI self-update."
	}
	return ""
}

func manualCommands(install InstallStateResponse, latest string) []string {
	target := strings.TrimPrefix(latest, "v")
	if target == "" {
		target = "latest"
	}
	installCmd, restartCmd := manualServiceCommands(install)
	switch install.Kind {
	case installKindHomebrew:
		return []string{"brew upgrade kandev", installCmd, restartCmd}
	case installKindNPM:
		return []string{"npm install -g kandev@" + target, installCmd, restartCmd}
	case installKindNPX:
		// installCmd already starts with "kandev "; npx supplies the package, so
		// strip the duplicate binary name (npx -y kandev@X service install ...).
		return []string{"npx -y kandev@" + target + " " + strings.TrimPrefix(installCmd, "kandev "), restartCmd}
	default:
		return []string{installCmd, restartCmd}
	}
}

func manualServiceCommands(install InstallStateResponse) (string, string) {
	if install.Mode == installModeSystem {
		return manualServiceInstall + " --system", manualServiceRestart + " --system"
	}
	return manualServiceInstall, manualServiceRestart
}

func escapeXML(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(value)
}
