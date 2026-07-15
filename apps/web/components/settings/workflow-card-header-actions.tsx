"use client";

import { IconDownload, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useToast } from "@/components/toast-provider";
import { handleExportWorkflow } from "./workflow-card-actions";

type WorkflowCardHeaderActionsProps = {
  workflowId: string;
  setExportYaml: (json: string) => void;
  setExportOpen: (open: boolean) => void;
  toast: ReturnType<typeof useToast>["toast"];
  onDeleteClick: () => Promise<void>;
  deleteDisabled: boolean;
  exportDisabled: boolean;
  readOnly: boolean;
};

const SYNCED_READ_ONLY_REASON =
  "Managed by workflow sync — edit or remove it in the synced repository";

export function WorkflowCardHeaderActions({
  workflowId,
  setExportYaml,
  setExportOpen,
  toast,
  onDeleteClick,
  deleteDisabled,
  exportDisabled,
  readOnly,
}: WorkflowCardHeaderActionsProps) {
  return (
    <div className="flex flex-wrap justify-end gap-2">
      <Tooltip>
        <TooltipTrigger asChild>
          <span
            tabIndex={exportDisabled ? 0 : undefined}
            aria-label={exportDisabled ? "Save the workflow before exporting." : undefined}
          >
            <Button
              type="button"
              variant="outline"
              onClick={() =>
                handleExportWorkflow({ workflowId, setExportYaml, setExportOpen, toast })
              }
              className="cursor-pointer"
              disabled={exportDisabled}
            >
              <IconDownload className="h-4 w-4 mr-2" />
              Export
            </Button>
          </span>
        </TooltipTrigger>
        {exportDisabled && <TooltipContent>Save the workflow before exporting.</TooltipContent>}
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <span tabIndex={readOnly ? 0 : undefined} className="inline-flex">
            <Button
              type="button"
              variant="destructive"
              onClick={() => {
                void onDeleteClick().catch((error) => {
                  toast({
                    title: "Failed to delete workflow",
                    description: error instanceof Error ? error.message : "Request failed",
                    variant: "error",
                  });
                });
              }}
              disabled={deleteDisabled}
              className="cursor-pointer"
              data-testid="delete-workflow-button"
            >
              <IconTrash className="h-4 w-4 mr-2" />
              Delete Workflow
            </Button>
          </span>
        </TooltipTrigger>
        {readOnly && <TooltipContent>{SYNCED_READ_ONLY_REASON}</TooltipContent>}
      </Tooltip>
    </div>
  );
}
