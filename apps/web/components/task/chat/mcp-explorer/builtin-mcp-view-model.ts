import type { BuiltinMCPPackage, BuiltinMCPSessionView } from "@/lib/types/builtin-mcp";
import { validBuiltinAcceptance } from "@/lib/state/slices/session-runtime/builtin-mcp-validation";
export function builtinPackageState(
  view: BuiltinMCPSessionView | undefined,
  id: BuiltinMCPPackage,
) {
  const desired = view?.desired?.packages.includes(id);
  const accepted = view?.bound;
  const owned =
    view &&
    accepted &&
    view.execution_id &&
    accepted.execution_id === view.execution_id &&
    accepted.manifest.workspace_id === view.workspace_id &&
    validBuiltinAcceptance(accepted, view);
  const bound = owned ? accepted.manifest.packages.includes(id) : undefined;
  return {
    desired,
    bound,
    pending: desired !== undefined && bound !== undefined && desired !== bound,
  };
}
export function builtinSelectionKey(value: boolean | undefined) {
  if (value === undefined) return "settings:builtinMcp.unknown";
  return value ? "settings:builtinMcp.selected" : "settings:builtinMcp.notSelected";
}

export function isReservedBuiltinName(value: string): boolean {
  const name = value.trim().toLowerCase();
  return (
    name.startsWith("kandev.") ||
    [
      "kandev",
      "kandev-configuration",
      "kandev-rich-output",
      "kandev-walkthroughs",
      "kandev-diagnostics",
    ].includes(name)
  );
}
