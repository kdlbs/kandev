'use client';

import { useCallback, useState } from 'react';
import { IconAlertTriangle, IconCheck, IconX, IconLoader2 } from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import { cn } from '@/lib/utils';
import { getWebSocketClient } from '@/lib/ws/connection';
import type { Message } from '@/lib/types/http';

type PermissionOption = {
  option_id: string;
  name: string;
  kind: string;
};

type PermissionRequestMetadata = {
  pending_id: string;
  tool_call_id?: string;
  options: PermissionOption[];
  action_type: string;
  action_details?: {
    command?: string;
    path?: string;
    cwd?: string;
  };
  status?: 'pending' | 'approved' | 'rejected';
};

type PermissionRequestMessageProps = {
  comment: Message;
};

export function PermissionRequestMessage({ comment }: PermissionRequestMessageProps) {
  const [isResponding, setIsResponding] = useState(false);
  const metadata = comment.metadata as PermissionRequestMetadata | undefined;

  const isResolved = metadata?.status === 'approved' || metadata?.status === 'rejected';
  const isPending = !isResolved;

  const handleRespond = useCallback(
    async (optionId: string, cancelled: boolean = false) => {
      if (!metadata) return;

      const client = getWebSocketClient();
      if (!client) {
        console.error('WebSocket client not available');
        return;
      }

      setIsResponding(true);
      try {
        await client.request('permission.respond', {
          session_id: comment.session_id,
          pending_id: metadata.pending_id,
          option_id: cancelled ? undefined : optionId,
          cancelled,
        });
      } catch (error) {
        console.error('Failed to respond to permission request:', error);
      } finally {
        setIsResponding(false);
      }
    },
    [comment.session_id, metadata]
  );

  const handleApprove = useCallback(() => {
    const allowOption = metadata?.options.find(
      (opt) => opt.kind === 'allow_once' || opt.kind === 'allow_always'
    );
    if (allowOption) {
      handleRespond(allowOption.option_id);
    }
  }, [metadata, handleRespond]);

  const handleReject = useCallback(() => {
    const rejectOption = metadata?.options.find(
      (opt) => opt.kind === 'reject_once' || opt.kind === 'reject_always'
    );
    if (rejectOption) {
      handleRespond(rejectOption.option_id);
    } else {
      handleRespond('', true);
    }
  }, [metadata, handleRespond]);

  const getStatusBadge = () => {
    if (metadata?.status === 'approved') {
      return (
        <span className="inline-flex items-center gap-1 text-xs text-green-600 dark:text-green-400">
          <IconCheck className="h-3 w-3" /> Approved
        </span>
      );
    }
    if (metadata?.status === 'rejected') {
      return (
        <span className="inline-flex items-center gap-1 text-xs text-red-600 dark:text-red-400">
          <IconX className="h-3 w-3" /> Rejected
        </span>
      );
    }
    return (
      <span className="inline-flex items-center gap-1 text-xs text-amber-600 dark:text-amber-400">
        <IconLoader2 className="h-3 w-3 animate-spin" /> Pending Approval
      </span>
    );
  };

  return (
    <div
      className={cn(
        'w-full rounded-md border overflow-hidden',
        isPending ? 'border-amber-500/50 bg-amber-500/5' : 'border-border/50 bg-muted/20'
      )}
    >
      {/* Header */}
      <div className="flex items-center gap-2 px-3 py-2 text-sm">
        <IconAlertTriangle
          className={cn(
            'h-4 w-4 flex-shrink-0',
            isPending ? 'text-amber-600 dark:text-amber-400' : 'text-muted-foreground'
          )}
        />
        <span className="flex-1 font-medium text-foreground truncate">
          {comment.content || 'Permission Required'}
        </span>
        {getStatusBadge()}
      </div>

      {/* Approval buttons - only show when pending */}
      {isPending && (
        <div className="flex items-center gap-2 px-3 py-2 border-t border-amber-500/30 bg-amber-500/10">
          <span className="text-xs text-amber-600 dark:text-amber-400 flex-1">
            Approve this action?
          </span>
          <Button
            size="sm"
            variant="outline"
            onClick={handleReject}
            disabled={isResponding}
            className="h-7 px-3 text-red-600 border-red-500/40 bg-red-500/10 hover:bg-red-500/20 hover:text-red-700 hover:border-red-500/60"
          >
            <IconX className="h-4 w-4 mr-1" />
            Deny
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={handleApprove}
            disabled={isResponding}
            className="h-7 px-3 text-green-600 border-green-500/40 bg-green-500/10 hover:bg-green-500/20 hover:text-green-700 hover:border-green-500/60"
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
    </div>
  );
}
