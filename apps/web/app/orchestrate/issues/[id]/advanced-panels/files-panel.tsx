"use client";

import { IconFiles } from "@tabler/icons-react";

type AdvancedFilesPanelProps = {
  taskId: string;
  hasActiveSession: boolean;
};

export function AdvancedFilesPanel({
  taskId: _taskId,
  hasActiveSession,
}: AdvancedFilesPanelProps) {
  if (!hasActiveSession) {
    return (
      <div className="flex flex-col items-center justify-center h-full min-h-[200px] p-4">
        <IconFiles className="h-8 w-8 text-muted-foreground/40 mb-3" />
        <p className="text-sm text-muted-foreground">No active session</p>
        <p className="text-xs text-muted-foreground mt-1 text-center">
          Start an agent session to browse workspace files.
        </p>
      </div>
    );
  }

  return (
    <div className="flex flex-col items-center justify-center h-full min-h-[200px] p-4">
      <IconFiles className="h-8 w-8 text-muted-foreground/40 mb-3" />
      <p className="text-sm text-muted-foreground">File browser</p>
      <p className="text-xs text-muted-foreground mt-1 text-center">
        File browser available when agent session is active.
      </p>
    </div>
  );
}
