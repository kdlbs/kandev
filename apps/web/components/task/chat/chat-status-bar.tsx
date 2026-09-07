"use client";

/**
 * The status row above the chat composer: todos, the autopilot / dependency /
 * PR / MR chips, the queue chip, archive banners, transcript navigation, and
 * the proceed button.
 *
 * Split out of `chat-input-area.tsx`, which was at its 600-line limit. The row
 * is a self-contained unit: it reads the task and session ids and renders
 * indicators, so nothing here participates in composing or sending a message.
 */

import { IconArrowRight } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { ReactNode } from "react";
import { useAppStore } from "@/components/state-provider";
import { PRStatusChip } from "@/components/github/pr-status-chip";
import { MRStatusChip } from "@/components/gitlab/mr-status-chip";
import { TaskDependencyChip } from "@/components/task/task-dependency-chip";
import { AzureDevOpsTaskPullRequestChip } from "@/components/azure-devops/azure-devops-task-pull-request-chip";
import { RegisteredChangeRequestStatus } from "@/components/integrations/registered-change-request-status";
import { shareableSessionStateClient } from "@/components/task/share/share-button";
import { TranscriptNavGroup } from "@/components/task/chat/transcript-nav-group";
import { OpenInThreadsButton } from "@/components/threads/open-in-threads-button";
import { useIsDeckThread } from "@/hooks/domains/threads/use-deck-thread";
import { TodoIndicator } from "./todo-indicator";
import { AutoScrollToggleButton } from "./auto-scroll-toggle-button";
import { PRMergedBanner, PRClosedBanner } from "./pr-archive-banners";
import { AutopilotChatChip, useTaskAutopilot } from "./task-autopilot-chat-chip";

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
}: {
  hasTask: boolean;
  hasTodos: boolean;
  hasQueueChip: boolean;
  showRightControls: boolean;
  showProceed: boolean;
}): boolean {
  return hasTask || hasTodos || hasQueueChip || showRightControls || showProceed;
}

function getRightControlVisibility({
  taskId,
  sessionId,
  sessionState,
  showAutoScrollControl,
  showScrollToLastPrompt,
  showScrollToStart,
  showThreadsLink,
}: {
  taskId: string | null;
  sessionId: string | null;
  sessionState: string | null;
  showAutoScrollControl: boolean;
  showScrollToLastPrompt: boolean | undefined;
  showScrollToStart: boolean | undefined;
  showThreadsLink: boolean;
}) {
  const canShare = !!taskId && !!sessionId && shareableSessionStateClient(sessionState);
  const showRightControls =
    (showAutoScrollControl && !!sessionId) ||
    canShare ||
    showThreadsLink ||
    !!showScrollToLastPrompt ||
    !!showScrollToStart;
  return { canShare, showRightControls };
}

/**
 * Row above the composer showing todo progress, PR/CI status chips,
 * merged/closed PR banners, the auto-scroll toggle + Share (right-aligned),
 * and a "move to next step" action when the workflow allows it.
 */
export type ChatStatusBarProps = {
  todoItems: TodoDisplayItem[];
  taskId: string | null;
  sessionId: string | null;
  sessionState: string | null;
  nextStepName: string | null;
  onProceed: () => void;
  isAgentBusy: boolean;
  isMoving: boolean;
  queueChip?: ReactNode;
  showScrollToLastPrompt?: boolean;
  onScrollToLastPrompt?: () => void;
  lastPromptScrollDirection?: "up" | "down";
  showScrollToStart?: boolean;
  onScrollToStart?: () => void;
};

export function ChatStatusBar({
  todoItems,
  taskId,
  sessionId,
  sessionState,
  nextStepName,
  onProceed,
  isAgentBusy,
  isMoving,
  queueChip,
  showScrollToLastPrompt,
  onScrollToLastPrompt,
  lastPromptScrollDirection,
  showScrollToStart,
  onScrollToStart,
}: ChatStatusBarProps) {
  const { t } = useTranslation();
  const showTodos = todoItems.length > 0;
  const showProceed = !!nextStepName && !isAgentBusy;
  const autopilot = useTaskAutopilot(taskId);
  const showAutoScrollControl = useAppStore(
    (state) => state.userSettings.showTranscriptAutoScrollControl,
  );
  // Asked here rather than inside the button so the cluster still renders when
  // the Threads jump is the only right-hand control this session qualifies for.
  const showThreadsLink = useIsDeckThread(taskId, sessionId);
  const { canShare, showRightControls } = getRightControlVisibility({
    taskId,
    sessionId,
    sessionState,
    showAutoScrollControl,
    showScrollToLastPrompt,
    showScrollToStart,
    showThreadsLink,
  });
  if (
    !shouldRenderChatStatusBar({
      hasTask: !!taskId,
      hasTodos: showTodos,
      hasQueueChip: !!queueChip,
      showRightControls,
      showProceed,
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
      <PRStatusChip taskId={taskId} />
      <MRStatusChip taskId={taskId} />
      <AzureDevOpsTaskPullRequestChip taskId={taskId} />
      <RegisteredChangeRequestStatus taskId={taskId} sessionId={sessionId} surface="composer" />
      {queueChip}
      {/* Distinct per-banner keys: the key remounts the banner on task switch
          so its dismissed state re-initialises, and keeping the two suffixes
          different avoids a duplicate-sibling-key collision. */}
      {taskId && <PRMergedBanner key={`${taskId}-merged`} taskId={taskId} />}
      {taskId && <PRClosedBanner key={`${taskId}-closed`} taskId={taskId} />}
      {showRightControls && (
        <div className="ml-auto flex shrink-0 items-center gap-1.5">
          <OpenInThreadsButton taskId={taskId} sessionId={sessionId} />
          {sessionId && <AutoScrollToggleButton sessionId={sessionId} />}
          <TranscriptNavGroup
            canShare={canShare}
            taskId={taskId}
            sessionId={sessionId}
            showScrollToLastPrompt={showScrollToLastPrompt}
            onScrollToLastPrompt={onScrollToLastPrompt}
            lastPromptScrollDirection={lastPromptScrollDirection}
            showScrollToStart={showScrollToStart}
            onScrollToStart={onScrollToStart}
          />
        </div>
      )}
      {showProceed && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              type="button"
              variant="outline"
              size="sm"
              className={`${showRightControls ? "" : "ml-auto "}h-6 gap-1 px-2.5 text-xs cursor-pointer text-primary`}
              onClick={onProceed}
              disabled={isMoving}
              data-testid="proceed-next-step"
            >
              {nextStepName}
              <IconArrowRight className="h-3.5 w-3.5" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t("task:moveTaskToTheNextWorkflow")}</TooltipContent>
        </Tooltip>
      )}
    </div>
  );
}
