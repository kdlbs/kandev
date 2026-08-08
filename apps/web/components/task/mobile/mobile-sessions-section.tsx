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
import { HandoffDropdownMenuSub } from "../handoff-profile-menu-items";
import { NewSessionDialog, type HandoffPreset } from "../new-session-dialog";
import { MobilePillButton } from "./mobile-pill-button";
import { MobilePickerSheet } from "./mobile-picker-sheet";
import { formatTaskSessionStateLabel } from "@/lib/ui/state-labels";
import { getSessionStateIcon } from "@/lib/ui/state-icons";
import { repositorySlug } from "@/lib/repository-slug";
import { useSessionPendingInput, type PendingInput } from "@/hooks/use-task-pending-input";
import type { ForegroundActivity, TaskSession, TaskSessionState } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";
import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";

type SessionRow = {
  id: string;
  agentName: string | null;
  agentLabel: string;
  repositoryLabel: string | null;
  state: TaskSessionState | null;
  foregroundActivity: ForegroundActivity | null;
  isPrimary: boolean;
  index: number;
  startedAt: string;
};

function buildSessionRows(
  sessions: TaskSession[],
  agentProfiles: AgentProfileOption[],
  primarySessionId: string | null | undefined,
  repositoryLabelsById: ReadonlyMap<string, string>,
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
      // User-supplied session name wins over the derived profile label,
      // matching the desktop session tab title precedence.
      agentLabel: s.name || labelParts[1] || labelParts[0] || profile?.label || t("task:agent"),
      repositoryLabel: s.repository_id ? (repositoryLabelsById.get(s.repository_id) ?? null) : null,
      state: (s.state as TaskSessionState | undefined) ?? null,
      foregroundActivity: s.foreground_activity ?? null,
      isPrimary: primarySessionId ? s.id === primarySessionId : !!s.is_primary,
      index: idx + 1,
      startedAt: s.started_at,
    };
  });
}

// The mobile sessions section reads the per-session substate through the same
// shared vocabulary as the desktop session switcher and the reopen menu
// `getSessionStateIcon` carries background-running
// as a spinner distinct — by SHAPE and MOTION, not hue alone — from generating
// (a static dot) and from done (a check), so the three read apart even in a
// grayscale scan. The label reinforces the distinction in words: while spawned
// background work runs, the row reads "Working in background" rather than the
// bare "Running", so background is never confused with generating.
// A catalog key, not copy: `t()` at module scope would freeze at the boot locale.
const BACKGROUND_RUNNING_LABEL_KEY = "task:workingInBackground";

function sessionStateLabel(
  state: TaskSessionState,
  foregroundActivity: ForegroundActivity | null,
  pending: PendingInput,
): string {
  const canRequestInput = state === "RUNNING" || state === "WAITING_FOR_INPUT";
  if (canRequestInput && pending.permission) return t("task:permissionRequested");
  if (canRequestInput && pending.clarification) {
    return formatTaskSessionStateLabel("WAITING_FOR_INPUT");
  }
  if (canRequestInput && foregroundActivity === "background") {
    return t(BACKGROUND_RUNNING_LABEL_KEY);
  }
  return formatTaskSessionStateLabel(state);
}

function StateBadge({
  sessionId,
  state,
  foregroundActivity,
  testId,
}: {
  sessionId: string;
  state: TaskSessionState | null;
  foregroundActivity: ForegroundActivity | null;
  testId?: string;
}) {
  const pending = useSessionPendingInput(sessionId);
  if (!state) return null;
  const label = sessionStateLabel(state, foregroundActivity, pending);
  return (
    <span
      data-testid={testId}
      title={label}
      className="flex items-center gap-1 whitespace-nowrap text-[10px] font-medium leading-none text-muted-foreground shrink-0"
    >
      {getSessionStateIcon(
        state,
        "h-3 w-3 shrink-0",
        foregroundActivity,
        pending.clarification,
        pending.permission,
      )}
      {label}
    </span>
  );
}

function SessionActionsMenu({
  taskId,
  state,
  isPrimary,
  onSetPrimary,
  onStop,
  onResume,
  onAskDelete,
  onHandoffProfile,
}: {
  taskId: string;
  state: TaskSessionState | null;
  isPrimary: boolean;
  onSetPrimary: () => void;
  onStop: () => void;
  onResume: () => void;
  onAskDelete: () => void;
  onHandoffProfile: (profileId: string) => void;
}) {
  const { t } = useTranslation();
  const hasLifecycleAction =
    !!state &&
    (isSessionStoppable(state) || isSessionResumable(state) || isSessionDeletable(state));

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon-sm"
          className="cursor-pointer h-7 w-7"
          onClick={(e) => e.stopPropagation()}
          aria-label={t("task:sessionActions")}
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
          {t("task:setAsPrimary")}
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        {state && isSessionStoppable(state) && (
          <DropdownMenuItem className="cursor-pointer" onSelect={onStop}>
            {t("task:stop")}
          </DropdownMenuItem>
        )}
        {state && isSessionResumable(state) && (
          <DropdownMenuItem className="cursor-pointer" onSelect={onResume}>
            {t("task:resume")}
          </DropdownMenuItem>
        )}
        {state && isSessionDeletable(state) && (
          <DropdownMenuItem
            className="cursor-pointer text-destructive focus:text-destructive"
            onSelect={onAskDelete}
          >
            {t("task:delete")}
          </DropdownMenuItem>
        )}
        {hasLifecycleAction && <DropdownMenuSeparator />}
        <HandoffDropdownMenuSub taskId={taskId} onSelectProfile={onHandoffProfile} />
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
  const { t } = useTranslation();
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("task:deleteSession")}</AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div>
              <p>{t("task:thisWillPermanentlyDeleteTheConversation")}</p>
              {isPrimary && !isOnlySession && (
                <p className="mt-2 font-medium">{t("task:thisIsThePrimarySessionAnother")}</p>
              )}
              {isOnlySession && (
                <p className="mt-2 font-medium">{t("task:thisIsTheOnlySessionFor")}</p>
              )}
            </div>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">{t("common:cancel")}</AlertDialogCancel>
          <AlertDialogAction
            onClick={() => {
              onOpenChange(false);
              onConfirm();
            }}
            className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            {t("task:delete")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function SessionIdentity({ row }: { row: SessionRow }) {
  return (
    <div className="flex min-w-0 flex-1 flex-col">
      <span className="truncate text-sm">{row.agentLabel}</span>
      {row.repositoryLabel && (
        <span
          className="truncate text-[11px] text-muted-foreground"
          data-testid={`mobile-session-repository-${row.id}`}
        >
          {row.repositoryLabel}
        </span>
      )}
    </div>
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
  const [handoffOpen, setHandoffOpen] = useState(false);
  const [handoffPreset, setHandoffPreset] = useState<HandoffPreset | null>(null);
  const actions = useSessionActions({ sessionId: row.id, taskId });
  const isOnly = totalSessions === 1;
  const showBadges = totalSessions > 1;
  const handleHandoffProfile = useCallback(
    (profileId: string) => {
      setHandoffPreset({ sourceSessionId: row.id, targetProfileId: profileId });
      setHandoffOpen(true);
    },
    [row.id],
  );

  return (
    <>
      <div
        role="button"
        tabIndex={0}
        aria-current={isActive ? "true" : undefined}
        onClick={() => onSelect(row.id)}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            onSelect(row.id);
          }
        }}
        data-testid={`mobile-session-row-${row.id}`}
        className={`flex items-center gap-2 border px-2 py-2 rounded-md cursor-pointer select-none ${
          isActive ? "border-primary/50 bg-card" : "border-transparent hover:bg-muted"
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
        <SessionIdentity row={row} />
        <StateBadge
          sessionId={row.id}
          state={row.state}
          foregroundActivity={row.foregroundActivity}
          testId={`mobile-session-state-${row.id}`}
        />
        <SessionActionsMenu
          taskId={taskId}
          state={row.state}
          isPrimary={row.isPrimary}
          onSetPrimary={() => void actions.setPrimary()}
          onStop={() => void actions.stop()}
          onResume={() => void actions.resume()}
          onAskDelete={() => setConfirmDelete(true)}
          onHandoffProfile={handleHandoffProfile}
        />
      </div>
      {handoffPreset && (
        <NewSessionDialog
          open={handoffOpen}
          onOpenChange={(open) => {
            setHandoffOpen(open);
            if (!open) setHandoffPreset(null);
          }}
          taskId={taskId}
          handoff={handoffPreset}
        />
      )}
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
  // `buildSessionRows` resolves its agent-name fallback through the module-level
  // `t`, so the language has to reach the memo below for that label to follow a
  // runtime locale switch.
  const { i18n } = useTranslation();
  const agentProfiles = useAppStore((s) => s.agentProfiles.items);
  const repositoriesByWorkspace = useAppStore((s) => s.repositories.itemsByWorkspaceId);
  const primarySessionId = useAppStore((s) => {
    if (!taskId) return null;
    const task = s.kanban.tasks.find((t: { id: string }) => t.id === taskId);
    return task?.primarySessionId ?? null;
  });
  const { sessions, isLoading } = useTaskSessions(taskId);
  const repositoryLabelsById = useMemo(() => {
    const sessionRepositoryIds = new Set(
      sessions.flatMap((session) => (session.repository_id ? [session.repository_id] : [])),
    );
    if (sessionRepositoryIds.size <= 1) return new Map<string, string>();
    const labels = new Map<string, string>();
    for (const repository of Object.values(repositoriesByWorkspace).flat()) {
      if (sessionRepositoryIds.has(repository.id)) {
        labels.set(repository.id, repositorySlug(repository));
      }
    }
    return labels;
  }, [repositoriesByWorkspace, sessions]);
  const rows = useMemo(
    () => buildSessionRows(sessions, agentProfiles, primarySessionId, repositoryLabelsById),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- i18n.language is the locale trigger
    [sessions, agentProfiles, primarySessionId, repositoryLabelsById, i18n.language],
  );
  return { rows, isLoading, primarySessionId };
}

const MobileSessionsList = memo(function MobileSessionsList({
  taskId,
  activeSessionId,
  onClose,
}: {
  taskId: string | null;
  activeSessionId: string | null;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const setActiveSession = useAppStore((s) => s.setActiveSession);
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
      <div className="text-xs text-muted-foreground px-2 py-6 text-center">
        {t("task:noActiveTask")}
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-2 px-1">
      <div className="flex items-center justify-between px-1">
        <span className="text-xs font-medium text-muted-foreground">
          {t("task:sessionCount", { count: rows.length })}
        </span>
        <Button
          size="sm"
          variant="outline"
          className="h-7 gap-1 cursor-pointer"
          onClick={() => setLaunchOpen(true)}
          data-testid="mobile-launch-session"
        >
          <IconPlus className="h-4 w-4" />
          {t("task:newSession")}
        </Button>
      </div>
      <div className="flex flex-col gap-0.5">
        {isLoading && rows.length === 0 && (
          <div className="text-xs text-muted-foreground px-2 py-4 text-center">
            {t("task:loadingSessions")}
          </div>
        )}
        {!isLoading && rows.length === 0 && (
          <div className="text-xs text-muted-foreground px-2 py-4 text-center">
            {t("task:noSessionsYetLaunchOneTo")}
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

function useActiveSessionPillLabel(
  taskId: string | null,
  sessionId: string | null | undefined,
): {
  label: string;
  count: string | undefined;
  agentName: string | null;
  effectiveSessionId: string | null;
  ariaLabel: string;
} {
  const { t } = useTranslation();
  const storedActiveSessionId = useAppStore((s) => s.tasks.activeSessionId);
  const { rows } = useSessionRows(taskId);
  const effectiveSessionId = sessionId === undefined ? storedActiveSessionId : sessionId;
  const activeRow = rows.find((r) => r.id === effectiveSessionId);
  const total = rows.length;
  const idx = activeRow?.index;
  let count: string | undefined;
  if (total > 1 && idx) count = `${idx}/${total}`;
  else if (total > 1) count = `${total}`;
  const agentLabel = activeRow?.agentLabel ?? t("task:sessionFallbackLabel");
  const repositoryLabel = activeRow?.repositoryLabel;
  return {
    label: repositoryLabel ? `${agentLabel} · ${repositoryLabel}` : agentLabel,
    count,
    agentName: activeRow?.agentName ?? null,
    effectiveSessionId,
    ariaLabel: repositoryLabel
      ? t("task:activeSessionRepositoryTapToSwitch", { agentLabel, repositoryLabel })
      : t("task:activeSessionTapToSwitch", { agentLabel }),
  };
}

export const MobileSessionsPicker = memo(function MobileSessionsPicker({
  taskId,
  sessionId,
  compact,
  fullWidth,
}: {
  taskId: string | null;
  sessionId?: string | null;
  compact?: boolean;
  fullWidth?: boolean;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const { label, count, agentName, effectiveSessionId, ariaLabel } = useActiveSessionPillLabel(
    taskId,
    sessionId,
  );
  if (!taskId) return null;
  return (
    <>
      <MobilePillButton
        icon={
          agentName ? (
            <span
              className="flex shrink-0 items-center"
              data-testid="mobile-session-agent-icon"
              data-agent-name={agentName}
            >
              <AgentLogo agentName={agentName} size={16} className="shrink-0" />
            </span>
          ) : undefined
        }
        label={label}
        count={count}
        compact={compact}
        fullWidth={fullWidth}
        isOpen={open}
        onClick={() => setOpen(true)}
        data-testid="mobile-sessions-pill"
        ariaLabel={ariaLabel}
      />
      <MobilePickerSheet open={open} onOpenChange={setOpen} title={t("task:sessions")}>
        <MobileSessionsList
          taskId={taskId}
          activeSessionId={effectiveSessionId}
          onClose={() => setOpen(false)}
        />
      </MobilePickerSheet>
    </>
  );
});
