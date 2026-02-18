'use client';

import { memo, useCallback, useEffect, useMemo, useState } from 'react';
import { IconStack2, IconPlus, IconStar } from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@kandev/ui/dropdown-menu';
import { Badge } from '@kandev/ui/badge';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { TaskCreateDialog } from '../task-create-dialog';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import { linkToSession } from '@/lib/links';
import { useTaskSessions } from '@/hooks/use-task-sessions';
import { performLayoutSwitch } from '@/lib/state/dockview-store';
import type { TaskSession, TaskSessionState } from '@/lib/types/http';
import type { AgentProfileOption } from '@/lib/state/slices';
import { getSessionStateIcon } from '@/lib/ui/state-icons';

type SessionStatus = 'running' | 'waiting_input' | 'complete' | 'failed' | 'cancelled';

const STATUS_ORDER: Record<TaskSessionState, number> = {
  RUNNING: 1,
  STARTING: 1,
  WAITING_FOR_INPUT: 2,
  CREATED: 3,
  COMPLETED: 4,
  FAILED: 5,
  CANCELLED: 6,
};

// Format duration from start time
function formatDuration(startedAt: string, isRunning: boolean, now: number): string {
  const start = new Date(startedAt).getTime();
  const diff = Math.floor(((isRunning ? now : start) - start) / 1000);

  const hours = Math.floor(diff / 3600);
  const minutes = Math.floor((diff % 3600) / 60);
  const seconds = diff % 60;

  if (hours > 0) {
    return `${hours}h ${minutes}m`;
  }
  if (minutes > 0) {
    return `${minutes}m ${seconds}s`;
  }
  return `${seconds}s`;
}

function getStatusLabel(status: SessionStatus) {
  switch (status) {
    case 'running':
      return 'Running';
    case 'complete':
      return 'Complete';
    case 'waiting_input':
      return 'Waiting for input';
    case 'failed':
      return 'Failed';
    case 'cancelled':
      return 'Cancelled';
  }
}

function mapSessionStatus(state: TaskSessionState): SessionStatus {
  switch (state) {
    case 'RUNNING':
    case 'STARTING':
      return 'running';
    case 'WAITING_FOR_INPUT':
      return 'waiting_input';
    case 'COMPLETED':
      return 'complete';
    case 'FAILED':
      return 'failed';
    case 'CANCELLED':
      return 'cancelled';
    default:
      return 'running';
  }
}

type SessionsDropdownProps = {
  taskId: string | null;
  activeSessionId?: string | null;
  taskTitle?: string;
  taskDescription?: string;
  primarySessionId?: string | null;
  onSetPrimary?: (sessionId: string) => void;
};

export const SessionsDropdown = memo(function SessionsDropdown({
  taskId,
  activeSessionId = null,
  taskTitle = '',
  taskDescription = '',
  primarySessionId = null,
  onSetPrimary,
}: SessionsDropdownProps) {
  const [currentTime, setCurrentTime] = useState(() => Date.now());
  const [showNewSessionDialog, setShowNewSessionDialog] = useState(false);
  const [open, setOpen] = useState(false);
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  const setActiveSession = useAppStore((state) => state.setActiveSession);
  const appStore = useAppStoreApi();
  const { sessions, loadSessions } = useTaskSessions(taskId);

  const agentLabelsById = useMemo(() => {
    return Object.fromEntries(agentProfiles.map((profile: AgentProfileOption) => [profile.id, profile.label]));
  }, [agentProfiles]);

  const handleOpenChange = useCallback(
    (nextOpen: boolean) => {
      setOpen(nextOpen);
      if (!nextOpen || !taskId) return;
      loadSessions(true);
    },
    [loadSessions, taskId]
  );

  // Update timer every second for running sessions
  useEffect(() => {
    const hasRunningSessions = sessions.some(
      (session: TaskSession) => session.state === 'RUNNING' || session.state === 'STARTING'
    );
    if (!hasRunningSessions) return;

    const interval = setInterval(() => {
      setCurrentTime(Date.now());
    }, 1000);

    return () => clearInterval(interval);
  }, [sessions]);

  const updateUrl = useCallback((sessionId: string) => {
    if (typeof window === 'undefined') return;
    window.history.replaceState({}, '', linkToSession(sessionId));
  }, []);

  const handleSelectSession = (sessionId: string) => {
    if (!taskId) return;
    const oldSessionId = appStore.getState().tasks.activeSessionId;
    setActiveSession(taskId, sessionId);
    performLayoutSwitch(oldSessionId, sessionId);
    updateUrl(sessionId);
    setOpen(false);
  };

  const sortedSessions = useMemo(() => {
    const visibleSessions = taskId ? sessions : [];
    return [...visibleSessions].sort((a: TaskSession, b: TaskSession) => {
      const orderDelta = (STATUS_ORDER[a.state] ?? 99) - (STATUS_ORDER[b.state] ?? 99);
      if (orderDelta !== 0) return orderDelta;
      return new Date(b.started_at).getTime() - new Date(a.started_at).getTime();
    });
  }, [sessions, taskId]);

  const resolveAgentLabel = useCallback(
    (session: TaskSession) => {
      if (session.agent_profile_id && agentLabelsById[session.agent_profile_id]) {
        return agentLabelsById[session.agent_profile_id];
      }
      return 'Unknown agent';
    },
    [agentLabelsById]
  );

  return (
    <>
      <DropdownMenu open={open} onOpenChange={handleOpenChange}>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="sm" className="h-7 gap-1.5 px-2 cursor-pointer hover:bg-muted/40">
            <IconStack2 className="h-4 w-4 text-muted-foreground" />
            <Badge variant="secondary" className="h-5 px-1.5 text-xs font-normal">
              {sortedSessions.length}
            </Badge>
          </Button>
        </DropdownMenuTrigger>
        <SessionDropdownContent
          sortedSessions={sortedSessions}
          activeSessionId={activeSessionId}
          primarySessionId={primarySessionId}
          currentTime={currentTime}
          resolveAgentLabel={resolveAgentLabel}
          onSelectSession={handleSelectSession}
          onSetPrimary={onSetPrimary}
          onNewSession={() => setShowNewSessionDialog(true)}
        />
      </DropdownMenu>
      <NewSessionDialog
        open={showNewSessionDialog}
        onOpenChange={setShowNewSessionDialog}
        taskId={taskId}
        taskTitle={taskTitle}
        taskDescription={taskDescription}
      />
    </>
  );
});

/** Dropdown content with header and session list */
function SessionDropdownContent({
  sortedSessions,
  activeSessionId,
  primarySessionId,
  currentTime,
  resolveAgentLabel,
  onSelectSession,
  onSetPrimary,
  onNewSession,
}: {
  sortedSessions: TaskSession[];
  activeSessionId: string | null;
  primarySessionId: string | null;
  currentTime: number;
  resolveAgentLabel: (session: TaskSession) => string;
  onSelectSession: (sessionId: string) => void;
  onSetPrimary?: (sessionId: string) => void;
  onNewSession: () => void;
}) {
  return (
    <DropdownMenuContent align="end" className="w-auto min-w-[240px] max-w-[420px]">
      <div className="flex items-center justify-between px-2 py-0">
        <span className="text-xs font-medium text-muted-foreground">Sessions</span>
        <button
          type="button"
          onClick={onNewSession}
          className="flex items-center gap-1 rounded-md border border-border/60 px-2 py-1 text-xs text-muted-foreground hover:text-foreground hover:border-border transition-colors cursor-pointer"
        >
          <IconPlus className="h-3.5 w-3.5" />
          New
        </button>
      </div>
      <DropdownMenuSeparator />
      <SessionDropdownList
        sessions={sortedSessions}
        activeSessionId={activeSessionId}
        primarySessionId={primarySessionId}
        currentTime={currentTime}
        resolveAgentLabel={resolveAgentLabel}
        onSelectSession={onSelectSession}
        onSetPrimary={onSetPrimary}
      />
    </DropdownMenuContent>
  );
}

/** New session dialog wrapper */
function NewSessionDialog({
  open,
  onOpenChange,
  taskId,
  taskTitle,
  taskDescription,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  taskId: string | null;
  taskTitle: string;
  taskDescription: string;
}) {
  return (
    <TaskCreateDialog
      open={open}
      onOpenChange={onOpenChange}
      mode="session"
      workspaceId={null}
      workflowId={null}
      defaultStepId={null}
      steps={[]}
      taskId={taskId}
      initialValues={{ title: taskTitle, description: taskDescription }}
    />
  );
}

/** Session list inside the dropdown */
function SessionDropdownList({
  sessions,
  activeSessionId,
  primarySessionId,
  currentTime,
  resolveAgentLabel,
  onSelectSession,
  onSetPrimary,
}: {
  sessions: TaskSession[];
  activeSessionId: string | null;
  primarySessionId: string | null;
  currentTime: number;
  resolveAgentLabel: (session: TaskSession) => string;
  onSelectSession: (sessionId: string) => void;
  onSetPrimary?: (sessionId: string) => void;
}) {
  if (sessions.length === 0) {
    return (
      <div className="max-h-[300px] overflow-y-auto">
        <div className="px-2 py-6 text-center text-sm text-muted-foreground">
          No sessions yet
        </div>
      </div>
    );
  }
  return (
    <div className="max-h-[300px] overflow-y-auto">
      <div className="space-y-0.5">
        {sessions.map((session, index) => (
          <SessionRow
            key={session.id}
            session={session}
            number={sessions.length - index}
            isActive={activeSessionId === session.id}
            isPrimary={session.id === primarySessionId}
            currentTime={currentTime}
            agentLabel={resolveAgentLabel(session)}
            onSelect={onSelectSession}
            onSetPrimary={onSetPrimary}
          />
        ))}
      </div>
    </div>
  );
}

/** Individual session row in the dropdown */
function SessionRow({
  session,
  number,
  isActive,
  isPrimary,
  currentTime,
  agentLabel,
  onSelect,
  onSetPrimary,
}: {
  session: TaskSession;
  number: number;
  isActive: boolean;
  isPrimary: boolean;
  currentTime: number;
  agentLabel: string;
  onSelect: (sessionId: string) => void;
  onSetPrimary?: (sessionId: string) => void;
}) {
  const status = mapSessionStatus(session.state);
  const duration = formatDuration(session.started_at, status === 'running', currentTime);
  const showDuration = duration !== '0s';

  return (
    <div
      onClick={() => onSelect(session.id)}
      className={`w-full flex items-center gap-3 px-2 py-1.5 hover:bg-muted/50 rounded-sm cursor-pointer transition-colors ${isActive ? 'bg-muted/50' : ''}`}
    >
      <span className="text-xs font-medium text-muted-foreground w-8 shrink-0">
        #{number}
      </span>
      <span className="text-xs text-foreground flex-1 text-left flex items-center gap-1.5">
        {agentLabel}
        {isPrimary && <IconStar className="h-3.5 w-3.5 text-amber-500 fill-amber-500" />}
      </span>
      {showDuration && (
        <span className="text-xs text-muted-foreground w-16 text-right shrink-0">
          {duration}
        </span>
      )}
      {!isPrimary && onSetPrimary && (
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type="button"
              onClick={(e) => { e.stopPropagation(); onSetPrimary(session.id); }}
              className="w-5 shrink-0 flex items-center justify-center text-muted-foreground hover:text-amber-500 transition-colors"
            >
              <IconStar className="h-3.5 w-3.5" />
            </button>
          </TooltipTrigger>
          <TooltipContent side="left">Set as Primary</TooltipContent>
        </Tooltip>
      )}
      <div className="w-5 shrink-0 flex items-center justify-center">
        <Tooltip>
          <TooltipTrigger asChild>
            <div>{getSessionStateIcon(session.state, 'h-3.5 w-3.5')}</div>
          </TooltipTrigger>
          <TooltipContent side="left">{getStatusLabel(status)}</TooltipContent>
        </Tooltip>
      </div>
    </div>
  );
}
