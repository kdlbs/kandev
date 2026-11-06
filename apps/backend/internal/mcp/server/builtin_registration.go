package mcp

import (
	"slices"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
)

func (s *Server) registerContextTools() {
	switch s.profile.Surface {
	case mcpprofile.SurfaceManagedConversation:
		return
	case mcpprofile.SurfaceAutomation:
		s.registerAutomationTools()
		s.registerCoreQuestionTools()
	case mcpprofile.SurfaceCoordinator:
		s.registerCoordinatorTools()
		s.registerCoreQuestionTools()
	case mcpprofile.SurfaceExternal:
		s.registerCoreTools()
		s.registerConfigurationTools()
	default:
		s.registerBuiltinTools()
	}
}

// Package selection controls assembly for both isolated and legacy transports.
// Retained legacy callers use the catalog defaults and the same package builders.
func (s *Server) registerBuiltinTools() {
	manifest := s.profile.BuiltinPackages
	if manifest != nil {
		if manifest.Validate() != nil || manifest.Surface != s.profile.Surface || manifest.TaskID != s.taskID {
			return
		}
		for _, id := range manifest.Packages {
			s.registerBuiltinPackage(id)
		}
		return
	}
	for _, descriptor := range mcpprofile.BuiltinPackageCatalog() {
		if slices.Contains(descriptor.Surfaces, s.profile.Surface) &&
			(descriptor.DefaultPolicy == "default" || slices.Contains(descriptor.RequiredSurfaces, s.profile.Surface)) {
			s.registerBuiltinPackage(descriptor.ID)
		}
	}
}

func (s *Server) registerBuiltinPackage(id mcpprofile.PackageID) {
	switch id {
	case mcpprofile.PackageCore:
		s.registerCoreTools()
	case mcpprofile.PackageConfiguration:
		s.registerConfigurationTools()
	case mcpprofile.PackageRichOutput:
		s.registerRichOutputTool()
	case mcpprofile.PackageWalkthroughs:
		s.registerWalkthroughTools()
	case mcpprofile.PackageDiagnostics:
		s.registerDiagnosticBundleTool()
	}
}

func (s *Server) registerConfigurationTools() {
	s.registerConfigWorkflowTools()
	s.registerConfigAgentTools()
	s.registerConfigMcpTools()
	s.registerConfigPromptTools()
	s.registerConfigExecutorTools()
	s.registerConfigSettingsTools()
	// The external API retains its fixed catalog without automation creation.
	if s.profile.Surface != mcpprofile.SurfaceExternal {
		s.registerConfigAutomationTools()
	}
}
