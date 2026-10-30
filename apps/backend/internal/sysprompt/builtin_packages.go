package sysprompt

import (
	"slices"
	"strings"

	p "github.com/kandev/kandev/internal/mcp/profile"
)

const officeRichOutputSection = `- show_rich_output_kandev: For structured comparisons, recorded checks, charts/graphs/plots, file previews or KPI/metrics with data, call now. Compose version=2,catalog_version=1,spec.root/spec.elements from the discovered catalog (Stack,Group,Text,Chart,Table,StatusList,Callout,Metrics,FilePreview). Inline and workspace CSV charts and tables are supported. Get exact properties and examples through tool discovery. No ASCII/SVG/HTML/separate app. Otherwise prose or small Markdown tables. Kandev owns axes, legends, tooltips, layout, and styling. Label series with units. Use Table for structured comparisons, StatusList for recorded checks, and Callout for key findings.`
const richOutputInstruction = "For charts, previews, or metrics with data, call show_rich_output_kandev with its discovered schema."
const walkthroughSection = "- show_walkthrough_kandev, get_walkthrough_kandev, delete_walkthrough_kandev."

func resolveBuiltinPrompt(name string, values map[string]string, manifest *p.BuiltinManifest) string {
	values["rich_output_instruction"] = richOutputInstruction
	values["rich_output_section"] = richOutputSection
	values["office_rich_output_section"] = officeRichOutputSection
	values["walkthrough_section"] = walkthroughSection
	values["builtin_package_section"] = ""
	values["builtin_tools_heading"] = "Selected tools from \"kandev\"."
	values["builtin_discovery_instruction"] = "Use callable tools from \"kandev\" with their current schemas; otherwise use native tool search or discovery. Search \"kandev\" plus the operation/canonical name; without search, inspect available MCP tools."
	values["office_discovery_instruction"] = "This Office session has exactly the tools listed below from the \"kandev\" server.\nAlways use the exact tool names shown below (they include the _kandev suffix)."
	if manifest != nil {
		values["builtin_tools_heading"] = "Selected tools from the built-in servers listed below."
		values["builtin_discovery_instruction"] = "Use callable tools from the selected built-in servers with their current schemas. Otherwise use native tool search or discovery with the selected server name and canonical tool name. Without search, inspect available MCP tools."
		values["office_discovery_instruction"] = "This Office session uses the selected built-in servers listed below. Tool names below are canonical MCP names. Discover and use the client-specific callable names and schemas within those servers."
		values["builtin_package_section"] = builtinPackageGuidance(manifest)
		if manifest.Validate() != nil || !slices.Contains(manifest.Packages, p.PackageRichOutput) {
			values["rich_output_instruction"] = "Use normal text or Markdown for structured output. Rich Output is unavailable in this execution."
			values["rich_output_section"], values["office_rich_output_section"] = "", ""
		}
		if manifest.Validate() != nil || !slices.Contains(manifest.Packages, p.PackageWalkthroughs) {
			values["walkthrough_section"] = ""
		}
	}
	return Resolve(name, values)
}

func builtinPackageGuidance(manifest *p.BuiltinManifest) string {
	if manifest.Validate() != nil {
		return "Built-in MCP binding is unavailable. Do not infer tool availability from previous turns."
	}
	names := make([]string, 0, len(manifest.Packages))
	for _, descriptor := range p.BuiltinPackageCatalog() {
		if slices.Contains(manifest.Packages, descriptor.ID) {
			names = append(names, descriptor.RuntimeName)
		}
	}
	text := "BUILT-IN PACKAGES: " + strings.Join(names, ", ") + ". This execution binding supersedes earlier optional-tool guidance. Discover tools only within these selected built-in servers. Do not rediscover an excluded package. Canonical tool names keep their _kandev suffix."
	if slices.Contains(manifest.Packages, p.PackageConfiguration) {
		text += " Configuration tools can change authorized shared resources, including workflows, agents, profiles, MCP settings, prompts, executors, and automations. Tool availability does not authorize a change. A regular task keeps its task identity and cannot change built-in package policy or grant packages."
	}
	return text
}

// AppendBuiltinBindingGuidance states the new process binding after retained
// conversation content. Empty prompts must remain empty for deferred starts.
func AppendBuiltinBindingGuidance(prompt string, manifest *p.BuiltinManifest) string {
	if manifest == nil || strings.TrimSpace(prompt) == "" {
		return prompt
	}
	return prompt + "\n\n<kandev-system>" + builtinPackageGuidance(manifest) + "</kandev-system>"
}

// FormatOfficeContextWithPackages retains Office lifecycle rules while reflecting
// the manifest prepared for this execution.
func FormatOfficeContextWithPackages(taskID, sessionID string, requiresSignal bool, manifest *p.BuiltinManifest) string {
	instruction := ""
	if requiresSignal {
		instruction = officeStepCompleteInstruction
	}
	return resolveBuiltinPrompt("office-context", map[string]string{"task_id": taskID, "session_id": sessionID, "step_complete_instruction": instruction}, manifest)
}

func InjectOfficeContextWithPackages(taskID, sessionID, prompt string, requiresSignal bool, manifest *p.BuiltinManifest, trustedContents ...string) string {
	return canonicalizeKandevContext(FormatOfficeContextWithPackages(taskID, sessionID, requiresSignal, manifest), prompt, trustedContextContents(sessionID, trustedContents...))
}
