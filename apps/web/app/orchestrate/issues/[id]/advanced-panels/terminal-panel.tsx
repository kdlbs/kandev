"use client";

import { IconTerminal2 } from "@tabler/icons-react";

type AdvancedTerminalPanelProps = {
  hasActiveSession: boolean;
};

export function AdvancedTerminalPanel({ hasActiveSession }: AdvancedTerminalPanelProps) {
  if (!hasActiveSession) {
    return (
      <div className="flex flex-col items-center justify-center h-full bg-muted/20">
        <IconTerminal2 className="h-8 w-8 text-muted-foreground/40 mb-3" />
        <p className="text-sm text-muted-foreground">No active session</p>
        <p className="text-xs text-muted-foreground mt-1">
          Start an agent session to access the terminal.
        </p>
      </div>
    );
  }

  return (
    <div className="flex flex-col items-center justify-center h-full bg-muted/20">
      <IconTerminal2 className="h-8 w-8 text-muted-foreground/40 mb-3" />
      <p className="text-sm text-muted-foreground">Terminal available when agent is running</p>
      <p className="text-xs text-muted-foreground mt-1">
        Terminal access requires a Docker or remote executor.
      </p>
    </div>
  );
}
