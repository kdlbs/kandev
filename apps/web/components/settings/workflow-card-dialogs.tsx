import { Button } from "@kandev/ui/button";
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
  hasUnsavedChanges: boolean;
};

function workflowDeleteDescription(taskCount: number | null, hasUnsavedChanges: boolean): string {
  const hasTasks = taskCount !== null && taskCount > 0;
  const base = hasTasks
    ? `This workflow has ${taskCount} task${taskCount === 1 ? "" : "s"}. Choose where to migrate them, or delete the workflow and archive the tasks.`
    : "This will permanently delete the workflow and all its steps.";
  return `${base}${hasUnsavedChanges ? " Unsaved workflow changes will be discarded." : ""}`;
}

export function WorkflowDeleteDialog({
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
  hasUnsavedChanges,
}: WorkflowDeleteDialogProps) {
  const hasTasks = workflowTaskCount !== null && workflowTaskCount > 0;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete workflow</DialogTitle>
          <DialogDescription>
            {workflowDeleteDescription(workflowTaskCount, hasUnsavedChanges)}
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
            {hasTasks ? "Delete & Archive Tasks" : "Delete Workflow"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

type StepDeleteDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  stepName: string;
  stepTaskCount: number | null;
  stepsForMigration: WorkflowStep[];
  targetStep: string;
  setTargetStep: (id: string) => void;
  loading: boolean;
  pending: boolean;
  onMigrateAndDelete: () => Promise<void>;
  onDeleteAndTasks: () => Promise<void>;
  hasUnsavedChanges: boolean;
};

export function StepDeleteDialog({
  open,
  onOpenChange,
  stepName,
  stepTaskCount,
  stepsForMigration,
  targetStep,
  setTargetStep,
  loading,
  pending,
  onMigrateAndDelete,
  onDeleteAndTasks,
  hasUnsavedChanges,
}: StepDeleteDialogProps) {
  const hasTasks = stepTaskCount !== null && stepTaskCount > 0;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete step</DialogTitle>
          <DialogDescription>
            {hasTasks
              ? `${stepName} has ${stepTaskCount} task${stepTaskCount === 1 ? "" : "s"}. Choose where to migrate them, or delete the step and its tasks.`
              : `This will permanently delete the ${stepName} workflow step.`}
            {hasUnsavedChanges ? " Unsaved step changes will be discarded." : ""}
          </DialogDescription>
        </DialogHeader>
        {stepsForMigration.length > 0 && (
          <div className="space-y-2 py-2">
            <Label>Target Step</Label>
            <Select value={targetStep} onValueChange={setTargetStep} disabled={loading || pending}>
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
        {pending && !loading && (
          <p className="text-sm text-muted-foreground" role="status">
            Waiting for the failed change to be retried.
          </p>
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
              disabled={!targetStep || loading || pending}
              className="cursor-pointer"
            >
              {loading ? "Migrating..." : "Migrate & Delete Step"}
            </Button>
          )}
          <Button
            type="button"
            variant="destructive"
            onClick={onDeleteAndTasks}
            disabled={loading || pending}
            className="cursor-pointer"
          >
            {hasTasks ? "Delete Step & Tasks" : "Delete Step"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
