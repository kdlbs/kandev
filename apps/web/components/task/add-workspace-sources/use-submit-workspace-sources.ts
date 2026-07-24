import { useCallback, type Dispatch, type SetStateAction } from "react";
import { flushSync } from "react-dom";
import { attachTaskWorkspaceSources } from "@/lib/api/domains/kanban-api";
import {
  buildWorkspaceSourcesPayload,
  type WorkspaceSourceRow,
} from "@/components/workspace-source-picker/workspace-source-state";

type Props = {
  errors: Record<string, string>;
  onOpenChange: (open: boolean) => void;
  onSuccess: () => void;
  reconcileWorkspaceSourcesAdopted: (sessionIds: string[]) => void;
  rows: WorkspaceSourceRow[];
  setSubmitting: Dispatch<SetStateAction<boolean>>;
  setSubmitError: Dispatch<SetStateAction<string | null>>;
  submitting: boolean;
  taskId: string;
};

export function useSubmitWorkspaceSources({
  errors,
  onOpenChange,
  onSuccess,
  reconcileWorkspaceSourcesAdopted,
  rows,
  setSubmitting,
  setSubmitError,
  submitting,
  taskId,
}: Props) {
  return useCallback(async () => {
    if (submitting) return;
    if (rows.length === 0) return setSubmitError("Add at least one source.");
    if (Object.keys(errors).length)
      return setSubmitError("Fix the marked sources before adding them.");

    setSubmitting(true);
    setSubmitError(null);
    try {
      const result = await attachTaskWorkspaceSources(taskId, buildWorkspaceSourcesPayload(rows));
      flushSync(() => {
        reconcileWorkspaceSourcesAdopted(result.adopted_session_ids ?? result.session_ids);
        onSuccess();
      });
      onOpenChange(false);
    } catch (error) {
      setSubmitError(
        error instanceof Error
          ? error.message
          : "Could not add sources. Your entries are still here to retry.",
      );
    } finally {
      setSubmitting(false);
    }
  }, [
    errors,
    onOpenChange,
    onSuccess,
    reconcileWorkspaceSourcesAdopted,
    rows,
    setSubmitting,
    setSubmitError,
    submitting,
    taskId,
  ]);
}
