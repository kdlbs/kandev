'use client';

import { memo, useEffect, useMemo, useState, useCallback } from 'react';
import { IconX, IconChevronDown, IconPlayerPlay, IconPlus } from '@tabler/icons-react';
import { TabsContent } from '@kandev/ui/tabs';
import { Button } from '@kandev/ui/button';
import { SessionPanel, SessionPanelContent } from '@kandev/ui/pannel-session';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@kandev/ui/dropdown-menu';
import { useAppStore } from '@/components/state-provider';
import { useLayoutStore } from '@/lib/state/layout-store';
// closeDocument kept for mobile/tablet backward compat
import { getWebSocketClient } from '@/lib/ws/connection';
import { useTask } from '@/hooks/use-task';
import { SessionTabs, type SessionTab } from '@/components/session-tabs';
import { TaskPlanPanel } from '../task-plan-panel';
import { TaskCreateDialog } from '@/components/task-create-dialog';
import type { ActiveDocument } from '@/lib/state/slices/ui/types';

type DocumentPanelProps = {
  sessionId: string | null;
};

export const DocumentPanel = memo(function DocumentPanel({ sessionId }: DocumentPanelProps) {
  const activeDocument = useAppStore((state) =>
    sessionId ? state.documentPanel.activeDocumentBySessionId[sessionId] ?? null : null
  );
  const setActiveDocument = useAppStore((state) => state.setActiveDocument);
  const closeDocument = useLayoutStore((state) => state.closeDocument);

  const handleClose = () => {
    if (!sessionId) return;
    closeDocument(sessionId);
    setActiveDocument(sessionId, null);
  };

  // Auto-close document layout when no document is active (e.g. stale localStorage)
  useEffect(() => {
    if (!activeDocument && sessionId) {
      closeDocument(sessionId);
    }
  }, [activeDocument, sessionId, closeDocument]);

  const tabs: SessionTab[] = useMemo(() => {
    if (!activeDocument) return [];
    if (activeDocument.type === 'plan') {
      return [{ id: 'plan', label: 'Plan' }];
    }
    return [{ id: 'file', label: activeDocument.name }];
  }, [activeDocument]);

  const activeTab = activeDocument?.type === 'plan' ? 'plan' : 'file';

  if (!activeDocument) {
    return null;
  }

  const isPlan = activeDocument.type === 'plan';

  const headerActions = (
    <div className="flex items-center gap-0.5">
      {isPlan && (
        <ImplementPlanDropdown sessionId={sessionId} onClose={handleClose} />
      )}
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon-sm"
            className="cursor-pointer"
            onClick={handleClose}
          >
            <IconX className="h-3.5 w-3.5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>Close document panel</TooltipContent>
      </Tooltip>
    </div>
  );

  return (
    <SessionPanel borderSide="right" margin="right">
      <SessionTabs
        tabs={tabs}
        activeTab={activeTab}
        onTabChange={() => { }}
        className="flex-1 min-h-0 flex flex-col gap-2"
        rightContent={headerActions}
      >
        <TabsContent value="plan" className="flex-1 min-h-0">
          <DocumentPlanContent />
        </TabsContent>
        <TabsContent value="file" className="flex-1 min-h-0">
          <DocumentFileContent doc={activeDocument} />
        </TabsContent>
      </SessionTabs>
    </SessionPanel>
  );
});

/** Dropdown button for implementing the plan */
const ImplementPlanDropdown = memo(function ImplementPlanDropdown({
  sessionId,
  onClose,
}: {
  sessionId: string | null;
  onClose: () => void;
}) {
  const taskId = useAppStore((state) => state.tasks.activeTaskId);
  const task = useTask(taskId);
  const plan = useAppStore((state) =>
    taskId ? state.taskPlans.byTaskId[taskId] ?? null : null
  );
  const [showNewSessionDialog, setShowNewSessionDialog] = useState(false);

  const handleImplement = useCallback(async () => {
    if (!taskId || !sessionId || !plan?.content) return;
    const client = getWebSocketClient();
    if (!client) return;

    // Close plan mode first
    onClose();

    // Send the plan as an implementation prompt (not in plan mode)
    try {
      await client.request(
        'message.add',
        {
          task_id: taskId,
          session_id: sessionId,
          content: `Implement the following plan:\n\n${plan.content}`,
        },
        10000
      );
    } catch (err) {
      console.error('Failed to send implement plan message:', err);
    }
  }, [taskId, sessionId, plan, onClose]);

  const handleImplementNewSession = useCallback(() => {
    // Close plan mode and open the new session dialog
    onClose();
    setShowNewSessionDialog(true);
  }, [onClose]);

  return (
    <>
      <div className="flex items-center">
        <Button
          variant="ghost"
          size="sm"
          className="h-6 gap-1 px-2 cursor-pointer text-xs hover:bg-muted/40"
          onClick={handleImplement}
          disabled={!plan?.content}
        >
          <IconPlayerPlay className="h-3 w-3" />
          Implement
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              className="h-6 w-5 p-0 cursor-pointer hover:bg-muted/40"
              disabled={!plan?.content}
            >
              <IconChevronDown className="h-3 w-3" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-64">
            <DropdownMenuItem onClick={handleImplementNewSession} className="cursor-pointer">
              <IconPlus className="h-4 w-4 mr-2 shrink-0 self-start mt-0.5" />
              <div>
                <div>Implement in a new session</div>
                <div className="text-[11px] text-muted-foreground font-normal">Starts fresh with a clean context window</div>
              </div>
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      <TaskCreateDialog
        open={showNewSessionDialog}
        onOpenChange={setShowNewSessionDialog}
        mode="session"
        workspaceId={null}
        boardId={null}
        defaultColumnId={null}
        columns={[]}
        taskId={taskId}
        initialValues={{
          title: task?.title ?? '',
          description: plan?.content ? `Implement the following plan:\n\n${plan.content}` : '',
        }}
      />
    </>
  );
});

const DocumentPlanContent = memo(function DocumentPlanContent() {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  return <TaskPlanPanel taskId={activeTaskId} visible />;
});

const DocumentFileContent = memo(function DocumentFileContent({
  doc,
}: {
  doc: ActiveDocument;
}) {
  if (doc.type === 'plan') return null;

  return (
    <SessionPanelContent>
      <div className="flex h-full items-center justify-center text-muted-foreground text-sm">
        File viewer: {doc.path}
      </div>
    </SessionPanelContent>
  );
});
