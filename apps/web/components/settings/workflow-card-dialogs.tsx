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
};

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

export function StepDeleteDialog({
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
