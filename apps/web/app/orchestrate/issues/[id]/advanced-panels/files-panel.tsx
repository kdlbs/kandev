"use client";

import { IconFiles } from "@tabler/icons-react";

type AdvancedFilesPanelProps = {
  taskId: string;
};

export function AdvancedFilesPanel({ taskId: _taskId }: AdvancedFilesPanelProps) {
  return (
    <div className="flex flex-col items-center justify-center h-full min-h-[200px] p-4">
      <IconFiles className="h-8 w-8 text-muted-foreground/40 mb-3" />
      <p className="text-sm text-muted-foreground">Files</p>
      <p className="text-xs text-muted-foreground mt-1 text-center">
        Workspace files will appear here when an agent session is active.
      </p>
    </div>
  );
}
