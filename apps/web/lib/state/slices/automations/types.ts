import type { Automation, AutomationRun } from "@/lib/types/automation";

export type AutomationsState = {
  items: Automation[];
  loaded: boolean;
  loading: boolean;
};

export type AutomationRunsState = {
  byAutomationId: Record<string, AutomationRun[]>;
  loading: Record<string, boolean>;
  /**
   * Monotonic delete-generation counter per automation, shared by every hook
   * instance (an editor can unmount mid-delete and remount). List responses
   * that captured an older generation are stale and must be discarded.
   */
  mutationEpoch: Record<string, number>;
  /**
   * Generation of the delete mutation currently in flight per automation,
   * or `false` when none. A mutation's `endDelete` only clears its own
   * generation, so a stale callback from an unmounted instance cannot lift
   * a newer instance's serialization gate.
   */
  deleting: Record<string, number | false>;
};

export type AutomationsSliceState = {
  automations: AutomationsState;
  automationRuns: AutomationRunsState;
};

export type AutomationsSliceActions = {
  setAutomations: (items: Automation[]) => void;
  setAutomationsLoading: (loading: boolean) => void;
  addAutomation: (automation: Automation) => void;
  updateAutomation: (automation: Automation) => void;
  removeAutomation: (id: string) => void;
  setAutomationRuns: (automationId: string, runs: AutomationRun[]) => void;
  setAutomationRunsLoading: (automationId: string, loading: boolean) => void;
  removeAutomationRun: (automationId: string, runId: string) => void;
  clearAutomationRuns: (automationId: string) => void;
  restoreAutomationRun: (automationId: string, run: AutomationRun) => void;
  /**
   * Atomically claims the per-automation delete serialization slot: bumps
   * the mutation epoch, records the new generation as in-flight, and returns
   * that generation, or `null` when another delete is already in flight.
   */
  beginAutomationRunDelete: (automationId: string) => number | null;
  /** Releases the serialization slot if (and only if) it still holds the
   * given generation — a stale callback from an unmounted hook instance
   * cannot clear a newer mutation's slot. */
  endAutomationRunDelete: (automationId: string, generation: number) => void;
  /**
   * Advances the mutation epoch without claiming the serialization slot. A
   * delete calls this when it settles so list responses that were captured
   * while it was in flight go stale — they may hold pre-delete rows.
   */
  advanceAutomationRunEpoch: (automationId: string) => void;
};

export type AutomationsSlice = AutomationsSliceState & AutomationsSliceActions;
