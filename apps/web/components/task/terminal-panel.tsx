"use client";

import { memo } from "react";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { PassthroughTerminal } from "./passthrough-terminal";
import { ShellTerminal } from "./shell-terminal";
import { useAppStore } from "@/components/state-provider";
import { useIsTaskArchived, ArchivedPanelPlaceholder } from "./task-archived-context";
import { useEnvironmentSessionId } from "@/hooks/use-environment-session-id";

type TerminalPanelProps = {
  panelId: string;
  params: Record<string, unknown>;
};

export const TerminalPanel = memo(function TerminalPanel({ params }: TerminalPanelProps) {
  const terminalId = params.terminalId as string;
  const type = (params.type as string) ?? "shell";
  const processId = params.processId as string | undefined;

  // Only reconnect the terminal when the environment changes, not on every
  // session tab switch within the same environment.
  const sessionId = useEnvironmentSessionId();

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
