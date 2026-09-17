"use client";

import { useCallback, useEffect, useRef, useState, type ChangeEvent, type RefObject } from "react";
import { useTranslation } from "react-i18next";
import { importWorkflowsAction, previewWorkflowImportAction } from "@/app/actions/workspaces";
import { useToast } from "@/components/toast-provider";
import { useRouter } from "@/lib/routing/client-router";
import type {
  WorkflowImportPreview,
  WorkflowImportProfileBinding,
  WorkflowImportProfileCandidate,
  WorkflowImportProfileConflict,
  WorkflowImportProfilesRequiredResponse,
  WorkflowImportProfileStep,
  ImportWorkflowsResult,
  Workspace,
} from "@/lib/types/http";

export type WorkflowImportPhase = "editing" | "previewing" | "resolving" | "submitting";
export type WorkflowImportSelections = Record<string, string>;

export function workflowImportStepKey(
  step: Pick<WorkflowImportProfileStep, "workflow_index" | "step_position">,
): string {
  return `${step.workflow_index}:${step.step_position}`;
}

function workflowImportConflictKey(conflict: WorkflowImportProfileConflict): string {
  return workflowImportStepKey(conflict);
}

export function workflowImportProfileLabel(profile: WorkflowImportProfileCandidate): string {
  return [profile.name, profile.agent_name, profile.model, profile.mode]
    .filter(Boolean)
    .join(" · ");
}

export function workflowImportMissingSteps(
  preview: WorkflowImportPreview | null,
  selections: WorkflowImportSelections,
): WorkflowImportProfileStep[] {
  if (!preview) return [];
  return preview.steps.filter((step) => !selections[workflowImportStepKey(step)]);
}

function sameCandidateRevision(
  previous: WorkflowImportPreview | null,
  next: WorkflowImportPreview,
  profileId: string,
): boolean {
  const previousProfile = previous?.profiles.find((profile) => profile.id === profileId);
  const nextProfile = next.profiles.find((profile) => profile.id === profileId);
  return Boolean(
    previousProfile && nextProfile && previousProfile.updated_at === nextProfile.updated_at,
  );
}

export function reconcileWorkflowImportSelections(
  previous: WorkflowImportPreview | null,
  next: WorkflowImportPreview,
  selections: WorkflowImportSelections,
): WorkflowImportSelections {
  const nextSelections: WorkflowImportSelections = {};
  for (const step of next.steps) {
    const key = workflowImportStepKey(step);
    const previousSelection = selections[key];
    if (previousSelection && sameCandidateRevision(previous, next, previousSelection)) {
      nextSelections[key] = previousSelection;
      continue;
    }
    if (previousSelection) continue;
    if (step.matched_profile) nextSelections[key] = step.matched_profile.id;
  }
  return nextSelections;
}

function buildWorkflowImportBindings(
  preview: WorkflowImportPreview,
  selections: WorkflowImportSelections,
): WorkflowImportProfileBinding[] {
  return preview.steps.flatMap((step) => {
    const profileId = selections[workflowImportStepKey(step)];
    const profile = preview.profiles.find((candidate) => candidate.id === profileId);
    if (!profileId || !profile) return [];
    return [
      {
        workflow_index: step.workflow_index,
        step_position: step.step_position,
        requested_profile: step.requested_profile,
        profile_id: profile.id,
        profile_updated_at: profile.updated_at,
      },
    ];
  });
}

export function getWorkflowImportConflict(
  error: unknown,
): WorkflowImportProfilesRequiredResponse | null {
  if (!error || typeof error !== "object") return null;
  const body = "body" in error ? (error as { body?: unknown }).body : error;
  if (!body || typeof body !== "object" || !("code" in body)) return null;
  const candidate = body as Partial<WorkflowImportProfilesRequiredResponse>;
  if (candidate.code !== "workflow_import_profiles_required" || !Array.isArray(candidate.steps)) {
    return null;
  }
  return candidate as WorkflowImportProfilesRequiredResponse;
}

type WorkflowImportArgs = {
  workspace: Workspace | null;
  router: ReturnType<typeof useRouter>;
  toast: ReturnType<typeof useToast>["toast"];
};

function clearConflictedSelections(
  current: WorkflowImportSelections,
  conflicts: WorkflowImportProfileConflict[],
): WorkflowImportSelections {
  const conflictedKeys = new Set(conflicts.map(workflowImportConflictKey));
  return Object.fromEntries(Object.entries(current).filter(([key]) => !conflictedKeys.has(key)));
}

function importResultDescription(
  result: ImportWorkflowsResult,
  translate: (key: string, options?: Record<string, unknown>) => string,
): string {
  const parts: string[] = [];
  if (result.created?.length) {
    parts.push(translate("workflows:importCreated", { names: result.created.join(", ") }));
  }
  if (result.skipped?.length) {
    parts.push(translate("workflows:importSkipped", { names: result.skipped.join(", ") }));
  }
  return parts.join(". ");
}

function notifyImportFailure(
  toast: WorkflowImportArgs["toast"],
  translate: (key: string) => string,
  error: unknown,
): void {
  toast({
    title: translate("workflows:failedToImportWorkflows"),
    description: error instanceof Error ? error.message : translate("workflows:invalidYaml"),
    variant: "error",
  });
}

function useWorkflowImportState(workspaceId: string | undefined) {
  const [isImportDialogOpen, setIsImportDialogOpen] = useState(false);
  const [importYaml, setImportYamlState] = useState("");
  const [phase, setPhase] = useState<WorkflowImportPhase>("editing");
  const [preview, setPreview] = useState<WorkflowImportPreview | null>(null);
  const [selections, setSelections] = useState<WorkflowImportSelections>({});
  const [profileConflicts, setProfileConflicts] = useState<WorkflowImportProfileConflict[]>([]);
  const [activeStepKey, setActiveStepKey] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const generationRef = useRef(0);
  const submissionRef = useRef(false);
  const workspaceIdRef = useRef(workspaceId);

  const clearReview = useCallback(() => {
    setPreview(null);
    setSelections({});
    setProfileConflicts([]);
    setActiveStepKey(null);
    setPhase("editing");
  }, []);

  const invalidatePreview = useCallback(() => {
    if (submissionRef.current) return;
    generationRef.current += 1;
    submissionRef.current = false;
    clearReview();
  }, [clearReview]);

  useEffect(() => {
    if (workspaceIdRef.current === workspaceId) return;
    workspaceIdRef.current = workspaceId;
    invalidatePreview();
  }, [invalidatePreview, workspaceId]);

  useEffect(() => {
    return () => {
      generationRef.current += 1;
    };
  }, []);

  const setImportYaml = useCallback(
    (value: string) => {
      if (submissionRef.current) return;
      setImportYamlState(value);
      invalidatePreview();
    },
    [invalidatePreview],
  );

  const handleFileUpload = useCallback(
    (event: ChangeEvent<HTMLInputElement>) => {
      if (submissionRef.current) return;
      const file = event.target.files?.[0];
      if (!file) return;
      const generation = ++generationRef.current;
      clearReview();
      const reader = new FileReader();
      reader.onload = (loadEvent) => {
        if (generationRef.current !== generation) return;
        setImportYamlState(String(loadEvent.target?.result ?? ""));
      };
      reader.readAsText(file);
      event.target.value = "";
    },
    [clearReview],
  );

  const cancelPendingImport = useCallback(() => {
    if (submissionRef.current) return;
    generationRef.current += 1;
    submissionRef.current = false;
    clearReview();
  }, [clearReview]);

  const resetAfterSuccess = useCallback(() => {
    generationRef.current += 1;
    submissionRef.current = false;
    setIsImportDialogOpen(false);
    setImportYamlState("");
    clearReview();
  }, [clearReview]);

  return {
    isImportDialogOpen,
    setIsImportDialogOpen,
    importYaml,
    phase,
    setPhase,
    preview,
    setPreview,
    selections,
    setSelections,
    profileConflicts,
    setProfileConflicts,
    activeStepKey,
    setActiveStepKey,
    fileInputRef,
    generationRef,
    submissionRef,
    setImportYaml,
    handleFileUpload,
    cancelPendingImport,
    resetAfterSuccess,
  };
}

type WorkflowImportState = ReturnType<typeof useWorkflowImportState>;

function useWorkflowImportSubmission({
  workspace,
  router,
  toast,
  translate,
  importYaml,
  state,
}: WorkflowImportArgs & {
  translate: (key: string, options?: Record<string, unknown>) => string;
  importYaml: string;
  state: WorkflowImportState;
}) {
  return useCallback(
    async (
      targetPreview: WorkflowImportPreview,
      targetSelections: WorkflowImportSelections,
      generation: number,
    ) => {
      const { generationRef, submissionRef, setPhase, setActiveStepKey } = state;
      if (submissionRef.current || generationRef.current !== generation || !workspace) return;
      const missing = workflowImportMissingSteps(targetPreview, targetSelections);
      if (missing.length > 0) {
        setPhase("resolving");
        setActiveStepKey(workflowImportStepKey(missing[0]));
        return;
      }
      submissionRef.current = true;
      setPhase("submitting");
      try {
        const result = await importWorkflowsAction(
          workspace.id,
          importYaml.trim(),
          buildWorkflowImportBindings(targetPreview, targetSelections),
        );
        if (generationRef.current !== generation) return;
        toast({
          title: translate("workflows:importCompleteTitle"),
          description: importResultDescription(result, translate),
        });
        state.resetAfterSuccess();
        if (result.created?.length) router.refresh();
      } catch (error) {
        if (generationRef.current !== generation) return;
        const conflict = getWorkflowImportConflict(error);
        if (conflict) {
          state.setProfileConflicts(conflict.steps);
          state.setSelections((current) => clearConflictedSelections(current, conflict.steps));
          setPhase("resolving");
          return;
        }
        notifyImportFailure(toast, translate, error);
        setPhase("editing");
      } finally {
        if (generationRef.current === generation) submissionRef.current = false;
      }
    },
    [importYaml, router, state, toast, translate, workspace],
  );
}

function useWorkflowImportPreview({
  workspace,
  importYaml,
  preview,
  selections,
  profileConflicts,
  state,
  submit,
  toast,
  translate,
}: {
  workspace: Workspace | null;
  importYaml: string;
  preview: WorkflowImportPreview | null;
  selections: WorkflowImportSelections;
  profileConflicts: WorkflowImportProfileConflict[];
  state: WorkflowImportState;
  submit: (
    targetPreview: WorkflowImportPreview,
    targetSelections: WorkflowImportSelections,
    generation: number,
  ) => Promise<void>;
  toast: WorkflowImportArgs["toast"];
  translate: (key: string) => string;
}) {
  return useCallback(async () => {
    const {
      generationRef,
      submissionRef,
      setPhase,
      setProfileConflicts,
      setPreview,
      setSelections,
      setActiveStepKey,
    } = state;
    if (!workspace || !importYaml.trim() || submissionRef.current) return;
    const generation = ++generationRef.current;
    setPhase("previewing");
    setProfileConflicts([]);
    try {
      const nextPreview = await previewWorkflowImportAction(workspace.id, importYaml.trim());
      if (generationRef.current !== generation) return;
      const nextSelections = clearConflictedSelections(
        reconcileWorkflowImportSelections(preview, nextPreview, selections),
        profileConflicts,
      );
      setPreview(nextPreview);
      setSelections(nextSelections);
      const missing = workflowImportMissingSteps(nextPreview, nextSelections);
      if (missing.length > 0) {
        setPhase("resolving");
        setActiveStepKey(workflowImportStepKey(missing[0]));
        return;
      }
      await submit(nextPreview, nextSelections, generation);
    } catch (error) {
      if (generationRef.current !== generation) return;
      notifyImportFailure(toast, translate, error);
      setPhase("editing");
    }
  }, [
    importYaml,
    preview,
    profileConflicts,
    selections,
    state,
    submit,
    toast,
    translate,
    workspace,
  ]);
}

export function useWorkflowImport({ workspace, router, toast }: WorkflowImportArgs) {
  const { t } = useTranslation();
  const state = useWorkflowImportState(workspace?.id);
  const { importYaml, preview, selections, phase, submissionRef } = state;
  const submit = useWorkflowImportSubmission({
    workspace,
    router,
    toast,
    translate: t,
    importYaml,
    state,
  });
  const runPreview = useWorkflowImportPreview({
    workspace,
    importYaml,
    preview,
    selections,
    profileConflicts: state.profileConflicts,
    state,
    submit,
    toast,
    translate: t,
  });

  const handleImport = useCallback(async () => {
    if (!workspace || !importYaml.trim() || submissionRef.current) return;
    if (preview) {
      await submit(preview, selections, state.generationRef.current);
      return;
    }
    await runPreview();
  }, [
    importYaml,
    preview,
    runPreview,
    selections,
    state.generationRef,
    submit,
    submissionRef,
    workspace,
  ]);

  const selectProfile = useCallback(
    (stepKey: string, profileId: string) => {
      state.setSelections((current) => ({ ...current, [stepKey]: profileId }));
      state.setProfileConflicts((current) =>
        current.filter((conflict) => workflowImportStepKey(conflict) !== stepKey),
      );
      state.setActiveStepKey(null);
    },
    [state],
  );

  const setDialogOpen = useCallback(
    (open: boolean) => {
      if (open) {
        if (!state.isImportDialogOpen && state.preview) state.cancelPendingImport();
        state.setIsImportDialogOpen(true);
        return;
      }
      if (phase === "submitting" || submissionRef.current) return;
      state.cancelPendingImport();
      state.setIsImportDialogOpen(open);
    },
    [phase, state, submissionRef],
  );

  const missingSteps = workflowImportMissingSteps(preview, selections);
  return {
    isImportDialogOpen: state.isImportDialogOpen,
    setIsImportDialogOpen: setDialogOpen,
    importYaml,
    setImportYaml: state.setImportYaml,
    handleFileUpload: state.handleFileUpload,
    fileInputRef: state.fileInputRef as RefObject<HTMLInputElement | null>,
    handleImport,
    importLoading: phase === "previewing" || phase === "submitting",
    importPhase: phase,
    preview,
    selections,
    missingSteps,
    profileConflicts: state.profileConflicts,
    activeStepKey: state.activeStepKey,
    setActiveStepKey: state.setActiveStepKey,
    selectProfile,
    retryPreview: runPreview,
  };
}
