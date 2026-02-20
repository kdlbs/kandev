import type { Task, Branch } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { StepType } from "@/components/task-create-dialog-types";
import { selectPreferredBranch } from "@/lib/utils";
import { getLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "@/lib/settings/constants";

export function autoSelectBranch(branchList: Branch[], setBranch: (value: string) => void): void {
  const lastUsedBranch = getLocalStorage<string | null>(STORAGE_KEYS.LAST_BRANCH, null);
  if (
    lastUsedBranch &&
    branchList.some((b) => {
      const displayName = b.type === "remote" && b.remote ? `${b.remote}/${b.name}` : b.name;
      return displayName === lastUsedBranch;
    })
  ) {
    setBranch(lastUsedBranch);
    return;
  }
  const preferredBranch = selectPreferredBranch(branchList);
  if (preferredBranch) setBranch(preferredBranch);
}

export function computePassthroughProfile(
  agentProfileId: string,
  agentProfiles: AgentProfileOption[],
) {
  if (!agentProfileId) return false;
  return (
    agentProfiles.find((p: AgentProfileOption) => p.id === agentProfileId)?.cli_passthrough === true
  );
}

export function computeEffectiveStepId(
  selectedWorkflowId: string | null,
  workflowId: string | null,
  fetchedSteps: StepType[] | null,
  defaultStepId: string | null,
) {
  return selectedWorkflowId && selectedWorkflowId !== workflowId && fetchedSteps
    ? (fetchedSteps[0]?.id ?? null)
    : defaultStepId;
}

export function computeIsTaskStarted(
  isEditMode: boolean,
  editingTask?: { state?: Task["state"] } | null,
) {
  if (!isEditMode || !editingTask?.state) return false;
  return editingTask.state !== "TODO" && editingTask.state !== "CREATED";
}
