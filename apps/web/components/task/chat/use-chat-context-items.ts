import { useMemo } from "react";
import { useCustomPrompts } from "@/hooks/domains/settings/use-custom-prompts";
import { usePanelActions } from "@/hooks/use-panel-actions";
import type { ContextFile } from "@/lib/state/context-files-store";
import type { ContextItem } from "@/lib/types/context";
import type { TaskPreviewFeedback } from "@/lib/types/http";
import { buildContextItems } from "../chat-context-items";
import type { CommentsState } from "./use-chat-panel-state";

type ChatContextItemsOptions = {
  planContextEnabled: boolean;
  contextFiles: ContextFile[];
  resolvedSessionId: string | null;
  removeContextFile: (sid: string, path: string) => void;
  unpinFile: (sid: string, path: string) => void;
  comments: CommentsState;
  previewFeedback: TaskPreviewFeedback[];
  taskId: string | null;
  onOpenFile?: (path: string, repo?: string) => void;
  onOpenFileAtLine?: (filePath: string, repositoryName?: string) => void;
  onOpenPreviewFeedback: () => void;
};

export function useChatContextItems(opts: ChatContextItemsOptions) {
  const {
    planContextEnabled,
    contextFiles,
    resolvedSessionId,
    removeContextFile,
    unpinFile,
    comments,
    previewFeedback,
    taskId,
    onOpenFile,
    onOpenFileAtLine,
    onOpenPreviewFeedback,
  } = opts;
  const { addPlan } = usePanelActions();
  const { prompts } = useCustomPrompts();

  const promptsMap = useMemo(() => {
    const map = new Map<string, { content: string }>();
    for (const p of prompts) map.set(p.id, { content: p.content });
    return map;
  }, [prompts]);

  const contextItems = useMemo<ContextItem[]>(
    () =>
      buildContextItems({
        planContextEnabled,
        contextFiles,
        resolvedSessionId,
        removeContextFile,
        unpinFile,
        addPlan,
        promptsMap,
        onOpenFile,
        pendingCommentsByFile: comments.pendingCommentsByFile,
        handleRemoveCommentFile: comments.handleRemoveCommentFile,
        handleRemoveComment: comments.handleRemoveComment,
        onOpenFileAtLine,
        planComments: comments.planComments,
        handleClearPlanComments: comments.clearSessionPlanComments,
        previewFeedback,
        pendingPRFeedback: comments.pendingPRFeedback,
        handleRemovePRFeedback: comments.handleRemovePRFeedback,
        handleClearPRFeedback: comments.handleClearPRFeedback,
        walkthroughComments: comments.walkthroughComments,
        handleRemoveWalkthroughComment: comments.handleRemoveWalkthroughComment,
        handleClearWalkthroughComments: comments.handleClearWalkthroughComments,
        messageComments: comments.messageComments,
        handleClearMessageComments: comments.handleClearMessageComments,
        taskId,
        onOpenPreviewFeedback,
      }),
    [
      planContextEnabled,
      contextFiles,
      resolvedSessionId,
      removeContextFile,
      unpinFile,
      addPlan,
      promptsMap,
      onOpenFile,
      comments.pendingCommentsByFile,
      comments.handleRemoveCommentFile,
      comments.handleRemoveComment,
      onOpenFileAtLine,
      comments.planComments,
      comments.clearSessionPlanComments,
      previewFeedback,
      comments.pendingPRFeedback,
      comments.handleRemovePRFeedback,
      comments.handleClearPRFeedback,
      comments.walkthroughComments,
      comments.handleRemoveWalkthroughComment,
      comments.handleClearWalkthroughComments,
      comments.messageComments,
      comments.handleClearMessageComments,
      taskId,
      onOpenPreviewFeedback,
    ],
  );

  return { contextItems, prompts };
}
