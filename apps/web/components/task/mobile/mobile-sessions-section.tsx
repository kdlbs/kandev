"use client";

import { memo, useCallback, useMemo, useState } from "react";
import { IconDotsVertical, IconPlus, IconStar } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";
import { AgentLogo } from "@/components/agent-logo";
import { useAppStore } from "@/components/state-provider";
import { useTaskSessions } from "@/hooks/use-task-sessions";
import {
  useSessionActions,
  isSessionStoppable,
  isSessionDeletable,
  isSessionResumable,
} from "@/hooks/domains/session/use-session-actions";
import { NewSessionDialog } from "../new-session-dialog";
import { MobilePillButton } from "./mobile-pill-button";
import { MobilePickerSheet } from "./mobile-picker-sheet";
import { formatTaskSessionStateLabel } from "@/lib/ui/state-labels";
import type { TaskSession, TaskSessionState } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";

type SessionRow = {
  id: string;
  agentName: string | null;
  agentLabel: string;
  state: TaskSessionState | null;
  isPrimary: boolean;
  index: number;
  startedAt: string;
};

function buildSessionRows(
  sessions: TaskSession[],
  agentProfiles: AgentProfileOption[],
  primarySessionId: string | null | undefined,
): SessionRow[] {
  const sorted = [...sessions].sort(
    (a, b) => new Date(a.started_at).getTime() - new Date(b.started_at).getTime(),
  );
  return sorted.map((s, idx) => {
    const profile = agentProfiles.find((p) => p.id === s.agent_profile_id);
    const labelParts = profile?.label.split(" • ") ?? [];
    return {
      id: s.id,
      agentName: profile?.agent_name ?? null,
      agentLabel: labelParts[1] || labelParts[0] || profile?.label || "Agent",
      state: (s.state as TaskSessionState | undefined) ?? null,
      isPrimary: primarySessionId ? s.id === primarySessionId : !!s.is_primary,
      index: idx + 1,
      startedAt: s.started_at,
    };
  });
}

const STATE_TONE: Partial<Record<TaskSessionState, string>> = {
  RUNNING: "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400",
  STARTING: "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400",
  WAITING_FOR_INPUT: "bg-amber-500/15 text-amber-600 dark:text-amber-400",
  FAILED: "bg-destructive/15 text-destructive",
};

function StateBadge({ state }: { state: TaskSessionState | null }) {
  if (!state) return null;
  const tone = STATE_TONE[state] ?? "bg-foreground/10 text-muted-foreground";
  return (
    <span className={`text-[10px] font-medium px-1.5 py-0.5 rounded leading-none ${tone}`}>
      {formatTaskSessionStateLabel(state)}
    </span>
  );
}

function SessionActionsMenu({
  state,
  isPrimary,
  onSetPrimary,
  onStop,
  onResume,
  onAskDelete,
}: {
  state: TaskSessionState | null;
  isPrimary: boolean;
  onSetPrimary: () => void;
  onStop: () => void;
  onResume: () => void;
  onAskDelete: () => void;
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon-sm"
          className="cursor-pointer h-7 w-7"
          onClick={(e) => e.stopPropagation()}
          aria-label="Session actions"
        >
          <IconDotsVertical className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" onClick={(e) => e.stopPropagation()}>
        <DropdownMenuItem
          className="cursor-pointer"
          onSelect={onSetPrimary}
          disabled={isPrimary || !state}
        >
          Set as Primary
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        {state && isSessionStoppable(state) && (
          <DropdownMenuItem className="cursor-pointer" onSelect={onStop}>
            Stop
          </DropdownMenuItem>
        )}
        {state && isSessionResumable(state) && (
          <DropdownMenuItem className="cursor-pointer" onSelect={onResume}>
            Resume
          </DropdownMenuItem>
        )}
        {state && isSessionDeletable(state) && (
          <DropdownMenuItem
            className="cursor-pointer text-destructive focus:text-destructive"
            onSelect={onAskDelete}
          >
            Delete
          </DropdownMenuItem>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function DeleteSessionConfirmDialog({
  open,
  onOpenChange,
  isPrimary,
  isOnlySession,
  onConfirm,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  isPrimary: boolean;
  isOnlySession: boolean;
  onConfirm: () => void;
}) {
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete session?</AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div>
              <p>This will permanently delete the conversation history with this session.</p>
              {isPrimary && !isOnlySession && (
                <p className="mt-2 font-medium">
                  This is the primary session. Another session will be set as primary.
                </p>
              )}
              {isOnlySession && (
                <p className="mt-2 font-medium">This is the only session for this task.</p>
              )}
            </div>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">Cancel</AlertDialogCancel>
          <AlertDialogAction
            onClick={() => {
              onOpenChange(false);
              onConfirm();
            }}
            className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            Delete
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function SessionRowItem({
  row,
  taskId,
  isActive,
  totalSessions,
  onSelect,
}: {
  row: SessionRow;
  taskId: string;
  isActive: boolean;
  totalSessions: number;
  onSelect: (sessionId: string) => void;
}) {
  const [confirmDelete, setConfirmDelete] = useState(false);
  const actions = useSessionActions({ sessionId: row.id, taskId });
  const isOnly = totalSessions === 1;
  const showBadges = totalSessions > 1;

  return (
    <>
      <div
        role="button"
        tabIndex={0}
        onClick={() => onSelect(row.id)}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            onSelect(row.id);
          }
        }}
        data-testid={`mobile-session-row-${row.id}`}
        className={`flex items-center gap-2 px-2 py-2 rounded-md cursor-pointer select-none ${
          isActive ? "bg-accent" : "hover:bg-accent/50"
        }`}
      >
        {row.isPrimary && showBadges && (
          <IconStar className="h-3.5 w-3.5 fill-foreground/60 stroke-0 shrink-0" />
        )}
        {showBadges && (
          <span className="text-[11px] font-medium leading-none text-muted-foreground bg-foreground/10 rounded px-1.5 py-0.5 shrink-0">
            {row.index}
          </span>
        )}
        {row.agentName && <AgentLogo agentName={row.agentName} size={16} className="shrink-0" />}
        <span className="text-sm truncate flex-1">{row.agentLabel}</span>
        <StateBadge state={row.state} />
        <SessionActionsMenu
          state={row.state}
          isPrimary={row.isPrimary}
          onSetPrimary={() => void actions.setPrimary()}
          onStop={() => void actions.stop()}
          onResume={() => void actions.resume()}
          onAskDelete={() => setConfirmDelete(true)}
        />
      </div>
      <DeleteSessionConfirmDialog
        open={confirmDelete}
        onOpenChange={setConfirmDelete}
        isPrimary={row.isPrimary}
        isOnlySession={isOnly}
        onConfirm={() => void actions.remove()}
      />
    </>
  );
}

function useSessionRows(taskId: string | null) {
  const agentProfiles = useAppStore((s) => s.agentProfiles.items);
  const primarySessionId = useAppStore((s) => {
    if (!taskId) return null;
    const task = s.kanban.tasks.find((t: { id: string }) => t.id === taskId);
    return task?.primarySessionId ?? null;
  });
  const { sessions, isLoading } = useTaskSessions(taskId);
  const rows = useMemo(
    () => buildSessionRows(sessions, agentProfiles, primarySessionId),
    [sessions, agentProfiles, primarySessionId],
  );
  return { rows, isLoading, primarySessionId };
}

const MobileSessionsList = memo(function MobileSessionsList({
  taskId,
  onClose,
}: {
  taskId: string | null;
  onClose: () => void;
}) {
  const setActiveSession = useAppStore((s) => s.setActiveSession);
  const activeSessionId = useAppStore((s) => s.tasks.activeSessionId);
  const { rows, isLoading } = useSessionRows(taskId);
  const [launchOpen, setLaunchOpen] = useState(false);

  const handleSelect = useCallback(
    (sessionId: string) => {
      if (!taskId) return;
      setActiveSession(taskId, sessionId);
      onClose();
    },
    [taskId, setActiveSession, onClose],
  );

  if (!taskId) {
    return (
      <div className="text-xs text-muted-foreground px-2 py-6 text-center">No active task</div>
    );
  }

  return (
    <div className="flex flex-col gap-2 px-1">
      <div className="flex items-center justify-between px-1">
        <span className="text-xs font-medium text-muted-foreground">
          {rows.length} session{rows.length === 1 ? "" : "s"}
        </span>
        <Button
          size="sm"
          variant="outline"
          className="h-7 gap-1 cursor-pointer"
          onClick={() => setLaunchOpen(true)}
          data-testid="mobile-launch-session"
        >
          <IconPlus className="h-4 w-4" />
          New session
        </Button>
      </div>
      <div className="flex flex-col gap-0.5">
        {isLoading && rows.length === 0 && (
          <div className="text-xs text-muted-foreground px-2 py-4 text-center">
            Loading sessions…
          </div>
        )}
        {!isLoading && rows.length === 0 && (
          <div className="text-xs text-muted-foreground px-2 py-4 text-center">
            No sessions yet. Launch one to get started.
          </div>
        )}
        {rows.map((row) => (
          <SessionRowItem
            key={row.id}
            row={row}
            taskId={taskId}
            isActive={row.id === activeSessionId}
            totalSessions={rows.length}
            onSelect={handleSelect}
          />
        ))}
      </div>
      {launchOpen && (
        <NewSessionDialog open={launchOpen} onOpenChange={setLaunchOpen} taskId={taskId} />
      )}
    </div>
  );
});

function useActiveSessionPillLabel(taskId: string | null): {
  label: string;
  count: string | undefined;
} {
  const activeSessionId = useAppStore((s) => s.tasks.activeSessionId);
  const { rows } = useSessionRows(taskId);
  const activeRow = rows.find((r) => r.id === activeSessionId);
  const total = rows.length;
  const idx = activeRow?.index;
  let count: string | undefined;
  if (total > 1 && idx) count = `${idx}/${total}`;
  else if (total > 1) count = `${total}`;
  return { label: activeRow?.agentLabel ?? "Session", count };
}

export const MobileSessionsPicker = memo(function MobileSessionsPicker({
  taskId,
  compact,
  fullWidth,
}: {
  taskId: string | null;
  compact?: boolean;
  fullWidth?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const { label, count } = useActiveSessionPillLabel(taskId);
  if (!taskId) return null;
  return (
    <>
      <MobilePillButton
        label={label}
        count={count}
        compact={compact}
        fullWidth={fullWidth}
        isOpen={open}
        onClick={() => setOpen(true)}
        data-testid="mobile-sessions-pill"
        ariaLabel={`Active session: ${label}. Tap to switch.`}
      />
      <MobilePickerSheet open={open} onOpenChange={setOpen} title="Sessions">
        <MobileSessionsList taskId={taskId} onClose={() => setOpen(false)} />
      </MobilePickerSheet>
    </>
  );
});
