'use client';

import { useEffect, useMemo, useState } from 'react';
import { IconCheck, IconLoader2, IconAlertCircle, IconPlayerStopFilled, IconPlus, IconX } from '@tabler/icons-react';
import type { TaskSession, TaskSessionState } from '@/lib/types/http';
import { Badge } from '@kandev/ui/badge';
import { Command, CommandGroup, CommandItem, CommandList } from '@kandev/ui/command';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';

type SessionSwitcherProps = {
  taskId: string | null;
  activeSessionId: string | null;
  sessions: TaskSession[];
  agentLabelsById: Record<string, string>;
  onSelectSession: (taskId: string, sessionId: string) => void;
  showHeader?: boolean;
  onCreateSession?: () => void;
};

const STATUS_ORDER: Record<TaskSessionState, number> = {
  RUNNING: 1,
  STARTING: 1,
  WAITING_FOR_INPUT: 2,
  CREATED: 3,
  COMPLETED: 4,
  FAILED: 5,
  CANCELLED: 6,
};

type SessionStatus = 'running' | 'waiting_input' | 'complete' | 'failed' | 'cancelled' | 'starting';

function getSessionStatus(state: TaskSessionState): SessionStatus {
  switch (state) {
    case 'RUNNING':
      return 'running';
    case 'STARTING':
      return 'starting';
    case 'WAITING_FOR_INPUT':
      return 'waiting_input';
    case 'COMPLETED':
      return 'complete';
    case 'FAILED':
      return 'failed';
    case 'CANCELLED':
      return 'cancelled';
    default:
      return 'waiting_input';
  }
}

function getSessionIcon(status: SessionStatus) {
  switch (status) {
    case 'running':
    case 'starting':
      return <IconLoader2 className="h-3.5 w-3.5 text-blue-500 animate-spin" />;
    case 'waiting_input':
      return <IconAlertCircle className="h-3.5 w-3.5 text-yellow-500" />;
    case 'complete':
      return <IconCheck className="h-3.5 w-3.5 text-green-500" />;
    case 'failed':
    case 'cancelled':
      return <IconX className="h-3.5 w-3.5 text-red-500" />;
    default:
      return null;
  }
}

function formatSessionLabel(index: number, total: number) {
  const number = total - index;
  return `#${number}`;
}

function formatDuration(startedAt: string, isRunning: boolean, now: number) {
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

export function TaskSessionSwitcher({
  taskId,
  activeSessionId,
  sessions,
  agentLabelsById,
  onSelectSession,
  showHeader = true,
  onCreateSession,
}: SessionSwitcherProps) {
  const [currentTime, setCurrentTime] = useState(() => Date.now());
  const sortedSessions = useMemo(() => {
    return [...sessions].sort((a, b) => {
      const orderDelta = (STATUS_ORDER[a.state] ?? 99) - (STATUS_ORDER[b.state] ?? 99);
      if (orderDelta !== 0) return orderDelta;
      return new Date(b.started_at).getTime() - new Date(a.started_at).getTime();
    });
  }, [sessions]);

  const resolveAgentLabel = (session: TaskSession) => {
    if (session.agent_profile_id && agentLabelsById[session.agent_profile_id]) {
      return agentLabelsById[session.agent_profile_id];
    }
    return 'Unknown agent';
  };

  useEffect(() => {
    const hasRunning = sortedSessions.some((session) =>
      ['RUNNING', 'STARTING'].includes(session.state)
    );
    if (!hasRunning) return;
    const interval = setInterval(() => {
      setCurrentTime(Date.now());
    }, 1000);
    return () => clearInterval(interval);
  }, [sortedSessions]);

  return (
    <div className="space-y-2">
      {showHeader ? (
        <div className="flex items-center justify-between">
          <span className="text-xs uppercase tracking-wide text-muted-foreground">Sessions</span>
          <div className="flex items-center gap-1.5">
            {onCreateSession ? (
              <Tooltip>
                <TooltipTrigger asChild>
                    <button
                      type="button"
                      onClick={onCreateSession}
                    className="text-muted-foreground hover:text-foreground rounded-md p-1 hover:bg-muted/50 cursor-pointer"
                      aria-label="New session"
                    >
                    <IconPlus className="h-3.5 w-3.5" />
                  </button>
                </TooltipTrigger>
                <TooltipContent side="left">New session</TooltipContent>
              </Tooltip>
            ) : null}
            <Badge variant="secondary" className="text-[10px] px-1.5">
              {sortedSessions.length}
            </Badge>
          </div>
        </div>
      ) : null}
      {sortedSessions.length === 0 ? (
        <div className="text-xs text-muted-foreground">No sessions yet.</div>
      ) : (
        <Command className="bg-transparent p-0 shadow-none">
          <CommandList>
            <CommandGroup className="p-0">
              {sortedSessions.map((session, index) => {
                const isActive = session.id === activeSessionId;
                const agentLabel = resolveAgentLabel(session);
                const status = getSessionStatus(session.state);
                const duration = formatDuration(
                  session.started_at,
                  status === 'running' || status === 'starting',
                  currentTime
                );
                return (
                  <CommandItem
                    key={session.id}
                    value={session.id}
                    data-selected={isActive}
                    onSelect={() => {
                      if (!taskId) return;
                      onSelectSession(taskId, session.id);
                    }}
                    className="group flex w-full cursor-pointer items-center gap-3 px-2 py-1.5 rounded-sm text-left transition-colors data-[selected=true]:bg-muted"
                  >
                    <span className="text-xs font-medium text-muted-foreground w-8 shrink-0">
                      {formatSessionLabel(index, sortedSessions.length)}
                    </span>
                    <span className="text-xs text-foreground flex-1 text-left truncate">
                      {agentLabel}
                    </span>
                    <span className="text-xs text-muted-foreground w-16 text-right shrink-0">
                      {duration}
                    </span>
                    <div className="ml-auto group/status relative h-5 w-5 shrink-0 flex items-center justify-center">
                      <div className="transition-opacity group-hover/status:opacity-0">
                        {getSessionIcon(status)}
                      </div>
                      {(status === 'running' || status === 'starting') && (
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button
                              type="button"
                              aria-label="Stop session"
                              className="absolute h-5 w-5 flex items-center justify-center rounded-md border border-border bg-background text-muted-foreground hover:text-destructive hover:border-destructive transition-colors opacity-0 group-hover/status:opacity-100"
                              onClick={(event) => {
                                event.stopPropagation();
                              }}
                            >
                              <IconPlayerStopFilled className="h-3 w-3" />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent side="left">Stop session</TooltipContent>
                        </Tooltip>
                      )}
                    </div>
                    <span data-slot="command-shortcut" className="hidden" />
                  </CommandItem>
                );
              })}
            </CommandGroup>
          </CommandList>
        </Command>
      )}
    </div>
  );
}
