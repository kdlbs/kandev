"use client";

import { useState } from "react";
import { IconBrandGithub } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { WorkflowSyncDialog } from "@/components/settings/workflow-sync-dialog";
import { WorkflowSyncStatusCard } from "@/components/settings/workflow-sync-status-banner";
import { useWorkflowSync } from "@/hooks/domains/settings/use-workflow-sync";

// WorkflowSyncSection is the collapsed GitHub-sync entry point on the
// workspace Workflows settings page: a single "GitHub Sync" button that opens
// the configuration dialog, plus — once a sync is configured — a compact
// status card showing what is syncing and how the last attempt went.
export function WorkflowSyncSection({ workspaceId }: { workspaceId: string }) {
  const sync = useWorkflowSync(workspaceId);
  const [dialogOpen, setDialogOpen] = useState(false);

  return (
    <div className="space-y-3" data-testid="workflow-sync-section">
      <div className="flex items-center">
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => setDialogOpen(true)}
          disabled={sync.loading}
          className="cursor-pointer"
          data-testid="workflow-sync-open"
        >
          <IconBrandGithub className="h-4 w-4 mr-2" />
          GitHub Sync
        </Button>
      </div>
      {sync.config && (
        <WorkflowSyncStatusCard
          config={sync.config}
          syncing={sync.syncing}
          onSyncNow={sync.handleSyncNow}
        />
      )}
      <WorkflowSyncDialog open={dialogOpen} onOpenChange={setDialogOpen} sync={sync} />
    </div>
  );
}
