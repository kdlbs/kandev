import type {
  RunState,
  WorkloadActionKind,
  WorkloadCapabilities,
  WorkloadRunObservation,
} from "@/lib/types/background-work";

/**
 * Whether a workload in this state may accept a control right now. Unknown,
 * disconnected, and terminal states are never actionable, even if an earlier
 * declaration reported the capability as available.
 */
export function isWorkloadActionableState(state: RunState): boolean {
  return state === "running" || state === "waiting";
}

/** Whether the provider actually supplied a capability declaration. */
export function hasCapabilityDeclaration(
  capabilities: WorkloadCapabilities | undefined | null,
): boolean {
  return !!capabilities?.discovery;
}

/** The contract's zero value: no provider declaration. */
export function emptyWorkloadCapabilities(): WorkloadCapabilities {
  return {
    discovery: "",
    output: "",
    transcript: false,
    parentage: false,
    reasoning_summary: false,
    attributable_usage: false,
  };
}

/**
 * Keep static support but clear dynamic availability for a state that cannot
 * accept a control. This prevents stale `available: true` values from leaving
 * enabled Stop or input controls on a terminal or unknown job.
 */
export function applyWorkloadActionAvailability(
  declared: WorkloadCapabilities | undefined,
  state: RunState,
): WorkloadCapabilities {
  const capabilities = declared ?? emptyWorkloadCapabilities();
  if (!capabilities.actions || isWorkloadActionableState(state)) return capabilities;
  const actions: WorkloadCapabilities["actions"] = {};
  for (const [kind, capability] of Object.entries(capabilities.actions)) {
    actions[kind as WorkloadActionKind] = capability
      ? { ...capability, available: false }
      : undefined;
  }
  return { ...capabilities, actions };
}

/**
 * Whether the shared viewer may render an action control. A missing capability
 * map never implies an action, and a non-actionable state always disables one.
 */
export function canRunWorkloadAction(
  workload: WorkloadRunObservation,
  action: WorkloadActionKind,
): boolean {
  if (!isWorkloadActionableState(workload.state)) return false;
  const capability = workload.capabilities?.actions?.[action];
  return !!capability?.supported && !!capability?.available;
}

/** Whether the provider advertises any readable output for this workload. */
export function hasReadableOutput(capabilities: WorkloadCapabilities | undefined): boolean {
  return capabilities?.output === "stream" || capabilities?.output === "snapshot";
}
