'use client';

import { useCallback, useState, useRef, memo } from 'react';
import {
  IconCheck,
  IconCode,
  IconEdit,
  IconEye,
  IconFile,
  IconSearch,
  IconTerminal2,
  IconX,
} from '@tabler/icons-react';
import { GridSpinner } from '@/components/grid-spinner';
import { cn, transformPathsInText } from '@/lib/utils';
import { getWebSocketClient } from '@/lib/ws/connection';
import type { Message } from '@/lib/types/http';
import type { ToolCallMetadata } from '@/components/task/chat/types';
import { PermissionActionRow } from './permission-action-row';
import { ExpandableRow } from './expandable-row';

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

type ToolCallMessageProps = {
  comment: Message;
  permissionMessage?: Message;
  worktreePath?: string;
};

export const ToolCallMessage = memo(function ToolCallMessage({ comment, permissionMessage, worktreePath }: ToolCallMessageProps) {
  const metadata = comment.metadata as ToolCallMetadata | undefined;
  const permissionMetadata = permissionMessage?.metadata as PermissionRequestMetadata | undefined;

  const toolName = metadata?.tool_name ?? '';
  const rawTitle = metadata?.title ?? comment.content ?? 'Tool call';
  const title = transformPathsInText(rawTitle, worktreePath);
  const status = metadata?.status;

  // Get output from normalized payload (generic tools or http_request)
  const normalizedGeneric = metadata?.normalized?.generic;
  const normalizedHttpRequest = metadata?.normalized?.http_request;
  const output = normalizedHttpRequest?.response ?? normalizedGeneric?.output ?? metadata?.result;
  const isHttpError = normalizedHttpRequest?.is_error;

  // Permission state
  const hasPermission = !!permissionMessage;
  const permissionStatus = permissionMetadata?.status;
  const isPermissionPending = hasPermission && permissionStatus !== 'approved' && permissionStatus !== 'rejected';

  // Expand state management
  const [manualExpandState, setManualExpandState] = useState<boolean | null>(null);
  const prevStatusRef = useRef(status);
  const [isResponding, setIsResponding] = useState(false);

  // Reset manual state when status changes
  if (prevStatusRef.current !== status) {
    prevStatusRef.current = status;
    if (manualExpandState !== null) {
      setManualExpandState(null);
    }
  }

  // Auto-expand when running or permission pending
  const autoExpanded = status === 'running' || isPermissionPending;
  const isExpanded = manualExpandState ?? autoExpanded;

  // Determine if we have expandable content
  const hasOutput = output && (typeof output === 'string' ? output.length > 0 : Object.keys(output).length > 0);
  const hasExpandableContent = hasOutput || isPermissionPending;

  const handleToggle = useCallback(() => {
    setManualExpandState((prev) => !(prev ?? autoExpanded));
  }, [autoExpanded]);

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

  const isSuccess = status === 'complete' && !permissionStatus;

  const getStatusIcon = () => {
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
        return <GridSpinner className="text-muted-foreground" />;
      default:
        return null;
    }
  };

  // Format output for display - beautify JSON if detected
  const formatOutput = (value: unknown): { content: string; isJson: boolean } => {
    if (typeof value === 'string') {
      // Try to parse as JSON and beautify
      try {
        const parsed = JSON.parse(value);
        return { content: JSON.stringify(parsed, null, 2), isJson: true };
      } catch {
        // Not valid JSON, return as-is
        return { content: value, isJson: false };
      }
    }
    // Already an object, stringify with formatting
    return { content: JSON.stringify(value, null, 2), isJson: true };
  };

  const formattedOutput = hasOutput ? formatOutput(output) : null;

  return (
    <ExpandableRow
      icon={getToolIcon(toolName, cn(
        'h-4 w-4',
        isPermissionPending ? 'text-amber-600 dark:text-amber-400' : 'text-muted-foreground'
      ))}
      header={
        <div className="flex items-center gap-2 text-xs">
          <span className={cn(
            'inline-flex items-center gap-1.5',
            isPermissionPending && 'text-amber-600 dark:text-amber-400'
          )}>
            <span className="font-mono text-xs text-muted-foreground">{title}</span>
            {!isSuccess && getStatusIcon()}
          </span>
        </div>
      }
      hasExpandableContent={!!hasExpandableContent}
      isExpanded={isExpanded}
      onToggle={handleToggle}
    >
      <div className="pl-4 border-l-2 border-border/30 space-y-2">
        {/* Output display */}
        {formattedOutput && (
          <pre className={cn(
            "text-xs rounded p-2 overflow-x-auto whitespace-pre-wrap max-h-[200px] overflow-y-auto",
            formattedOutput.isJson ? "bg-muted/30 font-mono text-[11px]" : "bg-muted/30",
            isHttpError && "text-red-600 dark:text-red-400 bg-red-50 dark:bg-red-950/30"
          )}>
            {formattedOutput.content}
          </pre>
        )}

        {/* Inline permission approval UI */}
        {isPermissionPending && (
          <PermissionActionRow
            onApprove={handleApprove}
            onReject={handleReject}
            isResponding={isResponding}
          />
        )}
      </div>
    </ExpandableRow>
  );
});
