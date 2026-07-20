"use client";

import { useRef, useState } from "react";
import type { Dispatch, SetStateAction } from "react";
import { deleteWorkflowAction } from "@/app/actions/workspaces";
import type { Workflow, WorkflowStep } from "@/lib/types/http";
import { useToast } from "@/components/toast-provider";
import { SettingsSaveCancelledError, useSettingsSaveContributor } from "./settings-save-provider";
import {
  areStepDraftsEqual,
  createWorkflowDraftSaveProgress,
  persistWorkflowDraft,
  remapWorkflowDraftSteps,
} from "./workflow-card-actions";
import type { useWorkflowMutationGuard } from "./workflow-mutation-guard";

const TEMP_WORKFLOW_PREFIX = "temp-workflow-";

type WorkflowSavedParams = {
  clientWorkflow: Workflow;
  submittedWorkflow: Workflow;
  savedWorkflow: Workflow;
  currentSteps: WorkflowStep[];
  savedSteps: WorkflowStep[];
  finalizeIdentity: boolean;
};

type WorkflowDraftContributorArgs = {
  workflow: Workflow;
  isWorkflowDirty: boolean;
  workflowSteps: WorkflowStep[];
  savedWorkflowSteps: WorkflowStep[];
  setWorkflowSteps: Dispatch<SetStateAction<WorkflowStep[]>>;
  setSavedWorkflowSteps: Dispatch<SetStateAction<WorkflowStep[]>>;
  mutationGuard: ReturnType<typeof useWorkflowMutationGuard>;
  toast: ReturnType<typeof useToast>["toast"];
  onWorkflowSaved: (params: WorkflowSavedParams) => void;
  onDiscardWorkflow: () => void;
  onDeleteWorkflow: () => Promise<unknown>;
};

function useWorkflowDraftPersistence(args: WorkflowDraftContributorArgs) {
  const { workflow, workflowSteps, savedWorkflowSteps } = args;
  const latestStepsRef = useRef(workflowSteps);
  latestStepsRef.current = workflowSteps;
  const saveProgressRef = useRef(createWorkflowDraftSaveProgress());
  const saveFailedRef = useRef(false);
  const revision = JSON.stringify({
    workflow: [workflow.name, workflow.description, workflow.agent_profile_id],
    steps: workflowSteps,
  });
  const latestRevisionRef = useRef(revision);
  latestRevisionRef.current = revision;

  const persistSubmittedDraft = async (submittedRevision: string | number) => {
    let result: Awaited<ReturnType<typeof persistWorkflowDraft>>;
    try {
      result = await persistWorkflowDraft({
        workflow,
        draftSteps: workflowSteps,
        savedSteps: savedWorkflowSteps,
        progress: saveProgressRef.current,
      });
      saveFailedRef.current = false;
    } catch (error) {
      saveFailedRef.current = true;
      throw error;
    }
    const unchanged = submittedRevision === latestRevisionRef.current;
    const currentSteps = unchanged
      ? result.steps
      : remapWorkflowDraftSteps(
          latestStepsRef.current,
          result.workflow.id,
          saveProgressRef.current.stepIds,
        );
    args.setSavedWorkflowSteps(result.steps);
    args.setWorkflowSteps(currentSteps);
    args.onWorkflowSaved({
      clientWorkflow: workflow,
      submittedWorkflow: workflow,
      savedWorkflow: result.workflow,
      currentSteps,
      savedSteps: result.steps,
      finalizeIdentity: unchanged,
    });
  };

  return { revision, persistSubmittedDraft, saveProgressRef, saveFailedRef };
}

export function useWorkflowDraftContributor(args: WorkflowDraftContributorArgs) {
  const { workflow, workflowSteps, savedWorkflowSteps, mutationGuard, toast } = args;
  const persistence = useWorkflowDraftPersistence(args);
  const removingDraftRef = useRef(false);
  const [isRemovingDraft, setIsRemovingDraft] = useState(false);
  const stepsDirty = !areStepDraftsEqual(workflowSteps, savedWorkflowSteps);

  useSettingsSaveContributor({
    id: `workflow:${workflow.id}`,
    order: 100,
    revision: persistence.revision,
    isDirty: args.isWorkflowDirty || stepsDirty,
    canSave: workflow.name.trim().length > 0,
    invalidReason: workflow.name.trim() ? undefined : "Workflow name is required",
    save: async (submittedRevision) => {
      if (!workflow.id.startsWith(TEMP_WORKFLOW_PREFIX)) {
        await persistence.persistSubmittedDraft(submittedRevision);
        return;
      }
      let guardReturned = false;
      let operationStarted = false;
      await mutationGuard.guardMutation({
        baselineSteps: [],
        proposedSteps: workflowSteps,
        intent: "create",
        operation: async () => {
          operationStarted = true;
          try {
            await persistence.persistSubmittedDraft(submittedRevision);
          } catch (error) {
            if (!guardReturned) throw error;
            toast({
              title: "Failed to save workflow changes",
              description: error instanceof Error ? error.message : "Request failed",
              variant: "error",
            });
          }
        },
      });
      guardReturned = true;
      if (!operationStarted) throw new SettingsSaveCancelledError();
    },
    discard: async () => {
      const persistedDraft = persistence.saveProgressRef.current.workflow;
      if (workflow.id.startsWith(TEMP_WORKFLOW_PREFIX) && persistedDraft) {
        await deleteWorkflowAction(persistedDraft.id);
      } else if (persistence.saveFailedRef.current) {
        throw new Error("Retry the partial workflow save before leaving");
      }
      args.setWorkflowSteps(savedWorkflowSteps);
      args.onDiscardWorkflow();
    },
  });

  const removeDraftWorkflow = async () => {
    if (removingDraftRef.current) return;
    removingDraftRef.current = true;
    setIsRemovingDraft(true);
    try {
      const persistedDraft = persistence.saveProgressRef.current.workflow;
      if (workflow.id.startsWith(TEMP_WORKFLOW_PREFIX) && persistedDraft) {
        await deleteWorkflowAction(persistedDraft.id);
      }
      await args.onDeleteWorkflow();
    } finally {
      removingDraftRef.current = false;
      setIsRemovingDraft(false);
    }
  };

  return {
    hasUnsavedChanges: args.isWorkflowDirty || stepsDirty,
    removeDraftWorkflow,
    isRemovingDraft,
  };
}
