package agents

import "strings"

// RuntimeReleaseSource contains trusted read-only release metadata. It never
// authorizes an installer for an externally owned executable.
type RuntimeReleaseSource struct {
	NPM              string
	GitHubRepository string
	GuidanceURL      string
}

type RuntimeReleaseAgent interface{ RuntimeReleaseSource() RuntimeReleaseSource }

// RuntimeUpdateCapability describes the runtime that structured execution uses.
// Separately installed authentication and passthrough CLIs retain their owners.
type RuntimeUpdateCapability struct {
	RuntimeID       string
	Owner           string
	Mechanism       string
	Management      string
	Source          RuntimeReleaseSource
	Managed         *ManagedNPMRuntimeSpec
	ManagedFallback *ManagedNPMRuntimeSpec
}

const RuntimeManagementUnsupported = "unsupported"

func RuntimeUpdateCapabilities(ag Agent) RuntimeUpdateCapability {
	cap := RuntimeUpdateCapability{RuntimeID: ag.ID(), Owner: "external", Mechanism: "native", Management: "manual"}
	if IsVirtualAgent(ag) {
		cap.Owner, cap.Mechanism, cap.Management = "none", "virtual", RuntimeManagementUnsupported
		return cap
	}
	switch ag.(type) {
	case *CustomACPAgent, *TUIAgent:
		cap.Mechanism, cap.Management = "custom", RuntimeManagementUnsupported
		return cap
	case *MockAgent:
		cap.Owner, cap.Mechanism, cap.Management = "kandev", "fixture", RuntimeManagementUnsupported
		return cap
	}
	if managed, ok := ag.(ManagedNPMRuntimeAgent); ok {
		spec := managed.ManagedNPMRuntime()
		if spec.Package != "" && spec.DefaultVersionOrPinned() != "" {
			return managedRuntimeUpdateCapability(ag, spec)
		}
	}
	if source, ok := ag.(RuntimeReleaseAgent); ok {
		cap.Source = source.RuntimeReleaseSource()
		return cap
	}
	argv := ag.BuildCommand(CommandOptions{}).Args()
	if len(argv) >= 3 && argv[0] == "npx" && (argv[1] == "-y" || argv[1] == "--yes") && !strings.HasPrefix(argv[2], "-") {
		cap.Source = RuntimeReleaseSource{NPM: argv[2], GuidanceURL: npmGuidance(argv[2])}
		cap.RuntimeID, cap.Mechanism = "npm:"+argv[2], "unmanaged_npm"
	}
	return cap
}

func npmGuidance(packageName string) string { return "https://www.npmjs.com/package/" + packageName }

func managedRuntimeUpdateCapability(ag Agent, spec ManagedNPMRuntimeSpec) RuntimeUpdateCapability {
	cap := RuntimeUpdateCapability{RuntimeID: "npm:" + spec.Package, Owner: "kandev", Mechanism: "npm_candidate", Management: "managed", Source: RuntimeReleaseSource{NPM: spec.Package, GuidanceURL: npmGuidance(spec.Package)}, Managed: &spec}
	if !spec.NativeBinaryOnPath() {
		return cap
	}
	cap.RuntimeID, cap.Owner, cap.Mechanism, cap.Management = "native:"+spec.NativeBinary, "external", "native", "manual"
	cap.Managed = nil
	cap.ManagedFallback = &spec
	if source, ok := ag.(RuntimeReleaseAgent); ok {
		cap.Source = source.RuntimeReleaseSource()
	}
	return cap
}
