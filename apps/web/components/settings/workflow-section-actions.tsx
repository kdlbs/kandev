"use client";

import { IconDownload, IconPlus, IconUpload } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { WorkflowSyncButton } from "@/components/settings/workflow-sync-section";

type WorkflowSectionActionsProps = {
  onExport: () => void;
  onImport: () => void;
  onAdd: () => void;
  onGitHubSync: () => void;
};

// WorkflowSectionActions is the toolbar of the Workflows settings section:
// GitHub Sync, Export All, Import, and Add Workflow.
export function WorkflowSectionActions({
  onExport,
  onImport,
  onAdd,
  onGitHubSync,
}: WorkflowSectionActionsProps) {
  return (
    // sm:justify-end keeps wrapped rows right-aligned next to the section
    // title; below sm the toolbar sits under the title, left-aligned.
    <div className="flex flex-wrap gap-2 sm:justify-end">
      <WorkflowSyncButton onClick={onGitHubSync} />
      <Button
        type="button"
        size="sm"
        variant="outline"
        onClick={onExport}
        className="cursor-pointer"
      >
        <IconDownload className="h-4 w-4 mr-2" />
        Export All
      </Button>
      <Button
        type="button"
        size="sm"
        variant="outline"
        onClick={onImport}
        className="cursor-pointer"
      >
        <IconUpload className="h-4 w-4 mr-2" />
        Import
      </Button>
      <Button
        type="button"
        size="sm"
        onClick={onAdd}
        className="cursor-pointer"
        data-testid="add-workflow-button"
      >
        <IconPlus className="h-4 w-4 mr-2" />
        Add Workflow
      </Button>
    </div>
  );
}
