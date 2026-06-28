"use client";

import { useEffect } from "react";
import type { AgentProfileOption } from "@/lib/state/slices";
import { getLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "@/lib/settings/constants";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import type { DialogFormState, StoreSelections } from "@/components/task-create-dialog-types";

/**
 * Agent-profile autopick: the logic that decides which profile to pre-select
 * when the task-create dialog opens (or when its inputs change). Lives in its
 * own file so `task-create-dialog-effects.ts` stays under the 600-line lint
 * cap. Tests in `task-create-dialog-effects.test.ts` exercise both effects
 * through the same import surface re-exported from there.
 */

const autopickDebug = createDebugLogger("executor-compat:autopick");
const workflowAutopickDebug = createDebugLogger("executor-compat:workflow-autopick");

/**
 * Pure decision function for the agent-profile autopick. Extracted so the
 * effect can stay below the 100-line lint cap once the autopick trace logs
 * are inlined, and so the same decision can be tested without rendering.
 *
 * Order: lastId (localStorage) → defId (workspace default) → first
 * compatible. Every candidate is filtered against `compatibleAgentProfiles`
 * so a previously-used profile that's not wired for the chosen executor
 * isn't restored, then immediately fails the executor-compat gate.
 */
export type AutopickDecision =
  | { kind: "skip"; reason: string }
  | { kind: "defer"; reason: string }
  | { kind: "pick"; source: "lastId" | "defId" | "first"; id: string };

type AgentProfileAutopickInput = {
  open: boolean;
  agentProfileId: string;
  workflowAgentProfileId: string;
  workflowHasAgent: boolean;
  agentProfiles: AgentProfileOption[];
  compatibleAgentProfiles: AgentProfileOption[];
  authLoaded: boolean;
  executorProfileId: string;
  hasExecutors: boolean;
  lastAgentProfileId: string | null;
  userSettingsLoaded?: boolean;
  defaultAgentProfileId: string | null;
};

function getAgentAutopickGate(input: AgentProfileAutopickInput): AutopickDecision | null {
  if (!input.open) return { kind: "skip", reason: "closed" };
  if (input.agentProfileId) return { kind: "skip", reason: "already-set" };
  if (input.workflowAgentProfileId) return { kind: "skip", reason: "workflow-locked" };
  if (input.workflowHasAgent) return { kind: "skip", reason: "workflow-has-agent" };
  if (input.agentProfiles.length === 0) return { kind: "skip", reason: "no-profiles" };
  if (!input.authLoaded) return { kind: "defer", reason: "auth-not-loaded" };
  // Defer until the executor profile is selected too - useExecutorProfileCompat
  // short-circuits to the unfiltered list when `selectedExecutorProfile` is null,
  // so without this gate we'd happily restore an incompatible lastId during the
  // single render where `authLoaded` is already true but the executor
  // auto-select hasn't queued through yet. Only deferrable if there ARE
  // executors to pick from - otherwise the executor effect will never fire and
  // we'd defer forever.
  if (!input.executorProfileId && input.hasExecutors) {
    return { kind: "defer", reason: "executor-not-selected" };
  }
  if (input.compatibleAgentProfiles.length === 0) return { kind: "skip", reason: "no-compatible" };
  if (!input.lastAgentProfileId && input.userSettingsLoaded === false) {
    return { kind: "defer", reason: "user-settings-not-loaded" };
  }
  return null;
}

export function decideAgentProfileAutopick(input: AgentProfileAutopickInput): AutopickDecision {
  const gate = getAgentAutopickGate(input);
  if (gate) return gate;
  const lastId = input.lastAgentProfileId;
  if (lastId && input.compatibleAgentProfiles.some((p) => p.id === lastId)) {
    return { kind: "pick", source: "lastId", id: lastId };
  }
  const defId = input.defaultAgentProfileId;
  if (defId && input.compatibleAgentProfiles.some((p) => p.id === defId)) {
    return { kind: "pick", source: "defId", id: defId };
  }
  return { kind: "pick", source: "first", id: input.compatibleAgentProfiles[0].id };
}

function buildAgentAutopickDebugFields(input: {
  decision: AutopickDecision;
  agentProfileId: string;
  selectedWorkflowId: string | null;
  executorProfileId: string;
  agentProfiles: AgentProfileOption[];
  compatibleAgentProfiles: AgentProfileOption[];
  authLoaded: boolean;
  lastAgentProfileId: string | null;
  userSettingsLoaded?: boolean;
  defaultAgentProfileId: string | null;
}) {
  const lastId = input.lastAgentProfileId;
  const defId = input.defaultAgentProfileId;
  return {
    reason: input.decision.kind === "pick" ? input.decision.source : input.decision.reason,
    pick: input.decision.kind === "pick" ? input.decision.id : "-",
    current: input.agentProfileId || "-",
    workflow_id: input.selectedWorkflowId ?? "-",
    executor_profile_id: input.executorProfileId || "-",
    local_storage_id: lastId ?? "-",
    local_storage_valid: Boolean(
      lastId && input.compatibleAgentProfiles.some((p) => p.id === lastId),
    ),
    workspace_default_id: defId ?? "-",
    workspace_default_valid: Boolean(
      defId && input.compatibleAgentProfiles.some((p) => p.id === defId),
    ),
    agent_count: input.agentProfiles.length,
    compat_count: input.compatibleAgentProfiles.length,
    auth_loaded: input.authLoaded,
    user_settings_loaded: input.userSettingsLoaded ?? true,
  };
}

function resolveLastUsedAgentProfileId(
  compatibleAgentProfiles: AgentProfileOption[],
  settingsAgentProfileId?: string | null,
) {
  const localId = getLocalStorage<string | null>(STORAGE_KEYS.LAST_AGENT_PROFILE_ID, null);
  if (localId && compatibleAgentProfiles.some((p) => p.id === localId)) return localId;
  if (
    settingsAgentProfileId &&
    compatibleAgentProfiles.some((p) => p.id === settingsAgentProfileId)
  ) {
    return settingsAgentProfileId;
  }
  return null;
}

export function useWorkflowAgentProfileEffect(
  fs: DialogFormState,
  workflows: Array<{ id: string; agent_profile_id?: string }>,
  agentProfiles: AgentProfileOption[],
  compatibleAgentProfiles: AgentProfileOption[],
  lastUsedAgentProfileId?: string | null,
) {
  const { selectedWorkflowId, setAgentProfileId, setWorkflowAgentProfileId } = fs;
  useEffect(() => {
    if (!selectedWorkflowId) {
      setWorkflowAgentProfileId("");
      workflowAutopickDebug("no-workflow", { cleared: "workflowAgentProfileId" });
      return;
    }
    const workflow = workflows.find((w) => w.id === selectedWorkflowId);
    if (workflow?.agent_profile_id) {
      // Always lock the selector when the workflow specifies an agent profile.
      // This prevents the race condition where agentProfiles hasn't loaded yet.
      setWorkflowAgentProfileId(workflow.agent_profile_id);
      // Only set the agentProfileId once the profile is confirmed available.
      const profileExists = agentProfiles.some((p) => p.id === workflow.agent_profile_id);
      if (profileExists) {
        setAgentProfileId(workflow.agent_profile_id);
        workflowAutopickDebug("locked", {
          workflow: selectedWorkflowId,
          set_to: workflow.agent_profile_id,
        });
      } else {
        workflowAutopickDebug("locked-missing", {
          workflow: selectedWorkflowId,
          missing: workflow.agent_profile_id,
        });
      }
    } else {
      setWorkflowAgentProfileId("");
      // Restore the user's last-used agent profile when unlocking. Filter
      // against `compatibleAgentProfiles` (not the full `agentProfiles` list)
      // so an executor-incompatible id from a previous session - including
      // stale UUIDs from a wiped DB - is dropped rather than restored.
      // useDefaultSelectionsEffect would otherwise see agentProfileId become
      // truthy, early-exit on "already-set", and leave the dialog stuck on
      // "No compatible agent profiles".
      const lastId = resolveLastUsedAgentProfileId(compatibleAgentProfiles, lastUsedAgentProfileId);
      const isValidLastId = Boolean(lastId);
      const finalId = lastId ?? "";
      setAgentProfileId(finalId);
      workflowAutopickDebug("workflow-no-override", {
        last_id: lastId ?? "-",
        valid: isValidLastId,
        set_to: finalId || "-empty-",
      });
    }
  }, [
    selectedWorkflowId,
    workflows,
    agentProfiles,
    compatibleAgentProfiles,
    lastUsedAgentProfileId,
    setAgentProfileId,
    setWorkflowAgentProfileId,
  ]);
}

export function useAgentProfileAutopickEffect(
  fs: DialogFormState,
  open: boolean,
  sel: StoreSelections,
  workflows: Array<{ id: string; agent_profile_id?: string }>,
) {
  const { agentProfiles, compatibleAgentProfiles, authLoaded, executors, workspaceDefaults } = sel;
  const {
    agentProfileId,
    workflowAgentProfileId,
    selectedWorkflowId,
    executorProfileId,
    setAgentProfileId,
  } = fs;
  useEffect(() => {
    // Check synchronously whether the selected workflow has an agent override.
    // This avoids a race condition where workflowAgentProfileId state hasn't
    // been committed yet by the workflow effect running in the same cycle.
    const workflowHasAgent = selectedWorkflowId
      ? workflows.some((w) => w.id === selectedWorkflowId && w.agent_profile_id)
      : false;
    const lastAgentProfileId = resolveLastUsedAgentProfileId(
      compatibleAgentProfiles,
      sel.lastUsedAgentProfileId,
    );
    const defaultAgentProfileId = workspaceDefaults?.default_agent_profile_id ?? null;
    const decision = decideAgentProfileAutopick({
      open,
      agentProfileId,
      workflowAgentProfileId,
      workflowHasAgent,
      agentProfiles,
      compatibleAgentProfiles,
      authLoaded,
      executorProfileId,
      hasExecutors: executors.length > 0,
      lastAgentProfileId,
      userSettingsLoaded: sel.userSettingsLoaded,
      defaultAgentProfileId,
    });
    if (isDebug()) {
      autopickDebug(
        decision.kind,
        buildAgentAutopickDebugFields({
          decision,
          agentProfileId,
          selectedWorkflowId,
          executorProfileId,
          agentProfiles,
          compatibleAgentProfiles,
          authLoaded,
          lastAgentProfileId,
          userSettingsLoaded: sel.userSettingsLoaded,
          defaultAgentProfileId,
        }),
      );
    }
    if (decision.kind === "pick") {
      const id = decision.id;
      void Promise.resolve().then(() => setAgentProfileId(id));
    }
  }, [
    open,
    agentProfileId,
    workflowAgentProfileId,
    selectedWorkflowId,
    workflows,
    agentProfiles,
    compatibleAgentProfiles,
    authLoaded,
    executorProfileId,
    executors,
    workspaceDefaults,
    sel.lastUsedAgentProfileId,
    sel.userSettingsLoaded,
    setAgentProfileId,
  ]);
}
