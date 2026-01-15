'use client';

import { useState, useEffect } from 'react';
import {
  IconStack2,
  IconLoader2,
  IconCheck,
  IconAlertCircle,
  IconX,
  IconPlus,
  IconPlayerStopFilled,
} from '@tabler/icons-react';
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

type SessionStatus = 'running' | 'waiting_input' | 'complete' | 'failed';

type Session = {
  id: string;
  number: number;
  agentProfile: string;
  startedAt: Date;
  status: SessionStatus;
};

// Dummy data - will be replaced with real data later
const DUMMY_SESSIONS: Session[] = [
  { id: '1', number: 1, agentProfile: 'Gemini Flash 2.0', startedAt: new Date(Date.now() - 114000), status: 'running' },
  { id: '2', number: 2, agentProfile: 'Auggie Sonnet 4.5', startedAt: new Date(Date.now() - 192000), status: 'complete' },
  { id: '3', number: 3, agentProfile: 'Gemini Flash 2.0', startedAt: new Date(Date.now() - 45000), status: 'waiting_input' },
];

// Format duration from start time
function formatDuration(startedAt: Date, isRunning: boolean): string {
  const now = isRunning ? Date.now() : startedAt.getTime();
  const diff = Math.floor((now - startedAt.getTime()) / 1000);

  const hours = Math.floor(diff / 3600);
  const minutes = Math.floor((diff % 3600) / 60);
  const seconds = diff % 60;

  if (hours > 0) {
    return `${hours}h ${minutes}m`;
  } else if (minutes > 0) {
    return `${minutes}m ${seconds}s`;
  } else {
    return `${seconds}s`;
  }
}

function getStatusIcon(status: SessionStatus) {
  switch (status) {
    case 'running':
      return <IconLoader2 className="h-3.5 w-3.5 text-blue-500 animate-spin" />;
    case 'complete':
      return <IconCheck className="h-3.5 w-3.5 text-green-500" />;
    case 'waiting_input':
      return <IconAlertCircle className="h-3.5 w-3.5 text-yellow-500" />;
    case 'failed':
      return <IconX className="h-3.5 w-3.5 text-red-500" />;
  }
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
  }
}

type SessionsDropdownProps = {
  taskTitle?: string;
  taskDescription?: string;
};

export function SessionsDropdown({ taskTitle = '', taskDescription = '' }: SessionsDropdownProps) {
  const [sessions] = useState<Session[]>(DUMMY_SESSIONS);
  const [selectedSessionId, setSelectedSessionId] = useState<string | null>(null);
  const [currentTime, setCurrentTime] = useState(() => Date.now());
  const [showNewSessionDialog, setShowNewSessionDialog] = useState(false);

  // Update timer every second for running sessions
  useEffect(() => {
    const hasRunningSessions = sessions.some(s => s.status === 'running');
    if (!hasRunningSessions) return;

    const interval = setInterval(() => {
      setCurrentTime(Date.now());
    }, 1000);

    return () => clearInterval(interval);
  }, [sessions]);

  const handleCancelSession = (e: React.MouseEvent, sessionId: string) => {
    e.stopPropagation();
    // TODO: Implement session cancellation
    console.log('Cancel session:', sessionId);
  };

  const handleSelectSession = (sessionId: string) => {
    setSelectedSessionId(sessionId);
    // TODO: Implement session switching
    console.log('Select session:', sessionId);
  };

  const handleCreateSession = (data: {
    prompt: string;
    agentProfileId: string;
    executorId: string;
    environmentId: string;
  }) => {
    // TODO: Implement session creation
    console.log('Create new session:', data);
  };

  return (
    <>
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="h-8 gap-1.5 px-2 cursor-pointer hover:bg-muted/40 border border-border"
        >
          <IconStack2 className="h-4 w-4 text-muted-foreground" />
          <Badge variant="secondary" className="h-5 px-1.5 text-xs font-normal">
            {sessions.length}
          </Badge>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-[480px]">
        <div className="flex items-center justify-between px-2 py-1.5">
          <span className="text-xs font-medium text-muted-foreground">Agent Sessions</span>
          <button
            type="button"
            onClick={() => setShowNewSessionDialog(true)}
            className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
          >
            <IconPlus className="h-3.5 w-3.5" />
            New session
          </button>
        </div>
        <DropdownMenuSeparator />
        <div className="max-h-[300px] overflow-y-auto">
          {sessions.length === 0 ? (
            <div className="px-2 py-6 text-center text-sm text-muted-foreground">
              No sessions yet
            </div>
          ) : (
            <div className="space-y-0.5">
              {sessions.map((session) => {
                const duration = formatDuration(session.startedAt, session.status === 'running');
                // Use currentTime to force re-render when timer updates
                void currentTime;

                return (
                  <div
                    key={session.id}
                    onClick={() => handleSelectSession(session.id)}
                    className={`w-full flex items-center gap-3 px-2 py-1.5 hover:bg-muted/50 rounded-sm cursor-pointer transition-colors ${
                      selectedSessionId === session.id ? 'bg-muted/50' : ''
                    }`}
                  >
                    {/* Session Number - Fixed width */}
                    <span className="text-xs font-medium text-muted-foreground w-8 shrink-0">
                      #{session.number}
                    </span>

                    {/* Agent Profile - Flexible width */}
                    <span className="text-xs text-foreground flex-1 text-left truncate">
                      {session.agentProfile}
                    </span>

                    {/* Duration - Fixed width */}
                    <span className="text-xs text-muted-foreground w-16 text-right shrink-0">
                      {duration}
                    </span>

                    {/* Status Icon - Fixed width with tooltip */}
                    <div className="w-5 shrink-0 flex items-center justify-center">
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <div>{getStatusIcon(session.status)}</div>
                        </TooltipTrigger>
                        <TooltipContent side="left">
                          {getStatusLabel(session.status)}
                        </TooltipContent>
                      </Tooltip>
                    </div>

                    {/* Cancel Button - Only for running sessions */}
                    <div className="w-7 shrink-0 flex items-center justify-center">
                      {session.status === 'running' && (
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button
                              type="button"
                              onClick={(e) => handleCancelSession(e, session.id)}
                              className="h-5 w-5 flex items-center justify-center rounded-md border border-border bg-background text-muted-foreground hover:text-destructive hover:border-destructive transition-colors cursor-pointer"
                            >
                              <IconPlayerStopFilled className="h-3 w-3" />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent side="left">Cancel session</TooltipContent>
                        </Tooltip>
                      )}
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

