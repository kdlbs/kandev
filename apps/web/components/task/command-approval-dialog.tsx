'use client';

import { useCallback, useMemo, useState } from 'react';
import { IconTerminal2, IconAlertTriangle, IconLoader2 } from '@tabler/icons-react';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@kandev/ui/alert-dialog';
import { useAppStore } from '@/components/state-provider';
import { getWebSocketClient } from '@/lib/ws/connection';

type CommandApprovalDialogProps = {
  taskId: string;
  /** Only show permissions that don't have a matching tool call (standalone permissions like workspace indexing) */
  standaloneOnly?: boolean;
};

export function CommandApprovalDialog({ taskId, standaloneOnly = false }: CommandApprovalDialogProps) {
  const [isResponding, setIsResponding] = useState(false);
  const pendingPermissions = useAppStore((state) => state.permissions.pending);
  const removePendingPermission = useAppStore((state) => state.removePendingPermission);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const messages = useAppStore((state) =>
    activeSessionId ? state.messages.bySession[activeSessionId] ?? [] : []
  );

  // Get tool_call_ids from all tool call messages
  const toolCallIds = useMemo(() => {
    const ids = new Set<string>();
    for (const msg of messages) {
      const tcid = msg.metadata?.tool_call_id;
      if (msg.type === 'tool_call' && typeof tcid === 'string' && tcid) {
        ids.add(tcid);
      }
    }
    return ids;
  }, [messages]);

  // Get the first pending permission for this task
  // If standaloneOnly, filter out permissions that have a matching tool call
  const permission = useMemo(() => {
    return pendingPermissions.find((p) => {
      if (activeSessionId) {
        if (p.session_id !== activeSessionId) return false;
      } else if (p.task_id !== taskId) {
        return false;
      }
      if (standaloneOnly && p.tool_call_id && toolCallIds.has(p.tool_call_id)) {
        return false; // This permission will be handled by inline UI
      }
      return true;
    });
  }, [pendingPermissions, activeSessionId, taskId, standaloneOnly, toolCallIds]);

  const handleRespond = useCallback(
    async (optionId: string, cancelled: boolean = false) => {
      if (!permission) return;
      setIsResponding(true);

      const client = getWebSocketClient();
      if (!client) {
        console.error('WebSocket client not available');
        setIsResponding(false);
        return;
      }

      try {
        await client.request('permission.respond', {
          session_id: permission.session_id,
          pending_id: permission.pending_id,
          option_id: cancelled ? undefined : optionId,
          cancelled,
        });
        removePendingPermission(permission.pending_id);
      } catch (error) {
        console.error('Failed to respond to permission request:', error);
      } finally {
        setIsResponding(false);
      }
    },
    [permission, removePendingPermission]
  );

  const handleApprove = useCallback(() => {
    // Find the "allow" option (allow_once or allow_always)
    const allowOption = permission?.options.find(
      (opt) => opt.kind === 'allow_once' || opt.kind === 'allow_always'
    );
    if (allowOption) {
      handleRespond(allowOption.option_id);
    }
  }, [permission, handleRespond]);

  const handleReject = useCallback(() => {
    // Find the "reject" option (reject_once or reject_always)
    const rejectOption = permission?.options.find(
      (opt) => opt.kind === 'reject_once' || opt.kind === 'reject_always'
    );
    if (rejectOption) {
      handleRespond(rejectOption.option_id);
    } else {
      // If no reject option, cancel the request
      handleRespond('', true);
    }
  }, [permission, handleRespond]);

  if (!permission) {
    return null;
  }

  // Extract command details from action_details
  const actionDetails = permission.action_details || {};
  const command = actionDetails.command as string | undefined;
  const cwd = actionDetails.cwd as string | undefined;
  const reasoning = actionDetails.reasoning as string | undefined;

  return (
    <AlertDialog open={!!permission}>
      <AlertDialogContent className="max-w-lg">
        <AlertDialogHeader>
          <div className="flex items-center gap-2">
            <div className="flex h-10 w-10 items-center justify-center rounded-full bg-amber-500/10">
              <IconAlertTriangle className="h-5 w-5 text-amber-500" />
            </div>
            <AlertDialogTitle className="text-base">
              {permission.title || 'Command Execution Approval'}
            </AlertDialogTitle>
          </div>
          <AlertDialogDescription className="text-left">
            The agent is requesting permission to execute a command.
          </AlertDialogDescription>
        </AlertDialogHeader>

        <div className="space-y-3">
          {command && (
            <div className="rounded-md border bg-muted/50 p-3">
              <div className="mb-1 flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                <IconTerminal2 className="h-3.5 w-3.5" />
                Command
              </div>
              <code className="block whitespace-pre-wrap break-all font-mono text-sm">
                {command}
              </code>
            </div>
          )}

          {cwd && (
            <div className="text-xs text-muted-foreground">
              <span className="font-medium">Working directory:</span> {cwd}
            </div>
          )}

          {reasoning && (
            <div className="text-sm text-muted-foreground">
              <span className="font-medium">Reason:</span> {reasoning}
            </div>
          )}
        </div>

        <AlertDialogFooter>
          <AlertDialogCancel onClick={handleReject} disabled={isResponding}>
            Reject
          </AlertDialogCancel>
          <AlertDialogAction onClick={handleApprove} variant="default" disabled={isResponding}>
            {isResponding ? (
              <>
                <IconLoader2 className="h-4 w-4 mr-1 animate-spin" />
                Approving...
              </>
            ) : (
              'Approve'
            )}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
