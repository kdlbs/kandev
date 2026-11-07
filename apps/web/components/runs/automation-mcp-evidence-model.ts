import type { Automation, AutomationRun } from "@/lib/types/automation";
import type { BuiltinMCPSessionView, BuiltinMCPManifest } from "@/lib/types/builtin-mcp";
function sameValues(left: readonly string[], right: readonly string[]) {
  return JSON.stringify([...left].sort()) === JSON.stringify([...right].sort());
}
/**
 * Accept runtime evidence only when its workspace, task, and session match the displayed automation
 * run.
 */
function ownedView(automation: Automation, run: AutomationRun, view?: BuiltinMCPSessionView) {
  if (
    !view ||
    view.task_id !== run.task_id ||
    view.session_id !== run.session_id ||
    view.workspace_id !== automation.workspace_id
  )
    return undefined;
  return view;
}
/**
 * Expose bound packages only after the matching run's process binding is accepted.
 */
function acceptedManifest(run: AutomationRun, view?: BuiltinMCPSessionView) {
  if (view?.binding_state !== "accepted" || !view.bound) return undefined;
  if (view.bound.manifest.task_id !== run.task_id || view.bound.session_id !== run.session_id)
    return undefined;
  return view.bound.manifest;
}
/**
 * Compare desired packages and definition references with the accepted process manifest.
 */
function selectionPending(
  automation: Automation,
  bound: BuiltinMCPManifest | undefined,
  desired: readonly string[] | undefined,
) {
  if (!bound || !desired) return false;
  if (!sameValues(bound.packages, desired)) return true;
  return Boolean(
    bound.automation_id &&
    !sameValues(
      bound.automation_definition_ids ?? [],
      automation.mcp_selection?.definition_ids ?? [],
    ),
  );
}
/**
 * Keep desired choices, accepted process packages, and observed tools separate for the matching run.
 */
export function automationMCPEvidence(
  automation: Automation,
  run: AutomationRun,
  view?: BuiltinMCPSessionView,
) {
  const owned = ownedView(automation, run, view);
  const bound = acceptedManifest(run, owned);
  const desired =
    owned?.desired?.packages ??
    (automation.task_mode !== "normal_task"
      ? automation.mcp_selection?.builtin_packages
      : undefined);
  const observed =
    owned?.history?.current.servers
      ?.filter((server) => Boolean(server.tools_listed_at))
      .map((server) => server.name) ?? [];
  return {
    desired,
    bound: bound?.packages,
    observed,
    pending: selectionPending(automation, bound, desired),
  };
}
