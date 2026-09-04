"use client";

import { useCallback, useLayoutEffect, useMemo, useRef, useState } from "react";
import { AgentLogo } from "@/components/agent-logo";
import { GridSpinner } from "@/components/grid-spinner";
import { PanelLoadingState } from "@/components/panel-loading-state";
import { SessionTabs, type SessionTab } from "@/components/session-tabs";
import { useAppStore } from "@/components/state-provider";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";
import { useSessionResumption } from "@/hooks/domains/session/use-session-resumption";
import { useTaskSessions } from "@/hooks/use-task-sessions";
import type { UseEnsureTaskSessionResult } from "@/hooks/domains/session/use-ensure-task-session";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { TaskSession } from "@/lib/types/http";
import { EnsureSessionErrorEmptyState, SessionRecoveryFeedback } from "./ensure-session-error";
import { PassthroughToolbar } from "./passthrough-toolbar";
import { PreviewSessionTabMenu } from "./preview-session-tab-menu";
import { SessionTabDialogs } from "./session-tab-menu";
import { TabRenameInput } from "./tab-rename-input";
import { PreviewPlanPanel, usePreviewPlanSummary } from "./preview-plan-panel";
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
const PLAN_TAB_ID = "plan";
const PREVIEW_TAB_CLASS_NAME =
  "bg-muted/50 data-[state=active]:bg-muted [@media(pointer:coarse)]:!min-h-11";

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
          className: PREVIEW_TAB_CLASS_NAME,
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

/**
 * Session tabs for the kanban preview panel.
 *
 * Right-click a tab for the same lifecycle context menu as the full-page
 * task view (rename, set primary, stop/resume, delete, share, handoff).
 * Creating a new session is still restricted to the full-page view.
 */
// eslint-disable-next-line max-lines-per-function -- preview owns the shared session and Plan-tab composition.
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
    (state) =>
      (
        state.kanban.tasks.find((task) => task.id === taskId) ??
        findTaskInSnapshots(taskId, state.kanbanMulti.snapshots)
      )?.primarySessionId ?? null,
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
  // `handleSessionRemoved` is captured once by `useSessionActions`'s `remove`
  // closure at the moment delete is confirmed, and `session.delete` can take
  // a WS round-trip to settle. Reading these off refs (updated every render)
  // instead of closing over `activeSessionId`/`sortedSessions` means the
  // eventual `onDeleted` call sees which tab is active *now*, not which one
  // was active when the delete was requested.
  const activeSessionIdRef = useRef(activeSessionId);
  activeSessionIdRef.current = activeSessionId;
  const sortedSessionsRef = useRef(sortedSessions);
  sortedSessionsRef.current = sortedSessions;
  const handleSessionRemoved = useCallback(
    (removedSessionId: string) => {
      if (removedSessionId !== activeSessionIdRef.current) return;
      const remaining = sortedSessionsRef.current.filter((s) => s.id !== removedSessionId);
      onSessionChange?.(remaining[0]?.id ?? null);
    },
    [onSessionChange],
  );

  // Do not force the Plan view while the session list is still loading. The
  // first render can have zero sessions before the list arrives, and treating
  // that transient state as sessionless would mark an unseen plan as seen.
  const hasNoSessions = isLoaded && sortedSessions.length === 0;
  const planTabState = usePreviewPlanTab(taskId, hasNoSessions, onSessionChange);
  const { viewMode, planTab, hasPlan } = planTabState;

  const sessionTabs = useBuildPreviewTabs({
    sortedSessions,
    profilesById,
    agentLabelsById,
    taskId,
    isPrimarySession,
    dialogs,
    handleSessionRemoved,
  });

  const tabs = useMemo<SessionTab[]>(() => [...sessionTabs, planTab], [sessionTabs, planTab]);

  if (!isLoaded && sortedSessions.length === 0) {
    return <PreviewLoadingState label={t("task:loadingAgents")} />;
  }

  if (hasNoSessions && !hasPlan) {
    return (
      <PreviewNoSessionsState
        ensureSession={ensureSession}
        resumption={resumption}
        workspaceId={workspaceId}
      />
    );
  }

  // A task can lose every session (deletion doesn't delete its plan) and
  // still have a plan worth showing — fall back to the Plan view rather than
  // an empty session body when there's nothing else to select.
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
          activeTab={viewMode === "plan" ? PLAN_TAB_ID : (activeSessionId ?? "")}
          onTabChange={planTabState.handleTabChange}
          listClassName="bg-transparent p-0 !h-7 gap-1 overflow-x-auto overflow-y-hidden min-w-0 shrink [@media(pointer:coarse)]:!h-11 [&::-webkit-scrollbar]:hidden [-ms-overflow-style:none] [scrollbar-width:none]"
        />
      </div>
      <div className="flex-1 min-h-0">
        <PreviewTabBody
          viewMode={viewMode}
          planState={planTabState.planState}
          activeSession={activeSession}
          taskId={taskId}
        />
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

/** Renders the Plan tab's content or the active session's chat, per `viewMode`. */
function PreviewTabBody({
  viewMode,
  planState,
  activeSession,
  taskId,
}: {
  viewMode: "session" | "plan";
  planState: ReturnType<typeof usePreviewPlanSummary>;
  activeSession: TaskSession | null;
  taskId: string;
}) {
  if (viewMode === "plan") {
    return <PreviewPlanPanel {...planState} onRetry={planState.retry} />;
  }
  if (!activeSession) return null;
  return <PreviewSessionBody key={activeSession.id} session={activeSession} taskId={taskId} />;
}

/**
 * Owns the preview panel's Plan tab: local session-vs-plan view mode, the
 * unseen-plan indicator (mirrors `plan-tab.tsx`'s dot logic, scoped to this
 * task instead of the global active task), and clearing it on selection.
 */
function usePreviewPlanTab(
  taskId: string,
  hasNoSessions: boolean,
  onSessionChange?: (sessionId: string | null) => void,
) {
  const { t } = useTranslation();
  const planState = usePreviewPlanSummary(taskId);
  const { plan, loaded, failed, retry } = planState;
  const hasPlan = loaded && plan !== null;
  const lastSeenPlanAt = useAppStore((state) => state.taskPlans.lastSeenUpdatedAtByTaskId[taskId]);
  const markTaskPlanSeen = useAppStore((state) => state.markTaskPlanSeen);
  const hasUnseenPlan = plan?.created_by === "agent" && lastSeenPlanAt !== plan.updated_at;

  const [viewMode, setViewMode] = useState<"session" | "plan">("session");
  // A new preview task shouldn't inherit the previous task's Plan selection.
  // Reset during render (not a passive effect) because `PreviewSessionTabs`
  // is reused across tasks on card click without remounting: a passive
  // effect runs after the layout effect below, so it would still see
  // `viewMode === "plan"` on the taskId-change render and mark the new
  // task's plan seen before the reset took effect.
  const [prevTaskId, setPrevTaskId] = useState(taskId);
  if (prevTaskId !== taskId) {
    setPrevTaskId(taskId);
    setViewMode("session");
  }

  const displayViewMode = hasNoSessions && hasPlan ? "plan" : viewMode;

  // While the Plan tab is the active view, re-mark seen whenever the plan's
  // updated_at changes. Covers both clicking Plan before the fetch resolves
  // (nothing is marked seen until the plan actually arrives) and a WS plan
  // update landing while the tab is already open. useLayoutEffect so the
  // seen-mark commits before paint, matching plan-tab.tsx's dot logic.
  const planUpdatedAt = plan?.updated_at;
  useLayoutEffect(() => {
    if (displayViewMode === "plan" && planUpdatedAt !== undefined) {
      markTaskPlanSeen(taskId);
    }
  }, [displayViewMode, taskId, markTaskPlanSeen, planUpdatedAt]);

  const handleTabChange = useCallback(
    (id: string) => {
      if (id === PLAN_TAB_ID) {
        setViewMode("plan");
        if (loaded) markTaskPlanSeen(taskId);
        // A prior fetch for this task failed; re-selecting the tab is the
        // user's explicit signal to try again, since the panel is reused
        // across tasks without remounting and would otherwise stay stuck.
        else if (failed) retry();
      } else {
        setViewMode("session");
        onSessionChange?.(id);
      }
    },
    [taskId, loaded, failed, retry, markTaskPlanSeen, onSessionChange],
  );

  const planTab = useMemo<SessionTab>(
    () => ({
      id: PLAN_TAB_ID,
      label: t("task:plan"),
      icon: <PlanTabIcon hasUnseen={hasUnseenPlan} />,
      testId: "preview-plan-tab",
      className: PREVIEW_TAB_CLASS_NAME,
    }),
    [t, hasUnseenPlan],
  );

  return { viewMode: displayViewMode, handleTabChange, planTab, planState, hasPlan };
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

/** Matches the full-page Plan tab's unseen dot (`plan-tab.tsx`), scoped to the previewed task. */
function PlanTabIcon({ hasUnseen }: { hasUnseen: boolean }) {
  return (
    <span aria-hidden="true" className="relative inline-flex h-3 w-3 shrink-0 items-center">
      <span className="h-1.5 w-1.5 rounded-full bg-muted-foreground/40" />
      {hasUnseen && (
        <span
          data-testid="preview-plan-tab-indicator"
          className="absolute -top-0.5 -right-0.5 size-1.5 rounded-full bg-primary"
        />
      )}
    </span>
  );
}

export function PreviewSessionBody({ session, taskId }: { session: TaskSession; taskId: string }) {
  if (session.is_passthrough) {
    return <PassthroughToolbar sessionId={session.id} taskId={taskId} />;
  }

  return (
    <div className="flex h-full flex-col">
      <TaskChatPanel
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

/** Sessionless state: preparing/error/empty, per `ensureSession`. */
function PreviewNoSessionsState({
  ensureSession,
  resumption,
  workspaceId,
}: {
  ensureSession?: UseEnsureTaskSessionResult;
  resumption: ReturnType<typeof useSessionResumption>;
  workspaceId?: string | null;
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
  return <PreviewEmptyState />;
}
