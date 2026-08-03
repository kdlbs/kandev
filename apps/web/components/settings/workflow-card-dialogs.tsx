import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
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

function workflowDeleteDescription(
  t: TFunction,
  taskCount: number | null,
  hasUnsavedChanges: boolean,
): string {
  const hasTasks = taskCount !== null && taskCount > 0;
  const base = hasTasks
    ? t("workflows:deleteWorkflowWithTasks", { count: taskCount })
    : t("workflows:deleteWorkflowNoTasks");
  if (!hasUnsavedChanges) return base;
  return `${base} ${t("workflows:unsavedWorkflowChangesDiscarded")}`;
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
  const { t } = useTranslation();
  const hasTasks = workflowTaskCount !== null && workflowTaskCount > 0;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("workflows:deleteWorkflowDialogTitle")}</DialogTitle>
          <DialogDescription>
            {workflowDeleteDescription(t, workflowTaskCount, hasUnsavedChanges)}
          </DialogDescription>
        </DialogHeader>
        {hasTasks && otherWorkflows.length > 0 && (
          <div className="space-y-3 py-2">
            <div className="space-y-2">
              <Label>{t("workflows:targetWorkflow")}</Label>
              <Select value={targetWorkflowId} onValueChange={setTargetWorkflowId}>
                <SelectTrigger>
                  <SelectValue placeholder={t("workflows:selectWorkflow")} />
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
                <Label>{t("workflows:targetStep")}</Label>
                <Select value={targetStepId} onValueChange={setTargetStepId}>
                  <SelectTrigger>
                    <SelectValue placeholder={t("workflows:selectStep")} />
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
            {t("common:cancel")}
          </Button>
          {hasTasks && otherWorkflows.length > 0 && (
            <Button
              type="button"
              onClick={onMigrateAndDelete}
              disabled={!targetWorkflowId || !targetStepId || migrateLoading || deleteLoading}
              className="cursor-pointer"
            >
              {migrateLoading ? t("workflows:migrating") : t("workflows:migrateAndDelete")}
            </Button>
          )}
          <Button
            type="button"
            variant="destructive"
            onClick={onDelete}
            disabled={deleteLoading || migrateLoading}
            className="cursor-pointer"
          >
            {hasTasks ? t("workflows:deleteAndArchiveTasks") : t("workflows:deleteWorkflow")}
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

function stepDeleteDescription(
  t: TFunction,
  stepName: string,
  stepTaskCount: number | null,
  hasMigrationTarget: boolean,
) {
  // `stepName` is user data (steps are renamed freely), so it always travels as
  // an interpolated value and never as part of the message.
  if (!stepTaskCount) return t("workflows:deleteStepNoTasks", { stepName });
  if (hasMigrationTarget) {
    return t("workflows:deleteStepWithMigration", { stepName, count: stepTaskCount });
  }
  return t("workflows:deleteStepAffectsTasks", { stepName, count: stepTaskCount });
}

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
  const { t } = useTranslation();
  const hasTasks = stepTaskCount !== null && stepTaskCount > 0;
  const baseDescription = stepDeleteDescription(
    t,
    stepName,
    stepTaskCount,
    stepsForMigration.length > 0,
  );
  const description = hasUnsavedChanges
    ? `${baseDescription} ${t("workflows:unsavedStepChangesDiscarded")}`
    : baseDescription;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("workflows:deleteStepDialogTitle")}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        {stepsForMigration.length > 0 && (
          <div className="space-y-2 py-2">
            <Label>{t("workflows:targetStep")}</Label>
            <Select value={targetStep} onValueChange={setTargetStep} disabled={loading || pending}>
              <SelectTrigger>
                <SelectValue placeholder={t("workflows:selectStep")} />
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
            {t("workflows:waitingForRetry")}
          </p>
        )}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
            className="cursor-pointer"
          >
            {t("common:cancel")}
          </Button>
          {stepsForMigration.length > 0 && (
            <Button
              type="button"
              onClick={onMigrateAndDelete}
              disabled={!targetStep || loading || pending}
              className="cursor-pointer"
            >
              {loading ? t("workflows:migrating") : t("workflows:migrateAndDeleteStep")}
            </Button>
          )}
          <Button
            type="button"
            variant="destructive"
            onClick={onDeleteAndTasks}
            disabled={loading || pending}
            className="cursor-pointer"
          >
            {hasTasks ? t("workflows:deleteStepAndTasks") : t("workflows:deleteStep")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
