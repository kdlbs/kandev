"use client";

import { useCallback, type RefObject } from "react";
import { QueryClient, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/toast-provider";
import { useAppStore } from "@/components/state-provider";
import { useCommentsStore } from "@/lib/state/slices/comments/comments-store";
import { useAllWorkflowSnapshots } from "@/hooks/domains/kanban/use-all-workflow-snapshots";
import { formatReviewCommentsAsMarkdown } from "@/lib/state/slices/comments/format";
import { buildSubmitMessage } from "./chat/chat-input-area";
import {
  ChatInputContainer,
  type ChatInputContainerHandle,
  type MessageAttachment,
} from "./chat/chat-input-container";
import type { useChatPanelState } from "./chat/use-chat-panel-state";
import type { DiffComment } from "@/lib/diff/types";
import type { ContextFile } from "@/lib/state/context-files-store";
import type { TaskMentionData } from "@/hooks/use-inline-mention";
import { buildContextFilesContext, buildTaskMentionsContext } from "@/hooks/use-message-handler";
import { getWebSocketClient } from "@/lib/ws/connection";
import { taskPlanQueryOptions } from "@/lib/query/query-options";
import type { TaskPlan } from "@/lib/types/http";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";

const PLAN_CONTEXT_PATH = "plan:context";
const standalonePlanQueryClient = new QueryClient({
  defaultOptions: { queries: { retry: false } },
});

export type PassthroughSubmitHandler = (
  content: string,
  reviewComments?: DiffComment[],
  attachments?: MessageAttachment[],
  inlineMentions?: ContextFile[],
  inlineTaskMentions?: TaskMentionData[],
) => Promise<void>;

export function PassthroughComposerPanel({
  refHandle,
  onSubmit,
  onCancel,
  panelState,
  taskId,
  isMoving,
  isSending,
  onImplementPlan,
}: {
  refHandle: RefObject<ChatInputContainerHandle | null>;
  onSubmit: PassthroughSubmitHandler;
  onCancel: () => void;
  panelState: ReturnType<typeof useChatPanelState>;
  taskId: string | null;
  isMoving: boolean;
  isSending: boolean;
  onImplementPlan?: (fresh: boolean) => void;
}) {
  const hasContextComments =
    panelState.planComments.length > 0 ||
    panelState.pendingPRFeedback.length > 0 ||
    panelState.walkthroughComments.length > 0;
  return (
    <div
      data-testid="passthrough-composer"
      onKeyDownCapture={(event) => {
        if (event.key === "Escape") onCancel();
      }}
    >
      <ChatInputContainer
        ref={refHandle}
        onSubmit={onSubmit}
        sessionId={panelState.resolvedSessionId}
        taskId={taskId}
        taskTitle={panelState.task?.title}
        taskDescription={panelState.taskDescription ?? ""}
        planModeEnabled={panelState.planModeEnabled}
        planModeAvailable={panelState.planModeAvailable}
        mcpServers={panelState.mcpServers}
        onPlanModeChange={panelState.handlePlanModeChange}
        isAgentBusy={false}
        isStarting={panelState.isStarting}
        isPreparingEnvironment={panelState.isPreparingEnvironment}
        isMoving={isMoving}
        isSending={isSending}
        onCancel={onCancel}
        placeholder="Type a message, @mention files or prompts, Shift+Enter for newline"
        pendingCommentsByFile={panelState.pendingCommentsByFile}
        hasContextComments={hasContextComments}
        submitKey={panelState.chatSubmitKey}
        hasAgentCommands={false}
        contextItems={panelState.contextItems}
        planContextEnabled={panelState.planContextEnabled}
        contextFiles={panelState.contextFiles}
        onToggleContextFile={panelState.handleToggleContextFile}
        onAddContextFile={panelState.handleAddContextFile}
        onImplementPlan={onImplementPlan}
        hideSessionsDropdown
        hideAgentControls
      />
    </div>
  );
}

type PassthroughFinalMessage = {
  content: string;
  commentsToSend: DiffComment[];
  contextFilesMeta?: Array<{ path: string; name: string }>;
};

export function formatPassthroughBaseMessage(
  content: string,
  reviewComments: DiffComment[] | undefined,
  pendingComments: DiffComment[],
  panelState: ReturnType<typeof useChatPanelState>,
) {
  const commentsToSend = reviewComments ?? pendingComments;
  const hasStructuredComments =
    !!reviewComments ||
    panelState.pendingPRFeedback.length > 0 ||
    panelState.planComments.length > 0 ||
    panelState.walkthroughComments.length > 0;
  if (hasStructuredComments) {
    return {
      formatted: buildSubmitMessage(
        content,
        commentsToSend.length > 0 ? commentsToSend : undefined,
        panelState.pendingPRFeedback,
        panelState.planComments,
        panelState.walkthroughComments,
      ),
      commentsToSend,
    };
  }
  if (pendingComments.length > 0) {
    return {
      formatted: formatReviewCommentsAsMarkdown(pendingComments) + content,
      commentsToSend,
    };
  }
  return { formatted: content, commentsToSend };
}

function hasPlanContext(files: ContextFile[]) {
  return files.some((file) => file.path === PLAN_CONTEXT_PATH);
}

function stripSelectedPlanMentions(content: string, files: ContextFile[]) {
  if (!hasPlanContext(files)) return content;
  return content.replace(/\s*@Plan(?=\s|$)/g, "").trim();
}

function sanitizeSystemBlockContent(content: string) {
  return content.replace(/<\/kandev-system>/gi, "</ kandev-system>");
}

export function buildPassthroughPlanContext(planContent: string | undefined | null) {
  const trimmed = planContent?.trim();
  if (!trimmed) return "";
  return (
    `\n\n<kandev-system>\n` +
    `CONTEXT PLAN: The user has attached the current task plan as context. ` +
    `Use this plan content to understand what they mean by the plan:\n` +
    `${sanitizeSystemBlockContent(trimmed)}\n` +
    `</kandev-system>`
  );
}

async function loadTaskPlanContent(taskId: string | null, queryClient: QueryClient) {
  if (!taskId) return "";
  const options = taskPlanQueryOptions(taskId);
  const cached = queryClient.getQueryData<TaskPlan | null>(options.queryKey);
  if (cached !== undefined) return cached?.content ?? "";
  const plan = await queryClient.fetchQuery(options);
  return plan?.content ?? "";
}

export async function buildPassthroughFinalMessage({
  taskId,
  content,
  reviewComments,
  pendingComments,
  panelState,
  inlineMentions,
  inlineTaskMentions,
  workflowSnapshots = {},
  queryClient = standalonePlanQueryClient,
}: {
  taskId: string | null;
  content: string;
  reviewComments?: DiffComment[];
  pendingComments: DiffComment[];
  panelState: ReturnType<typeof useChatPanelState>;
  inlineMentions?: ContextFile[];
  inlineTaskMentions?: TaskMentionData[];
  workflowSnapshots?: Record<string, WorkflowSnapshotData>;
  queryClient?: QueryClient;
}): Promise<PassthroughFinalMessage> {
  const { formatted, commentsToSend } = formatPassthroughBaseMessage(
    content,
    reviewComments,
    pendingComments,
    panelState,
  );
  const allContextFiles = [...panelState.contextFiles, ...(inlineMentions ?? [])];
  const visibleContent = stripSelectedPlanMentions(formatted, allContextFiles);
  const contextFilesContext = buildContextFilesContext(allContextFiles, panelState.prompts);
  const planContext = hasPlanContext(allContextFiles)
    ? buildPassthroughPlanContext(await loadTaskPlanContent(taskId, queryClient))
    : "";
  const taskMentionsContext =
    inlineTaskMentions && inlineTaskMentions.length > 0
      ? buildTaskMentionsContext(inlineTaskMentions, workflowSnapshots)
      : "";
  return {
    content: visibleContent + contextFilesContext + planContext + taskMentionsContext,
    commentsToSend,
    contextFilesMeta: buildContextFilesMeta(allContextFiles),
  };
}

export function buildContextFilesMeta(files: ContextFile[]) {
  const realContextFiles = files.filter(
    (f) => !f.path.startsWith("prompt:") && f.path !== PLAN_CONTEXT_PATH,
  );
  if (realContextFiles.length === 0) return undefined;
  return realContextFiles.map((f) => ({ path: f.path, name: f.name }));
}

async function requestPassthroughMessage({
  taskId,
  sessionId,
  message,
  attachments,
}: {
  taskId: string;
  sessionId: string;
  message: PassthroughFinalMessage;
  attachments?: MessageAttachment[];
}) {
  const client = getWebSocketClient();
  if (!client) throw new Error("WebSocket client not available");
  const hasAttachments = !!(attachments && attachments.length > 0);
  await client.request(
    "message.add",
    {
      task_id: taskId,
      session_id: sessionId,
      content: message.content,
      ...(hasAttachments && { attachments }),
      ...(message.contextFilesMeta && { context_files: message.contextFilesMeta }),
    },
    hasAttachments ? 30_000 : 10_000,
  );
}

export function clearPassthroughComposerContext(panelState: ReturnType<typeof useChatPanelState>) {
  if (panelState.pendingPRFeedback.length > 0) {
    panelState.handleClearPRFeedback();
  }
  if (panelState.planComments.length > 0) {
    panelState.clearSessionPlanComments();
  }
  if (panelState.walkthroughComments.length > 0) {
    panelState.handleClearWalkthroughComments();
  }
  if (!panelState.resolvedSessionId) return;
  panelState.clearEphemeral(panelState.resolvedSessionId);
  if (panelState.planModeEnabled) {
    panelState.addContextFile(panelState.resolvedSessionId, {
      path: "plan:context",
      name: "Plan",
    });
  }
}

export function useSendPassthroughMessage({
  taskId,
  sessionId,
  pendingComments,
  panelState,
  onSent,
}: {
  taskId: string | null;
  sessionId: string | null | undefined;
  pendingComments: DiffComment[];
  panelState: ReturnType<typeof useChatPanelState>;
  onSent: () => void;
}) {
  const { toast } = useToast();
  const markCommentsSent = useCommentsStore((s) => s.markCommentsSent);
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  const { snapshots } = useAllWorkflowSnapshots(activeWorkspaceId);
  const queryClient = useQueryClient();

  return useCallback(
    async (
      content: string,
      reviewComments?: DiffComment[],
      attachments?: MessageAttachment[],
      inlineMentions?: ContextFile[],
      inlineTaskMentions?: TaskMentionData[],
    ) => {
      if (!taskId || !sessionId) {
        toast({ title: "Session not ready", variant: "error" });
        throw new Error("Session not ready");
      }
      try {
        const message = await buildPassthroughFinalMessage({
          taskId,
          content,
          reviewComments,
          pendingComments,
          panelState,
          inlineMentions,
          inlineTaskMentions,
          workflowSnapshots: snapshots,
          queryClient,
        });
        await requestPassthroughMessage({ taskId, sessionId, message, attachments });
        if (message.commentsToSend.length > 0) {
          markCommentsSent(message.commentsToSend.map((c) => c.id));
        }
        clearPassthroughComposerContext(panelState);
        onSent();
      } catch (error) {
        console.error("Failed to send passthrough message:", error);
        toast({ title: "Failed to send message", variant: "error" });
        throw error;
      }
    },
    [
      taskId,
      sessionId,
      toast,
      pendingComments,
      panelState,
      snapshots,
      queryClient,
      markCommentsSent,
      onSent,
    ],
  );
}
