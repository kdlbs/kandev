"use client";

import { useState, useEffect } from "react";
import { CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type { Workflow, WorkflowStep } from "@/lib/types/http";
import type { WorkflowReplayCycleDiagnostic } from "@/lib/workflows/replay-cycle-analysis";
import { useHealthyAgentProfiles } from "@/hooks/domains/settings/use-healthy-agent-profiles";
import { useRequest } from "@/lib/http/use-request";
import { useToast } from "@/components/toast-provider";
import { WorkflowExportDialog } from "@/components/settings/workflow-export-dialog";
import { WorkflowPipelineEditor } from "@/components/settings/workflow-pipeline-editor";
import { listWorkflowStepsAction } from "@/app/actions/workspaces";
import { HelpTip } from "./workflow-pipeline-editor-helpers";
import { WorkflowDeleteDialog, StepDeleteDialog } from "./workflow-card-dialogs";
import {
  useWorkflowStepActions,
  useWorkflowDeleteHandlers,
  useStepDeleteHandlers,
} from "./workflow-card-actions";
import { WorkflowCardHeaderActions } from "./workflow-card-header-actions";
import { SettingsCard } from "./settings-card";
import { isWorkflowFieldDirty } from "./workflow-dirty-state";
import { WorkflowSyncedBadge } from "./workflow-synced-badge";
import { useWorkflowMutationGuard } from "./workflow-mutation-guard";
import { WorkflowCycleGuardDialog } from "./workflow-cycle-diagnostic";
import { useWorkflowDraftContributor } from "./use-workflow-draft-contributor";

const TEMP_WORKFLOW_PREFIX = "temp-workflow-";

type WorkflowCardProps = {
  workflow: Workflow;
  savedWorkflow?: Workflow;
  isWorkflowDirty: boolean;
  initialWorkflowSteps?: WorkflowStep[];
  otherWorkflows?: Workflow[];
  onUpdateWorkflow: (updates: {
    name?: string;
    description?: string;
    agent_profile_id?: string;
  }) => void;
  onDeleteWorkflow: () => Promise<unknown>;
  onWorkflowSaved: (params: {
    clientWorkflow: Workflow;
    submittedWorkflow: Workflow;
    savedWorkflow: Workflow;
    currentSteps: WorkflowStep[];
    savedSteps: WorkflowStep[];
    finalizeIdentity: boolean;
  }) => void;
  onDiscardWorkflow: () => void;
};

function useWorkflowSteps(
  workflowId: string,
  initialSteps: WorkflowStep[] | undefined,
  toast: ReturnType<typeof useToast>["toast"],
) {
  const [workflowSteps, setWorkflowSteps] = useState<WorkflowStep[]>(initialSteps ?? []);
  const [savedWorkflowSteps, setSavedWorkflowSteps] = useState<WorkflowStep[]>(initialSteps ?? []);
  const [workflowLoading, setWorkflowLoading] = useState(false);

  useEffect(() => {
    if (workflowId.startsWith(TEMP_WORKFLOW_PREFIX)) return;
    let cancelled = false;
    const load = async () => {
      setWorkflowLoading(true);
      try {
        const res = await listWorkflowStepsAction(workflowId);
        if (!cancelled) {
          setWorkflowSteps(res.steps ?? []);
          setSavedWorkflowSteps(res.steps ?? []);
        }
      } catch {
        if (!cancelled) toast({ title: "Failed to load workflow steps", variant: "error" });
      } finally {
        if (!cancelled) setWorkflowLoading(false);
      }
    };
    load();
    return () => {
      cancelled = true;
    };
  }, [workflowId, initialSteps, toast]);

  const refreshWorkflowSteps = async () => {
    try {
      const res = await listWorkflowStepsAction(workflowId);
      setWorkflowSteps(res.steps ?? []);
      setSavedWorkflowSteps(res.steps ?? []);
    } catch {
      /* ignore */
    }
  };

  return {
    workflowSteps,
    setWorkflowSteps,
    savedWorkflowSteps,
    setSavedWorkflowSteps,
    workflowLoading,
    refreshWorkflowSteps,
  };
}

type WorkflowDeleteState = {
  deleteOpen: boolean;
  setDeleteOpen: (v: boolean) => void;
  workflowTaskCount: number | null;
  setWorkflowTaskCount: (v: number | null) => void;
  workflowDeleteLoading: boolean;
  setWorkflowDeleteLoading: (v: boolean) => void;
  targetWorkflowId: string;
  setTargetWorkflowId: (v: string) => void;
  targetWorkflowSteps: WorkflowStep[];
  setTargetWorkflowSteps: (v: WorkflowStep[]) => void;
  targetStepId: string;
  setTargetStepId: (v: string) => void;
  migrateLoading: boolean;
  setMigrateLoading: (v: boolean) => void;
};

function useWorkflowDeleteState(): WorkflowDeleteState {
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [workflowTaskCount, setWorkflowTaskCount] = useState<number | null>(null);
  const [workflowDeleteLoading, setWorkflowDeleteLoading] = useState(false);
  const [targetWorkflowId, setTargetWorkflowId] = useState<string>("");
  const [targetWorkflowSteps, setTargetWorkflowSteps] = useState<WorkflowStep[]>([]);
  const [targetStepId, setTargetStepId] = useState<string>("");
  const [migrateLoading, setMigrateLoading] = useState(false);
  return {
    deleteOpen,
    setDeleteOpen,
    workflowTaskCount,
    setWorkflowTaskCount,
    workflowDeleteLoading,
    setWorkflowDeleteLoading,
    targetWorkflowId,
    setTargetWorkflowId,
    targetWorkflowSteps,
    setTargetWorkflowSteps,
    targetStepId,
    setTargetStepId,
    migrateLoading,
    setMigrateLoading,
  };
}

type StepDeleteState = {
  stepDeleteOpen: boolean;
  setStepDeleteOpen: (v: boolean) => void;
  stepToDelete: string | null;
  setStepToDelete: (v: string | null) => void;
  stepTaskCount: number | null;
  setStepTaskCount: (v: number | null) => void;
  targetStepForMigration: string;
  setTargetStepForMigration: (v: string) => void;
  stepMigrateLoading: boolean;
  setStepMigrateLoading: (v: boolean) => void;
  stepDeletePending: boolean;
  setStepDeletePending: (v: boolean) => void;
};

function useStepDeleteState(): StepDeleteState {
  const [stepDeleteOpen, setStepDeleteOpen] = useState(false);
  const [stepToDelete, setStepToDelete] = useState<string | null>(null);
  const [stepTaskCount, setStepTaskCount] = useState<number | null>(null);
  const [targetStepForMigration, setTargetStepForMigration] = useState<string>("");
  const [stepMigrateLoading, setStepMigrateLoading] = useState(false);
  const [stepDeletePending, setStepDeletePending] = useState(false);
  return {
    stepDeleteOpen,
    setStepDeleteOpen,
    stepToDelete,
    setStepToDelete,
    stepTaskCount,
    setStepTaskCount,
    targetStepForMigration,
    setTargetStepForMigration,
    stepMigrateLoading,
    setStepMigrateLoading,
    stepDeletePending,
    setStepDeletePending,
  };
}

type WorkflowCardDialogsProps = {
  wfDel: WorkflowDeleteState;
  otherWorkflows: Workflow[];
  deleteWorkflowLoading: boolean;
  wfDeleteHandlers: {
    handleDeleteWorkflow: () => Promise<void>;
    handleMigrateAndDeleteWorkflow: () => Promise<void>;
  };
  exportOpen: boolean;
  setExportOpen: (open: boolean) => void;
  exportYaml: string;
  stepDel: StepDeleteState;
  stepToDeleteName: string;
  stepsForStepMigration: WorkflowStep[];
  stepDeleteHandlers: {
    handleMigrateAndDeleteStep: () => Promise<void>;
    handleDeleteStepAndTasks: () => Promise<void>;
  };
  hasUnsavedChanges: boolean;
  mutationGuard: ReturnType<typeof useWorkflowMutationGuard>;
};

type WorkflowCardBodyProps = {
  workflow: Workflow;
  savedWorkflow?: Workflow;
  onUpdateWorkflow: (updates: {
    name?: string;
    description?: string;
    agent_profile_id?: string;
  }) => void;
  workflowLoading: boolean;
  workflowSteps: WorkflowStep[];
  savedWorkflowSteps: WorkflowStep[];
  diagnostics: WorkflowReplayCycleDiagnostic[];
  mutationPending: boolean;
  stepActions: {
    handleUpdateWorkflowStep: (id: string, updates: Partial<WorkflowStep>) => Promise<void>;
    handleAddWorkflowStep: () => Promise<void>;
    handleRemoveWorkflowStep: (id: string) => Promise<void>;
    handleReorderWorkflowSteps: (steps: WorkflowStep[]) => Promise<void>;
  };
  readOnly: boolean;
};

function WorkflowCardBody({
  workflow,
  savedWorkflow,
  onUpdateWorkflow,
  workflowLoading,
  workflowSteps,
  savedWorkflowSteps,
  diagnostics,
  mutationPending,
  stepActions,
  readOnly,
}: WorkflowCardBodyProps) {
  const healthyProfiles = useHealthyAgentProfiles(workflow.agent_profile_id);

  return (
    <>
      <Label>Workflow details</Label>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:gap-2">
        <div className="flex-1 space-y-1.5">
          <Label className="flex items-center gap-2">
            <span>Workflow Name</span>
            {readOnly && <WorkflowSyncedBadge sourcePath={workflow.source_path} />}
            {readOnly && (
              <span className="text-xs text-muted-foreground">
                Read-only — managed by workflow sync
              </span>
            )}
          </Label>
          <Input
            value={workflow.name}
            onChange={(e) => onUpdateWorkflow({ name: e.target.value })}
            disabled={readOnly}
            data-settings-dirty={isWorkflowFieldDirty(workflow, savedWorkflow, "name")}
          />
        </div>
        <div className="w-full space-y-1.5 sm:w-[240px] sm:shrink-0">
          <Label className="flex items-center gap-1">
            <span>Agent Profile</span>
            <HelpTip text="Default agent profile for tasks in this workflow. When set, the agent selector is locked in the task creation dialog." />
          </Label>
          <Select
            value={workflow.agent_profile_id || "none"}
            onValueChange={(value) =>
              onUpdateWorkflow({ agent_profile_id: value === "none" ? "" : value })
            }
            disabled={readOnly}
          >
            <SelectTrigger
              className="w-full cursor-pointer"
              data-testid="workflow-agent-profile-select"
              data-settings-dirty={isWorkflowFieldDirty(
                workflow,
                savedWorkflow,
                "agent_profile_id",
              )}
            >
              <SelectValue placeholder="None (use task default)" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="none" className="cursor-pointer">
                None (use task default)
              </SelectItem>
              {healthyProfiles.map((p) => (
                <SelectItem key={p.id} value={p.id} className="cursor-pointer">
                  {p.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>
      <div className="space-y-2">
        <Label>Workflow Steps</Label>
        {workflowLoading ? (
          <div className="text-sm text-muted-foreground">Loading workflow steps...</div>
        ) : (
          <WorkflowPipelineEditor
            steps={workflowSteps}
            savedSteps={savedWorkflowSteps}
            diagnostics={diagnostics}
            onUpdateStep={stepActions.handleUpdateWorkflowStep}
            onAddStep={stepActions.handleAddWorkflowStep}
            onRemoveStep={stepActions.handleRemoveWorkflowStep}
            onReorderSteps={stepActions.handleReorderWorkflowSteps}
            readOnly={mutationPending || readOnly}
          />
        )}
      </div>
    </>
  );
}

function WorkflowCardDialogs({
  wfDel,
  otherWorkflows,
  deleteWorkflowLoading,
  wfDeleteHandlers,
  exportOpen,
  setExportOpen,
  exportYaml,
  stepDel,
  stepToDeleteName,
  stepsForStepMigration,
  stepDeleteHandlers,
  hasUnsavedChanges,
  mutationGuard,
}: WorkflowCardDialogsProps) {
  return (
    <>
      <WorkflowDeleteDialog
        open={wfDel.deleteOpen}
        onOpenChange={wfDel.setDeleteOpen}
        workflowTaskCount={wfDel.workflowTaskCount}
        otherWorkflows={otherWorkflows}
        targetWorkflowId={wfDel.targetWorkflowId}
        setTargetWorkflowId={wfDel.setTargetWorkflowId}
        targetWorkflowSteps={wfDel.targetWorkflowSteps}
        targetStepId={wfDel.targetStepId}
        setTargetStepId={wfDel.setTargetStepId}
        migrateLoading={wfDel.migrateLoading}
        deleteLoading={deleteWorkflowLoading}
        onDelete={wfDeleteHandlers.handleDeleteWorkflow}
        onMigrateAndDelete={wfDeleteHandlers.handleMigrateAndDeleteWorkflow}
        hasUnsavedChanges={hasUnsavedChanges}
      />
      <WorkflowExportDialog
        open={exportOpen}
        onOpenChange={setExportOpen}
        title="Export Workflow"
        content={exportYaml}
      />
      <StepDeleteDialog
        open={stepDel.stepDeleteOpen}
        onOpenChange={stepDel.setStepDeleteOpen}
        stepName={stepToDeleteName}
        stepTaskCount={stepDel.stepTaskCount}
        stepsForMigration={stepsForStepMigration}
        targetStep={stepDel.targetStepForMigration}
        setTargetStep={stepDel.setTargetStepForMigration}
        loading={stepDel.stepMigrateLoading}
        pending={stepDel.stepDeletePending}
        onMigrateAndDelete={stepDeleteHandlers.handleMigrateAndDeleteStep}
        onDeleteAndTasks={stepDeleteHandlers.handleDeleteStepAndTasks}
        hasUnsavedChanges={hasUnsavedChanges}
      />
      <WorkflowCycleGuardDialog
        proposal={mutationGuard.proposal}
        onCancel={mutationGuard.cancelProposal}
        onConfirm={mutationGuard.confirmProposal}
      />
    </>
  );
}

function useWorkflowCardState(props: WorkflowCardProps) {
  const { workflow, initialWorkflowSteps, otherWorkflows = [] } = props;
  const { onDeleteWorkflow } = props;
  const { toast } = useToast();
  const [exportOpen, setExportOpen] = useState(false);
  const [exportYaml, setExportYaml] = useState("");
  const wfDel = useWorkflowDeleteState();
  const stepDel = useStepDeleteState();
  // Workflows synced from a configured GitHub repo are read-only: the
  // backend rejects definition mutations with a 409, so the UI disables the
  // matching affordances (name/agent-profile/steps/delete) up front.
  const readOnly = workflow.source === "github";
  const deleteWorkflowRequest = useRequest(onDeleteWorkflow);
  const {
    workflowSteps,
    setWorkflowSteps,
    savedWorkflowSteps,
    setSavedWorkflowSteps,
    workflowLoading,
    refreshWorkflowSteps,
  } = useWorkflowSteps(workflow.id, initialWorkflowSteps, toast);
  const isNewWorkflow = workflow.id.startsWith(TEMP_WORKFLOW_PREFIX);
  const mutationGuard = useWorkflowMutationGuard(workflowSteps);
  const stepActions = useWorkflowStepActions({
    workflow,
    isNewWorkflow,
    readOnly,
    workflowSteps,
    setWorkflowSteps,
    refreshWorkflowSteps,
    setStepToDelete: stepDel.setStepToDelete,
    setStepTaskCount: stepDel.setStepTaskCount,
    setTargetStepForMigration: stepDel.setTargetStepForMigration,
    setStepDeleteOpen: stepDel.setStepDeleteOpen,
    toast,
    mutationGuard,
  });
  const workflowDraft = useWorkflowDraftContributor({
    workflow,
    isWorkflowDirty: props.isWorkflowDirty,
    workflowSteps,
    savedWorkflowSteps,
    setWorkflowSteps,
    setSavedWorkflowSteps,
    mutationGuard,
    toast,
    onWorkflowSaved: props.onWorkflowSaved,
    onDiscardWorkflow: props.onDiscardWorkflow,
    onDeleteWorkflow: props.onDeleteWorkflow,
  });
  const wfDeleteHandlers = useWorkflowDeleteHandlers({
    workflow,
    readOnly,
    otherWorkflows,
    wfDel,
    deleteWorkflowRun: deleteWorkflowRequest.run,
    toast,
  });
  const stepDeleteHandlers = useStepDeleteHandlers({
    workflow,
    stepDel,
    refreshWorkflowSteps,
    runMutation: stepActions.runMutation,
  });
  const stepsForStepMigration = stepDel.stepToDelete
    ? workflowSteps.filter((s) => s.id !== stepDel.stepToDelete)
    : [];
  return {
    toast,
    exportOpen,
    setExportOpen,
    exportYaml,
    setExportYaml,
    wfDel,
    stepDel,
    readOnly,
    mutationGuard,
    deleteWorkflowRequest,
    workflowSteps,
    savedWorkflowSteps,
    workflowLoading,
    stepActions,
    wfDeleteHandlers,
    stepDeleteHandlers,
    stepsForStepMigration,
    ...workflowDraft,
  };
}

export function WorkflowCard(props: WorkflowCardProps) {
  const { workflow, savedWorkflow, otherWorkflows = [], onUpdateWorkflow } = props;
  const s = useWorkflowCardState(props);
  const visibleSavedSteps = savedWorkflow ? s.savedWorkflowSteps : [];

  return (
    <SettingsCard isDirty={s.hasUnsavedChanges} data-testid={`workflow-card-${workflow.id}`}>
      <CardContent className="pt-6">
        <div className="space-y-4">
          <WorkflowCardBody
            workflow={workflow}
            savedWorkflow={savedWorkflow}
            onUpdateWorkflow={onUpdateWorkflow}
            workflowLoading={s.workflowLoading}
            workflowSteps={s.workflowSteps}
            savedWorkflowSteps={visibleSavedSteps}
            diagnostics={s.mutationGuard.diagnostics}
            mutationPending={s.mutationGuard.isMutationPending}
            stepActions={s.stepActions}
            readOnly={s.readOnly}
          />
          <WorkflowCardHeaderActions
            workflowId={workflow.id}
            setExportYaml={s.setExportYaml}
            setExportOpen={s.setExportOpen}
            toast={s.toast}
            onDeleteClick={async () => {
              if (workflow.id.startsWith(TEMP_WORKFLOW_PREFIX)) await s.removeDraftWorkflow();
              else await s.wfDeleteHandlers.handleDeleteWorkflowClick();
            }}
            deleteDisabled={
              s.mutationGuard.isMutationPending ||
              s.deleteWorkflowRequest.isLoading ||
              s.wfDel.workflowDeleteLoading ||
              s.isRemovingDraft ||
              s.readOnly
            }
            readOnly={s.readOnly}
            exportDisabled={workflow.id.startsWith(TEMP_WORKFLOW_PREFIX)}
          />
        </div>
      </CardContent>
      <WorkflowCardDialogs
        wfDel={s.wfDel}
        otherWorkflows={otherWorkflows}
        deleteWorkflowLoading={s.deleteWorkflowRequest.isLoading}
        wfDeleteHandlers={s.wfDeleteHandlers}
        exportOpen={s.exportOpen}
        setExportOpen={s.setExportOpen}
        exportYaml={s.exportYaml}
        stepDel={s.stepDel}
        stepToDeleteName={
          s.workflowSteps.find((step) => step.id === s.stepDel.stepToDelete)?.name ?? "selected"
        }
        stepsForStepMigration={s.stepsForStepMigration}
        stepDeleteHandlers={s.stepDeleteHandlers}
        hasUnsavedChanges={s.hasUnsavedChanges}
        mutationGuard={s.mutationGuard}
      />
    </SettingsCard>
  );
}
