"use client";

import { useCallback, useMemo, useRef, useState } from "react";
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
import type { ChatSubmitPayload } from "./chat/chat-input-container";
import { EnsureSessionErrorEmptyState, SessionRecoveryFeedback } from "./ensure-session-error";
import { PassthroughToolbar } from "./passthrough-toolbar";
import { PreviewSessionTabMenu } from "./preview-session-tab-menu";
import { SessionTabDialogs } from "./session-tab-menu";
import { TabRenameInput } from "./tab-rename-input";
import { TaskChatPanel } from "./task-chat-panel";
import type { HandoffPreset } from "./new-session-dialog";
import { MAX_SESSION_NAME_LENGTH, useSessionRenameCommitter } from "./use-session-rename";
import {
  buildAgentLabelsById,
  isSessionActive,
  pickActiveSessionId,
  resolveAgentLabelFor,
  sortSessions,
} from "./session-sort";
import { useTranslation } from "react-i18next";

const LABEL_SEPARATOR = " \u2022 ";

type PreviewSessionTabsProps = {
  taskId: string;
  sessionId: string | null;
  ensureSession?: UseEnsureTaskSessionResult;
  workspaceId?: string | null;
  onSessionChange?: (sessionId: string | null) => void;
};

/** Delete-confirmation state hoisted out of the per-tab menu: `SessionTabs`
 * only gives each tab a slot for its `ContextMenuContent`, not a sibling
 * slot for a dialog, so the confirmation popover and the anchor it needs
 * live here instead. */
type MenuDeleteState = { sessionId: string; confirm: () => Promise<boolean> };

/** Holds the rename/delete/share/handoff UI state shared by every preview tab. */
function usePreviewSessionTabDialogs(taskId: string, sortedSessions: TaskSession[]) {
  const [renamingSessionId, setRenamingSessionId] = useState<string | null>(null);
  const [menuDeleteState, setMenuDeleteState] = useState<MenuDeleteState | null>(null);
  const menuDeleteAnchorRef = useRef<HTMLElement>(null);
  const menuDeleteFocusBoundaryRef = useRef<HTMLElement>(null);
  const [shareSessionId, setShareSessionId] = useState<string | null>(null);
  const [handoffOpen, setHandoffOpen] = useState(false);
  const [handoffPreset, setHandoffPreset] = useState<HandoffPreset | null>(null);

  const handleRequestDelete = useCallback(
    (targetSessionId: string, event: Event, confirmDelete: () => Promise<boolean>) => {
      const anchor = event.currentTarget;
      if (!(anchor instanceof HTMLElement)) return;
      menuDeleteAnchorRef.current = anchor;
      menuDeleteFocusBoundaryRef.current = anchor.closest('[data-slot="context-menu-content"]');
      setMenuDeleteState({ sessionId: targetSessionId, confirm: confirmDelete });
    },
    [],
  );
  const handleConfirmDelete = useCallback(() => {
    void menuDeleteState?.confirm();
  }, [menuDeleteState]);
  const handleMenuDeleteOpenChange = useCallback((open: boolean) => {
    if (!open) setMenuDeleteState(null);
  }, []);
  const handleHandoffProfile = useCallback((sourceSessionId: string, targetProfileId: string) => {
    setHandoffPreset({ sourceSessionId, targetProfileId });
    setHandoffOpen(true);
  }, []);

  const renamingSession = sortedSessions.find((s) => s.id === renamingSessionId) ?? null;
  const handleCommitRename = useSessionRenameCommitter(
    renamingSessionId ?? undefined,
    taskId,
    renamingSession?.name ?? null,
    () => setRenamingSessionId(null),
  );

  return {
    renamingSessionId,
    setRenamingSessionId,
    handleCommitRename,
    menuDeleteState,
    menuDeleteAnchorRef,
    menuDeleteFocusBoundaryRef,
    handleRequestDelete,
    handleConfirmDelete,
    handleMenuDeleteOpenChange,
    shareSessionId,
    setShareSessionId,
    handoffOpen,
    setHandoffOpen,
    handoffPreset,
    setHandoffPreset,
    handleHandoffProfile,
  };
}

type PreviewDialogsState = ReturnType<typeof usePreviewSessionTabDialogs>;

/** Builds the `SessionTab[]` for the preview strip, wiring each tab's
 * context menu and (while renaming) its inline rename input. */
function useBuildPreviewTabs({
  sortedSessions,
  profilesById,
  agentLabelsById,
  taskId,
  isPrimarySession,
  dialogs,
  handleSessionRemoved,
}: {
  sortedSessions: TaskSession[];
  profilesById: Record<string, AgentProfileOption>;
  agentLabelsById: Record<string, string>;
  taskId: string;
  isPrimarySession: (session: TaskSession) => boolean;
  dialogs: PreviewDialogsState;
  handleSessionRemoved: (sessionId: string) => void;
}): SessionTab[] {
  return useMemo(
    () =>
      sortedSessions.map((session) => {
        const profile = session.agent_profile_id ? profilesById[session.agent_profile_id] : null;
        // Custom name wins over the agent-derived label, mirroring the
        // full-page tab title (resolveSessionTabTitle) so a rename is
        // visible here, not just persisted.
        const label = session.name || resolveProfileSubLabel(session, profile, agentLabelsById);
        return {
          id: session.id,
          label,
          icon: isSessionActive(session.state) ? (
            <RunningSpinner />
          ) : (
            <SessionAgentLogo profile={profile} />
          ),
          testId: `preview-session-tab-${session.id}`,
          className: "bg-muted/50 data-[state=active]:bg-muted",
          renderContextMenu: () => (
            <PreviewSessionTabMenu
              session={session}
              taskId={taskId}
              isPrimary={isPrimarySession(session)}
              onRename={dialogs.setRenamingSessionId}
              onRequestDelete={dialogs.handleRequestDelete}
              onSessionRemoved={handleSessionRemoved}
              onShareRequested={dialogs.setShareSessionId}
              onHandoff={dialogs.handleHandoffProfile}
            />
          ),
          content:
            dialogs.renamingSessionId === session.id ? (
              <TabRenameInput
                initial={label}
                seqBadge={null}
                onCommit={dialogs.handleCommitRename}
                onCancel={() => dialogs.setRenamingSessionId(null)}
                testId="preview-session-tab-rename-input"
                maxLength={MAX_SESSION_NAME_LENGTH}
              />
            ) : undefined,
        };
      }),
    [
      sortedSessions,
      agentLabelsById,
      profilesById,
      taskId,
      isPrimarySession,
      dialogs,
      handleSessionRemoved,
    ],
  );
}

/** Hoisted delete/share/handoff dialogs, keyed off whichever session is
 * currently the target (see `usePreviewSessionTabDialogs`). */
function PreviewSessionTabDialogHost({
  dialogs,
  sortedSessions,
  profilesById,
  agentLabelsById,
  taskId,
  isPrimarySession,
}: {
  dialogs: PreviewDialogsState;
  sortedSessions: TaskSession[];
  profilesById: Record<string, AgentProfileOption>;
  agentLabelsById: Record<string, string>;
  taskId: string;
  isPrimarySession: (session: TaskSession) => boolean;
}) {
  const deleteTargetSession = dialogs.menuDeleteState
    ? (sortedSessions.find((s) => s.id === dialogs.menuDeleteState?.sessionId) ?? null)
    : null;
  const deleteTargetName = deleteTargetSession
    ? deleteTargetSession.name ||
      resolveProfileSubLabel(
        deleteTargetSession,
        deleteTargetSession.agent_profile_id
          ? profilesById[deleteTargetSession.agent_profile_id]
          : null,
        agentLabelsById,
      )
    : null;
  return (
    <SessionTabDialogs
      confirmDelete={false}
      menuDeleteOpen={dialogs.menuDeleteState !== null}
      menuDeleteAnchorRef={dialogs.menuDeleteAnchorRef}
      menuDeleteFocusBoundaryRef={dialogs.menuDeleteFocusBoundaryRef}
      setConfirmDelete={dialogs.handleMenuDeleteOpenChange}
      isPrimary={deleteTargetSession ? isPrimarySession(deleteTargetSession) : false}
      sessionCount={sortedSessions.length}
      onConfirmDelete={dialogs.handleConfirmDelete}
      targetName={deleteTargetName}
      taskId={taskId}
      sessionId={dialogs.shareSessionId ?? undefined}
      shareOpen={dialogs.shareSessionId !== null}
      setShareOpen={(open) => {
        if (!open) dialogs.setShareSessionId(null);
      }}
      handoffOpen={dialogs.handoffOpen}
      setHandoffOpen={dialogs.setHandoffOpen}
      handoffPreset={dialogs.handoffPreset}
      setHandoffPreset={dialogs.setHandoffPreset}
      groupId={undefined}
    />
  );
}

type SessionResumption = ReturnType<typeof useSessionResumption>;

/** The loading/empty states shared with the full-page view; returns `null`
 * once there's at least one session to render as tabs. */
function renderPreviewEmptyOrLoading({
  isLoaded,
  sortedSessions,
  ensureSession,
  resumption,
  workspaceId,
  t,
}: {
  isLoaded: boolean;
  sortedSessions: TaskSession[];
  ensureSession?: UseEnsureTaskSessionResult;
  resumption: SessionResumption;
  workspaceId?: string | null;
  t: (key: string) => string;
}) {
  if (!isLoaded && sortedSessions.length === 0) {
    return <PreviewLoadingState label={t("task:loadingAgents")} />;
  }
  if (sortedSessions.length > 0) return null;
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
  return <PreviewEmptyState />;
}

/**
 * Session tabs for the kanban preview panel.
 *
 * Right-click a tab for the same lifecycle context menu as the full-page
 * task view (rename, set primary, stop/resume, delete, share, handoff).
 * Creating a new session is still restricted to the full-page view.
 */
export function PreviewSessionTabs({
  taskId,
  sessionId,
  ensureSession,
  workspaceId,
  onSessionChange,
}: PreviewSessionTabsProps) {
  const { t } = useTranslation();
  const { sessions, isLoaded } = useTaskSessions(taskId);
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  const primarySessionId = useAppStore(
    (state) => state.kanban.tasks.find((task) => task.id === taskId)?.primarySessionId ?? null,
  );

  const sortedSessions = useMemo(() => sortSessions(sessions), [sessions]);
  const agentLabelsById = useMemo(() => buildAgentLabelsById(agentProfiles), [agentProfiles]);
  const profilesById = useMemo(
    () => Object.fromEntries(agentProfiles.map((p) => [p.id, p])),
    [agentProfiles],
  );
  const isPrimarySession = useCallback(
    (session: TaskSession) =>
      primarySessionId ? primarySessionId === session.id : session.is_primary === true,
    [primarySessionId],
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

  const dialogs = usePreviewSessionTabDialogs(taskId, sortedSessions);
  const handleSessionRemoved = useCallback(
    (removedSessionId: string) => {
      if (removedSessionId !== activeSessionId) return;
      const remaining = sortedSessions.filter((s) => s.id !== removedSessionId);
      onSessionChange?.(remaining[0]?.id ?? null);
    },
    [sortedSessions, activeSessionId, onSessionChange],
  );

  const tabs = useBuildPreviewTabs({
    sortedSessions,
    profilesById,
    agentLabelsById,
    taskId,
    isPrimarySession,
    dialogs,
    handleSessionRemoved,
  });

  const emptyOrLoading = renderPreviewEmptyOrLoading({
    isLoaded,
    sortedSessions,
    ensureSession,
    resumption,
    workspaceId,
    t,
  });
  if (emptyOrLoading) return emptyOrLoading;

  return (
    <div className="flex h-full flex-col min-h-0" data-testid="preview-session-tabs">
      <SessionRecoveryFeedback
        error={resumption.error}
        notice={resumption.notice}
        onRetry={() => void resumption.resumeSession()}
        workspaceId={workspaceId ?? null}
      />
      <div className="border-b px-2 py-1">
        <SessionTabs
          tabs={tabs}
          activeTab={activeSessionId ?? ""}
          onTabChange={(id) => onSessionChange?.(id)}
          listClassName="bg-transparent p-0 !h-7 gap-1 overflow-x-auto overflow-y-hidden min-w-0 shrink [&::-webkit-scrollbar]:hidden [-ms-overflow-style:none] [scrollbar-width:none]"
        />
      </div>
      <div className="flex-1 min-h-0">
        {activeSession && (
          <PreviewSessionBody key={activeSession.id} session={activeSession} taskId={taskId} />
        )}
      </div>
      <PreviewSessionTabDialogHost
        dialogs={dialogs}
        sortedSessions={sortedSessions}
        profilesById={profilesById}
        agentLabelsById={agentLabelsById}
        taskId={taskId}
        isPrimarySession={isPrimarySession}
      />
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

function PreviewEmptyState() {
  const { t } = useTranslation();
  return (
    <div className="flex h-full flex-col">
      <div
        className="flex flex-1 items-center justify-center text-sm text-muted-foreground"
        data-testid="preview-empty-state"
      >
        {t("task:noAgentsYet2")}
      </div>
    </div>
  );
}
