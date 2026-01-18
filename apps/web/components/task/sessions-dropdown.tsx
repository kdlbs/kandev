'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import { IconStack2, IconPlus } from '@tabler/icons-react';
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
import { useAppStore } from '@/components/state-provider';
import { getWebSocketClient } from '@/lib/ws/connection';
import { linkToSession } from '@/lib/links';
import { useTaskSessions } from '@/hooks/use-task-sessions';
import type { TaskSession, TaskSessionState } from '@/lib/types/http';
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
};

export function SessionsDropdown({
  taskId,
  activeSessionId = null,
  taskTitle = '',
  taskDescription = '',
}: SessionsDropdownProps) {
  const [currentTime, setCurrentTime] = useState(() => Date.now());
  const [showNewSessionDialog, setShowNewSessionDialog] = useState(false);
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  const setActiveSession = useAppStore((state) => state.setActiveSession);
  const { sessions, loadSessions } = useTaskSessions(taskId);

  const agentLabelsById = useMemo(() => {
    return Object.fromEntries(agentProfiles.map((profile) => [profile.id, profile.label]));
  }, [agentProfiles]);

  const handleOpenChange = useCallback(
    (open: boolean) => {
      if (!open || !taskId) return;
      loadSessions(true);
    },
    [loadSessions, taskId]
  );

  // Update timer every second for running sessions
  useEffect(() => {
    const hasRunningSessions = sessions.some(
      (session) => session.state === 'RUNNING' || session.state === 'STARTING'
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
    setActiveSession(taskId, sessionId);
    updateUrl(sessionId);
  };

  const handleCreateSession = async (data: {
    prompt: string;
    agentProfileId: string;
    executorId: string;
    environmentId: string;
  }) => {
    if (!taskId) return;
    const client = getWebSocketClient();
    if (!client) return;

    try {
      const response = await client.request<{
        success: boolean;
        task_id: string;
        agent_instance_id: string;
        session_id?: string;
        state: string;
      }>(
        'orchestrator.start',
        {
          task_id: taskId,
          agent_profile_id: data.agentProfileId,
          prompt: data.prompt.trim(),
        },
        15000
      );

      await loadSessions(true);

      if (response?.session_id) {
        setActiveSession(taskId, response.session_id);
        updateUrl(response.session_id);
      }
    } catch (error) {
      console.error('Failed to create task session:', error);
    }
  };

  const sortedSessions = useMemo(() => {
    const visibleSessions = taskId ? sessions : [];
    return [...visibleSessions].sort((a, b) => {
      const orderDelta = (STATUS_ORDER[a.state] ?? 99) - (STATUS_ORDER[b.state] ?? 99);
      if (orderDelta !== 0) return orderDelta;
      return new Date(b.started_at).getTime() - new Date(a.started_at).getTime();
    });
  }, [sessions, taskId]);

  const resolveAgentLabel = (session: TaskSession) => {
    if (session.agent_profile_id && agentLabelsById[session.agent_profile_id]) {
      return agentLabelsById[session.agent_profile_id];
    }
    return 'Unknown agent';
  };

  return (
    <>
      <DropdownMenu onOpenChange={handleOpenChange}>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className="h-9 gap-1.5 px-2 cursor-pointer hover:bg-muted/40 border border-border"
          >
            <IconStack2 className="h-4 w-4 text-muted-foreground" />
            <Badge variant="secondary" className="h-5 px-1.5 text-xs font-normal">
              {sortedSessions.length}
            </Badge>
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-[280px]">
          <div className="flex items-center justify-between px-2 py-1.5">
            <span className="text-xs font-medium text-muted-foreground">Sessions</span>
            <button
              type="button"
              onClick={() => setShowNewSessionDialog(true)}
              className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
            >
              <IconPlus className="h-3.5 w-3.5" />
              New
            </button>
          </div>
          <DropdownMenuSeparator />
          <div className="max-h-[300px] overflow-y-auto">
            {sortedSessions.length === 0 ? (
              <div className="px-2 py-6 text-center text-sm text-muted-foreground">
                No sessions yet
              </div>
            ) : (
              <div className="space-y-0.5">
                {sortedSessions.map((session, index) => {
                  const status = mapSessionStatus(session.state);
                  const duration = formatDuration(
                    session.started_at,
                    status === 'running',
                    currentTime
                  );
                  const number = sortedSessions.length - index;

                  return (
                    <div
                      key={session.id}
                      onClick={() => handleSelectSession(session.id)}
                      className={`w-full flex items-center gap-3 px-2 py-1.5 hover:bg-muted/50 rounded-sm cursor-pointer transition-colors ${activeSessionId === session.id ? 'bg-muted/50' : ''
                        }`}
                    >
                      <span className="text-xs font-medium text-muted-foreground w-8 shrink-0">
                        #{number}
                      </span>

                      <span className="text-xs text-foreground flex-1 text-left truncate">
                        {resolveAgentLabel(session)}
                      </span>

                      <span className="text-xs text-muted-foreground w-16 text-right shrink-0">
                        {duration}
                      </span>

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
                })}
              </div>
            )}
          </div>
        </DropdownMenuContent>
      </DropdownMenu>

      <TaskCreateDialog
        open={showNewSessionDialog}
        onOpenChange={setShowNewSessionDialog}
        mode="session"
        workspaceId={null}
        boardId={null}
        defaultColumnId={null}
        columns={[]}
        onCreateSession={handleCreateSession}
        initialValues={{
          title: taskTitle,
          description: taskDescription,
        }}
      />
    </>
  );
}
