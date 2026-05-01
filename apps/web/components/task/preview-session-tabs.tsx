"use client";

import { useCallback, useMemo } from "react";
import { IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { AgentLogo } from "@/components/agent-logo";
import { SessionTabs, type SessionTab } from "@/components/session-tabs";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { useSessionResumption } from "@/hooks/domains/session/use-session-resumption";
import { useTaskSessions } from "@/hooks/use-task-sessions";
import type { UseEnsureTaskSessionResult } from "@/hooks/domains/session/use-ensure-task-session";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { TaskSession, TaskSessionState } from "@/lib/types/http";
import { getWebSocketClient } from "@/lib/ws/connection";
import { PassthroughTerminal } from "./passthrough-terminal";
import { TaskChatPanel } from "./task-chat-panel";
import {
  buildAgentLabelsById,
  pickActiveSessionId,
  resolveAgentLabelFor,
  sortSessions,
} from "./session-sort";

const LABEL_SEPARATOR = " \u2022 ";

type PreviewSessionTabsProps = {
  taskId: string;
  sessionId: string | null;
  ensureSession?: UseEnsureTaskSessionResult;
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
  onSessionChange,
}: PreviewSessionTabsProps) {
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
  useSessionResumption(taskId, activeSessionId);

  const tabs = useMemo<SessionTab[]>(
    () =>
      sortedSessions.map((session) => {
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
          className: "bg-muted/50 data-[state=active]:bg-muted",
        };
      }),
    [sortedSessions, agentLabelsById, profilesById],
  );

  if (!isLoaded && sortedSessions.length === 0) {
    return <PreviewLoadingState label="Loading agents…" />;
  }

  if (sortedSessions.length === 0) {
    if (ensureSession?.status === "preparing") {
      return <PreviewLoadingState label="Preparing workspace…" />;
    }
    if (ensureSession?.status === "error") {
      return <PreviewEnsureError onRetry={ensureSession.retry} />;
    }
    return <PreviewEmptyState />;
  }

  return (
    <div className="flex h-full flex-col min-h-0" data-testid="preview-session-tabs">
      <div className="border-b px-2 py-1">
        <SessionTabs
          tabs={tabs}
          activeTab={activeSessionId ?? ""}
          onTabChange={(id) => onSessionChange?.(id)}
          listClassName="bg-transparent p-0 !h-7 gap-1 overflow-x-auto overflow-y-hidden min-w-0 shrink [&::-webkit-scrollbar]:hidden [-ms-overflow-style:none] [scrollbar-width:none]"
        />
      </div>
      <div className="flex-1 min-h-0">
        {activeSession && (
          <PreviewSessionBody key={activeSession.id} session={activeSession} taskId={taskId} />
        )}
      </div>
    </div>
  );
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

function PreviewSessionBody({ session, taskId }: { session: TaskSession; taskId: string }) {
  const { toast } = useToast();
  const handleSendMessage = useCallback(
    async (content: string) => {
      const client = getWebSocketClient();
      if (!client) return;
      try {
        await client.request(
          "message.add",
          { task_id: taskId, session_id: session.id, content },
          10000,
        );
      } catch (error) {
        console.error("Failed to send message:", error);
        toast({ title: "Failed to send message", variant: "error" });
      }
    },
    [taskId, session.id, toast],
  );

  if (session.is_passthrough) {
    return (
      <div className="h-full bg-card">
        <PassthroughTerminal sessionId={session.id} mode="agent" />
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col">
      <TaskChatPanel onSend={handleSendMessage} sessionId={session.id} hideSessionsDropdown />
    </div>
  );
}

function isSessionActive(state: TaskSessionState): boolean {
  return state === "RUNNING" || state === "STARTING";
}

function RunningSpinner() {
  return <IconLoader2 className="h-3 w-3 shrink-0 text-blue-500 animate-spin" />;
}

function PreviewLoadingState({ label }: { label: string }) {
  return (
    <div
      className="flex h-full items-center justify-center gap-2 text-sm text-muted-foreground"
      data-testid="preview-loading-state"
    >
      <IconLoader2 className="h-4 w-4 animate-spin" />
      {label}
    </div>
  );
}

function PreviewEmptyState() {
  return (
    <div className="flex h-full flex-col">
      <div
        className="flex flex-1 items-center justify-center text-sm text-muted-foreground"
        data-testid="preview-empty-state"
      >
        No agents yet.
      </div>
    </div>
  );
}

function PreviewEnsureError({ onRetry }: { onRetry: () => void }) {
  return (
    <div
      className="flex h-full flex-col items-center justify-center gap-3 text-sm"
      data-testid="preview-ensure-error"
    >
      <span className="text-muted-foreground">Failed to prepare workspace.</span>
      <Button variant="outline" size="sm" className="cursor-pointer" onClick={onRetry}>
        Retry
      </Button>
    </div>
  );
}
