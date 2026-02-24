import type { useRouter } from "next/navigation";
import type { Task, Branch, LocalRepository } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { AppState } from "@/lib/state/store";
import type { StepType } from "@/components/task-create-dialog-types";
import { selectPreferredBranch } from "@/lib/utils";
import { getLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "@/lib/settings/constants";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useContextFilesStore } from "@/lib/state/context-files-store";
import { linkToSession } from "@/lib/links";
import { createTask } from "@/lib/api";

type CreateTaskParams = Parameters<typeof createTask>[0];

export type { CreateTaskParams };

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

export type ActivatePlanModeArgs = {
  sessionId: string;
  taskId: string;
  setActiveDocument: AppState["setActiveDocument"];
  setPlanMode: AppState["setPlanMode"];
  router: ReturnType<typeof useRouter>;
};

export function activatePlanMode({
  sessionId,
  taskId,
  setActiveDocument,
  setPlanMode,
  router,
}: ActivatePlanModeArgs) {
  setActiveDocument(sessionId, { type: "plan", taskId });
  useDockviewStore.getState().queuePanelAction({
    id: "plan",
    component: "plan",
    title: "Plan",
    placement: "right",
    referencePanel: "chat",
  });
  setPlanMode(sessionId, true);
  useContextFilesStore.getState().addFile(sessionId, { path: "plan:context", name: "Plan" });
  router.push(linkToSession(sessionId));
}

export type BuildCreatePayloadArgs = {
  workspaceId: string;
  effectiveWorkflowId: string;
  trimmedTitle: string;
  trimmedDescription: string;
  repositoriesPayload: CreateTaskParams["repositories"];
  agentProfileId: string;
  executorId: string;
  executorProfileId: string;
  withAgent: boolean;
  planMode?: boolean;
};

export function buildCreateTaskPayload(args: BuildCreatePayloadArgs): CreateTaskParams {
  return {
    workspace_id: args.workspaceId,
    workflow_id: args.effectiveWorkflowId,
    title: args.trimmedTitle,
    description: args.trimmedDescription,
    repositories: args.repositoriesPayload,
    state: args.withAgent ? "IN_PROGRESS" : "CREATED",
    start_agent: args.withAgent ? true : undefined,
    prepare_session: args.withAgent ? undefined : true,
    agent_profile_id: args.agentProfileId || undefined,
    executor_id: args.executorId || undefined,
    executor_profile_id: args.executorProfileId || undefined,
    plan_mode: args.planMode || undefined,
  };
}

export function validateCreateInputs(inputs: {
  trimmedTitle: string;
  workspaceId: string | null;
  effectiveWorkflowId: string | null;
  repositoryId: string;
  selectedLocalRepo: LocalRepository | null;
  agentProfileId: string;
}): boolean {
  return Boolean(
    inputs.trimmedTitle &&
    inputs.workspaceId &&
    inputs.effectiveWorkflowId &&
    (inputs.repositoryId || inputs.selectedLocalRepo) &&
    inputs.agentProfileId,
  );
}
