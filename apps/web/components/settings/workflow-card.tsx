"use client";

import { useState, useEffect } from "react";
import { IconDownload, IconTrash } from "@tabler/icons-react";
import { Card, CardContent } from "@kandev/ui/card";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type { Workflow, WorkflowStep } from "@/lib/types/http";
import { useRequest } from "@/lib/http/use-request";
import { useToast } from "@/components/toast-provider";
import { WorkflowExportDialog } from "@/components/settings/workflow-export-dialog";
import { UnsavedChangesBadge, UnsavedSaveButton } from "@/components/settings/unsaved-indicator";
import { WorkflowPipelineEditor } from "@/components/settings/workflow-pipeline-editor";
import { listWorkflowStepsAction } from "@/app/actions/workspaces";
import {
  useWorkflowStepActions,
  useWorkflowDeleteHandlers,
  useStepDeleteHandlers,
  useWorkflowSaveActions,
  handleExportWorkflow,
} from "./workflow-card-actions";

type WorkflowCardProps = {
  workflow: Workflow;
  isWorkflowDirty: boolean;
  initialWorkflowSteps?: WorkflowStep[];
  otherWorkflows?: Workflow[];
  onUpdateWorkflow: (updates: { name?: string; description?: string }) => void;
  onDeleteWorkflow: () => Promise<unknown>;
  onSaveWorkflow: () => Promise<unknown>;
  onWorkflowCreated?: (created: Workflow) => void;
};

function useWorkflowSteps(
  workflowId: string,
  initialSteps: WorkflowStep[] | undefined,
  isNewWorkflow: boolean,
  toast: ReturnType<typeof useToast>["toast"],
) {
  const [workflowSteps, setWorkflowSteps] = useState<WorkflowStep[]>(initialSteps ?? []);
  const [workflowLoading, setWorkflowLoading] = useState(false);

  useEffect(() => {
    if (isNewWorkflow) {
      setWorkflowSteps(initialSteps ?? []);
      setWorkflowLoading(false);
      return;
    }
    let cancelled = false;
    const load = async () => {
      setWorkflowLoading(true);
      try {
        const res = await listWorkflowStepsAction(workflowId);
        if (!cancelled) setWorkflowSteps(res.steps ?? []);
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
  }, [workflowId, initialSteps, isNewWorkflow, toast]);

  const refreshWorkflowSteps = async () => {
    try {
      const res = await listWorkflowStepsAction(workflowId);
      setWorkflowSteps(res.steps ?? []);
    } catch {
      /* ignore */
    }
  };

  return { workflowSteps, setWorkflowSteps, workflowLoading, refreshWorkflowSteps };
}

type WorkflowDeleteDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workflowTaskCount: number | null;
  otherWorkflows: Workflow[];
  targetWorkflowId: string;
  setTargetWorkflowId: (id: string) => void;
  targetWorkflowSteps: WorkflowStep[];
  targetStepId: string;
  setTargetStepId: (id: string) => void;
  migrateLoading: boolean;
  deleteLoading: boolean;
  onDelete: () => Promise<void>;
  onMigrateAndDelete: () => Promise<void>;
};

function WorkflowDeleteDialog({
  open,
  onOpenChange,
  workflowTaskCount,
  otherWorkflows,
  targetWorkflowId,
  setTargetWorkflowId,
  targetWorkflowSteps,
  targetStepId,
  setTargetStepId,
  migrateLoading,
  deleteLoading,
  onDelete,
  onMigrateAndDelete,
}: WorkflowDeleteDialogProps) {
  const hasTasks = workflowTaskCount !== null && workflowTaskCount > 0;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete workflow</DialogTitle>
          <DialogDescription>
            {hasTasks
              ? `This workflow has ${workflowTaskCount} task${workflowTaskCount === 1 ? "" : "s"}. Choose where to migrate them, or delete everything.`
              : "This will permanently delete the workflow and all its steps."}
          </DialogDescription>
        </DialogHeader>
        {hasTasks && otherWorkflows.length > 0 && (
          <div className="space-y-3 py-2">
            <div className="space-y-2">
              <Label>Target Workflow</Label>
              <Select value={targetWorkflowId} onValueChange={setTargetWorkflowId}>
                <SelectTrigger>
                  <SelectValue placeholder="Select workflow" />
                </SelectTrigger>
                <SelectContent>
                  {otherWorkflows.map((w) => (
                    <SelectItem key={w.id} value={w.id}>
                      {w.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            {targetWorkflowSteps.length > 0 && (
              <div className="space-y-2">
                <Label>Target Step</Label>
                <Select value={targetStepId} onValueChange={setTargetStepId}>
                  <SelectTrigger>
                    <SelectValue placeholder="Select step" />
                  </SelectTrigger>
                  <SelectContent>
                    {targetWorkflowSteps.map((s) => (
                      <SelectItem key={s.id} value={s.id}>
                        {s.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            )}
          </div>
        )}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
            className="cursor-pointer"
          >
            Cancel
          </Button>
          {hasTasks && otherWorkflows.length > 0 && (
            <Button
              type="button"
              onClick={onMigrateAndDelete}
              disabled={!targetWorkflowId || !targetStepId || migrateLoading || deleteLoading}
              className="cursor-pointer"
            >
              {migrateLoading ? "Migrating..." : "Migrate & Delete"}
            </Button>
          )}
          <Button
            type="button"
            variant="destructive"
            onClick={onDelete}
            disabled={deleteLoading || migrateLoading}
            className="cursor-pointer"
          >
            {hasTasks ? "Delete Everything" : "Delete Workflow"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

type StepDeleteDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  stepTaskCount: number | null;
  stepsForMigration: WorkflowStep[];
  targetStep: string;
  setTargetStep: (id: string) => void;
  loading: boolean;
  onMigrateAndDelete: () => Promise<void>;
  onDeleteAndTasks: () => Promise<void>;
};

function StepDeleteDialog({
  open,
  onOpenChange,
  stepTaskCount,
  stepsForMigration,
  targetStep,
  setTargetStep,
  loading,
  onMigrateAndDelete,
  onDeleteAndTasks,
}: StepDeleteDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete step</DialogTitle>
          <DialogDescription>
            This step has {stepTaskCount} task{stepTaskCount === 1 ? "" : "s"}.
            {stepsForMigration.length > 0
              ? " Choose where to migrate them, or delete the step and its tasks."
              : " Deleting this step will affect these tasks."}
          </DialogDescription>
        </DialogHeader>
        {stepsForMigration.length > 0 && (
          <div className="space-y-2 py-2">
            <Label>Target Step</Label>
            <Select value={targetStep} onValueChange={setTargetStep}>
              <SelectTrigger>
                <SelectValue placeholder="Select step" />
              </SelectTrigger>
              <SelectContent>
                {stepsForMigration.map((s) => (
                  <SelectItem key={s.id} value={s.id}>
                    {s.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
            className="cursor-pointer"
          >
            Cancel
          </Button>
          {stepsForMigration.length > 0 && (
            <Button
              type="button"
              onClick={onMigrateAndDelete}
              disabled={!targetStep || loading}
              className="cursor-pointer"
            >
              {loading ? "Migrating..." : "Migrate & Delete Step"}
            </Button>
          )}
          <Button
            type="button"
            variant="destructive"
            onClick={onDeleteAndTasks}
            disabled={loading}
            className="cursor-pointer"
          >
            Delete Step & Tasks
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
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
};

function useStepDeleteState(): StepDeleteState {
  const [stepDeleteOpen, setStepDeleteOpen] = useState(false);
  const [stepToDelete, setStepToDelete] = useState<string | null>(null);
  const [stepTaskCount, setStepTaskCount] = useState<number | null>(null);
  const [targetStepForMigration, setTargetStepForMigration] = useState<string>("");
  const [stepMigrateLoading, setStepMigrateLoading] = useState(false);
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
  };
}

export function WorkflowCard({
  workflow,
  isWorkflowDirty,
  initialWorkflowSteps,
  otherWorkflows = [],
  onUpdateWorkflow,
  onDeleteWorkflow,
  onSaveWorkflow,
  onWorkflowCreated,
}: WorkflowCardProps) {
  const { toast } = useToast();
  const [exportOpen, setExportOpen] = useState(false);
  const [exportJson, setExportJson] = useState("");
  const wfDel = useWorkflowDeleteState();
  const stepDel = useStepDeleteState();
  const isNewWorkflow = workflow.id.startsWith("temp-");
  const deleteWorkflowRequest = useRequest(onDeleteWorkflow);
  const { workflowSteps, setWorkflowSteps, workflowLoading, refreshWorkflowSteps } =
    useWorkflowSteps(workflow.id, initialWorkflowSteps, isNewWorkflow, toast);
  const stepActions = useWorkflowStepActions({
    workflow, isNewWorkflow, workflowSteps, setWorkflowSteps, refreshWorkflowSteps,
    setStepToDelete: stepDel.setStepToDelete, setStepTaskCount: stepDel.setStepTaskCount,
    setTargetStepForMigration: stepDel.setTargetStepForMigration,
    setStepDeleteOpen: stepDel.setStepDeleteOpen, toast,
  });
  const { activeSaveRequest, handleSaveWorkflow } = useWorkflowSaveActions({
    workflow, isNewWorkflow, workflowSteps, onSaveWorkflow, onWorkflowCreated, toast,
  });
  const wfDeleteHandlers = useWorkflowDeleteHandlers({
    workflow, isNewWorkflow, otherWorkflows, wfDel,
    deleteWorkflowRun: deleteWorkflowRequest.run, toast,
  });
  const stepDeleteHandlers = useStepDeleteHandlers({ workflow, stepDel, refreshWorkflowSteps, toast });
  const stepsForStepMigration = stepDel.stepToDelete
    ? workflowSteps.filter((s) => s.id !== stepDel.stepToDelete)
    : [];

  return (
    <Card>
      <CardContent className="pt-6">
        <div className="space-y-4">
          <div className="flex items-center justify-between gap-3">
            <div className="space-y-2 flex-1">
              <Label className="flex items-center gap-2">
                <span>Workflow Name</span>
                {isWorkflowDirty && <UnsavedChangesBadge />}
              </Label>
              <div className="flex items-center gap-2">
                <Input value={workflow.name} onChange={(e) => onUpdateWorkflow({ name: e.target.value })} />
                <UnsavedSaveButton isDirty={isWorkflowDirty} isLoading={activeSaveRequest.isLoading} status={activeSaveRequest.status} onClick={handleSaveWorkflow} />
              </div>
            </div>
          </div>
          <div className="space-y-2">
            <Label>Workflow Steps</Label>
            {workflowLoading ? (
              <div className="text-sm text-muted-foreground">Loading workflow steps...</div>
            ) : (
              <WorkflowPipelineEditor steps={workflowSteps} onUpdateStep={stepActions.handleUpdateWorkflowStep} onAddStep={stepActions.handleAddWorkflowStep} onRemoveStep={stepActions.handleRemoveWorkflowStep} onReorderSteps={stepActions.handleReorderWorkflowSteps} />
            )}
          </div>
          <div className="flex justify-end gap-2">
            {!isNewWorkflow && (
              <Button type="button" variant="outline" onClick={() => handleExportWorkflow({ workflowId: workflow.id, setExportJson, setExportOpen, toast })} className="cursor-pointer">
                <IconDownload className="h-4 w-4 mr-2" />Export
              </Button>
            )}
            <Button type="button" variant="destructive" onClick={wfDeleteHandlers.handleDeleteWorkflowClick} disabled={deleteWorkflowRequest.isLoading || wfDel.workflowDeleteLoading} className="cursor-pointer">
              <IconTrash className="h-4 w-4 mr-2" />Delete Workflow
            </Button>
          </div>
        </div>
      </CardContent>
      <WorkflowDeleteDialog open={wfDel.deleteOpen} onOpenChange={wfDel.setDeleteOpen} workflowTaskCount={wfDel.workflowTaskCount} otherWorkflows={otherWorkflows} targetWorkflowId={wfDel.targetWorkflowId} setTargetWorkflowId={wfDel.setTargetWorkflowId} targetWorkflowSteps={wfDel.targetWorkflowSteps} targetStepId={wfDel.targetStepId} setTargetStepId={wfDel.setTargetStepId} migrateLoading={wfDel.migrateLoading} deleteLoading={deleteWorkflowRequest.isLoading} onDelete={wfDeleteHandlers.handleDeleteWorkflow} onMigrateAndDelete={wfDeleteHandlers.handleMigrateAndDeleteWorkflow} />
      <WorkflowExportDialog open={exportOpen} onOpenChange={setExportOpen} title="Export Workflow" json={exportJson} />
      <StepDeleteDialog open={stepDel.stepDeleteOpen} onOpenChange={stepDel.setStepDeleteOpen} stepTaskCount={stepDel.stepTaskCount} stepsForMigration={stepsForStepMigration} targetStep={stepDel.targetStepForMigration} setTargetStep={stepDel.setTargetStepForMigration} loading={stepDel.stepMigrateLoading} onMigrateAndDelete={stepDeleteHandlers.handleMigrateAndDeleteStep} onDeleteAndTasks={stepDeleteHandlers.handleDeleteStepAndTasks} />
    </Card>
  );
}
