"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type { Workflow, WorkflowStep, Workspace } from "@/lib/types/http";
import { useRouter } from "@/lib/routing/client-router";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import {
  areStepDraftsEqual,
  createWorkflowDraftSaveProgress,
  persistWorkflowDraft,
  remapWorkflowDraftSteps,
} from "@/components/settings/workflow-card-actions";
import { deleteWorkflowAction } from "@/app/actions/workspaces";
import { IMPROVE_KANDEV_WORKSPACE_NAME } from "@/components/improve-kandev-dialog-model";
import {
  addLocalStep,
  applyWorkflowStepUpdates,
  removeLocalStep,
} from "@/components/settings/workflow-step-mutations";
import {
  buildWorkflowEditorViewModel,
  repairWorkflowEditorSelection,
} from "@/lib/workflows/workflow-editor-view-model";
import { workflowEditorPath } from "./workflow-editor-paths";

export type WorkflowEditorDraftInput = {
  workspace: Workspace;
  initialWorkflow: Workflow;
  initialSteps: WorkflowStep[];
  isNewWorkflow: boolean;
};

type WorkflowEditorDraftState = {
  workspace: Workspace;
  initialWorkflow: Workflow;
  isNewWorkflow: boolean;
  workflow: Workflow;
  savedWorkflow?: Workflow;
  steps: WorkflowStep[];
  savedSteps: WorkflowStep[];
  selectedStepId: string | null;
  selectedStep: WorkflowStep | null;
  savedSelectedStep?: WorkflowStep;
  model: ReturnType<typeof buildWorkflowEditorViewModel>;
  readOnly: boolean;
  revision: string;
  isDirty: boolean;
  invalidIssue?: { messageKey: string };
  onUpdateWorkflow: (updates: Partial<Pick<Workflow, "name" | "description" | "prompt">>) => void;
  onSelectStep: (stepId: string) => void;
  onAddStep: () => void;
  onRemoveStep: (stepId: string) => void;
  onUpdateStep: (stepId: string, updates: Partial<WorkflowStep>) => void;
  onSessionConfigResolutionPendingChange: (pending: boolean) => void;
  sessionConfigPending: boolean;
  latestRevisionRef: React.MutableRefObject<string>;
  latestStepsRef: React.MutableRefObject<WorkflowStep[]>;
  deletedStepIdsRef: React.MutableRefObject<string[]>;
  saveProgressRef: React.MutableRefObject<ReturnType<typeof createWorkflowDraftSaveProgress>>;
  setWorkflow: React.Dispatch<React.SetStateAction<Workflow>>;
  setSavedWorkflow: React.Dispatch<React.SetStateAction<Workflow | undefined>>;
  setSteps: React.Dispatch<React.SetStateAction<WorkflowStep[]>>;
  setSavedSteps: React.Dispatch<React.SetStateAction<WorkflowStep[]>>;
};

export function useWorkflowEditorDraft(args: WorkflowEditorDraftInput) {
  const state = useWorkflowEditorDraftState(args);
  useWorkflowEditorSaveCoordinator(state);
  return state;
}

function useWorkflowEditorDraftState({
  workspace,
  initialWorkflow,
  initialSteps,
  isNewWorkflow,
}: WorkflowEditorDraftInput): WorkflowEditorDraftState {
  const { t } = useTranslation();
  const [workflow, setWorkflow] = useState(initialWorkflow);
  const [savedWorkflow, setSavedWorkflow] = useState<Workflow | undefined>(
    isNewWorkflow ? undefined : initialWorkflow,
  );
  const [steps, setSteps] = useState(initialSteps);
  const [savedSteps, setSavedSteps] = useState(isNewWorkflow ? [] : initialSteps);
  const [selectedStepId, setSelectedStepId] = useState<string | null>(initialSteps[0]?.id ?? null);
  const [sessionConfigPending, setSessionConfigPending] = useState(false);
  const deletedStepIdsRef = useRef<string[]>([]);
  const latestRevisionRef = useRef("");
  const latestStepsRef = useRef(steps);
  const saveProgressRef = useRef(createWorkflowDraftSaveProgress());
  latestStepsRef.current = steps;

  const model = useMemo(
    () => buildWorkflowEditorViewModel(workflow, steps, savedWorkflow, savedSteps),
    [savedSteps, savedWorkflow, steps, workflow],
  );
  const selectedStep = steps.find((step) => step.id === selectedStepId) ?? null;
  const savedSelectedStep = savedSteps.find((step) => step.id === selectedStepId);
  const readOnly = workflow.source === "github" || workspace.name === IMPROVE_KANDEV_WORKSPACE_NAME;
  const revision = JSON.stringify({ workflow: workflowDraftFields(workflow), steps });
  latestRevisionRef.current = revision;
  const stepsDirty = !areStepDraftsEqual(steps, savedSteps);
  const isDirty = model.workflowDirty || stepsDirty;
  const invalidIssue = model.issues.find((issue) => issue.target === "workflow") ?? model.issues[0];

  useEffect(() => {
    const repaired = repairWorkflowEditorSelection(selectedStepId, steps);
    if (repaired !== selectedStepId) setSelectedStepId(repaired);
  }, [selectedStepId, steps]);

  const onUpdateWorkflow = (
    updates: Partial<Pick<Workflow, "name" | "description" | "prompt">>,
  ) => {
    if (!readOnly) setWorkflow((current) => ({ ...current, ...updates }));
  };
  const onSelectStep = (stepId: string) => setSelectedStepId(stepId);
  const onAddStep = () => {
    if (!readOnly) addLocalStep(workflow, setSteps);
  };
  const onRemoveStep = (stepId: string) => {
    if (readOnly || !window.confirm(t("workflows:confirmDeleteStep"))) return;
    removeLocalStep(stepId, setSteps);
    if (!stepId.startsWith("temp-")) deletedStepIdsRef.current.push(stepId);
  };
  const onUpdateStep = (stepId: string, updates: Partial<WorkflowStep>) => {
    if (!readOnly) setSteps((current) => applyWorkflowStepUpdates(current, stepId, updates));
  };

  return {
    workspace,
    initialWorkflow,
    isNewWorkflow,
    workflow,
    savedWorkflow,
    steps,
    savedSteps,
    selectedStepId,
    selectedStep,
    savedSelectedStep,
    model,
    readOnly,
    revision,
    isDirty,
    invalidIssue,
    onUpdateWorkflow,
    onSelectStep,
    onAddStep,
    onRemoveStep,
    onUpdateStep,
    onSessionConfigResolutionPendingChange: setSessionConfigPending,
    sessionConfigPending,
    latestRevisionRef,
    latestStepsRef,
    deletedStepIdsRef,
    saveProgressRef,
    setWorkflow,
    setSavedWorkflow,
    setSteps,
    setSavedSteps,
  };
}

function useWorkflowEditorSaveCoordinator(state: WorkflowEditorDraftState) {
  const { t } = useTranslation();
  const router = useRouter();
  useSettingsSaveContributor({
    id: `workflow-editor:${state.workspace.id}:${state.initialWorkflow.id}`,
    order: 100,
    revision: state.revision,
    isDirty: state.isDirty,
    canSave: !state.readOnly && !state.sessionConfigPending && state.model.issues.length === 0,
    invalidReason: state.invalidIssue ? t(state.invalidIssue.messageKey) : undefined,
    save: async (submittedRevision) => {
      const result = await persistWorkflowDraft({
        workflow: state.workflow,
        draftSteps: state.steps,
        savedSteps: state.savedSteps,
        progress: state.saveProgressRef.current,
        deletedStepIds: state.deletedStepIdsRef.current,
      });
      const unchanged = submittedRevision === state.latestRevisionRef.current;
      const currentSteps = unchanged
        ? result.steps
        : remapWorkflowDraftSteps(
            state.latestStepsRef.current,
            result.workflow.id,
            state.saveProgressRef.current.stepIds,
          );
      state.setSavedWorkflow(result.workflow);
      state.setSavedSteps(result.steps);
      state.setWorkflow((current) =>
        unchanged ? result.workflow : { ...current, id: result.workflow.id },
      );
      state.setSteps(currentSteps);
      state.deletedStepIdsRef.current = [];
      if (unchanged && result.workflow.id !== state.initialWorkflow.id) {
        router.replace(workflowEditorPath(state.workspace.id, result.workflow.id));
      }
    },
    discard: async (submittedRevision) => {
      const persistedDraft = state.saveProgressRef.current.workflow;
      if (state.isNewWorkflow && persistedDraft) await deleteWorkflowAction(persistedDraft.id);
      if (submittedRevision !== undefined && submittedRevision !== state.latestRevisionRef.current)
        return;
      state.setWorkflow(state.savedWorkflow ?? state.initialWorkflow);
      state.setSteps(state.savedSteps);
      state.deletedStepIdsRef.current = [];
    },
  });
}

function workflowDraftFields(workflow: Workflow) {
  return [
    workflow.name,
    workflow.description ?? "",
    workflow.prompt ?? "",
    workflow.agent_profile_id ?? "",
  ];
}
