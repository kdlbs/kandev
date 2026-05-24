"use client";

import { useCallback, useMemo, useState } from "react";
import {
  IconArrowRight,
  IconMessageCircle,
  IconMessageDots,
  IconPlayerStopFilled,
  IconX,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useToast } from "@/components/toast-provider";
import { PRStatusChip } from "@/components/github/pr-status-chip";
import { PassthroughComposer } from "./passthrough-composer";
import { PRMergedBanner } from "./chat/chat-input-area";
import { useAppStore } from "@/components/state-provider";
import { useNextWorkflowStep } from "@/hooks/domains/kanban/use-plan-actions";
import { usePendingDiffCommentsByFile } from "@/hooks/domains/comments/use-diff-comments";
import { useCommentsStore } from "@/lib/state/slices/comments/comments-store";
import { formatReviewCommentsAsMarkdown } from "@/lib/state/slices/comments/format";
import type { DiffComment } from "@/lib/diff/types";
import { getWebSocketClient } from "@/lib/ws/connection";
import { PassthroughTerminal } from "./passthrough-terminal";

/**
 * PassthroughToolbar wraps the PTY terminal with the kandev surface that the
 * full ACP `ChatStatusBar` + `ChatInputArea` provide for chat mode: PR status,
 * merge banner, "Move to next step" workflow advancement, and a collapsible
 * compose box for typing follow-up prompts that get flushed to PTY stdin.
 *
 * Default focus stays on the xterm terminal — the composer is collapsed behind
 * a "Chat" button so the user can keep raw-terminal interaction front-and-centre
 * and only opt in to the kandev composer when they want multi-line editing or
 * want to attach review comments to a prompt.
 */
export function PassthroughToolbar({
  sessionId,
  taskId,
}: {
  sessionId: string | null | undefined;
  taskId: string | null;
}) {
  const [composerOpen, setComposerOpen] = useState(false);

  const sessionState = useAppStore((state) =>
    sessionId ? (state.taskSessions.items[sessionId]?.state ?? null) : null,
  );
  const isAgentBusy = sessionState === "RUNNING" || sessionState === "STARTING";

  const nextStep = useNextWorkflowStep(taskId);
  const showProceed = !!nextStep.proceedStepName && !isAgentBusy;

  const { pendingComments, pendingCount } = usePendingPassthroughComments(sessionId);
  const handleSendMessage = useSendPassthroughMessage({
    taskId,
    sessionId,
    pendingComments,
    onSent: () => setComposerOpen(false),
  });
  const { handleStop, isStopping } = usePassthroughCancel(sessionId);

  return (
    <div className="flex h-full flex-col bg-card" data-testid="passthrough-toolbar">
      <div className="flex-1 min-h-0">
        <PassthroughTerminal sessionId={sessionId} mode="agent" />
      </div>

      {composerOpen && (
        <PassthroughComposer
          onSubmit={handleSendMessage}
          onCancel={() => setComposerOpen(false)}
          autoFocus
          placeholder="Type a message, Shift+Enter for newline, Esc to close"
          header={pendingCount > 0 ? <PendingCommentsBanner count={pendingCount} /> : null}
        />
      )}

      <PassthroughStatusRow
        taskId={taskId}
        nextStepName={nextStep.proceedStepName}
        onProceed={nextStep.proceed}
        isMoving={nextStep.isMoving}
        showProceed={showProceed}
        composerOpen={composerOpen}
        onToggleComposer={() => setComposerOpen((open) => !open)}
        onStop={handleStop}
        isStopping={isStopping}
        canStop={isAgentBusy}
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

function useSendPassthroughMessage({
  taskId,
  sessionId,
  pendingComments,
  onSent,
}: {
  taskId: string | null;
  sessionId: string | null | undefined;
  pendingComments: DiffComment[];
  onSent: () => void;
}) {
  const { toast } = useToast();
  const markCommentsSent = useCommentsStore((s) => s.markCommentsSent);

  return useCallback(
    async (content: string) => {
      if (!taskId || !sessionId) {
        toast({ title: "Session not ready", variant: "error" });
        throw new Error("Session not ready");
      }
      const client = getWebSocketClient();
      if (!client) {
        toast({ title: "Not connected — please reload to retry", variant: "error" });
        throw new Error("WebSocket client not available");
      }
      // Format pending review comments as markdown and prepend so they reach
      // the agent's stdin alongside the user's typed prompt. Mark them sent
      // only after the WS request resolves so a failed send doesn't drop the
      // user's queued comments.
      const formatted =
        pendingComments.length > 0
          ? formatReviewCommentsAsMarkdown(pendingComments) + content
          : content;
      try {
        await client.request(
          "message.add",
          { task_id: taskId, session_id: sessionId, content: formatted },
          10_000,
        );
        if (pendingComments.length > 0) {
          markCommentsSent(pendingComments.map((c) => c.id));
        }
        onSent();
      } catch (error) {
        console.error("Failed to send passthrough message:", error);
        toast({ title: "Failed to send message", variant: "error" });
        throw error;
      }
    },
    [taskId, sessionId, toast, pendingComments, markCommentsSent, onSent],
  );
}

function usePassthroughCancel(sessionId: string | null | undefined) {
  const [isStopping, setIsStopping] = useState(false);
  const { toast } = useToast();

  const handleStop = useCallback(async () => {
    if (!sessionId || isStopping) return;
    const client = getWebSocketClient();
    if (!client) {
      toast({ title: "Not connected", variant: "error" });
      return;
    }
    setIsStopping(true);
    try {
      await client.request("agent.cancel", { session_id: sessionId }, 10_000);
    } catch (error) {
      console.error("Failed to stop passthrough session:", error);
      toast({ title: "Failed to stop agent", variant: "error" });
    } finally {
      setIsStopping(false);
    }
  }, [sessionId, isStopping, toast]);

  return { handleStop, isStopping };
}

function chatToggleVariant(
  composerOpen: boolean,
  pendingCommentsCount: number,
): "default" | "outline" {
  if (composerOpen) return "default";
  if (pendingCommentsCount > 0) return "default";
  return "outline";
}

function chatToggleTooltipLabel(composerOpen: boolean, pendingCommentsCount: number): string {
  if (composerOpen) return "Close compose box (Esc)";
  if (pendingCommentsCount > 0) {
    const plural = pendingCommentsCount === 1 ? "" : "s";
    return `${pendingCommentsCount} pending review comment${plural} — open to send`;
  }
  return "Open compose box to type a follow-up";
}

function ChatToggleButton({
  composerOpen,
  onToggle,
  pendingCommentsCount,
}: {
  composerOpen: boolean;
  onToggle: () => void;
  pendingCommentsCount: number;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant={chatToggleVariant(composerOpen, pendingCommentsCount)}
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
          Chat
          {!composerOpen && pendingCommentsCount > 0 && (
            <span
              data-testid="passthrough-pending-count"
              className="ml-1 rounded-full bg-amber-500/30 px-1.5 py-0 text-[10px] font-semibold"
            >
              {pendingCommentsCount}
            </span>
          )}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{chatToggleTooltipLabel(composerOpen, pendingCommentsCount)}</TooltipContent>
    </Tooltip>
  );
}

function PendingCommentsBanner({ count }: { count: number }) {
  return (
    <div
      data-testid="passthrough-pending-comments-banner"
      className="flex items-center gap-1.5 border-b bg-amber-500/10 px-2 py-1 text-xs text-amber-700 dark:text-amber-300"
    >
      <IconMessageDots className="h-3.5 w-3.5" />
      <span>
        {count} pending review comment{count === 1 ? "" : "s"} will be attached to this message.
      </span>
    </div>
  );
}

type StatusRowProps = {
  taskId: string | null;
  nextStepName: string | null;
  onProceed: () => void;
  isMoving: boolean;
  showProceed: boolean;
  composerOpen: boolean;
  onToggleComposer: () => void;
  onStop: () => void;
  isStopping: boolean;
  canStop: boolean;
  pendingCommentsCount: number;
};

function PassthroughStatusRow({
  taskId,
  nextStepName,
  onProceed,
  isMoving,
  showProceed,
  composerOpen,
  onToggleComposer,
  onStop,
  isStopping,
  canStop,
  pendingCommentsCount,
}: StatusRowProps) {
  return (
    <div
      data-testid="passthrough-status-row"
      className="flex flex-shrink-0 items-center gap-1.5 border-t bg-card px-2 py-1 text-xs text-muted-foreground"
    >
      <PRStatusChip taskId={taskId} />
      {taskId && <PRMergedBanner key={taskId} taskId={taskId} />}

      <div className="ml-auto flex items-center gap-1.5">
        {showProceed && nextStepName && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="h-6 gap-1 px-2.5 text-xs cursor-pointer text-primary"
                onClick={onProceed}
                disabled={isMoving}
                data-testid="passthrough-proceed-next-step"
              >
                {nextStepName}
                <IconArrowRight className="h-3.5 w-3.5" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Move task to the next workflow step</TooltipContent>
          </Tooltip>
        )}

        <ChatToggleButton
          composerOpen={composerOpen}
          onToggle={onToggleComposer}
          pendingCommentsCount={pendingCommentsCount}
        />

        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="h-6 gap-1 px-2.5 text-xs cursor-pointer text-red-600 dark:text-red-400"
              onClick={onStop}
              disabled={!canStop || isStopping}
              data-testid="passthrough-stop"
            >
              <IconPlayerStopFilled className="h-3.5 w-3.5" />
              Stop
            </Button>
          </TooltipTrigger>
          <TooltipContent>
            {canStop ? "Send Ctrl-C to the agent" : "Agent isn't running"}
          </TooltipContent>
        </Tooltip>
      </div>
    </div>
  );
}
