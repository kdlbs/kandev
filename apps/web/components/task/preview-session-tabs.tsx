"use client";

import { useCallback, useMemo } from "react";
import { AgentLogo } from "@/components/agent-logo";
import { GridSpinner } from "@/components/grid-spinner";
import { PanelLoadingState } from "@/components/panel-loading-state";
import { SessionTabs, type SessionTab } from "@/components/session-tabs";
import { useAppStore } from "@/components/state-provider";
import { useSessionResumption } from "@/hooks/domains/session/use-session-resumption";
import { useTaskSessions } from "@/hooks/use-task-sessions";
import type { UseEnsureTaskSessionResult } from "@/hooks/domains/session/use-ensure-task-session";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { TaskSession } from "@/lib/types/http";
import { sendMessageRequest } from "@/hooks/use-message-handler";
import { buildStartRequest } from "@/lib/services/session-launch-helpers";
import { launchSession } from "@/lib/services/session-launch-service";
import { useToast } from "@/components/toast-provider";
import type { ChatSubmitPayload } from "./chat/chat-input-container";
import { EnsureSessionErrorEmptyState, SessionRecoveryFeedback } from "./ensure-session-error";
import { PassthroughToolbar } from "./passthrough-toolbar";
import { SessionsDropdown, type SessionsDropdownCreateSessionData } from "./sessions-dropdown";
import { TaskChatPanel } from "./task-chat-panel";
import {
  buildAgentLabelsById,
  isSessionActive,
  pickActiveSessionId,
  resolveAgentLabelFor,
  sortSessions,
} from "./session-sort";
import { useTranslation } from "react-i18next";

const LABEL_SEPARATOR = " \u2022 ";

/**
 * New-session creation stays caller-local instead of the dialog's default
 * submit path, which navigates to the full task page
 * (`router.push(linkToTask(taskId))`) \u2014 wrong for an embedded surface like
 * the kanban preview panel.
 */
function usePreviewCreateSession(
  taskId: string,
  onSessionChange?: (sessionId: string | null) => void,
) {
  const { t } = useTranslation();
  const { toast } = useToast();
  return useCallback(
    async (data: SessionsDropdownCreateSessionData) => {
      const { request } = buildStartRequest(taskId, data.agentProfileId, {
        executorId: data.executorId,
        prompt: data.prompt,
        attachments: data.attachments,
      });
      try {
        const response = await launchSession(request);
        if (response.session_id) onSessionChange?.(response.session_id);
      } catch (error) {
        toast({
          title: t("task:failedToCreateSession"),
          description: error instanceof Error ? error.message : t("common:anErrorOccurred"),
          variant: "error",
        });
      }
    },
    [taskId, onSessionChange, toast, t],
  );
}

type PreviewSessionTabsProps = {
  taskId: string;
  sessionId: string | null;
  ensureSession?: UseEnsureTaskSessionResult;
  workspaceId?: string | null;
  primarySessionId?: string | null;
  onSessionChange?: (sessionId: string | null) => void;
};

/**
 * Session tabs for the kanban preview panel, paired with the Agents dropdown
 * for full session management (create, resume, delete, set primary) without
 * leaving the board. Selecting a session from the dropdown updates only this
 * panel's local preview state, not the global active session or dockview
 * layout — the preview has no dockview to switch.
 */
export function PreviewSessionTabs({
  taskId,
  sessionId,
  ensureSession,
  workspaceId,
  primarySessionId = null,
  onSessionChange,
}: PreviewSessionTabsProps) {
  const { t } = useTranslation();
  const { sessions, isLoaded } = useTaskSessions(taskId);
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  const handleCreateSession = usePreviewCreateSession(taskId, onSessionChange);

  const sortedSessions = useMemo(() => sortSessions(sessions), [sessions]);
  const agentLabelsById = useMemo(() => buildAgentLabelsById(agentProfiles), [agentProfiles]);
  const profilesById = useMemo(
    () => Object.fromEntries(agentProfiles.map((p) => [p.id, p])),
    [agentProfiles],
  );

  const activeSessionId = useMemo(
    () => pickActiveSessionId(sortedSessions, sessionId),
    [sortedSessions, sessionId],
  );
  const activeSession = useMemo(
    () => sortedSessions.find((s) => s.id === activeSessionId) ?? null,
    [sortedSessions, activeSessionId],
  );

  // Mirrors the full-page task view: ensure the backend execution for the
  // active session is ready (resumes / restores workspace after a kandev
  // restart where the session row is persisted but agentctl isn't alive).
  const resumption = useSessionResumption(taskId, activeSessionId);

  const tabs = useMemo<SessionTab[]>(
    () =>
      sortedSessions.map((session) => {
        const profile = session.agent_profile_id ? profilesById[session.agent_profile_id] : null;
        return {
          id: session.id,
          label: resolveProfileSubLabel(session, profile, agentLabelsById),
          icon: isSessionActive(session.state) ? (
            <RunningSpinner />
          ) : (
            <SessionAgentLogo profile={profile} />
          ),
          testId: `preview-session-tab-${session.id}`,
          className: "bg-muted/50 data-[state=active]:bg-muted",
        };
      }),
    [sortedSessions, agentLabelsById, profilesById],
  );

  if (!isLoaded && sortedSessions.length === 0) {
    return <PreviewLoadingState label={t("task:loadingAgents")} />;
  }

  if (sortedSessions.length === 0) {
    return (
      <PreviewNoSessionsState
        taskId={taskId}
        ensureSession={ensureSession}
        resumption={resumption}
        workspaceId={workspaceId}
        onCreateSession={handleCreateSession}
      />
    );
  }

  return (
    <div className="flex h-full flex-col min-h-0" data-testid="preview-session-tabs">
      <SessionRecoveryFeedback
        error={resumption.error}
        notice={resumption.notice}
        onRetry={() => void resumption.resumeSession()}
        workspaceId={workspaceId ?? null}
      />
      <div className="border-b px-2 py-1 flex items-center gap-1 min-w-0">
        <SessionTabs
          tabs={tabs}
          activeTab={activeSessionId ?? ""}
          onTabChange={(id) => onSessionChange?.(id)}
          listClassName="bg-transparent p-0 !h-7 gap-1 overflow-x-auto overflow-y-hidden min-w-0 shrink [&::-webkit-scrollbar]:hidden [-ms-overflow-style:none] [scrollbar-width:none]"
        />
        <SessionsDropdown
          taskId={taskId}
          activeSessionId={activeSessionId}
          primarySessionId={primarySessionId}
          onSelectSession={(id) => onSessionChange?.(id)}
          onCreateSession={handleCreateSession}
        />
      </div>
      <div className="flex-1 min-h-0">
        {activeSession && (
          <PreviewSessionBody key={activeSession.id} session={activeSession} taskId={taskId} />
        )}
      </div>
    </div>
  );
}

function resolveProfileSubLabel(
  session: TaskSession,
  profile: AgentProfileOption | null | undefined,
  agentLabelsById: Record<string, string>,
): string {
  const fullLabel = profile?.label ?? resolveAgentLabelFor(session, agentLabelsById);
  const parts = fullLabel.split(LABEL_SEPARATOR);
  return parts[1] ?? parts[0] ?? fullLabel;
}

function SessionAgentLogo({ profile }: { profile: AgentProfileOption | null | undefined }) {
  if (!profile?.agent_name) {
    // Keep tabs visually aligned when the agent profile is missing/unknown.
    return (
      <span aria-hidden="true" className="h-3 w-3 shrink-0 rounded-full bg-muted-foreground/40" />
    );
  }
  return <AgentLogo agentName={profile.agent_name} size={12} className="shrink-0" />;
}

export function PreviewSessionBody({ session, taskId }: { session: TaskSession; taskId: string }) {
  const handleSendMessage = useCallback(
    async (payload: ChatSubmitPayload) => {
      await sendMessageRequest({
        taskId,
        resolvedSessionId: session.id,
        finalMessage: payload.message,
        modelToSend: undefined,
        planMode: false,
        hasReviewComments: !!payload.reviewComments?.length,
        attachments: payload.attachments,
        entityReferences: payload.entityReferences,
      });
    },
    [taskId, session.id],
  );

  if (session.is_passthrough) {
    return <PassthroughToolbar sessionId={session.id} taskId={taskId} />;
  }

  return (
    <div className="flex h-full flex-col">
      <TaskChatPanel
        onSend={handleSendMessage}
        sessionId={session.id}
        taskId={taskId}
        hideSessionsDropdown
        // Read-only kanban hover preview — a transient glance, not "opening"
        // the task. Never advances the Slack-style read cursor.
        isVisible={false}
      />
    </div>
  );
}

function RunningSpinner() {
  return <GridSpinner className="text-muted-foreground shrink-0 text-[12px]" />;
}

function PreviewLoadingState({ label }: { label: string }) {
  return <PanelLoadingState testId="preview-loading-state" label={label} />;
}

function PreviewNoSessionsState({
  taskId,
  ensureSession,
  resumption,
  workspaceId,
  onCreateSession,
}: {
  taskId: string;
  ensureSession?: UseEnsureTaskSessionResult;
  resumption: ReturnType<typeof useSessionResumption>;
  workspaceId?: string | null;
  onCreateSession: (data: SessionsDropdownCreateSessionData) => void;
}) {
  const { t } = useTranslation();
  if (ensureSession?.status === "preparing") {
    return <PreviewLoadingState label={t("task:preparingWorkspace2")} />;
  }
  if (ensureSession?.status === "error") {
    return (
      <>
        <SessionRecoveryFeedback
          error={resumption.error}
          notice={resumption.notice}
          onRetry={() => void resumption.resumeSession()}
          workspaceId={workspaceId ?? null}
        />
        <EnsureSessionErrorEmptyState
          error={ensureSession.error}
          onRetry={ensureSession.retry}
          workspaceId={workspaceId ?? null}
        />
      </>
    );
  }
  return <PreviewEmptyState taskId={taskId} onCreateSession={onCreateSession} />;
}

function PreviewEmptyState({
  taskId,
  onCreateSession,
}: {
  taskId: string;
  onCreateSession: (data: SessionsDropdownCreateSessionData) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex h-full flex-col">
      <div className="flex justify-end border-b px-2 py-1">
        <SessionsDropdown
          taskId={taskId}
          activeSessionId={null}
          onCreateSession={onCreateSession}
        />
      </div>
      <div
        className="flex flex-1 items-center justify-center text-sm text-muted-foreground"
        data-testid="preview-empty-state"
      >
        {t("task:noAgentsYet2")}
      </div>
    </div>
  );
}
