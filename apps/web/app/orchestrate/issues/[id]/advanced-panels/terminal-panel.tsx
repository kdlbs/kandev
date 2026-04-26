"use client";

import { IconTerminal2 } from "@tabler/icons-react";

type AdvancedTerminalPanelProps = {
  taskId: string;
};

export function AdvancedTerminalPanel({ taskId: _taskId }: AdvancedTerminalPanelProps) {
  return (
    <div className="flex flex-col items-center justify-center h-full bg-muted/20">
      <IconTerminal2 className="h-8 w-8 text-muted-foreground/40 mb-3" />
      <p className="text-sm text-muted-foreground">Terminal</p>
      <p className="text-xs text-muted-foreground mt-1">
        Terminal access will be available when an agent session is active.
      </p>
    </div>
  );
}
