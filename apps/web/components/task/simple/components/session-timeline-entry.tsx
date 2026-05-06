"use client";

import { forwardRef, useEffect, useMemo, useRef, useState } from "react";
import { IconChevronDown, IconLoader2 } from "@tabler/icons-react";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { useAppStore } from "@/components/state-provider";
import { selectCommandCount } from "@/lib/state/slices/session/selectors";
import { AdvancedChatPanel } from "@/app/office/tasks/[id]/advanced-panels/chat-panel";
import { useActiveSessionRef } from "./active-session-ref-context";
import type { TaskSession } from "@/app/office/tasks/[id]/types";

const COLLAPSE_KEY_PREFIX = "office.session.collapsed.";

function formatDuration(ms: number): string {
  if (ms < 1000) return "<1s";
  const seconds = Math.floor(ms / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remaining = seconds % 60;
  return remaining > 0 ? `${minutes}m ${remaining}s` : `${minutes}m`;
}

/**
 * Returns the duration to surface in the entry header.
 *
 * - If this group has any RUNNING session (`activeSession` non-null), the
 *   duration counts from that session's started_at to "now".
 * - Otherwise, fall back to the most recent session's elapsed time
 *   (started_at → completedAt / updatedAt) so the user sees how long
 *   the last turn ran.
 *
 * Cumulative duration across all turns isn't computed — `started_at` /
 * `completed_at` aren't reliably populated across the whole history yet.
 * Single-turn duration is the pragmatic choice.
 */
function entryDuration(
  representative: TaskSession,
  activeSession: TaskSession | null,
): string | null {
  const target = activeSession ?? representative;
  if (!target.startedAt) return null;
  const start = new Date(target.startedAt).getTime();
  if (isNaN(start)) return null;
  const isRunning = target.state === "RUNNING";
  let end: number;
  if (target.completedAt) {
    end = new Date(target.completedAt).getTime();
  } else if (!isRunning && target.updatedAt) {
    end = new Date(target.updatedAt).getTime();
  } else {
    end = Date.now();
  }
  if (isNaN(end)) return null;
  const ms = end - start;
  return ms > 0 ? formatDuration(ms) : null;
}

function isOfficeSession(session: TaskSession): boolean {
  return Boolean(session.agentProfileId);
}

/**
 * A single session is "live" when:
 * - office (agent_profile_id set) and state === RUNNING. IDLE means
 *   the agent process + executor are torn down.
 * - kanban / quick-chat (no agent_profile_id) and state ∈
 *   {RUNNING, WAITING_FOR_INPUT} — they keep the warm-between-turns model.
 */
function isLiveSession(session: TaskSession): boolean {
  if (session.state === "RUNNING") return true;
  if (!isOfficeSession(session) && session.state === "WAITING_FOR_INPUT") {
    return true;
  }
  return false;
}

function isTerminalSession(session: TaskSession): boolean {
  return (
    session.state === "COMPLETED" || session.state === "FAILED" || session.state === "CANCELLED"
  );
}

function readPersistedCollapsed(sessionId: string): boolean | null {
  if (typeof window === "undefined") return null;
  try {
    const raw = globalThis.localStorage?.getItem(COLLAPSE_KEY_PREFIX + sessionId);
    if (raw === "1") return true;
    if (raw === "0") return false;
  } catch {
    // Access denied / quota / etc — fall back to defaults.
  }
  return null;
}

function writePersistedCollapsed(sessionId: string, collapsed: boolean): void {
  if (typeof window === "undefined") return;
  try {
    globalThis.localStorage?.setItem(COLLAPSE_KEY_PREFIX + sessionId, collapsed ? "1" : "0");
  } catch {
    // Ignore.
  }
}

function StateGlyph({ isLive, isTerminal }: { isLive: boolean; isTerminal: boolean }) {
  if (isLive) {
    return (
      <IconLoader2
        className="h-3 w-3 animate-spin text-primary shrink-0"
        data-testid="session-state-running"
      />
    );
  }
  return (
    <span
      className="h-2 w-2 rounded-full bg-muted-foreground/40 shrink-0 ml-0.5 mr-0.5"
      data-testid={isTerminal ? "session-state-terminal" : "session-state-idle"}
    />
  );
}

function HeaderText({
  verb,
  duration,
  commandsLabel,
}: {
  verb: string;
  duration: string | null;
  commandsLabel: string | null;
}) {
  return (
    <span className="text-muted-foreground">
      {verb}
      {duration && <span className="ml-1">for {duration}</span>}
      {commandsLabel && <span className="ml-1">· ran {commandsLabel}</span>}
    </span>
  );
}

function pickHeaderVerb(isLive: boolean, _isTerminal: boolean): string {
  if (isLive) return "working";
  // Office IDLE and kanban terminal both render "worked" — the agent
  // did real work and isn't doing more right now. The previous "idle"
  // verb was confusing: "Agent idle for 20s" reads as "the agent has
  // been doing nothing for 20s" when actually 20s was the turn duration.
  return "worked";
}

function buildCommandsLabel(count: number): string | null {
  if (count <= 0) return null;
  return `${count} command${count === 1 ? "" : "s"}`;
}

type SessionTimelineEntryProps = {
  taskId: string;
  /**
   * The "representative" session for the entry. When grouping by agent,
   * this is the most-recent session in the group — its id drives the
   * test id, collapse-persistence key, and the embedded chat panel.
   */
  session: TaskSession;
  /**
   * All sessions belonging to this entry's group. Defaults to `[session]`.
   * Used to:
   *   - sum command counts across launches (multi-turn agents accumulate
   *     messages in different rows historically, though post-spec a single
   *     office row carries them all).
   *   - detect "is the agent currently running" by checking any session
   *     in the group.
   */
  groupSessions?: TaskSession[];
  /**
   * Optional small chip rendered next to the agent name (e.g. "Reviewer",
   * "Approver"). Used by office tasks to make the role obvious when the
   * timeline shows multiple agents.
   */
  roleChip?: string | null;
};

function pickActiveSession(group: TaskSession[]): TaskSession | null {
  for (const s of group) {
    if (isLiveSession(s)) return s;
  }
  return null;
}

/**
 * Inline session entry shown in the unified task chat timeline.
 *
 * Office tasks: one entry per (task, agent). The entry covers the agent's
 * persistent session row across RUNNING ↔ IDLE cycles.
 *
 * Kanban / quick-chat: one entry per session (legacy per-launch model).
 *
 * - Live group (any session RUNNING for office, or RUNNING/WAITING for
 *   kanban) defaults to expanded.
 * - IDLE / terminal groups default to collapsed.
 * - Per-(representative-session) collapse preference persists in
 *   localStorage.
 * - When this entry's group is live, it registers its DOM node with the
 *   page's `ActiveSessionRefContext` so the topbar Working spinner can
 *   scroll to it.
 */
export const SessionTimelineEntry = forwardRef<HTMLDivElement, SessionTimelineEntryProps>(
  function SessionTimelineEntry({ taskId, session, groupSessions, roleChip }, forwardedRef) {
    const group = useMemo(
      () => (groupSessions && groupSessions.length > 0 ? groupSessions : [session]),
      [groupSessions, session],
    );
    const activeSession = pickActiveSession(group);
    const isLive = activeSession !== null;
    const isTerminal = isTerminalSession(session) && !isLive;
    const persistedCollapsed = useMemo(() => readPersistedCollapsed(session.id), [session.id]);
    const initialOpen = persistedCollapsed === null ? isLive : !persistedCollapsed;
    const [open, setOpen] = useState(initialOpen);
    // Track whether the user has explicitly toggled this entry's collapse
    // state. Without this, an auto-collapse on RUNNING → IDLE would
    // override a user's deliberate "leave it open" click.
    const userToggledRef = useRef(persistedCollapsed !== null);

    const handleOpenChange = (next: boolean) => {
      setOpen(next);
      userToggledRef.current = true;
      writePersistedCollapsed(session.id, !next);
    };

    // Auto-collapse when the agent's turn ends (live → not-live). This is
    // the "agent worked, now resting" affordance — the body holds the
    // (potentially long) chat transcript that's no longer interesting once
    // the turn finished. Only auto-collapses if the user hasn't manually
    // toggled, so deliberate "keep it open" choices stick.
    const wasLiveRef = useRef(isLive);
    useEffect(() => {
      const wasLive = wasLiveRef.current;
      wasLiveRef.current = isLive;
      if (wasLive && !isLive && !userToggledRef.current) {
        queueMicrotask(() => setOpen(false));
      }
    }, [isLive]);

    const groupIds = useMemo(() => group.map((g) => g.id), [group]);
    // Use the server-resolved per-session count when present (DTO field
    // `command_count`); fall back to the in-store message-derived count for
    // sessions whose messages have streamed in but the DTO predates them.
    const storeCommandCount = useAppStore((s) => {
      let total = 0;
      for (const id of groupIds) total += selectCommandCount(s, id);
      return total;
    });
    // Resolve the agent's current display name from the office store —
    // the per-session snapshot can be empty (then `agentName` defaults
    // to the raw profile id, which renders as a UUID in the header).
    const agentProfileId = session.agentProfileId;
    const resolvedAgentName = useAppStore((s) =>
      agentProfileId
        ? s.office.agentProfiles.find((a) => a.id === agentProfileId)?.name
        : undefined,
    );
    const displayAgentName =
      resolvedAgentName ||
      (session.agentName !== agentProfileId ? session.agentName : null) ||
      "Agent";
    const dtoCommandCount = group.reduce((sum, g) => sum + (g.commandCount ?? 0), 0);
    const commandCount = Math.max(dtoCommandCount, storeCommandCount);
    const duration = entryDuration(session, activeSession);

    const localRef = useRef<HTMLDivElement | null>(null);
    const { setActiveRef } = useActiveSessionRef();

    // Register / unregister the active-session DOM node with the page-scoped
    // registry so the topbar spinner can scroll to it. Keyed on the
    // representative session's id (stable across RUNNING ↔ IDLE cycles
    // when the underlying row is the same office session).
    useEffect(() => {
      if (!isLive) return;
      setActiveRef(session.id, localRef.current);
      return () => setActiveRef(session.id, null);
    }, [isLive, session.id, setActiveRef]);

    const setBothRefs = (node: HTMLDivElement | null) => {
      localRef.current = node;
      if (typeof forwardedRef === "function") forwardedRef(node);
      else if (forwardedRef) forwardedRef.current = node;
    };

    const headerVerb = pickHeaderVerb(isLive, isTerminal);
    const commandsLabel = buildCommandsLabel(commandCount);

    // The chat embed prefers the *active* session id when one exists, so a
    // RUNNING resume after IDLE shows live messages. Falls back to the
    // representative for terminal/idle groups.
    const embedSessionId = activeSession?.id ?? session.id;

    return (
      <div ref={setBothRefs} data-testid={`session-timeline-entry-${session.id}`}>
        <Collapsible open={open} onOpenChange={handleOpenChange} className="my-2">
          <CollapsibleTrigger className="flex items-center gap-2 w-full py-1.5 text-sm cursor-pointer hover:opacity-80 transition-opacity">
            <StateGlyph isLive={isLive} isTerminal={isTerminal} />
            <span className="font-medium text-muted-foreground">{displayAgentName}</span>
            {roleChip && (
              <span
                data-testid="session-role-chip"
                className="text-[10px] uppercase tracking-wide rounded bg-muted px-1.5 py-0.5 text-muted-foreground"
              >
                {roleChip}
              </span>
            )}
            <HeaderText verb={headerVerb} duration={duration} commandsLabel={commandsLabel} />
            <span className="flex-1" />
            <IconChevronDown
              className={`h-3.5 w-3.5 text-muted-foreground transition-transform ${open ? "rotate-180" : ""}`}
            />
          </CollapsibleTrigger>
          <CollapsibleContent>
            <div
              data-testid="session-chat-embed"
              className="ml-4 h-[350px] flex flex-col overflow-hidden border-l border-border/50 pl-3"
            >
              <AdvancedChatPanel taskId={taskId} sessionId={embedSessionId} hideInput />
            </div>
          </CollapsibleContent>
        </Collapsible>
      </div>
    );
  },
);
