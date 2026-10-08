"use client";

import { useCallback, useState } from "react";
import {
  IconArrowLeft,
  IconExternalLink,
  IconGitBranch,
  IconGitPullRequest,
  IconLoader2,
  IconPlayerStop,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { TaskChatPanel } from "@/components/task/task-chat-panel";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { useSessionResumption } from "@/hooks/domains/session/use-session-resumption";
import type { Task } from "@/lib/types/http";
import { useRouter } from "@/lib/routing/client-router";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useTranslation } from "react-i18next";
import { CursorCloudSubmissionRecovery } from "./cursor-cloud-submission-recovery";
import { useCursorCloudSubmissionResolution } from "./use-cursor-cloud-submission-resolution";

type RemoteStatus = {
  remote_state: string | null;
  remote_status_error: string | null;
  remote_repository_id: string | null;
  remote_branch: string | null;
  remote_pull_request_url: string | null;
  remote_agent_url: string | null;
  remote_history_gap: boolean;
};

type CloudTask = Pick<Task, "id" | "title" | "state" | "workspace_id">;

function RemoteResults({
  status,
  repositoryLabel,
}: {
  status: RemoteStatus;
  repositoryLabel: string | null;
}) {
  const { t } = useTranslation();
  const hasResults = Boolean(status.remote_branch || status.remote_pull_request_url);
  return (
    <div className="space-y-4 p-4" data-testid="cursor-cloud-results">
      {repositoryLabel && (
        <div className="space-y-1">
          <div className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
            {t("executors:cursorCloudRepository")}
          </div>
          <div className="break-all text-sm">{repositoryLabel}</div>
        </div>
      )}
      {status.remote_branch && (
        <div className="space-y-1">
          <div className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
            {t("executors:cursorCloudBranch")}
          </div>
          <div className="flex items-center gap-2 break-all text-sm">
            <IconGitBranch className="size-4 shrink-0" />
            {status.remote_branch}
          </div>
        </div>
      )}
      {status.remote_pull_request_url && (
        <a
          href={status.remote_pull_request_url}
          target="_blank"
          rel="noreferrer"
          className="inline-flex min-h-11 items-center gap-2 text-sm font-medium text-primary underline"
        >
          <IconGitPullRequest className="size-4" />
          {t("executors:cursorCloudOpenPullRequest")}
          <IconExternalLink className="size-4" />
        </a>
      )}
      {!hasResults && (
        <p className="text-sm text-muted-foreground">{t("executors:cursorCloudNoResults")}</p>
      )}
      {status.remote_status_error && (
        <p role="status" className="text-sm text-muted-foreground">
          {t("task:remoteExecutorStatusUnavailable")}
        </p>
      )}
    </div>
  );
}

function CursorCloudTaskHeader({
  taskTitle,
  stateLabel,
  agentUrl,
  isMobile,
  canStop,
  stopping,
  onStop,
}: {
  taskTitle: string;
  stateLabel: string;
  agentUrl: string | null;
  isMobile: boolean;
  canStop: boolean;
  stopping: boolean;
  onStop: () => void;
}) {
  const { t } = useTranslation();
  const router = useRouter();
  return (
    <header className="flex min-h-14 shrink-0 items-center gap-2 border-b px-3 pt-[env(safe-area-inset-top,0px)] md:min-h-12 md:px-4 md:pt-0">
      {isMobile && (
        <Button
          type="button"
          variant="ghost"
          className="size-11 shrink-0 p-0"
          onClick={() => router.back()}
          aria-label={t("common:back")}
        >
          <IconArrowLeft className="size-5" />
        </Button>
      )}
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-semibold">{taskTitle}</div>
        <div className="truncate text-xs text-muted-foreground">
          {t("executors:cursorCloudTitle")}
        </div>
      </div>
      <span role="status" className="hidden shrink-0 text-xs text-muted-foreground sm:block">
        {stateLabel}
      </span>
      {agentUrl && (
        <a
          href={agentUrl}
          target="_blank"
          rel="noreferrer"
          className="inline-flex min-h-11 shrink-0 items-center gap-1 px-2 text-sm font-medium underline"
        >
          {t("executors:cursorCloudOpenInCursor")}
          <IconExternalLink className="size-4" />
        </a>
      )}
      <Button
        type="button"
        variant="outline"
        className="min-h-11 shrink-0"
        disabled={!canStop}
        onClick={onStop}
      >
        {stopping ? (
          <IconLoader2 className="mr-2 size-4 animate-spin" />
        ) : (
          <IconPlayerStop className="mr-2 size-4" />
        )}
        {stopping ? t("executors:cursorCloudStatusStopping") : t("executors:stop")}
      </Button>
    </header>
  );
}

function MobileCloudResults({
  taskTitle,
  status,
  repositoryLabel,
  open,
  onOpenChange,
}: {
  taskTitle: string;
  status: RemoteStatus;
  repositoryLabel: string | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="shrink-0 border-t bg-background px-3 py-2 pb-[calc(0.5rem+env(safe-area-inset-bottom,0px))]">
      <Button
        type="button"
        variant="outline"
        className="min-h-11 w-full"
        onClick={() => onOpenChange(true)}
      >
        {t("executors:cursorCloudResults")}
      </Button>
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerContent className="max-h-[min(85dvh,calc(100dvh-1rem-env(safe-area-inset-top,0px)))] overflow-hidden pb-[env(safe-area-inset-bottom,0px)]">
          <DrawerHeader className="shrink-0 border-b px-4 py-3 text-left">
            <DrawerTitle>{t("executors:cursorCloudResults")}</DrawerTitle>
            <DrawerDescription className="sr-only">{taskTitle}</DrawerDescription>
          </DrawerHeader>
          <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain">
            <RemoteResults status={status} repositoryLabel={repositoryLabel} />
          </div>
        </DrawerContent>
      </Drawer>
    </div>
  );
}

function resolveComposerGate(
  state: string,
  stopping: boolean,
  connected: boolean,
  error: string | null,
  t: (key: string) => string,
) {
  const blocked =
    state === "loading" ||
    state === "pending" ||
    state === "unknown" ||
    state === "error" ||
    stopping ||
    !connected;
  if (state === "unknown") return { blocked, reason: t("executors:cursorCloudSubmissionUnknown") };
  if (state === "error" || state === "loading" || state === "pending") {
    return { blocked, reason: error ?? t("executors:cursorCloudCheckingSubmission") };
  }
  if (stopping) return { blocked, reason: t("executors:cursorCloudStatusStopping") };
  if (!connected) return { blocked, reason: t("executors:cursorCloudReconnect") };
  return { blocked, reason: undefined };
}

function resolveStatusLabel(
  stopping: boolean,
  connected: boolean,
  sessionState: string | undefined,
  t: (key: string) => string,
) {
  if (stopping) return t("executors:cursorCloudStatusStopping");
  if (!connected) return t("executors:cursorCloudReconnect");
  if (sessionState === "RUNNING" || sessionState === "STARTING") {
    return t("executors:cursorCloudStatusRunning");
  }
  return t("executors:cursorCloudStatusWaiting");
}

function CloudConversation({
  taskId,
  sessionId,
  gate,
}: {
  taskId: string;
  sessionId: string | null;
  gate: ReturnType<typeof resolveComposerGate>;
}) {
  return (
    <div className="min-h-0 min-w-0 flex-1">
      <TaskChatPanel
        sessionId={sessionId}
        taskId={sessionId ? taskId : null}
        statusTaskId={taskId}
        embedded
        hideSessionsDropdown
        hideLaunchQueueStatus
        hideWipQueueStatus
        hideAgentControls
        minimalToolbar
        externallyDisabled={gate.blocked}
        externalDisabledReason={gate.reason}
      />
    </div>
  );
}

function CursorCloudTaskNotices({
  connected,
  statusUnavailable,
  historyGap,
}: {
  connected: boolean;
  statusUnavailable: boolean;
  historyGap: boolean;
}) {
  const { t } = useTranslation();
  return (
    <>
      {!connected && (
        <div
          role="status"
          className="shrink-0 border-b bg-muted/40 px-4 py-2 text-sm text-muted-foreground"
        >
          {t("executors:cursorCloudReconnect")}
        </div>
      )}
      {statusUnavailable && (
        <div
          role="alert"
          className="shrink-0 border-b bg-muted/40 px-4 py-2 text-sm text-destructive"
        >
          {t("task:remoteExecutorStatusUnavailable")}
        </div>
      )}
      {historyGap && (
        <div
          role="status"
          data-testid="cursor-cloud-history-gap"
          className="shrink-0 border-b bg-muted/40 px-4 py-2 text-sm text-muted-foreground"
        >
          {t("executors:cursorCloudHistoryGap")}
        </div>
      )}
    </>
  );
}

function CursorCloudTaskPanels({
  taskId,
  taskTitle,
  sessionId,
  gate,
  isMobile,
  status,
  repositoryLabel,
  resultsOpen,
  onResultsOpenChange,
}: {
  taskId: string;
  taskTitle: string;
  sessionId: string | null;
  gate: ReturnType<typeof resolveComposerGate>;
  isMobile: boolean;
  status: RemoteStatus;
  repositoryLabel: string | null;
  resultsOpen: boolean;
  onResultsOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <div className="flex min-h-0 flex-1">
        <CloudConversation taskId={taskId} sessionId={sessionId} gate={gate} />
        {!isMobile && (
          <aside className="hidden w-72 shrink-0 overflow-y-auto border-l bg-card md:block">
            <div className="border-b px-4 py-3 text-sm font-semibold">
              {t("executors:cursorCloudResults")}
            </div>
            <RemoteResults status={status} repositoryLabel={repositoryLabel} />
          </aside>
        )}
      </div>
      {isMobile && (
        <MobileCloudResults
          taskTitle={taskTitle}
          status={status}
          repositoryLabel={repositoryLabel}
          open={resultsOpen}
          onOpenChange={onResultsOpenChange}
        />
      )}
    </>
  );
}

function CursorCloudTaskStatusRegion({
  taskTitle,
  status,
  isMobile,
  connected,
  sessionState,
  statusUnavailable,
  canStop,
  stopping,
  onStop,
  recovery,
}: {
  taskTitle: string;
  status: RemoteStatus;
  isMobile: boolean;
  connected: boolean;
  sessionState: string | undefined;
  statusUnavailable: boolean;
  canStop: boolean;
  stopping: boolean;
  onStop: () => void;
  recovery: ReturnType<typeof useCursorCloudSubmissionResolution>;
}) {
  const { t } = useTranslation();
  return (
    <>
      <CursorCloudTaskHeader
        taskTitle={taskTitle}
        stateLabel={resolveStatusLabel(stopping, connected, sessionState, t)}
        agentUrl={status.remote_agent_url}
        isMobile={isMobile}
        canStop={canStop}
        stopping={stopping}
        onStop={onStop}
      />
      <CursorCloudTaskNotices
        connected={connected}
        statusUnavailable={statusUnavailable}
        historyGap={status.remote_history_gap}
      />
      <CursorCloudSubmissionRecovery
        state={recovery.state}
        resolution={recovery.resolution}
        isMobile={isMobile}
        agentUrl={status.remote_agent_url}
        busy={recovery.busy}
        error={recovery.error}
        onBind={(runId) => void recovery.bind(runId)}
        onRetry={(operationId) => void recovery.retry(operationId)}
        onRefresh={() => void recovery.refresh()}
      />
    </>
  );
}

export function CursorCloudTaskSurface({
  task,
  sessionId,
  status,
  repositoryLabel,
  connectionStatus,
  resumption,
}: {
  task: CloudTask;
  sessionId: string | null;
  status: RemoteStatus;
  repositoryLabel: string | null;
  connectionStatus: string;
  resumption: ReturnType<typeof useSessionResumption>;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const [resultsOpen, setResultsOpen] = useState(false);
  const session = useAppStore((state) =>
    sessionId ? (state.taskSessions.items[sessionId] ?? null) : null,
  );
  const latestUserMessageId = useAppStore((state) => {
    if (!sessionId) return null;
    const messages = state.messages.bySession[sessionId] ?? [];
    for (let index = messages.length - 1; index >= 0; index -= 1) {
      const message = messages[index];
      if (message?.author_type === "user") return message.id;
    }
    return null;
  });
  const localCancellationPending = useAppStore((state) =>
    sessionId ? state.chatInput.cancellingBySessionId[sessionId] === true : false,
  );
  const storeApi = useAppStoreApi();
  const recovery = useCursorCloudSubmissionResolution({
    taskId: task.id,
    sessionId,
    latestUserMessageId,
    retrySessionStatus: resumption.retrySessionStatus,
  });
  const stopping = Boolean(
    session?.cancellation_pending ||
    localCancellationPending ||
    recovery.resolution?.state === "cancelling",
  );
  const connected = connectionStatus === "connected";
  const canStop = Boolean(
    sessionId &&
    session &&
    (session.state === "RUNNING" || session.state === "STARTING") &&
    !stopping,
  );
  const composerGate = resolveComposerGate(recovery.state, stopping, connected, recovery.error, t);
  const cancelRemoteWork = useCallback(async () => {
    if (!sessionId || stopping) return;
    const client = getWebSocketClient();
    if (!client) return;
    const setPending = storeApi.getState().setCancelTurnPending;
    setPending(sessionId, true);
    try {
      await client.request("agent.cancel", { session_id: sessionId }, 15000);
    } catch (error) {
      console.error("Failed to cancel Cursor Cloud work:", error);
    } finally {
      setPending(sessionId, false);
    }
  }, [sessionId, stopping, storeApi]);

  return (
    <div
      className="flex h-full min-h-0 flex-col bg-background"
      data-testid="cursor-cloud-task-surface"
    >
      <CursorCloudTaskStatusRegion
        taskTitle={task.title}
        status={status}
        isMobile={isMobile}
        connected={connected}
        sessionState={session?.state}
        statusUnavailable={Boolean(resumption.error)}
        canStop={canStop}
        stopping={stopping}
        onStop={() => void cancelRemoteWork()}
        recovery={recovery}
      />
      <CursorCloudTaskPanels
        taskId={task.id}
        taskTitle={task.title}
        sessionId={sessionId}
        gate={composerGate}
        isMobile={isMobile}
        status={status}
        repositoryLabel={repositoryLabel}
        resultsOpen={resultsOpen}
        onResultsOpenChange={setResultsOpen}
      />
    </div>
  );
}
