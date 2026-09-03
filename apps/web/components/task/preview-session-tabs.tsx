"use client";

import { useCallback, useLayoutEffect, useMemo, useState } from "react";
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
import { PreviewPlanPanel, usePreviewPlanSummary } from "./preview-plan-panel";
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

/**
 * Read-only session tabs for the kanban preview panel.
 *
 * Tabs only switch between existing sessions — creating or deleting sessions
 * is deliberately restricted to the full-page task view.
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

  // Do not force the Plan view while the session list is still loading. The
  // first render can have zero sessions before the list arrives, and treating
  // that transient state as sessionless would mark an unseen plan as seen.
  const hasNoSessions = isLoaded && sortedSessions.length === 0;
  const planTabState = usePreviewPlanTab(taskId, hasNoSessions, onSessionChange);
  const { viewMode, planTab, hasPlan } = planTabState;

  const tabs = useMemo<SessionTab[]>(
    () => [
      ...sortedSessions.map((session) => {
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
          className: PREVIEW_TAB_CLASS_NAME,
        };
      }),
      planTab,
    ],
    [sortedSessions, agentLabelsById, profilesById, planTab],
  );

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
