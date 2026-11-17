import type { AgentUpdateJob, AgentUpdateStatus, InstallJob } from "@/lib/api";

export type RuntimeUpdateGroup =
  | "available"
  | "up_to_date"
  | "unknown"
  | "external"
  | "unavailable";

export const RUNTIME_UPDATE_GROUPS: readonly RuntimeUpdateGroup[] = [
  "available",
  "up_to_date",
  "unknown",
  "external",
  "unavailable",
];

const ACTIVE_UPDATE_STATUSES = new Set<AgentUpdateJob["status"]>([
  "queued",
  "resolving",
  "updating",
  "probing",
  "saving",
  "refreshing",
]);

export type RuntimeUpdateManagerOptions = {
  hasRuntimeControl: (agentName: string) => boolean;
  installJobs?: Record<string, InstallJob>;
  updateJobs?: Record<string, AgentUpdateJob>;
  /** Policy controller keys whose draft differs from the saved baseline. */
  dirtyPolicyKeys?: ReadonlySet<string>;
};

export type RuntimeUpdateManagerRow = {
  key: string;
  status: AgentUpdateStatus;
  group: RuntimeUpdateGroup;
  individualActionable: boolean;
  bulkEligible: boolean;
  busy: boolean;
  dirtyPolicy: boolean;
};

export type RuntimeUpdateManagerModel = {
  rows: RuntimeUpdateManagerRow[];
  groups: Record<RuntimeUpdateGroup, RuntimeUpdateManagerRow[]>;
  availableCount: number;
  bulkCandidates: RuntimeUpdateManagerRow[];
  individualOnlyCount: number;
};

export function runtimePolicyKey(status: AgentUpdateStatus): string {
  return `${status.agent_name}:${status.runtime_id}`;
}

function maintenanceBusy(status: AgentUpdateStatus, options: RuntimeUpdateManagerOptions): boolean {
  const install = options.installJobs?.[status.agent_name];
  if (install && (install.status === "queued" || install.status === "running")) return true;
  const update = options.updateJobs?.[status.agent_name];
  return Boolean(update && ACTIVE_UPDATE_STATUSES.has(update.status));
}

function isManagedPinnedRuntime(status: AgentUpdateStatus): boolean {
  return (
    status.owner === "kandev" &&
    status.management === "managed" &&
    status.mechanism === "npm_candidate" &&
    status.update_mode === "pinned" &&
    !status.managed_fallback
  );
}

export function runtimeUpdateGroup(status: AgentUpdateStatus): RuntimeUpdateGroup {
  if (!status.available || !status.enabled || status.management === "unsupported") {
    return "unavailable";
  }
  if (status.management === "manual" && !status.managed_fallback) {
    if (status.update_mode !== "self_update") return "external";
    if (status.check_state === "update_available") return "available";
    if (status.check_state === "unknown") return "unknown";
    return "up_to_date";
  }
  if (status.managed_fallback) return "external";
  if (status.check_state === "unknown") return "unknown";
  if (status.check_state === "update_available") return "available";
  return "up_to_date";
}

function individualActionable(
  status: AgentUpdateStatus,
  options: RuntimeUpdateManagerOptions,
): boolean {
  if (!status.available || !status.enabled) return false;
  if (!options.hasRuntimeControl(status.agent_name)) return false;
  if (status.update_mode === "self_update") return true;
  return status.management === "managed" || Boolean(status.managed_fallback);
}

export function isBulkCandidateStatus(status: AgentUpdateStatus): boolean {
  return (
    isManagedPinnedRuntime(status) &&
    status.available &&
    status.enabled &&
    status.check_state === "update_available" &&
    status.runtime_source !== "native" &&
    Boolean(status.effective_version) &&
    Boolean(status.latest_version)
  );
}

function bulkEligible(
  status: AgentUpdateStatus,
  options: RuntimeUpdateManagerOptions,
  busy: boolean,
  dirtyPolicy: boolean,
): boolean {
  return (
    isBulkCandidateStatus(status) &&
    !busy &&
    !dirtyPolicy &&
    options.hasRuntimeControl(status.agent_name)
  );
}

export function buildRuntimeUpdateManagerModel(
  statuses: AgentUpdateStatus[],
  options: RuntimeUpdateManagerOptions,
): RuntimeUpdateManagerModel {
  const rows = statuses.map((status) => {
    const key = runtimePolicyKey(status);
    const busy = maintenanceBusy(status, options);
    const dirtyPolicy = options.dirtyPolicyKeys?.has(key) ?? false;
    const group = runtimeUpdateGroup(status);
    return {
      key,
      status,
      group,
      busy,
      dirtyPolicy,
      individualActionable: individualActionable(status, options),
      bulkEligible: bulkEligible(status, options, busy, dirtyPolicy),
    } satisfies RuntimeUpdateManagerRow;
  });
  const groups = Object.fromEntries(
    RUNTIME_UPDATE_GROUPS.map((group) => [group, [] as RuntimeUpdateManagerRow[]]),
  ) as Record<RuntimeUpdateGroup, RuntimeUpdateManagerRow[]>;
  for (const row of rows) groups[row.group].push(row);
  const availableCount = rows.filter(
    (row) =>
      row.group === "available" &&
      row.individualActionable &&
      !row.status.managed_fallback &&
      !row.busy &&
      !row.dirtyPolicy,
  ).length;
  const bulkCandidates = rows.filter((row) => row.bulkEligible);
  return {
    rows,
    groups,
    availableCount,
    bulkCandidates,
    individualOnlyCount: Math.max(0, availableCount - bulkCandidates.length),
  };
}

export function defaultRuntimeSelection(model: RuntimeUpdateManagerModel): Set<string> {
  return new Set(model.bulkCandidates.map((row) => row.key));
}

/**
 * Keeps an operator's explicit selection while dropping rows that are no longer
 * eligible. Newly discovered candidates stay unselected until explicitly added.
 */
export function reconcileRuntimeSelection(
  selection: ReadonlySet<string>,
  model: RuntimeUpdateManagerModel,
): Set<string> {
  const eligible = new Set(model.bulkCandidates.map((row) => row.key));
  return new Set([...selection].filter((key) => eligible.has(key)));
}

export function runtimeBatchActionLabel(
  selection: ReadonlySet<string>,
  model: RuntimeUpdateManagerModel,
  t: (key: string, values?: Record<string, unknown>) => string,
): string {
  if (selection.size >= model.bulkCandidates.length) {
    return t("agents:runtimeUpdateAll", { count: model.bulkCandidates.length });
  }
  return t("agents:runtimeUpdateSelected", { count: selection.size });
}
