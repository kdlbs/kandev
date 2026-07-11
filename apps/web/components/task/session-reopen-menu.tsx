"use client";

import { useCallback, useMemo } from "react";
import { IconMessagePlus, IconStar } from "@tabler/icons-react";
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
import { markSessionTabUserActivationIntent } from "@/components/task/session-tab-activation-intent";
import type { TaskSession } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";

type AgentInfo = { label: string; agentName: string };

function resolveAgentInfo(
  session: TaskSession,
  profilesById: Record<string, AgentProfileOption>,
): AgentInfo {
  const profile = session.agent_profile_id ? profilesById[session.agent_profile_id] : null;
  const agentName = profile?.agent_name ?? "";
  // A user-supplied session name wins over the derived profile label,
  // matching the session tab title precedence (resolveSessionTabTitle).
  if (session.name) return { label: session.name, agentName };
  if (!profile) return { label: "Unknown agent", agentName: "" };
  const parts = profile.label.split(" \u2022 ");
  return { label: parts[1] || parts[0] || profile.label, agentName };
}

/**
 * Renders session items inside the + dropdown menu.
 * Each item shows session number, agent label, primary star, and state icon.
 * Clicking focuses an existing tab or re-opens a closed one.
 */
export function SessionReopenMenuItems({
  taskId,
  groupId,
  onNewSession,
}: {
  taskId: string;
  groupId?: string;
  /**
   * Click handler for the leading "New Agent" item rendered as the
   * first row under the section label. Omit to hide the row.
   */
  onNewSession?: () => void;
}) {
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
      // Reopening a session within the same task = same env, so the env switch
      // action no-ops naturally. We just create the chat panel.
      markSessionTabUserActivationIntent(sessionId);
      addSessionPanel(api, groupId ?? centerGroupId, sessionId, label);
    },
    [api, centerGroupId],
  );

  // Render the section even when there are no sessions yet — the leading
  // "New Agent" row should still be reachable from the menu. Hide the
  // whole block only when neither the create handler nor any sessions
  // are present (e.g. unmounted contexts).
  if (sortedSessions.length === 0 && !onNewSession) return null;

  return (
    <>
      <DropdownMenuLabel className="text-xs text-muted-foreground">Agents</DropdownMenuLabel>
      {onNewSession && (
        <DropdownMenuItem
          onClick={onNewSession}
          className="cursor-pointer text-xs gap-1.5"
          data-testid="new-session-button"
        >
          <IconMessagePlus className="h-3.5 w-3.5 shrink-0" />
          <span className="flex-1 truncate">New Agent</span>
        </DropdownMenuItem>
      )}
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
            <span
              data-testid={`reopen-session-seq-${index + 1}`}
              className="shrink-0 text-[11px] font-medium leading-none text-muted-foreground bg-foreground/10 rounded px-1.5 py-0.5"
            >
              #{index + 1}
            </span>
            {info.agentName && (
              <AgentLogo agentName={info.agentName} size={14} className="shrink-0" />
            )}
            <span className="flex-1 truncate">{info.label}</span>
            {isPrimary && <IconStar className="h-3 w-3 fill-foreground/50 stroke-0 shrink-0" />}
            {session.state !== "RUNNING" &&
              session.state !== "STARTING" &&
              session.state !== "WAITING_FOR_INPUT" && (
                <span className="shrink-0">
                  {getSessionStateIcon(session.state, "h-3 w-3", session.foreground_activity)}
                </span>
              )}
          </DropdownMenuItem>
        );
      })}
      <DropdownMenuSeparator />
    </>
  );
}
