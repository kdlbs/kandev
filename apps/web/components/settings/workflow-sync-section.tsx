"use client";

import { IconBrandGithub } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { WorkflowSyncDialog } from "@/components/settings/workflow-sync-dialog";
import { WorkflowSyncStatusCard } from "@/components/settings/workflow-sync-status-banner";
import { useWorkflowSync } from "@/hooks/domains/settings/use-workflow-sync";

// WorkflowSyncButton is the GitHub Sync entry point, rendered alongside the
// other workflow actions (Export / Import / Add). The dialog open state lives
// with the caller so button and section can sit in different parts of the
// layout.
export function WorkflowSyncButton({ onClick }: { onClick: () => void }) {
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      onClick={onClick}
      className="cursor-pointer"
      data-testid="workflow-sync-open"
    >
      <IconBrandGithub className="h-4 w-4 mr-2" />
      GitHub Sync
    </Button>
  );
}

type WorkflowSyncSectionProps = {
  workspaceId: string;
  dialogOpen: boolean;
  onDialogOpenChange: (open: boolean) => void;
};

// WorkflowSyncSection renders the GitHub-sync state inside the Workflows
// settings section: the configuration dialog (opened via WorkflowSyncButton
// in the section's action row) and — once a sync is configured — a compact
// status card above the workflow list showing what is syncing and how the
// last attempt went. Unconfigured workspaces render nothing visible.
export function WorkflowSyncSection({
  workspaceId,
  dialogOpen,
  onDialogOpenChange,
}: WorkflowSyncSectionProps) {
  const sync = useWorkflowSync(workspaceId);

  return (
    <>
      {sync.config && (
        <div className="mb-4" data-testid="workflow-sync-section">
          <WorkflowSyncStatusCard
            config={sync.config}
            syncing={sync.syncing}
            onSyncNow={sync.handleSyncNow}
          />
        </div>
      )}
      <WorkflowSyncDialog open={dialogOpen} onOpenChange={onDialogOpenChange} sync={sync} />
    </>
  );
}
