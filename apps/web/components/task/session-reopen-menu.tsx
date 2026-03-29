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
import { AgentLogo } from "@/components/agent-logo";
import type { TaskSession } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";

type AgentInfo = { label: string; agentName: string };

function resolveAgentInfo(
  session: TaskSession,
  profilesById: Record<string, AgentProfileOption>,
): AgentInfo {
  const profile = session.agent_profile_id ? profilesById[session.agent_profile_id] : null;
  if (!profile) return { label: "Unknown agent", agentName: "" };
  const parts = profile.label.split(" \u2022 ");
  return { label: parts[1] || parts[0] || profile.label, agentName: profile.agent_name };
}

/**
 * Renders session items inside the + dropdown menu.
 * Each item shows session number, agent label, primary star, and state icon.
 * Clicking focuses an existing tab or re-opens a closed one.
 */
export function SessionReopenMenuItems({ taskId, groupId }: { taskId: string; groupId?: string }) {
  const { sessions } = useTaskSessions(taskId);
  const api = useDockviewStore((s) => s.api);
  const centerGroupId = useDockviewStore((s) => s.centerGroupId);
  const agentProfiles = useAppStore((s) => s.agentProfiles.items);
  const primarySessionId = useAppStore((s) => {
    const task = s.kanban.tasks.find((t: { id: string }) => t.id === taskId);
    return task?.primarySessionId ?? null;
  });

  const profilesById = useMemo(
    () => Object.fromEntries(agentProfiles.map((p: AgentProfileOption) => [p.id, p])),
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
    (sessionId: string, label: string, groupId?: string) => {
      if (!api) return;
      // Skip the next layout switch for this session to prevent a full rebuild.
      useDockviewStore.setState({ _skipLayoutSwitchForSession: sessionId });
      addSessionPanel(api, groupId ?? centerGroupId, sessionId, label);
    },
    [api, centerGroupId],
  );

  if (sortedSessions.length === 0) return null;

  return (
    <>
      <DropdownMenuLabel className="text-xs text-muted-foreground">Agents</DropdownMenuLabel>
      {sortedSessions.map((session, index) => {
        const info = resolveAgentInfo(session, profilesById);
        const isPrimary = session.id === primarySessionId;
        const isOpen = Boolean(api?.getPanel(`session:${session.id}`));
        return (
          <DropdownMenuItem
            key={session.id}
            onClick={() => handleClick(session.id, info.label, groupId)}
            className={`cursor-pointer text-xs gap-1.5 ${isOpen ? "opacity-50" : ""}`}
            data-testid={`reopen-session-${session.id}`}
          >
            <span className="w-5 shrink-0 text-muted-foreground text-right">#{index + 1}</span>
            {info.agentName && (
              <AgentLogo agentName={info.agentName} size={14} className="shrink-0" />
            )}
            <span className="flex-1 truncate">{info.label}</span>
            {isPrimary && <IconStar className="h-3 w-3 fill-foreground/50 stroke-0 shrink-0" />}
            {session.state !== "RUNNING" &&
              session.state !== "STARTING" &&
              session.state !== "WAITING_FOR_INPUT" && (
                <span className="shrink-0">{getSessionStateIcon(session.state, "h-3 w-3")}</span>
              )}
          </DropdownMenuItem>
        );
      })}
      <DropdownMenuSeparator />
    </>
  );
}
