"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import {
  IconArrowRight,
  IconMessageCircle,
  IconMessageDots,
  IconSend,
  IconTrash,
  IconX,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { PRStatusChip } from "@/components/github/pr-status-chip";
import { MRStatusChip } from "@/components/gitlab/mr-status-chip";
import { TaskDependencyChip } from "@/components/task/task-dependency-chip";
import { AzureDevOpsTaskPullRequestChip } from "@/components/azure-devops/azure-devops-task-pull-request-chip";
import { RegisteredChangeRequestStatus } from "@/components/integrations/registered-change-request-status";
import { PRMergedBanner } from "./chat/pr-archive-banners";
import { type ChatInputContainerHandle } from "./chat/chat-input-container";
import { useChatPanelState } from "./chat/use-chat-panel-state";
import { useAppStore } from "@/components/state-provider";
import { usePlanActions } from "@/hooks/domains/kanban/use-plan-actions";
import { useKeyboardShortcut } from "@/hooks/use-keyboard-shortcut";
import { usePendingDiffCommentsByFile } from "@/hooks/domains/comments/use-diff-comments";
import { useCommentsStore } from "@/lib/state/slices/comments/comments-store";
import { useFileEditors } from "@/hooks/use-file-editors";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { getShortcut, isUnboundShortcut } from "@/lib/keyboard/shortcut-overrides";
import { formatShortcut } from "@/lib/keyboard/utils";
import type { KeyboardShortcut } from "@/lib/keyboard/constants";
import type { DiffComment } from "@/lib/diff/types";
import { PassthroughTerminal } from "./passthrough-terminal";
import { PassthroughComposerPanel, useSendPassthroughMessage } from "./passthrough-chat-composer";
import { Trans, useTranslation } from "react-i18next";

function isEditableElement(element: Element | null) {
  if (element instanceof HTMLElement && element.closest(".xterm")) return false;
  return (
    element instanceof HTMLInputElement ||
    element instanceof HTMLTextAreaElement ||
    element instanceof HTMLSelectElement ||
    (element instanceof HTMLElement && element.isContentEditable)
  );
}

function usePassthroughComposerShortcut({
  focusShortcut,
  setComposerOpen,
}: {
  focusShortcut: KeyboardShortcut;
  setComposerOpen: Dispatch<SetStateAction<boolean>>;
}) {
  useKeyboardShortcut(
    focusShortcut,
    useCallback(() => {
      const activeElement = document.activeElement;
      setComposerOpen((open) => {
        if (open) return false;
        if (isEditableElement(activeElement)) return open;
        return true;
      });
    }, [setComposerOpen]),
    { capture: true, enabled: !isUnboundShortcut(focusShortcut) },
  );
}

/**
 * PassthroughToolbar wraps the PTY terminal with the kandev surface that the
 * full ACP `ChatStatusBar` + `ChatInputArea` provide for chat mode: PR status,
 * merge banner, "Move to next step" workflow advancement, a collapsible Chat
 * compose box, and a collapsible Comments panel showing pending review comments
 * (each editable inline with a clickable file reference).
 *
 * Default focus stays on the xterm terminal — both Chat and Comments are
 * collapsed behind toolbar buttons so the user can keep raw-terminal
 * interaction front-and-centre and only opt in when they want to compose a
 * follow-up or review what's about to be attached. Ctrl-C in the xterm
 * cancels the agent, so no dedicated Stop button is needed.
 */
export function PassthroughToolbar({
  sessionId,
  taskId,
}: {
  sessionId: string | null | undefined;
  taskId: string | null;
}) {
  const [composerOpen, setComposerOpen] = useState(false);
  const [commentsOpenState, setCommentsOpen] = useState(false);
  const [isSending, setIsSending] = useState(false);
  const chatInputRef = useRef<ChatInputContainerHandle | null>(null);

  const sessionState = useAppStore((state) =>
    sessionId ? (state.taskSessions.items[sessionId]?.state ?? null) : null,
  );
  const keyboardShortcuts = useAppStore((state) => state.userSettings?.keyboardShortcuts);
  const focusShortcut = getShortcut("FOCUS_PASSTHROUGH_INPUT", keyboardShortcuts);
  const isAgentBusy = sessionState === "RUNNING" || sessionState === "STARTING";

  const { pendingComments, pendingCount } = usePendingPassthroughComments(sessionId);
  const { isFinePointer: finePointer } = useResponsiveBreakpoint();
  const { openFile } = useFileEditors();
  const panelState = useChatPanelState({ sessionId: sessionId ?? null, onOpenFile: openFile });
  const planActions = usePlanActions({
    resolvedSessionId: panelState.resolvedSessionId,
    taskId,
    planModeEnabled: panelState.planModeEnabled,
    handlePlanModeChange: panelState.handlePlanModeChange,
    chatInputRef,
  });
  const showProceed = !!planActions.proceedStepName && !isAgentBusy;
  const implementPlanHandler =
    isAgentBusy || !panelState.planModeEnabled ? undefined : planActions.implementPlanHandler;

  // Derive open state: auto-close when pending comments are cleared
  const commentsOpen = commentsOpenState && pendingCount > 0;

  const sendPassthroughMessage = useSendPassthroughMessage({
    taskId,
    sessionId,
    pendingComments,
    panelState,
    onSent: () => {
      setComposerOpen(false);
      setCommentsOpen(false);
    },
  });
  const handleSendMessage = useCallback(
    async (...args: Parameters<typeof sendPassthroughMessage>) => {
      if (isSending) return;
      setIsSending(true);
      try {
        await sendPassthroughMessage(...args);
      } finally {
        setIsSending(false);
      }
    },
    [isSending, sendPassthroughMessage],
  );

  useEffect(() => {
    if (!composerOpen) return;
    requestAnimationFrame(() => chatInputRef.current?.focusInput());
  }, [composerOpen]);

  usePassthroughComposerShortcut({ focusShortcut, setComposerOpen });

  return (
    <div className="flex h-full flex-col bg-card" data-testid="passthrough-toolbar">
      <div className="flex-1 min-h-0">
        <PassthroughTerminal sessionId={sessionId} mode="agent" enableTouchScroll={!finePointer} />
      </div>

      {commentsOpen && pendingCount > 0 && (
        <CommentsPanel
          comments={pendingComments}
          openFile={openFile}
          onSend={() => handleSendMessage({ message: "" })}
        />
      )}

      {composerOpen && (
        <PassthroughComposerPanel
          refHandle={chatInputRef}
          onSubmit={handleSendMessage}
          onCancel={() => setComposerOpen(false)}
          panelState={panelState}
          taskId={taskId}
          isMoving={planActions.isMoving}
          isSending={isSending}
          onImplementPlan={implementPlanHandler}
        />
      )}

      <PassthroughStatusRow
        taskId={taskId}
        sessionId={sessionId}
        nextStepName={planActions.proceedStepName}
        onProceed={planActions.proceed}
        isMoving={planActions.isMoving}
        showProceed={showProceed}
        composerOpen={composerOpen}
        focusShortcut={focusShortcut}
        onToggleComposer={() => setComposerOpen((open) => !open)}
        commentsOpen={commentsOpen}
        onToggleComments={() => setCommentsOpen((open) => !open)}
        pendingCommentsCount={pendingCount}
      />
    </div>
  );
}

function flattenComments(byFile: Record<string, DiffComment[]>): DiffComment[] {
  const all: DiffComment[] = [];
  for (const list of Object.values(byFile)) all.push(...list);
  return all;
}

function usePendingPassthroughComments(sessionId: string | null | undefined) {
  const pendingCommentsByFile = usePendingDiffCommentsByFile(sessionId ?? null);
  const pendingComments = useMemo(
    () => flattenComments(pendingCommentsByFile),
    [pendingCommentsByFile],
  );
  return { pendingComments, pendingCount: pendingComments.length };
}

function ChatToggleButton({
  composerOpen,
  focusShortcut,
  onToggle,
}: {
  composerOpen: boolean;
  focusShortcut: KeyboardShortcut;
  onToggle: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant={composerOpen ? "default" : "outline"}
          size="sm"
          className="h-6 gap-1 px-2.5 text-xs cursor-pointer"
          onClick={onToggle}
          data-testid="passthrough-toggle-composer"
          aria-pressed={composerOpen}
        >
          {composerOpen ? (
            <IconX className="h-3.5 w-3.5" />
          ) : (
            <IconMessageCircle className="h-3.5 w-3.5" />
          )}
          {t("task:chat")}
        </Button>
      </TooltipTrigger>
      <TooltipContent className="max-w-xs">
        {composerOpen ? (
          <div className="space-y-1">
            <p>
              <Trans i18nKey="task:closeComposeBoxHelp">
                Close the compose box (or press <kbd>Esc</kbd> inside it). The CLI agent terminal
                keeps focus.
              </Trans>
            </p>
            <PassthroughChatShortcutHint shortcut={focusShortcut} />
          </div>
        ) : (
          <div className="space-y-1">
            <p>
              <Trans i18nKey="task:openComposeBoxHelp">
                Open a kandev-controlled compose box above the terminal to type a follow-up message.
                Press <kbd>Enter</kbd> to send, <kbd>Shift+Enter</kbd> for a newline.
              </Trans>
            </p>
            <PassthroughChatShortcutHint shortcut={focusShortcut} />
            <p className="text-muted-foreground">{t("task:theTextIsDeliveredStraightTo")}</p>
          </div>
        )}
      </TooltipContent>
    </Tooltip>
  );
}

function PassthroughChatShortcutHint({ shortcut }: { shortcut: KeyboardShortcut }) {
  if (isUnboundShortcut(shortcut)) return null;
  return (
    <p className="text-muted-foreground">
      <Trans
        i18nKey="task:shortcutTogglesChatInput"
        values={{ shortcut: formatShortcut(shortcut) }}
      >
        Shortcut: <kbd>{formatShortcut(shortcut)}</kbd> toggles this chat input.
      </Trans>
    </p>
  );
}

function commentsToggleClassName(count: number, commentsOpen: boolean): string {
  // Vivid amber when there are pending comments so the user sees "something
  // to do" — washes back to plain outline once they're cleared / sent.
  if (count === 0) return "h-6 gap-1 px-2.5 text-xs cursor-pointer";
  if (commentsOpen) return "h-6 gap-1 px-2.5 text-xs cursor-pointer";
  return "h-6 gap-1 px-2.5 text-xs cursor-pointer border-amber-500/60 bg-amber-500/15 text-amber-700 hover:bg-amber-500/25 hover:text-amber-700 dark:text-amber-300 dark:hover:text-amber-200";
}

function CommentsToggleButton({
  commentsOpen,
  onToggle,
  pendingCommentsCount,
}: {
  commentsOpen: boolean;
  onToggle: () => void;
  pendingCommentsCount: number;
}) {
  const { t } = useTranslation();
  const disabled = pendingCommentsCount === 0;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant={commentsOpen ? "default" : "outline"}
          size="sm"
          className={commentsToggleClassName(pendingCommentsCount, commentsOpen)}
          onClick={onToggle}
          disabled={disabled}
          data-testid="passthrough-toggle-comments"
          aria-pressed={commentsOpen}
        >
          {commentsOpen ? (
            <IconX className="h-3.5 w-3.5" />
          ) : (
            <IconMessageDots className="h-3.5 w-3.5" />
          )}
          {t("task:comments")}
          {pendingCommentsCount > 0 && (
            <span
              data-testid="passthrough-pending-count"
              className="ml-1 rounded-full bg-amber-500/40 px-1.5 py-0 text-[10px] font-semibold text-amber-900 dark:text-amber-100"
            >
              {pendingCommentsCount}
            </span>
          )}
        </Button>
      </TooltipTrigger>
      <TooltipContent className="max-w-xs">
        <CommentsTooltipBody commentsOpen={commentsOpen} count={pendingCommentsCount} />
      </TooltipContent>
    </Tooltip>
  );
}

function CommentsTooltipBody({ commentsOpen, count }: { commentsOpen: boolean; count: number }) {
  const { t } = useTranslation();
  if (count === 0) {
    return (
      <div className="space-y-1">
        <p>{t("task:noPendingReviewComments")}</p>
        <p className="text-muted-foreground">{t("task:addACommentFromTheDiff")}</p>
      </div>
    );
  }
  if (commentsOpen) {
    return <p>{t("task:hideTheCommentListTheComments")}</p>;
  }
  return (
    <div className="space-y-1">
      <p>{t("task:pendingReviewCommentsHelp", { count })}</p>
      <p className="text-muted-foreground">
        <Trans i18nKey="task:sendToAgentHelp">
          Hit <strong>Send to agent</strong> inside the panel to deliver them to the CLI agent right
          away, or just open the chat box and type a follow-up - the comments will be prepended.
        </Trans>
      </p>
    </div>
  );
}

function CommentsPanel({
  comments,
  openFile,
  onSend,
}: {
  comments: DiffComment[];
  openFile: (path: string) => void;
  onSend: () => Promise<void> | void;
}) {
  const { t } = useTranslation();
  const [isSending, setIsSending] = useState(false);
  const handleSend = useCallback(async () => {
    if (isSending) return;
    setIsSending(true);
    try {
      await onSend();
    } catch {
      // onSend already toasted the error; keep panel open so the user can retry.
    } finally {
      setIsSending(false);
    }
  }, [isSending, onSend]);

  const count = comments.length;
  return (
    <div
      data-testid="passthrough-comments-panel"
      className="flex max-h-72 flex-col border-t bg-amber-500/5"
    >
      <div className="flex items-center justify-between gap-2 border-b border-amber-500/20 bg-amber-500/10 px-2 py-1 text-xs">
        <span className="font-medium text-amber-700 dark:text-amber-300">
          {t("task:reviewCommentsReadyToSend", { count })}
        </span>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              type="button"
              size="sm"
              variant="default"
              onClick={handleSend}
              disabled={isSending}
              className="h-6 gap-1 px-2.5 text-xs cursor-pointer"
              data-testid="passthrough-send-comments"
            >
              <IconSend className="h-3.5 w-3.5" />
              {t("task:sendToAgent")}
            </Button>
          </TooltipTrigger>
          <TooltipContent className="max-w-xs">
            <p>{t("task:deliverTheseCommentsToTheCli")}</p>
          </TooltipContent>
        </Tooltip>
      </div>
      <div className="flex-1 space-y-2 overflow-y-auto px-2 py-2">
        {comments.map((comment) => (
          <CommentCard key={comment.id} comment={comment} openFile={openFile} />
        ))}
      </div>
    </div>
  );
}

function formatLineRange(comment: DiffComment): string {
  return comment.startLine === comment.endLine
    ? `${comment.startLine}`
    : `${comment.startLine}-${comment.endLine}`;
}

function CommentCard({
  comment,
  openFile,
}: {
  comment: DiffComment;
  openFile: (path: string) => void;
}) {
  const { t } = useTranslation();
  const updateComment = useCommentsStore((s) => s.updateComment);
  const removeComment = useCommentsStore((s) => s.removeComment);
  const lineRange = formatLineRange(comment);

  const handleOpenFile = useCallback(() => {
    openFile(comment.filePath);
  }, [openFile, comment.filePath]);

  return (
    <div
      data-testid="passthrough-comment-card"
      className="rounded-md border border-amber-500/30 bg-card px-2 py-1.5 text-xs"
    >
      <div className="mb-1 flex items-center justify-between gap-2">
        <button
          type="button"
          onClick={handleOpenFile}
          className="truncate text-left font-mono text-[11px] text-primary hover:underline cursor-pointer"
          data-testid="passthrough-comment-file-ref"
          title={`${comment.filePath}:${lineRange}`}
        >
          {comment.filePath}:{lineRange}
        </button>
        <button
          type="button"
          onClick={() => removeComment(comment.id)}
          className="rounded p-0.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive cursor-pointer"
          aria-label={t("task:removeComment")}
          data-testid="passthrough-comment-remove"
        >
          <IconTrash className="h-3.5 w-3.5" />
        </button>
      </div>
      {comment.codeContent && (
        <pre className="mb-1 max-h-16 overflow-y-auto rounded bg-muted/50 px-1.5 py-1 text-[10px] font-mono leading-tight">
          {comment.codeContent}
        </pre>
      )}
      <Textarea
        value={comment.text}
        onChange={(e) => updateComment(comment.id, { text: e.target.value })}
        className="min-h-[2rem] resize-none text-xs"
        rows={Math.min(4, Math.max(1, comment.text.split("\n").length))}
        placeholder={t("task:writeAComment")}
        data-testid="passthrough-comment-textarea"
      />
    </div>
  );
}

type StatusRowProps = {
  taskId: string | null;
  sessionId?: string | null;
  nextStepName: string | null;
  onProceed: () => void;
  isMoving: boolean;
  showProceed: boolean;
  composerOpen: boolean;
  focusShortcut: KeyboardShortcut;
  onToggleComposer: () => void;
  commentsOpen: boolean;
  onToggleComments: () => void;
  pendingCommentsCount: number;
};

function PassthroughStatusRow({
  taskId,
  sessionId,
  nextStepName,
  onProceed,
  isMoving,
  showProceed,
  composerOpen,
  focusShortcut,
  onToggleComposer,
  commentsOpen,
  onToggleComments,
  pendingCommentsCount,
}: StatusRowProps) {
  const { t } = useTranslation();
  return (
    <div
      data-testid="passthrough-status-row"
      className="flex min-w-0 flex-shrink-0 flex-wrap items-center gap-1.5 border-t bg-card px-2 py-1 text-xs text-muted-foreground"
    >
      <ChatToggleButton
        composerOpen={composerOpen}
        focusShortcut={focusShortcut}
        onToggle={onToggleComposer}
      />
      <CommentsToggleButton
        commentsOpen={commentsOpen}
        onToggle={onToggleComments}
        pendingCommentsCount={pendingCommentsCount}
      />

      <div className="ml-auto flex min-w-0 max-w-full flex-wrap items-center justify-end gap-1.5">
        <TaskDependencyChip taskId={taskId} />
        <PRStatusChip taskId={taskId} />
        <MRStatusChip taskId={taskId} />
        <AzureDevOpsTaskPullRequestChip taskId={taskId} />
        <RegisteredChangeRequestStatus taskId={taskId} sessionId={sessionId} surface="composer" />
        {taskId && <PRMergedBanner key={taskId} taskId={taskId} />}
        {showProceed && nextStepName && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="h-6 shrink-0 gap-1 px-2.5 text-xs cursor-pointer text-primary"
                onClick={onProceed}
                disabled={isMoving}
                data-testid="passthrough-proceed-next-step"
              >
                {nextStepName}
                <IconArrowRight className="h-3.5 w-3.5" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t("task:moveTaskToTheNextWorkflow")}</TooltipContent>
          </Tooltip>
        )}
      </div>
    </div>
  );
}
