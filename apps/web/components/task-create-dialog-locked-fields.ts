"use client";

import { useEffect } from "react";
import type {
  DialogFormState,
  TaskCreateDialogInitialValues,
} from "@/components/task-create-dialog-types";

type LockedFieldFormState = Pick<
  DialogFormState,
  "selectedWorkflowId" | "setSelectedWorkflowId" | "repositories" | "setRepositories"
>;

/**
 * Pushes late-arriving locked field values into form state when async feature
 * wrappers resolve them after the dialog is already open. Multi-repo: locks
 * are expressed as the first chip in `fs.repositories[]`; the dialog's
 * single-repo locking flow (Improve Kandev) overwrites `repositories[0]`.
 */
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
    if (!repoId) return;
    const current = fs.repositories[0];
    if (current?.repositoryId === repoId && current?.branch === (branch ?? "")) return;
    fs.setRepositories([{ key: "row-0", repositoryId: repoId, branch: branch ?? "" }]);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, workflowId, repoId, branch]);
}
