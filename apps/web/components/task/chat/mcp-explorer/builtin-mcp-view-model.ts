import type { BuiltinMCPPackage, BuiltinMCPSessionView } from "@/lib/types/builtin-mcp";
import { currentProviderBinding, providerSelectionApplied } from "./builtin-provider-binding";
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
  const accepted = ownedInitialBinding(view);
  const provider = currentProviderBinding(view);
  let bound: boolean | undefined;
  if (provider && view?.provider_available !== false)
    bound = provider.manifest.packages.includes(id);
  else if (accepted) bound = accepted.manifest.packages.includes(id);
  return {
    desired,
    bound,
    pending: desired !== undefined && bound !== undefined && desired !== bound,
  };
}
function ownedInitialBinding(view: BuiltinMCPSessionView | undefined) {
  const accepted = view?.bound;
  if (!view || !accepted || !view.execution_id || accepted.execution_id !== view.execution_id)
    return undefined;
  if (view.provider_available === false || view.binding_state !== "accepted") return undefined;
  if (
    accepted.manifest.workspace_id !== view.workspace_id ||
    !validBuiltinAcceptance(accepted, view)
  )
    return undefined;
  return accepted;
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

function pendingApplicationMessage(reason: string | undefined) {
  if (reason === "policy_changed") return "settings:builtinMcp.applicationPolicyChanged";
  if (reason === "access_changed") return "settings:builtinMcp.applicationAccessChanged";
  if (reason === "helper_unavailable") return "settings:builtinMcp.applicationHelperUnavailable";
  if (reason === "unsupported_resume" || reason === "helper_update_required")
    return "settings:builtinMcp.applicationUnsupported";
  return "settings:builtinMcp.applicationPending";
}

export function builtinApplicationStatus(view: BuiltinMCPSessionView | undefined) {
  const application = view?.application;
  if (
    !view ||
    !application ||
    application.session_id !== view.session_id ||
    application.task_id !== view.task_id ||
    application.workspace_id !== view.workspace_id
  ) {
    return {
      state: "unknown",
      retry: false,
      messageKey: "settings:builtinMcp.applicationSaved",
    } as const;
  }
  if (applicationFailed(application)) {
    return {
      state: "failed",
      retry: view.can_retry_application === true,
      messageKey:
        view.provider_available === true
          ? "settings:builtinMcp.applicationFailedPrevious"
          : "settings:builtinMcp.applicationFailedUnavailable",
    } as const;
  }
  if (savedSelectionAlreadyCurrent(view)) {
    return {
      state: "applied",
      retry: false,
      messageKey: "settings:builtinMcp.applicationApplied",
    } as const;
  }
  if (application.stage === "applied" && !providerSelectionApplied(view)) {
    return {
      state: "pending",
      retry: false,
      messageKey: "settings:builtinMcp.applicationSaved",
    } as const;
  }
  if (application.stage === "applied") {
    return {
      state: "applied",
      retry: false,
      messageKey: "settings:builtinMcp.applicationApplied",
    } as const;
  }
  if (["preparing", "retiring", "starting", "verifying", "accepting"].includes(application.stage)) {
    return {
      state: "applying",
      retry: false,
      messageKey: "settings:builtinMcp.applicationApplying",
    } as const;
  }
  return {
    state: "pending",
    retry: false,
    messageKey: pendingApplicationMessage(application.reason),
  } as const;
}

function applicationFailed(application: NonNullable<BuiltinMCPSessionView["application"]>) {
  return (
    application.stage === "failed" ||
    (application.stage === "accepting" && application.reason === "acceptance_unconfirmed")
  );
}

function savedSelectionAlreadyCurrent(view: BuiltinMCPSessionView) {
  if (
    view.application?.stage !== "superseded" ||
    view.application.reason !== "already_current" ||
    view.provider_available !== true
  )
    return false;
  if (providerSelectionApplied(view)) return true;
  const accepted = ownedInitialBinding(view)?.manifest.packages;
  const desired = view.desired?.packages;
  return (
    !!accepted &&
    !!desired &&
    accepted.length === desired.length &&
    desired.every((id) => accepted.includes(id))
  );
}
