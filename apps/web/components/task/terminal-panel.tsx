"use client";

import { memo } from "react";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { PassthroughTerminal } from "./passthrough-terminal";
import { ShellTerminal } from "./shell-terminal";
import { PanelLoadingState } from "@/components/panel-loading-state";
import { useAppStore } from "@/components/state-provider";
import { useIsTaskArchived, ArchivedPanelPlaceholder } from "./task-archived-context";
import { useEnvironmentId } from "@/hooks/use-environment-session-id";
import { DEV_SERVER_PANEL_ID } from "@/lib/state/layout-manager/constants";
import { makeUserShellScope, userShellScopeKey } from "@/lib/state/user-shell-scope";
import { useTranslation } from "react-i18next";

type TerminalPanelProps = {
  panelId: string;
  params: Record<string, unknown>;
};

type ShellStateSlice = {
  userShells: {
    byScope: Record<string, Array<{ terminalId: string; state?: string }>>;
    byEnvironmentId: Record<string, Array<{ terminalId: string; kind?: string; state?: string }>>;
  };
};

function isManagedTerminalId(terminalId: unknown): terminalId is string {
  return typeof terminalId === "string" && terminalId.startsWith("shell-");
}

function panelStampConflicts(
  activeTaskID: string | null,
  activeEnvironmentID: string | null,
  stampedTaskID: string | undefined,
  stampedEnvironmentID: string | undefined,
) {
  return {
    taskConflict: !!stampedTaskID && stampedTaskID !== activeTaskID,
    environmentConflict: !!stampedEnvironmentID && stampedEnvironmentID !== activeEnvironmentID,
  };
}

function shouldBlockOrdinaryPanel(
  isOrdinaryTerminal: boolean,
  isDevServer: boolean,
  ownsTerminal: boolean,
): boolean {
  return isOrdinaryTerminal && !isDevServer && !ownsTerminal;
}

function isTerminalOwned(
  state: ShellStateSlice,
  args: {
    isOrdinaryTerminal: boolean;
    activeEnvironmentId: string | null;
    scopeKey: string;
    taskConflict: boolean;
    environmentConflict: boolean;
    terminalId: string;
  },
): boolean {
  if (!args.isOrdinaryTerminal) return true;
  if (args.environmentConflict) return false;
  // Legacy backends returned `shell-` ids without ordinary metadata; those
  // descriptors live in the environment namespace and remain attachable.
  const environmentRows = args.activeEnvironmentId
    ? (state.userShells.byEnvironmentId[args.activeEnvironmentId] ?? [])
    : [];
  const legacy = environmentRows.some(
    (shell) => shell.terminalId === args.terminalId && shell.kind !== "ordinary",
  );
  if (legacy) return true;
  if (!args.scopeKey || args.taskConflict) return false;
  return (state.userShells.byScope[args.scopeKey] ?? []).some(
    (shell) => shell.terminalId === args.terminalId && shell.state !== "parked",
  );
}

export const TerminalPanel = memo(function TerminalPanel({ params }: TerminalPanelProps) {
  const { t } = useTranslation();
  const terminalId = params.terminalId as string;
  const type = (params.type as string) ?? "shell";
  const isDevServer = type === DEV_SERVER_PANEL_ID;

  const hookEnvironmentId = useEnvironmentId();
  const environmentId = (params.environmentId as string | undefined) ?? hookEnvironmentId;
  const activeTaskID = useAppStore((state) => state.tasks.activeTaskId ?? null);
  const stampedTaskID = params.taskID as string | undefined;
  const stampedEnvironmentID = params.environmentId as string | undefined;
  const isOrdinaryTerminal = isManagedTerminalId(terminalId);
  // Ordinary terminals belong to the active task/environment. A panel's own
  // stamps may not be trusted on their own: a retained panel from task B can
  // still name B's terminal while task A is active, and B's cache may contain
  // that row. Both stamps must agree with the visible scope.
  const scope = makeUserShellScope(hookEnvironmentId, activeTaskID);
  const scopeKey = scope ? userShellScopeKey(scope) : "";
  const { taskConflict, environmentConflict } = panelStampConflicts(
    activeTaskID,
    hookEnvironmentId,
    stampedTaskID,
    stampedEnvironmentID,
  );
  const terminalEnvironmentId = isOrdinaryTerminal ? hookEnvironmentId : environmentId;
  // Ordinary terminals are task-owned. A panel stamped with an ID that the
  // active scope does not list must not open a WebSocket; the gateway would
  // reject it and the tab would show a dead "Connecting" state.
  const ownsTerminal = useAppStore((state) =>
    isTerminalOwned(state, {
      isOrdinaryTerminal,
      activeEnvironmentId: hookEnvironmentId,
      scopeKey,
      taskConflict,
      environmentConflict,
      terminalId,
    }),
  );

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

  if (shouldBlockOrdinaryPanel(isOrdinaryTerminal, isDevServer, ownsTerminal)) {
    // An owned panel becomes attachable as soon as its scope lists the row.
    // A foreign or stale ID never reaches the transport; layout
    // reconciliation removes the panel rather than reconnecting it.
    return (
      <PanelRoot data-testid="terminal-panel">
        <PanelLoadingState label={t("task:connectingTerminal")} />
      </PanelRoot>
    );
  }

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
        <PassthroughTerminal
          mode="shell"
          environmentId={terminalEnvironmentId}
          terminalId={terminalId}
        />
      </PanelBody>
    </PanelRoot>
  );
});
