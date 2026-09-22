"use client";

import { t } from "@/lib/i18n";
import { useToast } from "@/components/toast-provider";
import { exportWorkflowAction } from "@/app/actions/workspaces";

type WorkflowExportActionsParams = {
  workflowId: string;
  setExportYaml: (yaml: string) => void;
  setExportOpen: (open: boolean) => void;
  toast: ReturnType<typeof useToast>["toast"];
};

export async function handleExportWorkflow({
  workflowId,
  setExportYaml,
  setExportOpen,
  toast,
}: WorkflowExportActionsParams) {
  try {
    const yamlText = await exportWorkflowAction(workflowId);
    setExportYaml(yamlText);
    setExportOpen(true);
  } catch (error) {
    toast({
      title: t("workflows:failedToExportWorkflow"),
      description: error instanceof Error ? error.message : t("common:requestFailed"),
      variant: "error",
    });
  }
}
