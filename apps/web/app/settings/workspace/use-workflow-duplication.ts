"use client";

import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { listWorkflowStepsAction } from "@/app/actions/workspaces";
import type { Workflow, WorkflowStep } from "@/lib/types/http";
import { useToast } from "@/components/toast-provider";

type UseWorkflowDuplicationArgs = {
  workflow: Workflow;
  hasUnsavedChanges: boolean;
  mutationPending: boolean;
  isImproveWorkspace?: boolean;
  onDuplicateWorkflow: (steps: WorkflowStep[]) => void;
  toast: ReturnType<typeof useToast>["toast"];
};

export function useWorkflowDuplication({
  workflow,
  hasUnsavedChanges,
  mutationPending,
  isImproveWorkspace,
  onDuplicateWorkflow,
  toast,
}: UseWorkflowDuplicationArgs) {
  const { t } = useTranslation();
  const [duplicateLoading, setDuplicateLoading] = useState(false);
  const duplicateLoadingRef = useRef(false);
  let duplicateDisabledReason: string | undefined;

  if (isImproveWorkspace) {
    duplicateDisabledReason = t("workflows:duplicateUnavailableInImproveKandev");
  } else if (workflow.id.startsWith("temp-workflow-") || hasUnsavedChanges) {
    duplicateDisabledReason = t("workflows:saveBeforeDuplicating");
  } else if (mutationPending) {
    duplicateDisabledReason = t("workflows:workflowMutationInProgress");
  }

  const duplicateDisabled = duplicateLoading || duplicateDisabledReason !== undefined;

  const handleDuplicateWorkflow = async () => {
    if (duplicateLoadingRef.current || duplicateDisabled) return;
    duplicateLoadingRef.current = true;
    setDuplicateLoading(true);
    try {
      const result = await listWorkflowStepsAction(workflow.id);
      onDuplicateWorkflow(result.steps ?? []);
    } catch (error) {
      toast({
        title: t("workflows:failedToDuplicateWorkflow"),
        description: error instanceof Error ? error.message : t("common:requestFailed"),
        variant: "error",
      });
    } finally {
      duplicateLoadingRef.current = false;
      setDuplicateLoading(false);
    }
  };

  return {
    duplicateDisabled,
    duplicateDisabledReason,
    duplicateLoading,
    handleDuplicateWorkflow,
  };
}
