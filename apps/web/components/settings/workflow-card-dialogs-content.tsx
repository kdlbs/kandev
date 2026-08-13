"use client";

import { useTranslation } from "react-i18next";
import type { Workflow, WorkflowStep } from "@/lib/types/http";
import { WorkflowExportDialog } from "@/components/settings/workflow-export-dialog";
import { WorkflowCycleGuardDialog } from "./workflow-cycle-diagnostic";
import { WorkflowDeleteDialog, StepDeleteDialog } from "./workflow-card-dialogs";
import type { WorkflowMutationGuardController } from "./workflow-mutation-guard";

export type WorkflowDeleteState = {
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

export type StepDeleteState = {
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
  mutationGuard: WorkflowMutationGuardController;
};

export function WorkflowCardDialogs({
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
  const { t } = useTranslation();
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
        title={t("workflows:exportWorkflowTitle")}
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
