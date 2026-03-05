"use client";

import { useState, memo } from "react";
import { IconAlertTriangle, IconRefresh, IconPlayerPlay } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { Message, TaskSessionState } from "@/lib/types/http";
import type { RecoveryMetadata } from "@/components/task/chat/types";

type RecoveryState = "pending" | "recovering" | "recovered" | "error";

export const AgentErrorRecoveryMessage = memo(function AgentErrorRecoveryMessage({
  comment,
  sessionState,
}: {
  comment: Message;
  sessionState?: TaskSessionState;
}) {
  const [state, setState] = useState<RecoveryState>("pending");
  const metadata = comment.metadata as RecoveryMetadata | undefined;
  const canRecover = Boolean(metadata?.task_id && metadata?.session_id);

  // Hide entirely once recovery succeeded or session is active again (handles page refresh)
  const isSessionActive =
    sessionState === "RUNNING" || sessionState === "STARTING" || sessionState === "COMPLETED";
  if (state === "recovered" || isSessionActive) {
    return null;
  }

  const handleRecover = async (action: "resume" | "fresh_start") => {
    if (!canRecover || !metadata) return;

    const client = getWebSocketClient();
    if (!client) {
      console.error("WebSocket client not available");
      return;
    }

    setState("recovering");
    try {
      await client.request("session.recover", {
        task_id: metadata.task_id,
        session_id: metadata.session_id,
        action,
      });
      setState("recovered");
    } catch (error) {
      console.error("Failed to recover session:", error);
      setState("error");
      setTimeout(() => setState("pending"), 3000);
    }
  };

  const message = comment.content || "Agent encountered an error";

  return (
    <div className="w-full">
      <div className="flex items-start gap-3 w-full rounded px-2 py-1 -mx-2">
        <div className="flex-shrink-0 mt-0.5">
          <IconAlertTriangle className="h-4 w-4 text-red-500" />
        </div>

        <div className="flex-1 min-w-0 pt-0.5">
          <div className="text-xs font-mono text-red-600 dark:text-red-400">{message}</div>

          <div className="mt-2 flex items-center gap-2">
            {metadata?.has_resume_token && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="outline"
                    size="sm"
                    className={cn("h-7 text-xs cursor-pointer gap-1.5")}
                    disabled={state === "recovering" || !canRecover}
                    onClick={() => handleRecover("resume")}
                    data-testid="recovery-resume-button"
                  >
                    <IconRefresh className="h-3 w-3" />
                    Resume session
                  </Button>
                </TooltipTrigger>
                <TooltipContent side="top">
                  Re-launch with resume flag — keeps all previous messages and context
                </TooltipContent>
              </Tooltip>
            )}
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="outline"
                  size="sm"
                  className={cn("h-7 text-xs cursor-pointer gap-1.5")}
                  disabled={state === "recovering" || !canRecover}
                  onClick={() => handleRecover("fresh_start")}
                  data-testid="recovery-fresh-button"
                >
                  <IconPlayerPlay className="h-3 w-3" />
                  Start fresh session
                </Button>
              </TooltipTrigger>
              <TooltipContent side="top">
                New agent process on the same workspace — no previous conversation context
              </TooltipContent>
            </Tooltip>
            {state === "error" && <span className="text-xs text-red-500">Failed — try again</span>}
          </div>
        </div>
      </div>
    </div>
  );
});
