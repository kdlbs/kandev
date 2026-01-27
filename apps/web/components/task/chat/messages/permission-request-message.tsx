'use client';

import { useCallback, useState } from 'react';
import { IconAlertTriangle, IconCheck, IconX } from '@tabler/icons-react';
import { cn } from '@/lib/utils';
import { getWebSocketClient } from '@/lib/ws/connection';
import type { Message } from '@/lib/types/http';
import { PermissionActionRow } from './permission-action-row';

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
        Pending Approval
      </span>
    );
  };

  return (
    <div className="w-full">
      {/* Icon + Content Row */}
      <div className="flex items-start gap-3 w-full">
        {/* Icon */}
        <div className="flex-shrink-0 mt-0.5">
          <IconAlertTriangle
            className={cn(
              'h-4 w-4',
              isPending ? 'text-amber-600 dark:text-amber-400' : 'text-muted-foreground'
            )}
          />
        </div>

        {/* Content */}
        <div className="flex-1 min-w-0 pt-0.5">
          <div className="flex items-center gap-2 text-xs">
            <span className={cn(
              'font-mono text-xs',
              isPending ? 'text-amber-600 dark:text-amber-400' : 'text-muted-foreground'
            )}>
              {comment.content || 'Permission Required'}
            </span>
            {getStatusBadge()}
          </div>

          {/* Approval buttons - only show when pending */}
          {isPending && (
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
