"use client";

import { useState } from "react";
import {
  IconAlertTriangle,
  IconChevronDown,
  IconPlayerPlay,
  IconRefresh,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { formatRelativeTime } from "@/lib/utils";
import { AgentAvatar } from "@/app/office/components/agent-avatar";
import type { RunError } from "@/app/office/tasks/[id]/types";

type RunErrorEntryProps = {
  taskId: string;
  error: RunError;
};

/**
 * Top-level chat entry rendered when an office session is in FAILED
 * state. Replaces the legacy red action-message banner: shows a short
 * generic header, a Show details collapsible exposing the verbatim
 * raw payload (for bug reports), and the Resume / Start fresh
 * buttons. Click handlers wire to the existing `session.recover` WS
 * request so the recovery semantics are unchanged.
 */
export function RunErrorEntry({ taskId, error }: RunErrorEntryProps) {
  const agentName = useAppStore((s) =>
    error.agentProfileId
      ? (s.office.agentProfiles.find((a) => a.id === error.agentProfileId)?.name ?? "Agent")
      : "Agent",
  );
  const [showDetails, setShowDetails] = useState(false);

  const handleRecover = async (action: "resume" | "fresh_start") => {
    const client = getWebSocketClient();
    if (!client) return;
    try {
      await client.request("session.recover", {
        task_id: taskId,
        session_id: error.sessionId,
        action,
      });
    } catch {
      // No-op — the chat will reflect any subsequent state via WS.
    }
  };

  return (
    <div className="flex gap-3 py-3 border-b border-border/50">
      <AgentAvatar name={agentName} size="md" />
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 flex-wrap">
          <span className="font-medium text-sm">{agentName}</span>
          <span className="inline-flex items-center gap-1 text-xs text-red-600 dark:text-red-400">
            <IconAlertTriangle className="h-3.5 w-3.5" />
            stopped with an error
          </span>
          <span className="text-xs text-muted-foreground">
            {formatRelativeTime(error.failedAt)}
          </span>
        </div>
        <p className="mt-1 text-sm text-muted-foreground">
          The agent stopped with an error. Resume to retry the same conversation, or start a fresh
          session.
        </p>
        {error.rawPayload && (
          <Collapsible open={showDetails} onOpenChange={setShowDetails} className="mt-2">
            <CollapsibleTrigger className="flex items-center gap-1 text-xs text-muted-foreground cursor-pointer hover:text-foreground transition-colors">
              <IconChevronDown
                className={`h-3.5 w-3.5 transition-transform ${showDetails ? "rotate-180" : ""}`}
              />
              Show details
            </CollapsibleTrigger>
            <CollapsibleContent>
              <pre
                className="mt-1 text-[11px] font-mono text-muted-foreground bg-muted/50 rounded p-2 overflow-auto max-h-[300px] whitespace-pre-wrap break-words"
                data-testid="run-error-raw-payload"
              >
                {error.rawPayload}
              </pre>
            </CollapsibleContent>
          </Collapsible>
        )}
        <div className="mt-2 flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            className="h-7 text-xs cursor-pointer gap-1.5"
            onClick={() => handleRecover("resume")}
            data-testid="run-error-resume-button"
          >
            <IconRefresh className="h-3 w-3" />
            Resume session
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="h-7 text-xs cursor-pointer gap-1.5"
            onClick={() => handleRecover("fresh_start")}
            data-testid="run-error-fresh-button"
          >
            <IconPlayerPlay className="h-3 w-3" />
            Start fresh session
          </Button>
        </div>
      </div>
    </div>
  );
}
