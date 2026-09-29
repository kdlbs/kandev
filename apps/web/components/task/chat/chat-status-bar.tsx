"use client";

/**
 * The status row above the chat composer: todos, the autopilot / dependency /
 * PR / MR chips, the queue chip, archive banners, right-hand chat controls,
 * and the proceed button.
 *
 * Split out of `chat-input-area.tsx`, which was at its 600-line limit. The row
 * is a self-contained unit: it reads the task and session ids and renders
 * indicators, so nothing here participates in composing or sending a message.
 */

import type { WorkflowMovePreviewTarget } from "../workflow-move-preview-footer";
import { useMemo, type ReactNode } from "react";
import { useAppStore } from "@/components/state-provider";
import { WorkflowMoveProceedButton } from "@/components/task/workflow-move-proceed-button";
import type { WorkflowMoveEntryOptions } from "@/lib/api/domains/kanban-api";
import { PRStatusChip } from "@/components/github/pr-status-chip";
import { MRStatusChip } from "@/components/gitlab/mr-status-chip";
import { TaskDependencyChip } from "@/components/task/task-dependency-chip";
import { AzureDevOpsTaskPullRequestChip } from "@/components/azure-devops/azure-devops-task-pull-request-chip";
import { RegisteredChangeRequestStatus } from "@/components/integrations/registered-change-request-status";
import { shareableSessionStateClient } from "@/components/task/share/share-button";
import { ConversationUsageDisplay } from "@/components/task/chat/conversation-usage-display";
import { TranscriptNavGroup } from "@/components/task/chat/transcript-nav-group";
import { OpenInThreadsButton } from "@/components/threads/open-in-threads-button";
import { useIsDeckThread } from "@/hooks/domains/threads/use-deck-thread";
import { TodoIndicator } from "./todo-indicator";
import { AutoScrollToggleButton } from "./auto-scroll-toggle-button";
import { PRMergedBanner, PRClosedBanner } from "./pr-archive-banners";
import { AutopilotChatChip, useTaskAutopilot } from "./task-autopilot-chat-chip";
import { AgentGoalChip } from "./agent-goal-chip";
import { BackgroundWorkChip } from "./background-work/background-work-chip";
import { shouldShowProceed } from "./types";
import { getAgentGoal } from "@/lib/agent-goal";
import { useComposerDisclosureContext } from "./composer-disclosure";
import { cn } from "@/lib/utils";

type TodoDisplayItem = {
  text: string;
  done?: boolean;
  status?: "pending" | "in_progress" | "completed" | "failed";
};

/**
 * The status row belongs to the TASK, not to the session.
 *
 * `panelState.taskId` is session-derived and is null on a task that has no
 * session yet, which is exactly the state a blocked dependency-chain step sits
 * in: with no id the whole row is hidden, taking the dependency chip with it on
 * the one kind of task the chip is about. Hosts that know their task pass it as
 * `statusTaskId`. The session-derived id still wins when present, so hosts that
 * pass nothing are unchanged.
 */
export function resolveStatusRowTaskId(
  sessionTaskId: string | null,
  statusTaskId: string | null,
): string | null {
  return sessionTaskId ?? statusTaskId;
}

export function shouldRenderChatStatusBar({
  hasTask,
  hasTodos,
  hasQueueChip,
  showRightControls,
  showProceed,
  hasGoal,
}: {
  hasTask: boolean;
  hasTodos: boolean;
  hasQueueChip: boolean;
  showRightControls: boolean;
  showProceed: boolean;
  hasGoal?: boolean;
}): boolean {
  return hasTask || hasTodos || hasQueueChip || showRightControls || showProceed || !!hasGoal;
}

function readACPMetadata(metadata: Record<string, unknown> | null | undefined): unknown {
  const acp = metadata?.acp;
  if (!acp || typeof acp !== "object" || Array.isArray(acp)) return undefined;
  return (acp as Record<string, unknown>).meta;
}

function getRightControlVisibility({
  taskId,
  sessionId,
  sessionState,
  showAutoScrollControl,
  showScrollToLastPrompt,
  showScrollToStart,
  showJumpToLatest,
  showThreadsLink,
}: {
  taskId: string | null;
  sessionId: string | null;
  sessionState: string | null;
  showAutoScrollControl: boolean;
  showScrollToLastPrompt: boolean | undefined;
  showScrollToStart: boolean | undefined;
  showJumpToLatest: boolean | undefined;
  showThreadsLink: boolean;
}) {
  const canShare = !!taskId && !!sessionId && shareableSessionStateClient(sessionState);
  const showConversationUsage = !!taskId && !!sessionId;
  const showRightControls =
    (showAutoScrollControl && !!sessionId) ||
    showConversationUsage ||
    canShare ||
    showThreadsLink ||
    !!showScrollToLastPrompt ||
    !!showScrollToStart ||
    !!showJumpToLatest;
  return { canShare, showConversationUsage, showRightControls };
}

/**
 * Row above the composer showing todo progress, PR/CI status chips,
 * merged/closed PR banners, the right-aligned chat controls, and a "move to
 * next step" action when the workflow allows it.
 */
export type ChatStatusBarProps = {
  todoItems: TodoDisplayItem[];
  taskId: string | null;
  sessionId: string | null;
  sessionState: string | null;
  previewTarget?: WorkflowMovePreviewTarget;
  nextStepName: string | null;
  onProceed: (options?: WorkflowMoveEntryOptions) => boolean | void | Promise<boolean | void>;
  isAgentBusy: boolean;
  hasPendingClarification: boolean;
  isMoving: boolean;
  queueChip?: ReactNode;
  showScrollToLastPrompt?: boolean;
  onScrollToLastPrompt?: () => void;
  lastPromptScrollDirection?: "up" | "down";
  showJumpToLatest?: boolean;
  onJumpToLatest?: () => void;
  showScrollToStart?: boolean;
  onScrollToStart?: () => void;
};

export function ComposerCIStatus({
  taskId,
  sessionId,
  standalone = false,
}: {
  taskId: string | null;
  sessionId: string | null;
  standalone?: boolean;
}) {
  return (
    <div
      className={cn("flex flex-wrap items-center gap-1.5 empty:hidden", standalone && "px-2 py-1")}
    >
      <PRStatusChip taskId={taskId} />
      <MRStatusChip taskId={taskId} />
      <AzureDevOpsTaskPullRequestChip taskId={taskId} />
      <RegisteredChangeRequestStatus taskId={taskId} sessionId={sessionId} surface="composer" />
    </div>
  );
}

function ChatStatusBarArchiveBanners({ taskId }: { taskId: string | null }) {
  if (!taskId) return null;
  return (
    <>
      {/* Distinct keys remount each banner on task switch and avoid a duplicate-sibling-key collision. */}
      <PRMergedBanner key={`${taskId}-merged`} taskId={taskId} />
      <PRClosedBanner key={`${taskId}-closed`} taskId={taskId} />
    </>
  );
}

function ChatStatusBarRightControls({
  canShare,
  showAutoScrollControl,
  showConversationUsage,
  showThreadsLink,
  taskId,
  sessionId,
  showJumpToLatest,
  onJumpToLatest,
  showScrollToLastPrompt,
  onScrollToLastPrompt,
  lastPromptScrollDirection,
  showScrollToStart,
  onScrollToStart,
}: Pick<
  ChatStatusBarProps,
  | "taskId"
  | "sessionId"
  | "showJumpToLatest"
  | "onJumpToLatest"
  | "showScrollToLastPrompt"
  | "onScrollToLastPrompt"
  | "lastPromptScrollDirection"
  | "showScrollToStart"
  | "onScrollToStart"
> & {
  canShare: boolean;
  showAutoScrollControl: boolean;
  showConversationUsage: boolean;
  showThreadsLink: boolean;
}) {
  return (
    <div
      data-testid="chat-status-bar-right-controls"
      className="ml-auto flex shrink-0 items-center gap-1.5 empty:hidden"
    >
      {showThreadsLink && <OpenInThreadsButton taskId={taskId} sessionId={sessionId} />}
      {showAutoScrollControl && sessionId && <AutoScrollToggleButton sessionId={sessionId} />}
      <TranscriptNavGroup
        canShare={canShare}
        taskId={taskId}
        sessionId={sessionId}
        showJumpToLatest={showJumpToLatest}
        onJumpToLatest={onJumpToLatest}
        showScrollToLastPrompt={showScrollToLastPrompt}
        onScrollToLastPrompt={onScrollToLastPrompt}
        lastPromptScrollDirection={lastPromptScrollDirection}
        showScrollToStart={showScrollToStart}
        onScrollToStart={onScrollToStart}
        usageControl={
          showConversationUsage && taskId && sessionId ? (
            <ConversationUsageDisplay taskId={taskId} sessionId={sessionId} />
          ) : null
        }
      />
    </div>
  );
}

export function ChatStatusBar({
  todoItems,
  taskId,
  sessionId,
  sessionState,
  previewTarget,
  nextStepName,
  onProceed,
  isAgentBusy,
  hasPendingClarification,
  isMoving,
  queueChip,
  showScrollToLastPrompt,
  onScrollToLastPrompt,
  lastPromptScrollDirection,
  showJumpToLatest,
  onJumpToLatest,
  showScrollToStart,
  onScrollToStart,
}: ChatStatusBarProps) {
  const separateCI = useComposerDisclosureContext()?.enabled;
  const showTodos = todoItems.length > 0;
  const showProceed = shouldShowProceed(nextStepName, isAgentBusy, hasPendingClarification);
  const autopilot = useTaskAutopilot(taskId);
  const showAutoScrollControl = useAppStore(
    (state) => state.userSettings.showTranscriptAutoScrollControl,
  );
  // Asked here rather than inside the button so the cluster still renders when
  // the Threads jump is the only right-hand control this session qualifies for.
  const showThreadsLink = useIsDeckThread(taskId, sessionId);
  const activeGoalMetadata = useAppStore((state) =>
    sessionId ? readACPMetadata(state.taskSessions.items[sessionId]?.metadata) : undefined,
  );
  const activeGoal = useMemo(() => {
    const goal = getAgentGoal(activeGoalMetadata);
    return goal?.status === "active" ? goal : null;
  }, [activeGoalMetadata]);
  const { canShare, showConversationUsage, showRightControls } = getRightControlVisibility({
    taskId,
    sessionId,
    sessionState,
    showAutoScrollControl,
    showScrollToLastPrompt,
    showScrollToStart,
    showJumpToLatest,
    showThreadsLink,
  });
  if (
    !shouldRenderChatStatusBar({
      hasTask: !!taskId,
      hasTodos: showTodos,
      hasQueueChip: !!queueChip,
      showRightControls,
      showProceed,
      hasGoal: !!activeGoal,
    })
  ) {
    return null;
  }
  // PRMergedBanner returns null internally when not applicable
  return (
    <div
      data-testid="chat-status-bar"
      className="flex min-w-0 flex-wrap items-center gap-1.5 py-1 text-xs text-muted-foreground"
    >
      {showTodos && <TodoIndicator todos={todoItems} />}
      {autopilot && <AutopilotChatChip />}
      <TaskDependencyChip taskId={taskId} />
      {!separateCI && <ComposerCIStatus taskId={taskId} sessionId={sessionId} />}
      {activeGoal && <AgentGoalChip key={sessionId ?? "none"} goal={activeGoal} />}
      <BackgroundWorkChip sessionId={sessionId} />
      {queueChip}
      <ChatStatusBarArchiveBanners taskId={taskId} />
      {showRightControls && (
        <ChatStatusBarRightControls
          canShare={canShare}
          showAutoScrollControl={showAutoScrollControl}
          showConversationUsage={showConversationUsage}
          showThreadsLink={showThreadsLink}
          taskId={taskId}
          sessionId={sessionId}
          showJumpToLatest={showJumpToLatest}
          onJumpToLatest={onJumpToLatest}
          showScrollToLastPrompt={showScrollToLastPrompt}
          onScrollToLastPrompt={onScrollToLastPrompt}
          lastPromptScrollDirection={lastPromptScrollDirection}
          showScrollToStart={showScrollToStart}
          onScrollToStart={onScrollToStart}
        />
      )}
      {showProceed && nextStepName && (
        <WorkflowMoveProceedButton
          previewTarget={previewTarget}
          nextStepName={nextStepName}
          onProceed={onProceed}
          isMoving={isMoving}
          className={`${showRightControls ? "" : "ml-auto "}h-6`}
          testId="proceed-next-step"
        />
      )}
    </div>
  );
}
