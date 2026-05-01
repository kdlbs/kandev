"use client";

import { useEffect } from "react";
import type {
  DialogFormState,
  TaskCreateDialogInitialValues,
} from "@/components/task-create-dialog-types";

type LockedFieldFormState = Pick<
  DialogFormState,
  | "selectedWorkflowId"
  | "setSelectedWorkflowId"
  | "repositoryId"
  | "setRepositoryId"
  | "branch"
  | "setBranch"
>;

// Pushes late-arriving locked field values into form state when async feature
// wrappers resolve them after the dialog is already open.
export function useLockedFieldSync(
  open: boolean,
  workflowId: string | null,
  initialValues: TaskCreateDialogInitialValues | undefined,
  fs: LockedFieldFormState,
) {
  const repoId = initialValues?.repositoryId;
  const branch = initialValues?.branch;
  useEffect(() => {
    if (!open) return;
    if (workflowId && workflowId !== fs.selectedWorkflowId) {
      fs.setSelectedWorkflowId(workflowId);
    }
    if (repoId && repoId !== fs.repositoryId) {
      fs.setRepositoryId(repoId);
    }
    if (branch && branch !== fs.branch) {
      fs.setBranch(branch);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, workflowId, repoId, branch]);
}
