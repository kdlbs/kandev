"use client";

import { memo } from "react";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { PassthroughTerminal } from "./passthrough-terminal";
import { ShellTerminal } from "./shell-terminal";
import { useAppStore } from "@/components/state-provider";
import { useIsTaskArchived, ArchivedPanelPlaceholder } from "./task-archived-context";
import { useEnvironmentId } from "@/hooks/use-environment-session-id";
import { DEV_SERVER_PANEL_ID } from "@/lib/state/layout-manager/constants";
import { useTranslation } from "react-i18next";

type TerminalPanelProps = {
  panelId: string;
  params: Record<string, unknown>;
};

export const TerminalPanel = memo(function TerminalPanel({ params }: TerminalPanelProps) {
  const { t } = useTranslation();
  const terminalId = params.terminalId as string;
  const type = (params.type as string) ?? "shell";
  const isDevServer = type === DEV_SERVER_PANEL_ID;

  const environmentId = useEnvironmentId();

  // The dev process is restarted under a new id on every start, so the store is
  // the authoritative source and a `processId` in the panel params is ignored:
  // a stale one would pin the panel to a process that no longer exists, showing
  // neither the restarted server's output nor its stopping state.
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const storeDevProcessId = useAppStore((state) =>
    activeSessionId ? state.processes.devProcessBySessionId[activeSessionId] : undefined,
  );
  const processId = isDevServer ? storeDevProcessId : undefined;

  const devOutput = useAppStore((state) =>
    processId ? (state.processes.outputsByProcessId[processId] ?? "") : "",
  );
  const devProcess = useAppStore((state) =>
    processId ? state.processes.processesById[processId] : undefined,
  );
  const isStopping = devProcess?.status === "stopping";
  const isArchived = useIsTaskArchived();

  if (isArchived)
    return <ArchivedPanelPlaceholder message={t("task:terminalNotAvailableThisTaskIs")} />;

  if (isDevServer) {
    return (
      <PanelRoot data-testid="dev-server-panel">
        <PanelBody padding={false} scroll={false}>
          <ShellTerminal
            processOutput={devOutput}
            processId={processId ?? null}
            isStopping={isStopping}
          />
        </PanelBody>
      </PanelRoot>
    );
  }

  return (
    <PanelRoot data-testid="terminal-panel">
      <PanelBody padding={false} scroll={false}>
        <PassthroughTerminal mode="shell" environmentId={environmentId} terminalId={terminalId} />
      </PanelBody>
    </PanelRoot>
  );
});
