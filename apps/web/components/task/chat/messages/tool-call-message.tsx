'use client';

import { useCallback, useState } from 'react';
import {
  IconCheck,
  IconChevronDown,
  IconChevronRight,
  IconCode,
  IconEdit,
  IconEye,
  IconFile,
  IconLoader2,
  IconSearch,
  IconTerminal2,
  IconX,
} from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import { cn } from '@/lib/utils';
import type { Message } from '@/lib/types/http';
import type { ToolCallMetadata } from '@/components/task/chat/types';
import { useAppStore } from '@/components/state-provider';
import { getWebSocketClient } from '@/lib/ws/connection';

function getToolIcon(toolName: string | undefined, className: string) {
  const name = toolName?.toLowerCase() ?? '';
  if (name === 'edit' || name.includes('edit') || name.includes('replace') || name.includes('write') || name.includes('save')) {
    return <IconEdit className={className} />;
  }
  if (name === 'read' || name.includes('view') || name.includes('read')) {
    return <IconEye className={className} />;
  }
  if (name === 'search' || name.includes('search') || name.includes('find') || name.includes('retrieval')) {
    return <IconSearch className={className} />;
  }
  if (name === 'execute' || name.includes('terminal') || name.includes('exec') || name.includes('launch') || name.includes('process')) {
    return <IconTerminal2 className={className} />;
  }
  if (name === 'delete' || name === 'move' || name.includes('file') || name.includes('create')) {
    return <IconFile className={className} />;
  }
  return <IconCode className={className} />;
}

function getStatusIcon(status?: string) {
  switch (status) {
    case 'complete':
      return <IconCheck className="h-3.5 w-3.5 text-green-500" />;
    case 'error':
      return <IconX className="h-3.5 w-3.5 text-red-500" />;
    case 'running':
      return <IconLoader2 className="h-3.5 w-3.5 text-blue-500 animate-spin" />;
    default:
      return null;
  }
}

type ToolCallMessageProps = {
  comment: Message;
  taskId?: string;
};

export function ToolCallMessage({ comment, taskId }: ToolCallMessageProps) {
  const [isExpanded, setIsExpanded] = useState(false);
  const [isResponding, setIsResponding] = useState(false);
  const metadata = comment.metadata as ToolCallMetadata | undefined;
  const pendingPermissions = useAppStore((state) => state.permissions.pending);
  const removePendingPermission = useAppStore((state) => state.removePendingPermission);

  const toolCallId = metadata?.tool_call_id;
  const toolName = metadata?.tool_name ?? '';
  const title = metadata?.title ?? comment.content ?? 'Tool call';
  const status = metadata?.status;
  const args = metadata?.args;
  const result = metadata?.result;

  // Find pending permission for this tool call (only if we have a valid tool_call_id)
  const pendingPermission = pendingPermissions.find(
    (p) => toolCallId && p.tool_call_id === toolCallId && (!taskId || p.task_id === taskId)
  );
  const isPendingApproval = !!pendingPermission;

  const handleRespond = useCallback(
    async (optionId: string, cancelled: boolean = false) => {
      if (!pendingPermission) return;
      setIsResponding(true);

      const client = getWebSocketClient();
      if (!client) {
        console.error('WebSocket client not available');
        setIsResponding(false);
        return;
      }

      try {
        await client.request('permission.respond', {
          task_id: pendingPermission.task_id,
          pending_id: pendingPermission.pending_id,
          option_id: cancelled ? undefined : optionId,
          cancelled,
        });
        removePendingPermission(pendingPermission.pending_id);
      } catch (error) {
        console.error('Failed to respond to permission request:', error);
      } finally {
        setIsResponding(false);
      }
    },
    [pendingPermission, removePendingPermission]
  );

  const handleApprove = useCallback(() => {
    const allowOption = pendingPermission?.options.find(
      (opt) => opt.kind === 'allow_once' || opt.kind === 'allow_always'
    );
    if (allowOption) {
      handleRespond(allowOption.option_id);
    }
  }, [pendingPermission, handleRespond]);

  const handleReject = useCallback(() => {
    const rejectOption = pendingPermission?.options.find(
      (opt) => opt.kind === 'reject_once' || opt.kind === 'reject_always'
    );
    if (rejectOption) {
      handleRespond(rejectOption.option_id);
    } else {
      handleRespond('', true);
    }
  }, [pendingPermission, handleRespond]);

  const toolIcon = getToolIcon(toolName, 'h-4 w-4 text-amber-600 dark:text-amber-400 flex-shrink-0');
  const hasDetails = args && Object.keys(args).length > 0;

  let filePath: string | undefined;
  const rawPath = args?.path ?? args?.file ?? args?.file_path;
  if (typeof rawPath === 'string') {
    filePath = rawPath;
  }
  if (!filePath && Array.isArray(args?.locations) && args.locations.length > 0) {
    const firstLoc = args.locations[0] as { path?: string } | undefined;
    if (firstLoc?.path) {
      filePath = firstLoc.path;
    }
  }

  return (
    <div className={cn(
      'w-full rounded-md border overflow-hidden',
      isPendingApproval
        ? 'border-amber-500/50 bg-amber-500/5'
        : 'border-border/50 bg-muted/20'
    )}>
      <button
        type="button"
        onClick={() => hasDetails && setIsExpanded(!isExpanded)}
        className={cn(
          'w-full flex items-center gap-2 px-3 py-2 text-sm text-left',
          hasDetails && 'cursor-pointer hover:bg-muted/40 transition-colors'
        )}
        disabled={!hasDetails}
      >
        {toolIcon}
        <span className="flex-1 font-mono text-xs text-muted-foreground truncate">
          {title}
        </span>
        {filePath && (
          <span className="text-xs text-muted-foreground/60 truncate max-w-[200px]">
            {filePath}
          </span>
        )}
        {!isPendingApproval && getStatusIcon(status)}
        {hasDetails && (
          isExpanded
            ? <IconChevronDown className="h-4 w-4 text-muted-foreground/50" />
            : <IconChevronRight className="h-4 w-4 text-muted-foreground/50" />
        )}
      </button>

      {/* Inline approval buttons when permission is pending */}
      {isPendingApproval && (
        <div className="flex items-center gap-2 px-3 py-2 border-t border-amber-500/30 bg-amber-500/10">
          <span className="text-xs text-amber-600 dark:text-amber-400 flex-1">
            Approve this action?
          </span>
          <Button
            size="sm"
            variant="ghost"
            onClick={handleReject}
            disabled={isResponding}
            className="h-7 px-2 text-red-600 hover:text-red-700 hover:bg-red-500/10"
          >
            <IconX className="h-4 w-4 mr-1" />
            Deny
          </Button>
          <Button
            size="sm"
            variant="ghost"
            onClick={handleApprove}
            disabled={isResponding}
            className="h-7 px-2 text-green-600 hover:text-green-700 hover:bg-green-500/10"
          >
            {isResponding ? (
              <IconLoader2 className="h-4 w-4 mr-1 animate-spin" />
            ) : (
              <IconCheck className="h-4 w-4 mr-1" />
            )}
            Approve
          </Button>
        </div>
      )}

      {isExpanded && hasDetails && (
        <div className="border-t border-border/30 bg-background/50 p-3 space-y-2">
          {args && Object.entries(args).map(([key, value]) => {
            const strValue = typeof value === 'string' ? value : JSON.stringify(value, null, 2);
            const isLongValue = strValue.length > 100 || strValue.includes('\n');

            return (
              <div key={key} className="text-xs">
                <span className="font-medium text-muted-foreground">{key}:</span>
                {isLongValue ? (
                  <pre className="mt-1 p-2 bg-muted/50 rounded text-[11px] overflow-x-auto max-h-[200px] overflow-y-auto whitespace-pre-wrap break-all">
                    {strValue}
                  </pre>
                ) : (
                  <span className="ml-2 font-mono text-foreground/80">{strValue}</span>
                )}
              </div>
            );
          })}
          {result && (
            <div className="text-xs border-t border-border/30 pt-2 mt-2">
              <span className="font-medium text-muted-foreground">Result:</span>
              <pre className="mt-1 p-2 bg-muted/50 rounded text-[11px] overflow-x-auto max-h-[150px] overflow-y-auto whitespace-pre-wrap">
                {result}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
