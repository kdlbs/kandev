import type { MCPAttachmentServer } from "@/lib/state/slices/session-runtime/types";
import { isKandevMcpServer } from "./mcp-explorer-view-model";
import type {
  BuiltinMCPPackage,
  BuiltinMCPProviderBinding,
  BuiltinMCPSessionView,
} from "@/lib/types/builtin-mcp";
import { isBuiltinMCPManifest } from "@/lib/state/slices/session-runtime/builtin-mcp-validation";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";

function completeCatalog(binding: BuiltinMCPProviderBinding) {
  const names = binding.manifest.packages.map((id) => (id === "core" ? "kandev" : `kandev-${id}`));
  const selected = names.every((name) => {
    const tools = binding.catalog[name];
    return (
      Array.isArray(tools) &&
      tools.length > 0 &&
      tools.every((tool) => typeof tool === "string" && tool.length > 0) &&
      new Set(tools).size === tools.length
    );
  });
  const known = [
    "kandev",
    "kandev-configuration",
    "kandev-rich-output",
    "kandev-walkthroughs",
    "kandev-diagnostics",
  ];
  return (
    selected &&
    Object.keys(binding.catalog).every(
      (name) => (!known.includes(name) && !name.startsWith("kandev.")) || names.includes(name),
    )
  );
}

export function currentProviderBinding(view: BuiltinMCPSessionView | undefined) {
  const binding = view?.provider_binding;
  if (!view || !binding || !isBuiltinMCPManifest(binding.manifest)) return undefined;
  if (!validProviderOwner(binding, view) || !validProviderScope(binding.scope)) return undefined;
  if (
    typeof binding.accepted_at !== "string" ||
    parseStrictRfc3339Timestamp(binding.accepted_at) === null
  )
    return undefined;
  if (!binding.catalog || Array.isArray(binding.catalog) || !completeCatalog(binding))
    return undefined;
  return binding;
}

function validProviderOwner(binding: BuiltinMCPProviderBinding, view: BuiltinMCPSessionView) {
  return (
    binding.session_id === view.session_id &&
    binding.manifest.task_id === view.task_id &&
    binding.manifest.workspace_id === view.workspace_id &&
    binding.scope?.execution_id === view.execution_id
  );
}
function validProviderScope(scope: BuiltinMCPProviderBinding["scope"]) {
  if (!scope || typeof scope.binding_id !== "string" || typeof scope.helper_id !== "string")
    return false;
  if (!scope.binding_id || !scope.helper_id || !/^[0-9a-f]{64}$/.test(scope.profile_digest))
    return false;
  return (
    Number.isSafeInteger(scope.process_generation) &&
    scope.process_generation > 0 &&
    Number.isSafeInteger(scope.harness_generation) &&
    scope.harness_generation > 0
  );
}

export function providerSelectionApplied(view: BuiltinMCPSessionView | undefined) {
  const binding = currentProviderBinding(view);
  const desired = view?.desired;
  if (!binding || !isBuiltinMCPManifest(desired) || view?.provider_available === false)
    return false;
  return (
    desired.packages.length === binding.manifest.packages.length &&
    desired.packages.every((id) => binding.manifest.packages.includes(id))
  );
}

// The historical attachment belongs to the initial process. A verified current
// binding supplies the catalog after provider replacement.
export function providerAwareServers(
  servers: MCPAttachmentServer[],
  view: BuiltinMCPSessionView | undefined,
): MCPAttachmentServer[] {
  if (view?.provider_available === false) {
    return servers.map((server) =>
      isKandevMcpServer(server)
        ? {
            ...server,
            status: "unavailable",
            tools: undefined,
            tool_count: undefined,
            tools_listed_at: undefined,
            tool_catalog_truncated: false,
          }
        : server,
    );
  }
  const binding = currentProviderBinding(view);
  if (!binding) return servers;
  const external = servers.filter((server) => !isKandevMcpServer(server));
  const current = binding.manifest.packages.map((id: BuiltinMCPPackage): MCPAttachmentServer => {
    const name = id === "core" ? "kandev" : `kandev-${id}`;
    const tools = binding.catalog[name].map((name) => ({ name }));
    return {
      name,
      source: "kandev",
      builtin_package: id,
      status: "active",
      accepted_at: binding.accepted_at,
      tools_listed_at: binding.accepted_at,
      tools,
      tool_count: tools.length,
    };
  });
  return [...external, ...current];
}
