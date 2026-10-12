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
import { isAgentMcpAuthWarning } from "@/lib/prepare/agent-mcp-warning";
import {
  normalizeNativeMcpDiagnostic,
  type NativeMCPDiagnostic,
  type NativeMCPDiagnosticOperation,
  type NativeMCPDiagnosticStage,
} from "@/lib/prepare/native-mcp-diagnostic";
import { cn } from "@/lib/utils";
import type { PrepareStepInfo, UserShellInfo } from "@/lib/state/slices/session-runtime/types";

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

const OPERATION_LABEL_KEYS: Record<NativeMCPDiagnosticOperation, string> = {
  enable: "task:agentMcpDiagnosticOperationApproval",
  list_tools: "task:agentMcpDiagnosticOperationListTools",
};

const STAGE_LABEL_KEYS: Record<NativeMCPDiagnosticStage, string> = {
  resolve: "task:agentMcpDiagnosticStageResolve",
  start: "task:agentMcpDiagnosticStageStart",
  wait: "task:agentMcpDiagnosticStageWait",
  cleanup: "task:agentMcpDiagnosticStageCleanup",
  output: "task:agentMcpDiagnosticStageOutput",
};

function diagnosticOperationFailureLabelKey(diagnostic?: NativeMCPDiagnostic): string | undefined {
  if (diagnostic?.operation === "enable") return "task:agentMcpApprovalCommandFailed";
  if (diagnostic?.operation === "list_tools") return "task:agentMcpVerificationCommandFailed";
  return undefined;
}

function userShellFromTerminalInfo(terminal: TerminalInfo, label: string): UserShellInfo | null {
  const terminalId = terminal.id ?? terminal.terminal_id;
  if (!terminalId || (terminal.kind && terminal.kind !== "ordinary")) return null;
  return {
    terminalId,
    kind: terminal.kind ?? "ordinary",
    seq: terminal.seq,
    customName: terminal.custom_name,
    displayName: label,
    state: terminal.state ?? "open",
    ptyStatus: terminal.pty_status ?? (terminal.running ? "running" : "stopped"),
    label,
    running: terminal.running ?? terminal.pty_status === "running",
    closable: true,
  };
}

async function resolveRecoveryShell(
  taskId: string,
  result: { terminal_id: string; task_environment_id: string; label: string },
): Promise<UserShellInfo> {
  try {
    const terminals = await fetchTerminals(taskId, result.task_environment_id);
    const terminal = terminals.find(
      (candidate) => (candidate.id ?? candidate.terminal_id) === result.terminal_id,
    );
    const shell = terminal ? userShellFromTerminalInfo(terminal, result.label) : null;
    if (shell) return shell;
  } catch {
    // Fall back to direct result shell
  }
  return {
    terminalId: result.terminal_id,
    kind: "ordinary",
    displayName: result.label,
    label: result.label,
    state: "open",
    ptyStatus: "stopped",
    running: false,
    closable: true,
  };
}

export function agentMcpFailureLabelKey(
  failureCode?: string,
  diagnostic?: NativeMCPDiagnostic,
): string {
  if (failureCode !== "authentication_required") {
    const commandFailureKey = diagnosticOperationFailureLabelKey(diagnostic);
    if (commandFailureKey) return commandFailureKey;
  }
  return FAILURE_LABEL_KEYS[failureCode ?? ""] ?? "task:agentMcpConnectionFailed";
}

function NativeMcpDiagnosticDetails({ diagnostic }: { diagnostic: NativeMCPDiagnostic }) {
  const { t } = useTranslation();
  return (
    <section
      className="min-w-0 max-w-full space-y-1 rounded bg-muted/40 p-2"
      data-testid="agent-mcp-diagnostic"
    >
      <p className="font-medium">{t("task:agentMcpErrorDetails")}</p>
      <dl className="min-w-0 space-y-1">
        <div className="grid min-w-0 grid-cols-[auto_minmax(0,1fr)] gap-x-2">
          <dt className="text-muted-foreground">{t("task:agentMcpDiagnosticOperation")}</dt>
          <dd className="min-w-0 break-words">{t(OPERATION_LABEL_KEYS[diagnostic.operation])}</dd>
        </div>
        <div className="grid min-w-0 grid-cols-[auto_minmax(0,1fr)] gap-x-2">
          <dt className="text-muted-foreground">{t("task:agentMcpDiagnosticStage")}</dt>
          <dd className="min-w-0 break-words">{t(STAGE_LABEL_KEYS[diagnostic.stage])}</dd>
        </div>
        {diagnostic.exitCode !== undefined && (
          <div>
            <dt className="sr-only">{t("task:agentMcpDiagnosticExitStatus")}</dt>
            <dd>{t("task:agentMcpDiagnosticExitStatusValue", { code: diagnostic.exitCode })}</dd>
          </div>
        )}
      </dl>
      <p
        className="min-w-0 max-w-full whitespace-pre-wrap break-words select-text"
        data-testid="agent-mcp-diagnostic-message"
      >
        {diagnostic.message}
      </p>
      {diagnostic.cleanupMessage && (
        <p className="min-w-0 max-w-full whitespace-pre-wrap break-words select-text">
          <span className="text-muted-foreground">{t("task:agentMcpDiagnosticCleanupError")} </span>
          {diagnostic.cleanupMessage}
        </p>
      )}
    </section>
  );
}

async function runAgentMcpRetry(
  sessionId: string,
  serverId: string,
): Promise<{
  ready: boolean;
  diagnostic?: NativeMCPDiagnostic;
  feedbackKey: string;
}> {
  try {
    const result = await retryAgentMcpConnection(sessionId, serverId);
    const diagnostic = normalizeNativeMcpDiagnostic(result.mcp_diagnostic);
    return {
      ready: result.status === "ready",
      diagnostic,
      feedbackKey:
        result.status === "ready"
          ? "task:agentMcpConnectionReady"
          : agentMcpFailureLabelKey(result.reason_code ?? result.status, diagnostic),
    };
  } catch (error) {
    return {
      ready: false,
      feedbackKey: isAgentMcpRecoveryBusyError(error)
        ? agentMcpFailureLabelKey("session_busy")
        : "task:agentMcpRecoveryFailed",
    };
  }
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
  const { isFinePointer, isMobile, usesDesktopWorkbench } = useResponsiveBreakpoint();
  const store = useAppStoreApi();
  const addTerminalPanel = useDockviewStore((state) => state.addTerminalPanel);
  const [pending, setPending] = useState<"authenticate" | "retry" | null>(null);
  const [feedback, setFeedback] = useState("");
  const [retryDiagnosticOverride, setRetryDiagnosticOverride] = useState<{
    source: PrepareStepInfo;
    diagnostic?: NativeMCPDiagnostic;
  } | null>(null);
  const serverId = step.mcpServerId;
  const failed = step.status === "failed";
  const currentDiagnostic =
    retryDiagnosticOverride?.source === step
      ? retryDiagnosticOverride.diagnostic
      : step.mcpDiagnostic;
  const buttonSize = isFinePointer && !isMobile ? "min-h-7" : "min-h-11 w-full";

  const authenticate = async () => {
    if (!serverId || pending) return;
    setPending("authenticate");
    setFeedback("");
    try {
      const result = await authenticateAgentMcp(sessionId, serverId);
      const shell = await resolveRecoveryShell(taskId, result);
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
      const result = await runAgentMcpRetry(sessionId, serverId);
      setRetryDiagnosticOverride({
        source: step,
        diagnostic: result.ready ? undefined : (result.diagnostic ?? currentDiagnostic),
      });
      setFeedback(t(result.feedbackKey));
    } finally {
      setPending(null);
    }
  };

  if (!failed || !serverId) return null;
  const isAuthWarning = isAgentMcpAuthWarning(step);

  return (
    <div
      className="mt-2 min-w-0 max-w-full space-y-2"
      data-testid="agent-mcp-recovery-actions"
      data-mcp-server-id={serverId}
    >
      <p
        role="status"
        aria-live="polite"
        className={cn(
          "text-xs",
          isAuthWarning ? "text-amber-700 dark:text-amber-400" : "text-destructive",
        )}
      >
        {t(agentMcpFailureLabelKey(step.failureCode, currentDiagnostic))}
      </p>
      {currentDiagnostic && <NativeMcpDiagnosticDetails diagnostic={currentDiagnostic} />}
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
