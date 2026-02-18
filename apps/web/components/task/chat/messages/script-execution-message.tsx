'use client';

import { memo } from 'react';
import { IconCheck, IconX, IconTerminal } from '@tabler/icons-react';
import { GridSpinner } from '@/components/grid-spinner';
import type { Message } from '@/lib/types/http';
import { Badge } from '@kandev/ui/badge';
import { ExpandableRow } from './expandable-row';
import { useExpandState } from './use-expand-state';

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

function getAgentBootVerb(isResuming: boolean | undefined, isSuccess: boolean, isRunning: boolean): string {
  if (isResuming) {
    if (isSuccess) return 'Resumed';
    if (isRunning) return 'Resuming';
    return 'Failed to resume';
  }
  if (isSuccess) return 'Started';
  if (isRunning) return 'Starting';
  return 'Failed to start';
}

function AgentBootHeader({ metadata, isSuccess, isRunning }: { metadata: ScriptExecutionMetadata; isSuccess: boolean; isRunning: boolean }) {
  const agentName = metadata.agent_name || 'agent';
  const verb = getAgentBootVerb(metadata.is_resuming, isSuccess, isRunning);
  return (
    <div className="flex items-center gap-2 text-xs">
      <span className="inline-flex items-center gap-1.5 shrink-0 whitespace-nowrap">
        <span className="text-xs text-muted-foreground">{verb} agent {agentName}</span>
        {isRunning && <GridSpinner className="text-muted-foreground" />}
      </span>
    </div>
  );
}

function ScriptHeader({ command, isSetup, isRunning, isSuccess }: { command: string; isSetup: boolean; isRunning: boolean; isSuccess: boolean }) {
  return (
    <div className="flex items-center gap-2 text-xs">
      <span className="inline-flex items-center gap-1.5 shrink-0 whitespace-nowrap">
        <Badge variant={isSetup ? 'default' : 'secondary'} className="text-xs">
          {isSetup ? 'Setup' : 'Cleanup'}
        </Badge>
        <span className="font-mono text-xs text-muted-foreground">{command}</span>
        {isRunning && <GridSpinner className="text-muted-foreground" />}
        {isSuccess && !isRunning && <IconCheck className="h-3.5 w-3.5 text-green-500" />}
        {!isSuccess && !isRunning && <IconX className="h-3.5 w-3.5 text-red-500" />}
      </span>
    </div>
  );
}

// Helper function to calculate duration
function calculateDuration(startedAt: string, completedAt: string): string {
  try {
    const start = new Date(startedAt).getTime();
    const end = new Date(completedAt).getTime();
    const durationMs = end - start;
    if (durationMs < 1000) return `${durationMs}ms`;
    if (durationMs < 60000) return `${(durationMs / 1000).toFixed(1)}s`;
    const minutes = Math.floor(durationMs / 60000);
    const seconds = Math.floor((durationMs % 60000) / 1000);
    return `${minutes}m ${seconds}s`;
  } catch {
    return 'N/A';
  }
}

type ScriptFooterProps = {
  isAgentBoot: boolean;
  isRunning: boolean;
  metadata: ScriptExecutionMetadata;
  exitCode: number | undefined;
};

function ScriptFooter({ isAgentBoot, isRunning, metadata, exitCode }: ScriptFooterProps) {
  if (isRunning) return null;
  if (isAgentBoot && metadata.started_at && metadata.completed_at) {
    return (
      <div className="text-[10px] text-muted-foreground pt-2 border-t border-border/30">
        Duration: {calculateDuration(metadata.started_at, metadata.completed_at)}
      </div>
    );
  }
  if (!isAgentBoot && exitCode !== undefined) {
    return (
      <div className="flex items-center justify-between text-[10px] text-muted-foreground pt-2 border-t border-border/30">
        <span>Exit code: {exitCode}</span>
        {metadata.started_at && metadata.completed_at && (
          <span>Duration: {calculateDuration(metadata.started_at, metadata.completed_at)}</span>
        )}
      </div>
    );
  }
  return null;
}

type ScriptBodyProps = {
  isAgentBoot: boolean;
  command: string;
  content: string;
  error: string | undefined;
  isRunning: boolean;
  metadata: ScriptExecutionMetadata;
  exitCode: number | undefined;
};

function ScriptExpandedContent({ isAgentBoot, command, content, error, isRunning, metadata, exitCode }: ScriptBodyProps) {
  return (
    <div className="pl-4 border-l-2 border-border/30 space-y-2">
      {isAgentBoot && command && (
        <div>
          <div className="text-[10px] uppercase tracking-wide text-muted-foreground/60 mb-1">Command</div>
          <pre className="font-mono text-xs bg-muted/30 rounded px-3 py-2 overflow-x-auto whitespace-pre-wrap break-words">
            {command}
          </pre>
        </div>
      )}
      {content && content.trim() !== '' && (
        <div>
          <div className="text-[10px] uppercase tracking-wide text-muted-foreground/60 mb-1">Output</div>
          <pre className="font-mono text-xs bg-muted/30 rounded px-3 py-2 overflow-x-auto max-h-[300px] overflow-y-auto whitespace-pre-wrap break-words">
            {content}
          </pre>
        </div>
      )}
      {error && (
        <div>
          <div className="text-[10px] uppercase tracking-wide text-red-600/70 dark:text-red-400/70 mb-1">Error</div>
          <div className="text-xs text-red-600 dark:text-red-400 bg-red-500/10 rounded px-2 py-1.5">{error}</div>
        </div>
      )}
      <ScriptFooter isAgentBoot={isAgentBoot} isRunning={isRunning} metadata={metadata} exitCode={exitCode} />
    </div>
  );
}

function parseScriptMetadata(comment: Message) {
  const metadata = comment.metadata as ScriptExecutionMetadata | undefined;
  const status = metadata?.status;
  const scriptType = metadata?.script_type;
  const isRunning = status === 'starting' || status === 'running';
  const isSuccess = status === 'exited' && (metadata?.exit_code === 0 || metadata?.exit_code === undefined);
  return { metadata, status, scriptType, isRunning, isSuccess };
}

export const ScriptExecutionMessage = memo(function ScriptExecutionMessage({ comment }: { comment: Message }) {
  const { metadata, status, scriptType, isRunning, isSuccess } = parseScriptMetadata(comment);
  const autoExpanded = isRunning;
  const { isExpanded, handleToggle } = useExpandState(status, autoExpanded);

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

  return (
    <ExpandableRow
      icon={<IconTerminal className="h-4 w-4 text-muted-foreground" />}
      header={isAgentBoot
        ? <AgentBootHeader metadata={metadata} isSuccess={isSuccess} isRunning={isRunning} />
        : <ScriptHeader command={command} isSetup={isSetup} isRunning={isRunning} isSuccess={isSuccess} />
      }
      hasExpandableContent={hasExpandableContent}
      isExpanded={isExpanded}
      onToggle={handleToggle}
    >
      <ScriptExpandedContent
        isAgentBoot={isAgentBoot}
        command={command}
        content={comment.content}
        error={error}
        isRunning={isRunning}
        metadata={metadata}
        exitCode={exit_code}
      />
    </ExpandableRow>
  );
});
