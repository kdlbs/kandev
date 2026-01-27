'use client';

import { useCallback, useEffect, useState } from 'react';
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
import { cn } from '@/lib/utils';
import { getWebSocketClient } from '@/lib/ws/connection';
import type { Message } from '@/lib/types/http';
import type { ToolCallMetadata } from '@/components/task/chat/types';
import { PermissionActionRow } from './permission-action-row';

type PermissionOption = {
  option_id: string;
  name: string;
  kind: string;
};

type PermissionRequestMetadata = {
  pending_id: string;
  tool_call_id: string;
  options: PermissionOption[];
  action_type: string;
  action_details: {
    command?: string;
    path?: string;
    cwd?: string;
  };
  status?: 'pending' | 'approved' | 'rejected';
};

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

function getStatusIcon(status?: string, permissionStatus?: string) {
  // If there's a permission status, show that instead
  if (permissionStatus === 'approved') {
    return <IconCheck className="h-3.5 w-3.5 text-green-500" />;
  }
  if (permissionStatus === 'rejected') {
    return <IconX className="h-3.5 w-3.5 text-red-500" />;
  }

  switch (status) {
    case 'complete':
      return <IconCheck className="h-3.5 w-3.5 text-green-500" />;
    case 'error':
      return <IconX className="h-3.5 w-3.5 text-red-500" />;
    case 'running':
    case 'in_progress':
      return <IconLoader2 className="h-3.5 w-3.5 text-blue-500 animate-spin" />;
    default:
      return null;
  }
}

type ToolCallMessageProps = {
  comment: Message;
  permissionMessage?: Message;
};

export function ToolCallMessage({ comment, permissionMessage }: ToolCallMessageProps) {
  const metadata = comment.metadata as ToolCallMetadata | undefined;
  const permissionMetadata = permissionMessage?.metadata as PermissionRequestMetadata | undefined;

  const toolName = metadata?.tool_name ?? '';
  const title = metadata?.title ?? comment.content ?? 'Tool call';
  const status = metadata?.status;
  const args = metadata?.args;
  const result = metadata?.result;

  // Permission state
  const hasPermission = !!permissionMessage;
  const permissionStatus = permissionMetadata?.status;
  const isPermissionPending = hasPermission && permissionStatus !== 'approved' && permissionStatus !== 'rejected';

  // Initialize isExpanded based on status - auto-expand if running
  const [isExpanded, setIsExpanded] = useState(() => {
    return status === 'running';
  });
  const [isManuallyToggled, setIsManuallyToggled] = useState(false);
  const [isResponding, setIsResponding] = useState(false);

  // Auto-collapse when complete (unless manually overridden)
  useEffect(() => {
    // Permission requests always stay expanded
    if (isPermissionPending) {
      setIsExpanded(true);
      return;
    }

    // Only auto-collapse if user hasn't manually toggled
    if (!isManuallyToggled) {
      if (status === 'running') {
        setIsExpanded(true);
      } else if (status === 'complete' || status === 'error') {
        setIsExpanded(false);
      }
    }
  }, [status, isManuallyToggled, isPermissionPending]);

  const handleRespond = useCallback(
    async (optionId: string, cancelled: boolean = false) => {
      if (!permissionMetadata || !permissionMessage) return;

      const client = getWebSocketClient();
      if (!client) {
        console.error('WebSocket client not available');
        return;
      }

      setIsResponding(true);
      try {
        await client.request('permission.respond', {
          session_id: permissionMessage.session_id,
          pending_id: permissionMetadata.pending_id,
          option_id: cancelled ? undefined : optionId,
          cancelled,
        });
      } catch (error) {
        console.error('Failed to respond to permission request:', error);
      } finally {
        setIsResponding(false);
      }
    },
    [permissionMessage, permissionMetadata]
  );

  const handleApprove = useCallback(() => {
    const allowOption = permissionMetadata?.options.find(
      (opt) => opt.kind === 'allow_once' || opt.kind === 'allow_always'
    );
    if (allowOption) {
      handleRespond(allowOption.option_id);
    }
  }, [permissionMetadata, handleRespond]);

  const handleReject = useCallback(() => {
    const rejectOption = permissionMetadata?.options.find(
      (opt) => opt.kind === 'reject_once' || opt.kind === 'reject_always'
    );
    if (rejectOption) {
      handleRespond(rejectOption.option_id);
    } else {
      handleRespond('', true);
    }
  }, [permissionMetadata, handleRespond]);

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

  const isSuccess = status === 'complete' && !permissionStatus;

  return (
    <div className="w-full">
      {/* Icon + Summary Row */}
      <div className="flex items-start gap-3 w-full">
        {/* Icon */}
        <div className="flex-shrink-0 mt-0.5">
          {getToolIcon(toolName, cn(
            'h-4 w-4',
            isPermissionPending ? 'text-amber-600 dark:text-amber-400' : 'text-muted-foreground'
          ))}
        </div>

        {/* Content */}
        <div className="flex-1 min-w-0 pt-0.5">
          <div className="flex items-center gap-2 text-xs">
            <button
              type="button"
              onClick={() => {
                if (hasDetails) {
                  setIsExpanded(!isExpanded);
                  setIsManuallyToggled(true);
                }
              }}
              className={cn(
                'inline-flex items-center gap-1.5 text-left',
                hasDetails && 'cursor-pointer hover:opacity-70 transition-opacity',
                isPermissionPending && 'text-amber-600 dark:text-amber-400'
              )}
              disabled={!hasDetails}
            >
              <span className="font-mono text-xs">
                {title}
              </span>
              {/* Status indicator - only show if not success */}
              {!isSuccess && getStatusIcon(status, permissionStatus)}
              {hasDetails && (
                isExpanded
                  ? <IconChevronDown className="h-3.5 w-3.5 text-muted-foreground/50 flex-shrink-0" />
                  : <IconChevronRight className="h-3.5 w-3.5 text-muted-foreground/50 flex-shrink-0" />
              )}
            </button>
            {filePath && (
              <span className="text-xs text-muted-foreground/60 truncate">
                {filePath}
              </span>
            )}
          </div>

          {/* Expanded Details */}
          {isExpanded && hasDetails && (
            <div className="mt-2 pl-4 border-l-2 border-border/30 space-y-2">
              {args && Object.entries(args).map(([key, value]) => {
                const strValue = typeof value === 'string' ? value : JSON.stringify(value, null, 2);
                const isLongValue = strValue.length > 100 || strValue.includes('\n');

                return (
                  <div key={key} className="text-xs">
                    <span className="font-medium text-muted-foreground">{key}:</span>
                    {isLongValue ? (
                      <pre className="mt-1 p-2 bg-muted/30 rounded text-[11px] overflow-x-auto max-h-[200px] overflow-y-auto whitespace-pre-wrap break-all">
                        {strValue}
                      </pre>
                    ) : (
                      <span className="ml-2 font-mono text-foreground/80">{strValue}</span>
                    )}
                  </div>
                );
              })}
              {result && (
                <div className="text-xs pt-2 mt-2 border-t border-border/30">
                  <span className="font-medium text-muted-foreground">Result:</span>
                  <pre className="mt-1 p-2 bg-muted/30 rounded text-[11px] overflow-x-auto max-h-[150px] overflow-y-auto whitespace-pre-wrap">
                    {result}
                  </pre>
                </div>
              )}
            </div>
          )}

          {/* Inline permission approval UI */}
          {isPermissionPending && (
            <div className="mt-2">
              <PermissionActionRow
                onApprove={handleApprove}
                onReject={handleReject}
                isResponding={isResponding}
              />
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
