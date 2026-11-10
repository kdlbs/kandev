import type {
  BuiltinMCPChoices,
  BuiltinMCPDescriptor,
  BuiltinMCPManagementView,
  BuiltinMCPPackage,
  BuiltinMCPPolicyChoice,
  BuiltinMCPPreference,
} from "@/lib/types/builtin-mcp";

type Scope = "tasks" | "workspaces";
type Choices = BuiltinMCPChoices<BuiltinMCPPolicyChoice | BuiltinMCPPreference>;

function baselineChoices(view: BuiltinMCPManagementView | null, scope: Scope) {
  if (!view) return {};
  return scope === "tasks" ? (view.preferences?.packages ?? {}) : view.policy.packages;
}

function optionalPackageEnabled(
  view: BuiltinMCPManagementView | null,
  item: BuiltinMCPDescriptor | undefined,
  scope: Scope,
  choice: Choices[BuiltinMCPPackage],
) {
  if (!view || !item || view.policy.packages[item.id] === "blocked") return false;
  if (view.surface && !item.surfaces.some((surface) => surface === view.surface)) return false;
  if (scope === "workspaces") return choice === "default";
  return choice === "on" || (choice === "inherit" && view.policy.packages[item.id] === "default");
}

function requiredPackage(
  view: BuiltinMCPManagementView | null,
  item: BuiltinMCPDescriptor | undefined,
  id: BuiltinMCPPackage,
) {
  if (id === "core") return true;
  if (!view?.surface || !item?.required_surfaces) return false;
  return item.required_surfaces.some((surface) => surface === view.surface);
}

export function builtinPackageChoice(
  view: BuiltinMCPManagementView | null,
  draft: Choices,
  scope: Scope,
  id: BuiltinMCPPackage,
) {
  const baseline = baselineChoices(view, scope);
  const choice = draft[id] ?? "inherit";
  const item = view?.catalog.find((entry) => entry.id === id);
  const required = requiredPackage(view, item, id);
  const enabled = required || optionalPackageEnabled(view, item, scope, choice);
  const taskLabel = enabled ? "on" : "off";
  const label = scope === "workspaces" ? choice : taskLabel;
  return {
    choice,
    dirty: !required && choice !== (baseline[id] ?? "inherit"),
    enabled,
    labelKey: required ? "settings:builtinMcp.required" : `settings:builtinMcp.choices.${label}`,
    inherited: !required && scope === "tasks" && choice === "inherit",
  };
}
