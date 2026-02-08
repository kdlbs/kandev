'use client';

import { useState, useRef, memo, useCallback } from 'react';
import { IconCheck, IconX, IconTerminal } from '@tabler/icons-react';
import { GridSpinner } from '@/components/grid-spinner';
import type { Message } from '@/lib/types/http';
import { Badge } from '@kandev/ui/badge';
import { ExpandableRow } from './expandable-row';

interface ScriptExecutionMetadata {
  script_type: 'setup' | 'cleanup' | 'agent_boot';
  agent_name?: string;
  command: string;
  status: 'starting' | 'running' | 'exited' | 'failed';
  exit_code?: number;
  is_resuming?: boolean;
  started_at?: string;
  completed_at?: string;
  process_id?: string;
  error?: string;
}

export const ScriptExecutionMessage = memo(function ScriptExecutionMessage({ comment }: { comment: Message }) {
  const metadata = comment.metadata as ScriptExecutionMetadata | undefined;
  const [manualExpandState, setManualExpandState] = useState<boolean | null>(null);
  const prevStatusRef = useRef(metadata?.status);

  const status = metadata?.status;
  const scriptType = metadata?.script_type;

  // Reset manual state when status changes (allows auto-expand behavior to resume)
  if (prevStatusRef.current !== status) {
    prevStatusRef.current = status;
    if (manualExpandState !== null) {
      setManualExpandState(null);
    }
  }

  const isRunning = status === 'starting' || status === 'running';
  const isSuccess = status === 'exited' && (metadata?.exit_code === 0 || metadata?.exit_code === undefined);

  // Auto-expand when running (matching tool-execute-message pattern)
  const autoExpanded = isRunning;
  const isExpanded = manualExpandState ?? autoExpanded;

  const handleToggle = useCallback(() => {
    setManualExpandState((prev) => !(prev ?? autoExpanded));
  }, [autoExpanded]);

  // Fallback for missing metadata
  if (!metadata || !scriptType) {
    const hasExpandableContent = !!comment.content;
    return (
      <ExpandableRow
        icon={<IconTerminal className="h-4 w-4 text-yellow-600 dark:text-yellow-400" />}
        header={
          <div className="flex items-center gap-2 text-xs">
            <span className="text-xs font-mono text-yellow-600 dark:text-yellow-400">
              Script Execution (metadata unavailable)
            </span>
          </div>
        }
        hasExpandableContent={hasExpandableContent}
        isExpanded={isExpanded}
        onToggle={handleToggle}
      >
        {comment.content && (
          <div className="pl-4 border-l-2 border-border/30">
            <pre className="font-mono text-xs bg-muted/30 rounded px-3 py-2 overflow-x-auto max-h-[300px] overflow-y-auto whitespace-pre-wrap break-words">
              {comment.content}
            </pre>
          </div>
        )}
      </ExpandableRow>
    );
  }

  const { command, exit_code, error } = metadata;
  const isAgentBoot = scriptType === 'agent_boot';
  const isSetup = scriptType === 'setup';
  const hasDuration = !!(metadata.started_at && metadata.completed_at);
  const hasExpandableContent = !!(command || comment.content || error || exit_code !== undefined || hasDuration);

  // Build header based on script_type
  let headerContent: React.ReactNode;

  if (isAgentBoot) {
    const agentName = metadata.agent_name || 'agent';
    headerContent = (
      <div className="flex items-center gap-2 text-xs">
        <span className="inline-flex items-center gap-1.5 shrink-0 whitespace-nowrap">
          <span className="text-xs text-muted-foreground">
            {metadata.is_resuming
              ? (isSuccess ? 'Resumed' : isRunning ? 'Resuming' : 'Failed to resume')
              : (isSuccess ? 'Started' : isRunning ? 'Starting' : 'Failed to start')
            } agent {agentName}
          </span>
          {isRunning && <GridSpinner className="text-muted-foreground" />}
        </span>
      </div>
    );
  } else {
    headerContent = (
      <div className="flex items-center gap-2 text-xs">
        <span className="inline-flex items-center gap-1.5 shrink-0 whitespace-nowrap">
          <Badge variant={isSetup ? 'default' : 'secondary'} className="text-xs">
            {isSetup ? 'Setup' : 'Cleanup'}
          </Badge>
          <span className="font-mono text-xs text-muted-foreground">
            {command}
          </span>
          {isRunning && <GridSpinner className="text-muted-foreground" />}
          {isSuccess && !isRunning && (
            <IconCheck className="h-3.5 w-3.5 text-green-500" />
          )}
          {!isSuccess && !isRunning && (
            <IconX className="h-3.5 w-3.5 text-red-500" />
          )}
        </span>
      </div>
    );
  }

  return (
    <ExpandableRow
      icon={<IconTerminal className="h-4 w-4 text-muted-foreground" />}
      header={headerContent}
      hasExpandableContent={hasExpandableContent}
      isExpanded={isExpanded}
      onToggle={handleToggle}
    >
      <div className="pl-4 border-l-2 border-border/30 space-y-2">
        {/* Command (agent_boot only — setup/cleanup already show it in header) */}
        {isAgentBoot && command && (
          <div>
            <div className="text-[10px] uppercase tracking-wide text-muted-foreground/60 mb-1">
              Command
            </div>
            <pre className="font-mono text-xs bg-muted/30 rounded px-3 py-2 overflow-x-auto whitespace-pre-wrap break-words">
              {command}
            </pre>
          </div>
        )}

        {/* Output */}
        {comment.content && comment.content.trim() !== '' && (
          <div>
            <div className="text-[10px] uppercase tracking-wide text-muted-foreground/60 mb-1">
              Output
            </div>
            <pre className="font-mono text-xs bg-muted/30 rounded px-3 py-2 overflow-x-auto max-h-[300px] overflow-y-auto whitespace-pre-wrap break-words">
              {comment.content}
            </pre>
          </div>
        )}

        {/* Error message */}
        {error && (
          <div>
            <div className="text-[10px] uppercase tracking-wide text-red-600/70 dark:text-red-400/70 mb-1">
              Error
            </div>
            <div className="text-xs text-red-600 dark:text-red-400 bg-red-500/10 rounded px-2 py-1.5">
              {error}
            </div>
          </div>
        )}

        {/* Footer with exit code and duration */}
        {isAgentBoot && !isRunning && metadata.started_at && metadata.completed_at && (
          <div className="text-[10px] text-muted-foreground pt-2 border-t border-border/30">
            Duration: {calculateDuration(metadata.started_at, metadata.completed_at)}
          </div>
        )}
        {!isAgentBoot && !isRunning && exit_code !== undefined && (
          <div className="flex items-center justify-between text-[10px] text-muted-foreground pt-2 border-t border-border/30">
            <span>Exit code: {exit_code}</span>
            {metadata.started_at && metadata.completed_at && (
              <span>
                Duration: {calculateDuration(metadata.started_at, metadata.completed_at)}
              </span>
            )}
          </div>
        )}
      </div>
    </ExpandableRow>
  );
});

// Helper function to calculate duration
function calculateDuration(startedAt: string, completedAt: string): string {
  try {
    const start = new Date(startedAt).getTime();
    const end = new Date(completedAt).getTime();
    const durationMs = end - start;

    if (durationMs < 1000) {
      return `${durationMs}ms`;
    } else if (durationMs < 60000) {
      return `${(durationMs / 1000).toFixed(1)}s`;
    } else {
      const minutes = Math.floor(durationMs / 60000);
      const seconds = Math.floor((durationMs % 60000) / 1000);
      return `${minutes}m ${seconds}s`;
    }
  } catch {
    return 'N/A';
  }
}
