'use client';

import { useEffect, useState } from 'react';
import { IconDownload, IconTrash } from '@tabler/icons-react';
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@kandev/ui/select';
import type { Workflow, WorkflowStep } from '@/lib/types/http';
import { useRequest } from '@/lib/http/use-request';
import { useToast } from '@/components/toast-provider';
import { WorkflowExportDialog } from '@/components/settings/workflow-export-dialog';
import { UnsavedChangesBadge, UnsavedSaveButton } from '@/components/settings/unsaved-indicator';
import { WorkflowPipelineEditor } from '@/components/settings/workflow-pipeline-editor';
import { generateUUID } from '@/lib/utils';
import {
  createWorkflowAction,
  listWorkflowStepsAction,
  createWorkflowStepAction,
  updateWorkflowStepAction,
  deleteWorkflowStepAction,
  reorderWorkflowStepsAction,
  getWorkflowTaskCount,
  getStepTaskCount,
  bulkMoveTasks,
  exportWorkflowAction,
} from '@/app/actions/workspaces';

type WorkflowCardProps = {
  workflow: Workflow;
  isWorkflowDirty: boolean;
  initialWorkflowSteps?: WorkflowStep[];
  otherWorkflows?: Workflow[];
  onUpdateWorkflow: (updates: { name?: string; description?: string }) => void;
  onDeleteWorkflow: () => Promise<unknown>;
  onSaveWorkflow: () => Promise<unknown>;
  onWorkflowCreated?: (created: Workflow) => void;
};

export function WorkflowCard({
  workflow,
  isWorkflowDirty,
  initialWorkflowSteps,
  otherWorkflows = [],
  onUpdateWorkflow,
  onDeleteWorkflow,
  onSaveWorkflow,
  onWorkflowCreated,
}: WorkflowCardProps) {
  const { toast } = useToast();
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [exportOpen, setExportOpen] = useState(false);
  const [exportJson, setExportJson] = useState('');

  // Workflow deletion migration state
  const [workflowTaskCount, setWorkflowTaskCount] = useState<number | null>(null);
  const [workflowDeleteLoading, setWorkflowDeleteLoading] = useState(false);
  const [targetWorkflowId, setTargetWorkflowId] = useState<string>('');
  const [targetWorkflowSteps, setTargetWorkflowSteps] = useState<WorkflowStep[]>([]);
  const [targetStepId, setTargetStepId] = useState<string>('');
  const [migrateLoading, setMigrateLoading] = useState(false);

  // Step deletion migration state
  const [stepDeleteOpen, setStepDeleteOpen] = useState(false);
  const [stepToDelete, setStepToDelete] = useState<string | null>(null);
  const [stepTaskCount, setStepTaskCount] = useState<number | null>(null);
  const [targetStepForMigration, setTargetStepForMigration] = useState<string>('');
  const [stepMigrateLoading, setStepMigrateLoading] = useState(false);

  const saveWorkflowRequest = useRequest(onSaveWorkflow);
  const deleteWorkflowRequest = useRequest(onDeleteWorkflow);

  // Workflow state
  const [workflowSteps, setWorkflowSteps] = useState<WorkflowStep[]>(initialWorkflowSteps ?? []);
  const [workflowLoading, setWorkflowLoading] = useState(false);

  const isNewWorkflow = workflow.id.startsWith('temp-');

  // Load workflow steps on mount (only for saved workflows)
  useEffect(() => {
    if (isNewWorkflow) {
      setWorkflowSteps(initialWorkflowSteps ?? []);
      setWorkflowLoading(false);
      return;
    }

    let cancelled = false;
    const load = async () => {
      setWorkflowLoading(true);
      try {
        const res = await listWorkflowStepsAction(workflow.id);
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
  }, [workflow.id, initialWorkflowSteps, isNewWorkflow, toast]);

  // Load target workflow steps when target workflow changes
  useEffect(() => {
    if (!targetWorkflowId) {
      setTargetWorkflowSteps([]);
      setTargetStepId('');
      return;
    }
    let cancelled = false;
    listWorkflowStepsAction(targetWorkflowId).then((res) => {
      if (!cancelled) {
        const steps = res.steps ?? [];
        setTargetWorkflowSteps(steps);
        setTargetStepId(steps.length > 0 ? steps[0].id : '');
      }
    }).catch(() => {
      if (!cancelled) setTargetWorkflowSteps([]);
    });
    return () => { cancelled = true; };
  }, [targetWorkflowId]);

  const refreshWorkflowSteps = async () => {
    try {
      const res = await listWorkflowStepsAction(workflow.id);
      setWorkflowSteps(res.steps ?? []);
    } catch {
      // Ignore errors on refresh
    }
  };

  const handleUpdateWorkflowStep = async (stepId: string, updates: Partial<WorkflowStep>) => {
    if (isNewWorkflow) {
      setWorkflowSteps((prev) =>
        prev.map((s) => (s.id === stepId ? { ...s, ...updates } : s))
      );
      return;
    }
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
    if (isNewWorkflow) {
      setWorkflowSteps((prev) => [
        ...prev,
        {
          id: `temp-step-${generateUUID()}`,
          workflow_id: workflow.id,
          name: 'New Step',
          position: prev.length,
          color: 'bg-slate-500',
          allow_manual_move: true,
          created_at: '',
          updated_at: '',
        },
      ]);
      return;
    }
    try {
      await createWorkflowStepAction({
        workflow_id: workflow.id,
        name: 'New Step',
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
    if (isNewWorkflow) {
      setWorkflowSteps((prev) =>
        prev.filter((s) => s.id !== stepId).map((s, i) => ({ ...s, position: i }))
      );
      return;
    }
    try {
      const { task_count } = await getStepTaskCount(stepId);
      if (task_count === 0) {
        await deleteWorkflowStepAction(stepId);
        await refreshWorkflowSteps();
        return;
      }
      // Has tasks — show migration dialog
      setStepToDelete(stepId);
      setStepTaskCount(task_count);
      const otherSteps = workflowSteps.filter((s) => s.id !== stepId);
      setTargetStepForMigration(otherSteps.length > 0 ? otherSteps[0].id : '');
      setStepDeleteOpen(true);
    } catch (error) {
      toast({
        title: 'Failed to check step tasks',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
    }
  };

  const handleReorderWorkflowSteps = async (reorderedSteps: WorkflowStep[]) => {
    if (isNewWorkflow) {
      setWorkflowSteps(reorderedSteps);
      return;
    }
    // Optimistically update the UI
    setWorkflowSteps(reorderedSteps);
    try {
      const stepIds = reorderedSteps.map((step) => step.id);
      await reorderWorkflowStepsAction(workflow.id, stepIds);
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

  // Workflow deletion: check task count, then show appropriate dialog
  const handleDeleteWorkflowClick = async () => {
    if (isNewWorkflow) {
      setWorkflowTaskCount(0);
      setDeleteOpen(true);
      return;
    }
    setWorkflowDeleteLoading(true);
    try {
      const { task_count } = await getWorkflowTaskCount(workflow.id);
      setWorkflowTaskCount(task_count);
      if (task_count > 0 && otherWorkflows.length > 0) {
        setTargetWorkflowId(otherWorkflows[0].id);
      }
      setDeleteOpen(true);
    } catch (error) {
      toast({
        title: 'Failed to check workflow tasks',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
    } finally {
      setWorkflowDeleteLoading(false);
    }
  };

  const handleDeleteWorkflow = async () => {
    try {
      await deleteWorkflowRequest.run();
      setDeleteOpen(false);
    } catch (error) {
      toast({
        title: 'Failed to delete workflow',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
    }
  };

  const handleMigrateAndDeleteWorkflow = async () => {
    if (!targetWorkflowId || !targetStepId) return;
    setMigrateLoading(true);
    try {
      await bulkMoveTasks({
        source_workflow_id: workflow.id,
        target_workflow_id: targetWorkflowId,
        target_step_id: targetStepId,
      });
      await deleteWorkflowRequest.run();
      setDeleteOpen(false);
    } catch (error) {
      toast({
        title: 'Failed to migrate tasks',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
    } finally {
      setMigrateLoading(false);
    }
  };

  const handleMigrateAndDeleteStep = async () => {
    if (!stepToDelete || !targetStepForMigration) return;
    setStepMigrateLoading(true);
    try {
      await bulkMoveTasks({
        source_workflow_id: workflow.id,
        source_step_id: stepToDelete,
        target_workflow_id: workflow.id,
        target_step_id: targetStepForMigration,
      });
      await deleteWorkflowStepAction(stepToDelete);
      await refreshWorkflowSteps();
      setStepDeleteOpen(false);
      setStepToDelete(null);
    } catch (error) {
      toast({
        title: 'Failed to migrate tasks',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
    } finally {
      setStepMigrateLoading(false);
    }
  };

  const handleDeleteStepAndTasks = async () => {
    if (!stepToDelete) return;
    setStepMigrateLoading(true);
    try {
      await deleteWorkflowStepAction(stepToDelete);
      await refreshWorkflowSteps();
      setStepDeleteOpen(false);
      setStepToDelete(null);
    } catch (error) {
      toast({
        title: 'Failed to delete step',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
    } finally {
      setStepMigrateLoading(false);
    }
  };

  const handleSaveNewWorkflow = async () => {
    const name = workflow.name.trim() || 'New Workflow';
    const created = await createWorkflowAction({
      workspace_id: workflow.workspace_id,
      name,
    });
    // Create each step with the user's customizations
    for (const step of workflowSteps) {
      await createWorkflowStepAction({
        workflow_id: created.id,
        name: step.name,
        position: step.position,
        color: step.color,
        prompt: step.prompt,
        events: step.events,
        is_start_step: step.is_start_step,
        allow_manual_move: step.allow_manual_move,
      });
    }
    onWorkflowCreated?.(created);
  };

  const saveNewWorkflowRequest = useRequest(handleSaveNewWorkflow);

  const handleSaveWorkflow = async () => {
    try {
      if (isNewWorkflow) {
        await saveNewWorkflowRequest.run();
      } else {
        await saveWorkflowRequest.run();
      }
    } catch (error) {
      toast({
        title: 'Failed to save workflow changes',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
    }
  };

  const handleExportWorkflow = async () => {
    try {
      const data = await exportWorkflowAction(workflow.id);
      setExportJson(JSON.stringify(data, null, 2));
      setExportOpen(true);
    } catch (error) {
      toast({
        title: 'Failed to export workflow',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
    }
  };

  const activeSaveRequest = isNewWorkflow ? saveNewWorkflowRequest : saveWorkflowRequest;

  const hasTasks = workflowTaskCount !== null && workflowTaskCount > 0;
  const stepsForStepMigration = stepToDelete
    ? workflowSteps.filter((s) => s.id !== stepToDelete)
    : [];

  return (
    <Card>
      <CardContent className="pt-6">
        <div className="space-y-4">
          <div className="flex items-center justify-between gap-3">
            <div className="space-y-2 flex-1">
              <Label className="flex items-center gap-2">
                <span>Workflow Name</span>
                {isWorkflowDirty && <UnsavedChangesBadge />}
              </Label>
              <div className="flex items-center gap-2">
                <Input
                  value={workflow.name}
                  onChange={(e) => onUpdateWorkflow({ name: e.target.value })}
                />
                <UnsavedSaveButton
                  isDirty={isWorkflowDirty}
                  isLoading={activeSaveRequest.isLoading}
                  status={activeSaveRequest.status}
                  onClick={handleSaveWorkflow}
                />
              </div>
            </div>
          </div>

          {/* Workflow Steps Section */}
          <div className="space-y-2">
            <Label>Workflow Steps</Label>
            {workflowLoading ? (
              <div className="text-sm text-muted-foreground">Loading workflow steps...</div>
            ) : (
              <WorkflowPipelineEditor
                steps={workflowSteps}
                onUpdateStep={handleUpdateWorkflowStep}
                onAddStep={handleAddWorkflowStep}
                onRemoveStep={handleRemoveWorkflowStep}
                onReorderSteps={handleReorderWorkflowSteps}
              />
            )}
          </div>

          <div className="flex justify-end gap-2">
            {!isNewWorkflow && (
              <Button
                type="button"
                variant="outline"
                onClick={handleExportWorkflow}
                className="cursor-pointer"
              >
                <IconDownload className="h-4 w-4 mr-2" />
                Export
              </Button>
            )}
            <Button
              type="button"
              variant="destructive"
              onClick={handleDeleteWorkflowClick}
              disabled={deleteWorkflowRequest.isLoading || workflowDeleteLoading}
              className="cursor-pointer"
            >
              <IconTrash className="h-4 w-4 mr-2" />
              Delete Workflow
            </Button>
          </div>
        </div>
      </CardContent>

      {/* Workflow Delete Dialog */}
      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete workflow</DialogTitle>
            <DialogDescription>
              {hasTasks
                ? `This workflow has ${workflowTaskCount} task${workflowTaskCount === 1 ? '' : 's'}. Choose where to migrate them, or delete everything.`
                : 'This will permanently delete the workflow and all its steps.'}
            </DialogDescription>
          </DialogHeader>

          {hasTasks && otherWorkflows.length > 0 && (
            <div className="space-y-3 py-2">
              <div className="space-y-2">
                <Label>Target Workflow</Label>
                <Select value={targetWorkflowId} onValueChange={setTargetWorkflowId}>
                  <SelectTrigger>
                    <SelectValue placeholder="Select workflow" />
                  </SelectTrigger>
                  <SelectContent>
                    {otherWorkflows.map((w) => (
                      <SelectItem key={w.id} value={w.id}>
                        {w.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              {targetWorkflowSteps.length > 0 && (
                <div className="space-y-2">
                  <Label>Target Step</Label>
                  <Select value={targetStepId} onValueChange={setTargetStepId}>
                    <SelectTrigger>
                      <SelectValue placeholder="Select step" />
                    </SelectTrigger>
                    <SelectContent>
                      {targetWorkflowSteps.map((s) => (
                        <SelectItem key={s.id} value={s.id}>
                          {s.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              )}
            </div>
          )}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setDeleteOpen(false)} className="cursor-pointer">
              Cancel
            </Button>
            {hasTasks && otherWorkflows.length > 0 && (
              <Button
                type="button"
                onClick={handleMigrateAndDeleteWorkflow}
                disabled={!targetWorkflowId || !targetStepId || migrateLoading || deleteWorkflowRequest.isLoading}
                className="cursor-pointer"
              >
                {migrateLoading ? 'Migrating...' : 'Migrate & Delete'}
              </Button>
            )}
            <Button
              type="button"
              variant="destructive"
              onClick={handleDeleteWorkflow}
              disabled={deleteWorkflowRequest.isLoading || migrateLoading}
              className="cursor-pointer"
            >
              {hasTasks ? 'Delete Everything' : 'Delete Workflow'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <WorkflowExportDialog
        open={exportOpen}
        onOpenChange={setExportOpen}
        title="Export Workflow"
        json={exportJson}
      />

      {/* Step Delete Migration Dialog */}
      <Dialog open={stepDeleteOpen} onOpenChange={setStepDeleteOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete step</DialogTitle>
            <DialogDescription>
              This step has {stepTaskCount} task{stepTaskCount === 1 ? '' : 's'}.
              {stepsForStepMigration.length > 0
                ? ' Choose where to migrate them, or delete the step and its tasks.'
                : ' Deleting this step will affect these tasks.'}
            </DialogDescription>
          </DialogHeader>

          {stepsForStepMigration.length > 0 && (
            <div className="space-y-2 py-2">
              <Label>Target Step</Label>
              <Select value={targetStepForMigration} onValueChange={setTargetStepForMigration}>
                <SelectTrigger>
                  <SelectValue placeholder="Select step" />
                </SelectTrigger>
                <SelectContent>
                  {stepsForStepMigration.map((s) => (
                    <SelectItem key={s.id} value={s.id}>
                      {s.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setStepDeleteOpen(false)} className="cursor-pointer">
              Cancel
            </Button>
            {stepsForStepMigration.length > 0 && (
              <Button
                type="button"
                onClick={handleMigrateAndDeleteStep}
                disabled={!targetStepForMigration || stepMigrateLoading}
                className="cursor-pointer"
              >
                {stepMigrateLoading ? 'Migrating...' : 'Migrate & Delete Step'}
              </Button>
            )}
            <Button
              type="button"
              variant="destructive"
              onClick={handleDeleteStepAndTasks}
              disabled={stepMigrateLoading}
              className="cursor-pointer"
            >
              Delete Step & Tasks
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  );
}
