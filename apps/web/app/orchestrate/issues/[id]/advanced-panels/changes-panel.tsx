"use client";

import { IconGitBranch } from "@tabler/icons-react";

type AdvancedChangesPanelProps = {
  taskId: string;
};

export function AdvancedChangesPanel({ taskId: _taskId }: AdvancedChangesPanelProps) {
  return (
    <div className="flex flex-col items-center justify-center h-full min-h-[200px] p-4">
      <IconGitBranch className="h-8 w-8 text-muted-foreground/40 mb-3" />
      <p className="text-sm text-muted-foreground">Changes</p>
      <p className="text-xs text-muted-foreground mt-1 text-center">
        Git changes and diffs will appear here when an agent session is active.
      </p>
    </div>
  );
}
