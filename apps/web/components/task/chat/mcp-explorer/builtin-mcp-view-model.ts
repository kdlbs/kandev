import type { BuiltinMCPPackage, BuiltinMCPSessionView } from "@/lib/types/builtin-mcp";
import { validBuiltinAcceptance } from "@/lib/state/slices/session-runtime/builtin-mcp-validation";

type BuiltinPackageChange = {
  id: BuiltinMCPPackage;
  action: "selected" | "add" | "remove";
};
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

export function builtinPendingPackages(view: BuiltinMCPSessionView | undefined) {
  if (
    !view?.desired ||
    view.desired.workspace_id !== view.workspace_id ||
    view.desired.task_id !== view.task_id
  )
    return [];
  const ids: BuiltinMCPPackage[] = [
    "core",
    "configuration",
    "rich-output",
    "walkthroughs",
    "diagnostics",
  ];
  return ids.flatMap<BuiltinPackageChange>((id) => {
    const state = builtinPackageState(view, id);
    if (state.bound === undefined)
      return state.desired ? [{ id, action: "selected" as const }] : [];
    return state.pending
      ? [{ id, action: state.desired ? ("add" as const) : ("remove" as const) }]
      : [];
  });
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
