"use client";

import { memo, useState } from "react";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { PassthroughTerminal } from "./passthrough-terminal";
import { ShellTerminal } from "./shell-terminal";
import { useAppStore } from "@/components/state-provider";
import { useIsTaskArchived, ArchivedPanelPlaceholder } from "./task-archived-context";

type TerminalPanelProps = {
  panelId: string;
  params: Record<string, unknown>;
};

export const TerminalPanel = memo(function TerminalPanel({ params }: TerminalPanelProps) {
  const terminalId = params.terminalId as string;
  const type = (params.type as string) ?? "shell";
  const processId = params.processId as string | undefined;

  // Capture the session ID at creation time — terminal stays connected
  // to its original session even when the user switches tasks
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const [sessionId] = useState(() => activeSessionId);

  const devOutput = useAppStore((state) =>
    processId ? (state.processes.outputsByProcessId[processId] ?? "") : "",
  );
  const devProcess = useAppStore((state) =>
    processId ? state.processes.processesById[processId] : undefined,
  );
  const isStopping = devProcess?.status === "stopping";
  const isArchived = useIsTaskArchived();

  if (isArchived)
    return <ArchivedPanelPlaceholder message="Terminal not available — this task is archived" />;

  if (type === "dev-server" && processId) {
    return (
      <PanelRoot data-testid="terminal-panel">
        <PanelBody padding={false} scroll={false}>
          <ShellTerminal processOutput={devOutput} processId={processId} isStopping={isStopping} />
        </PanelBody>
      </PanelRoot>
    );
  }

  return (
    <PanelRoot data-testid="terminal-panel">
      <PanelBody padding={false} scroll={false}>
        <PassthroughTerminal
          sessionId={sessionId ?? undefined}
          mode="shell"
          terminalId={terminalId}
        />
      </PanelBody>
    </PanelRoot>
  );
});
