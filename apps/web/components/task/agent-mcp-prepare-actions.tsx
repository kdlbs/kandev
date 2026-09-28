"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useAppStoreApi } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useDockviewStore } from "@/lib/state/dockview-store";
import {
  authenticateAgentMcp,
  isAgentMcpRecoveryBusyError,
  retryAgentMcpConnection,
} from "@/lib/api/domains/session-api";
import { fetchTerminals, type TerminalInfo } from "@/lib/api/domains/user-shell-api";
import type { PrepareStepInfo } from "@/lib/state/slices/session-runtime/types";

const FAILURE_LABEL_KEYS: Record<string, string> = {
  authentication_required: "task:agentMcpAuthenticationRequired",
  session_busy: "task:agentMcpSessionBusy",
  approval_failed: "task:agentMcpApprovalFailed",
  connection_failed: "task:agentMcpConnectionFailed",
  unavailable: "task:agentMcpUnavailable",
  session_reload_unsupported: "task:agentMcpSessionReloadUnsupported",
  canceled: "task:agentMcpPreparationCanceled",
  stale: "task:agentMcpSelectionChanged",
};

function userShellFromTerminalInfo(terminal: TerminalInfo, label: string) {
  const terminalId = terminal.id ?? terminal.terminal_id;
  if (!terminalId || terminal.kind !== "ordinary") return null;
  return {
    terminalId,
    kind: terminal.kind,
    seq: terminal.seq,
    customName: terminal.custom_name,
    displayName: label,
    state: terminal.state,
    ptyStatus: terminal.pty_status,
    label,
    running: terminal.pty_status === "running",
    closable: true,
  } as const;
}

export function agentMcpFailureLabelKey(failureCode?: string): string {
  return FAILURE_LABEL_KEYS[failureCode ?? ""] ?? "task:agentMcpConnectionFailed";
}

function RecoveryActionControls({
  failureCode,
  pending,
  feedback,
  isFinePointer,
  buttonSize,
  onAuthenticate,
  onRetry,
}: {
  failureCode?: string;
  pending: "authenticate" | "retry" | null;
  feedback: string;
  isFinePointer: boolean;
  buttonSize: string;
  onAuthenticate: () => Promise<void>;
  onRetry: () => Promise<void>;
}) {
  const { t } = useTranslation();
  let statusMessage = feedback;
  if (pending === "authenticate") statusMessage = t("task:openingAgentMcpAuthentication");
  if (pending === "retry") statusMessage = t("task:checkingAgentMcpConnection");

  return (
    <>
      <div className={`flex ${isFinePointer ? "flex-row" : "flex-col"} gap-2`}>
        {failureCode === "authentication_required" ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            className={`cursor-pointer ${buttonSize}`}
            disabled={pending !== null}
            onClick={() => void onAuthenticate()}
            data-testid="agent-mcp-authenticate"
          >
            {t("task:authenticateAgentMcp")}
          </Button>
        ) : null}
        <Button
          type="button"
          variant="outline"
          size="sm"
          className={`cursor-pointer ${buttonSize}`}
          disabled={pending !== null}
          onClick={() => void onRetry()}
          data-testid="agent-mcp-retry"
        >
          {t("task:retryAgentMcpConnection")}
        </Button>
      </div>
      <p className="min-h-4 text-xs text-muted-foreground" role="status" aria-live="polite">
        {statusMessage}
      </p>
    </>
  );
}

export function AgentMcpPrepareActions({
  step,
  sessionId,
  taskId,
}: {
  step: PrepareStepInfo;
  sessionId: string;
  taskId: string;
}) {
  const { t } = useTranslation();
  const { isFinePointer, usesDesktopWorkbench } = useResponsiveBreakpoint();
  const store = useAppStoreApi();
  const addTerminalPanel = useDockviewStore((state) => state.addTerminalPanel);
  const [pending, setPending] = useState<"authenticate" | "retry" | null>(null);
  const [feedback, setFeedback] = useState("");
  const serverId = step.mcpServerId;
  const failed = step.status === "failed";
  const buttonSize = isFinePointer ? "min-h-7" : "min-h-11 w-full";

  const authenticate = async () => {
    if (!serverId || pending) return;
    setPending("authenticate");
    setFeedback("");
    try {
      const result = await authenticateAgentMcp(sessionId, serverId);
      const terminals = await fetchTerminals(taskId, result.task_environment_id);
      const terminal = terminals.find(
        (candidate) => (candidate.id ?? candidate.terminal_id) === result.terminal_id,
      );
      const shell = terminal ? userShellFromTerminalInfo(terminal, result.label) : null;
      if (!shell) throw new Error();
      store.getState().addUserShell(result.task_environment_id, shell);
      if (usesDesktopWorkbench) {
        addTerminalPanel(
          result.terminal_id,
          undefined,
          result.task_environment_id,
          taskId,
          result.label,
        );
      } else {
        const state = store.getState();
        state.setRightPanelActiveTab(sessionId, result.terminal_id);
        state.setMobileSessionPanel(sessionId, "terminal");
      }
      setFeedback(t("task:agentMcpAuthenticationTerminalOpened"));
    } catch {
      setFeedback(t("task:agentMcpRecoveryFailed"));
    } finally {
      setPending(null);
    }
  };

  const retry = async () => {
    if (!serverId || pending) return;
    setPending("retry");
    setFeedback("");
    try {
      const result = await retryAgentMcpConnection(sessionId, serverId);
      setFeedback(
        result.status === "ready"
          ? t("task:agentMcpConnectionReady")
          : t(agentMcpFailureLabelKey(result.reason_code ?? result.status)),
      );
    } catch (error) {
      setFeedback(
        isAgentMcpRecoveryBusyError(error)
          ? t(agentMcpFailureLabelKey("session_busy"))
          : t("task:agentMcpRecoveryFailed"),
      );
    } finally {
      setPending(null);
    }
  };

  if (!failed || !serverId) return null;

  return (
    <div className="mt-2 space-y-2" data-testid="agent-mcp-recovery-actions">
      <p className="text-xs text-destructive">{t(agentMcpFailureLabelKey(step.failureCode))}</p>
      <RecoveryActionControls
        failureCode={step.failureCode}
        pending={pending}
        feedback={feedback}
        isFinePointer={isFinePointer}
        buttonSize={buttonSize}
        onAuthenticate={authenticate}
        onRetry={retry}
      />
    </div>
  );
}
