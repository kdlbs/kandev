'use client';

import { useEffect, useState } from 'react';
import { IconTrash } from '@tabler/icons-react';
import { Card, CardContent } from '@kandev/ui/card';
import { Button } from '@kandev/ui/button';
import { Input } from '@kandev/ui/input';
import { Label } from '@kandev/ui/label';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@kandev/ui/dialog';
import type { Board, WorkflowStep } from '@/lib/types/http';
import { useRequest } from '@/lib/http/use-request';
import { useToast } from '@/components/toast-provider';
import { UnsavedChangesBadge, UnsavedSaveButton } from '@/components/settings/unsaved-indicator';
import { WorkflowStepEditor } from '@/components/settings/workflow-step-editor';
import {
  listWorkflowStepsAction,
  createWorkflowStepAction,
  updateWorkflowStepAction,
  deleteWorkflowStepAction,
  reorderWorkflowStepsAction,
} from '@/app/actions/workspaces';

type BoardCardProps = {
  board: Board;
  isBoardDirty: boolean;
  onUpdateBoard: (updates: { name?: string; description?: string }) => void;
  onDeleteBoard: () => Promise<unknown>;
  onSaveBoard: () => Promise<unknown>;
};

export function BoardCard({
  board,
  isBoardDirty,
  onUpdateBoard,
  onDeleteBoard,
  onSaveBoard,
}: BoardCardProps) {
  const { toast } = useToast();
  const [deleteOpen, setDeleteOpen] = useState(false);

  const saveBoardRequest = useRequest(onSaveBoard);
  const deleteBoardRequest = useRequest(onDeleteBoard);

  // Workflow state
  const [workflowSteps, setWorkflowSteps] = useState<WorkflowStep[]>([]);
  const [workflowLoading, setWorkflowLoading] = useState(false);

  const isNewBoard = board.id.startsWith('temp-');

  // Load workflow steps on mount (only for saved boards)
  useEffect(() => {
    if (isNewBoard) return;

    let cancelled = false;
    const load = async () => {
      setWorkflowLoading(true);
      try {
        const res = await listWorkflowStepsAction(board.id);
        if (!cancelled) {
          setWorkflowSteps(res.steps ?? []);
        }
      } catch {
        if (!cancelled) {
          toast({
            title: 'Failed to load workflow steps',
            variant: 'error',
          });
        }
      } finally {
        if (!cancelled) {
          setWorkflowLoading(false);
        }
      }
    };
    load();
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [board.id, isNewBoard]);

  const refreshWorkflowSteps = async () => {
    try {
      const res = await listWorkflowStepsAction(board.id);
      setWorkflowSteps(res.steps ?? []);
    } catch {
      // Ignore errors on refresh
    }
  };

  const handleUpdateWorkflowStep = async (stepId: string, updates: Partial<WorkflowStep>) => {
    try {
      await updateWorkflowStepAction(stepId, updates);
      await refreshWorkflowSteps();
    } catch (error) {
      toast({
        title: 'Failed to update workflow step',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
    }
  };

  const handleAddWorkflowStep = async () => {
    try {
      await createWorkflowStepAction({
        board_id: board.id,
        name: 'New Step',
        step_type: 'implementation',
        position: workflowSteps.length,
        color: 'bg-slate-500',
      });
      await refreshWorkflowSteps();
    } catch (error) {
      toast({
        title: 'Failed to add workflow step',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
    }
  };

  const handleRemoveWorkflowStep = async (stepId: string) => {
    try {
      await deleteWorkflowStepAction(stepId);
      await refreshWorkflowSteps();
    } catch (error) {
      toast({
        title: 'Failed to delete workflow step',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
    }
  };

  const handleReorderWorkflowSteps = async (reorderedSteps: WorkflowStep[]) => {
    // Optimistically update the UI
    setWorkflowSteps(reorderedSteps);
    try {
      const stepIds = reorderedSteps.map((step) => step.id);
      await reorderWorkflowStepsAction(board.id, stepIds);
    } catch (error) {
      toast({
        title: 'Failed to reorder workflow steps',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
      // Refresh to get the actual order
      await refreshWorkflowSteps();
    }
  };

  const handleDeleteBoard = async () => {
    try {
      await deleteBoardRequest.run();
      setDeleteOpen(false);
    } catch (error) {
      toast({
        title: 'Failed to delete board',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
    }
  };

  const handleSaveBoard = async () => {
    try {
      await saveBoardRequest.run();
    } catch (error) {
      toast({
        title: 'Failed to save board changes',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
    }
  };

  return (
    <Card>
      <CardContent className="pt-6">
        <div className="space-y-4">
          <div className="flex items-center justify-between gap-3">
            <div className="space-y-2 flex-1">
              <Label className="flex items-center gap-2">
                <span>Board Name</span>
                {isBoardDirty && <UnsavedChangesBadge />}
              </Label>
              <div className="flex items-center gap-2">
                <Input
                  value={board.name}
                  onChange={(e) => onUpdateBoard({ name: e.target.value })}
                />
                <UnsavedSaveButton
                  isDirty={isBoardDirty}
                  isLoading={saveBoardRequest.isLoading}
                  status={saveBoardRequest.status}
                  onClick={handleSaveBoard}
                />
              </div>
            </div>
          </div>

          {/* Workflow Steps Section - only show for saved boards */}
          {!isNewBoard && (
            <div className="space-y-2">
              <Label>Workflow Steps</Label>
              {workflowLoading ? (
                <div className="text-sm text-muted-foreground">Loading workflow steps...</div>
              ) : (
                <WorkflowStepEditor
                  steps={workflowSteps}
                  onUpdateStep={handleUpdateWorkflowStep}
                  onAddStep={handleAddWorkflowStep}
                  onRemoveStep={handleRemoveWorkflowStep}
                  onReorderSteps={handleReorderWorkflowSteps}
                />
              )}
            </div>
          )}

          <div className="flex justify-end">
            <Button
              type="button"
              variant="destructive"
              onClick={() => setDeleteOpen(true)}
              disabled={deleteBoardRequest.isLoading}
            >
              <IconTrash className="h-4 w-4 mr-2" />
              Delete Board
            </Button>
          </div>
        </div>
      </CardContent>
      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete board</DialogTitle>
            <DialogDescription>
              This will remove the board and its workflow steps. Tasks will not be deleted. This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setDeleteOpen(false)}>
              Cancel
            </Button>
            <Button type="button" variant="destructive" onClick={handleDeleteBoard}>
              Delete Board
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  );
}
