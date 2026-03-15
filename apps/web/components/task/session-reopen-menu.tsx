"use client";

import { useCallback, useMemo } from "react";
import { IconStar } from "@tabler/icons-react";
import {
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from "@kandev/ui/dropdown-menu";
import { useAppStore } from "@/components/state-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useTaskSessions } from "@/hooks/use-task-sessions";
import { addSessionPanel } from "@/lib/state/dockview-panel-actions";
import { getSessionStateIcon } from "@/lib/ui/state-icons";
import type { TaskSession } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";

function resolveAgentLabel(session: TaskSession, labelsById: Record<string, string>): string {
  if (session.agent_profile_id && labelsById[session.agent_profile_id]) {
    return labelsById[session.agent_profile_id];
  }
  return "Unknown agent";
}

/**
 * Renders session items inside the + dropdown menu.
 * Each item shows session number, agent label, primary star, and state icon.
 * Clicking focuses an existing tab or re-opens a closed one.
 */
export function SessionReopenMenuItems({ taskId }: { taskId: string }) {
  const { sessions } = useTaskSessions(taskId);
  const api = useDockviewStore((s) => s.api);
  const centerGroupId = useDockviewStore((s) => s.centerGroupId);
  const agentProfiles = useAppStore((s) => s.agentProfiles.items);
  const primarySessionId = useAppStore((s) => {
    const task = s.kanban.tasks.find((t: { id: string }) => t.id === taskId);
    return task?.primarySessionId ?? null;
  });

  const agentLabelsById = useMemo(
    () =>
      Object.fromEntries(
        agentProfiles.map((p: AgentProfileOption) => [p.id, p.label]),
      ),
    [agentProfiles],
  );

  const sortedSessions = useMemo(
    () =>
      [...sessions].sort(
        (a, b) => new Date(a.started_at).getTime() - new Date(b.started_at).getTime(),
      ),
    [sessions],
  );

  const handleClick = useCallback(
    (sessionId: string, label: string) => {
      if (!api) return;
      // Pre-set currentLayoutSessionId so switchSessionLayout's guard
      // (currentLayoutSessionId === newSessionId) prevents a full layout rebuild.
      // We only want to add/focus the tab, not tear down the entire layout.
      useDockviewStore.setState({ currentLayoutSessionId: sessionId });
      addSessionPanel(api, centerGroupId, sessionId, label);
    },
    [api, centerGroupId],
  );

  if (sortedSessions.length === 0) return null;

  return (
    <>
      <DropdownMenuLabel className="text-xs text-muted-foreground">Sessions</DropdownMenuLabel>
      {sortedSessions.map((session, index) => {
        const label = resolveAgentLabel(session, agentLabelsById);
        const isPrimary = session.id === primarySessionId;
        const isOpen = Boolean(api?.getPanel(`session:${session.id}`));
        return (
          <DropdownMenuItem
            key={session.id}
            onClick={() => handleClick(session.id, label)}
            className={`cursor-pointer text-xs gap-1.5 ${isOpen ? "opacity-50" : ""}`}
            data-testid={`reopen-session-${session.id}`}
          >
            <span className="w-5 shrink-0 text-muted-foreground text-right">#{index + 1}</span>
            <span className="flex-1 truncate">{label}</span>
            {isPrimary && (
              <IconStar className="h-3 w-3 text-amber-500 fill-amber-500 shrink-0" />
            )}
            <span className="shrink-0">{getSessionStateIcon(session.state, "h-3 w-3")}</span>
          </DropdownMenuItem>
        );
      })}
      <DropdownMenuSeparator />
    </>
  );
}
