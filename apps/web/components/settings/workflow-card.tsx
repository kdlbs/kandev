'use client';

import { useEffect, useState } from 'react';
import { IconDownload, IconTrash } from '@tabler/icons-react';
import { Card, CardContent } from '@kandev/ui/card';
import { Button } from '@kandev/ui/button';
import { Input } from '@kandev/ui/input';
import { Label } from '@kandev/ui/label';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@kandev/ui/dialog';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@kandev/ui/select';
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

const FALLBACK_ERROR_MESSAGE = 'Request failed';

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

function useWorkflowSteps(workflowId: string, initialSteps: WorkflowStep[] | undefined, isNewWorkflow: boolean, toast: ReturnType<typeof useToast>['toast']) {
  const [workflowSteps, setWorkflowSteps] = useState<WorkflowStep[]>(initialSteps ?? []);
  const [workflowLoading, setWorkflowLoading] = useState(false);

  useEffect(() => {
    if (isNewWorkflow) { setWorkflowSteps(initialSteps ?? []); setWorkflowLoading(false); return; }
    let cancelled = false;
    const load = async () => {
      setWorkflowLoading(true);
      try {
        const res = await listWorkflowStepsAction(workflowId);
        if (!cancelled) setWorkflowSteps(res.steps ?? []);
      } catch {
        if (!cancelled) toast({ title: 'Failed to load workflow steps', variant: 'error' });
      } finally {
        if (!cancelled) setWorkflowLoading(false);
      }
    };
    load();
    return () => { cancelled = true; };
  }, [workflowId, initialSteps, isNewWorkflow, toast]);

  const refreshWorkflowSteps = async () => {
    try { const res = await listWorkflowStepsAction(workflowId); setWorkflowSteps(res.steps ?? []); } catch { /* ignore */ }
  };

  return { workflowSteps, setWorkflowSteps, workflowLoading, refreshWorkflowSteps };
}

type WorkflowStepActionsParams = {
  workflow: Workflow;
  isNewWorkflow: boolean;
  workflowSteps: WorkflowStep[];
  setWorkflowSteps: (updater: ((prev: WorkflowStep[]) => WorkflowStep[]) | WorkflowStep[]) => void;
  refreshWorkflowSteps: () => Promise<void>;
  setStepToDelete: (id: string | null) => void;
  setStepTaskCount: (count: number | null) => void;
  setTargetStepForMigration: (id: string) => void;
  setStepDeleteOpen: (open: boolean) => void;
  toast: ReturnType<typeof useToast>['toast'];
};

function useWorkflowStepActions({ workflow, isNewWorkflow, workflowSteps, setWorkflowSteps, refreshWorkflowSteps, setStepToDelete, setStepTaskCount, setTargetStepForMigration, setStepDeleteOpen, toast }: WorkflowStepActionsParams) {
  const handleUpdateWorkflowStep = async (stepId: string, updates: Partial<WorkflowStep>) => {
    if (isNewWorkflow) { setWorkflowSteps((prev) => prev.map((s) => (s.id === stepId ? { ...s, ...updates } : s))); return; }
    try { await updateWorkflowStepAction(stepId, updates); await refreshWorkflowSteps(); }
    catch (error) { toast({ title: 'Failed to update workflow step', description: error instanceof Error ? error.message : FALLBACK_ERROR_MESSAGE, variant: 'error' }); }
  };

  const handleAddWorkflowStep = async () => {
    if (isNewWorkflow) {
      setWorkflowSteps((prev) => [...prev, { id: `temp-step-${generateUUID()}`, workflow_id: workflow.id, name: 'New Step', position: prev.length, color: 'bg-slate-500', allow_manual_move: true, created_at: '', updated_at: '' }]);
      return;
    }
    try { await createWorkflowStepAction({ workflow_id: workflow.id, name: 'New Step', position: workflowSteps.length, color: 'bg-slate-500' }); await refreshWorkflowSteps(); }
    catch (error) { toast({ title: 'Failed to add workflow step', description: error instanceof Error ? error.message : FALLBACK_ERROR_MESSAGE, variant: 'error' }); }
  };

  const handleRemoveWorkflowStep = async (stepId: string) => {
    if (isNewWorkflow) { setWorkflowSteps((prev) => prev.filter((s) => s.id !== stepId).map((s, i) => ({ ...s, position: i }))); return; }
    try {
      const { task_count } = await getStepTaskCount(stepId);
      if (task_count === 0) { await deleteWorkflowStepAction(stepId); await refreshWorkflowSteps(); return; }
      setStepToDelete(stepId);
      setStepTaskCount(task_count);
      const otherSteps = workflowSteps.filter((s) => s.id !== stepId);
      setTargetStepForMigration(otherSteps.length > 0 ? otherSteps[0].id : '');
      setStepDeleteOpen(true);
    } catch (error) { toast({ title: 'Failed to check step tasks', description: error instanceof Error ? error.message : FALLBACK_ERROR_MESSAGE, variant: 'error' }); }
  };

  const handleReorderWorkflowSteps = async (reorderedSteps: WorkflowStep[]) => {
    if (isNewWorkflow) { setWorkflowSteps(reorderedSteps); return; }
    setWorkflowSteps(reorderedSteps);
    try { await reorderWorkflowStepsAction(workflow.id, reorderedSteps.map((s) => s.id)); }
    catch (error) { toast({ title: 'Failed to reorder workflow steps', description: error instanceof Error ? error.message : FALLBACK_ERROR_MESSAGE, variant: 'error' }); await refreshWorkflowSteps(); }
  };

  return { handleUpdateWorkflowStep, handleAddWorkflowStep, handleRemoveWorkflowStep, handleReorderWorkflowSteps };
}

type WorkflowDeleteDialogProps = {
  open: boolean; onOpenChange: (open: boolean) => void;
  workflowTaskCount: number | null; otherWorkflows: Workflow[];
  targetWorkflowId: string; setTargetWorkflowId: (id: string) => void;
  targetWorkflowSteps: WorkflowStep[]; targetStepId: string; setTargetStepId: (id: string) => void;
  migrateLoading: boolean; deleteLoading: boolean;
  onDelete: () => Promise<void>; onMigrateAndDelete: () => Promise<void>;
};

function WorkflowDeleteDialog({ open, onOpenChange, workflowTaskCount, otherWorkflows, targetWorkflowId, setTargetWorkflowId, targetWorkflowSteps, targetStepId, setTargetStepId, migrateLoading, deleteLoading, onDelete, onMigrateAndDelete }: WorkflowDeleteDialogProps) {
  const hasTasks = workflowTaskCount !== null && workflowTaskCount > 0;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete workflow</DialogTitle>
          <DialogDescription>
            {hasTasks ? `This workflow has ${workflowTaskCount} task${workflowTaskCount === 1 ? '' : 's'}. Choose where to migrate them, or delete everything.` : 'This will permanently delete the workflow and all its steps.'}
          </DialogDescription>
        </DialogHeader>
        {hasTasks && otherWorkflows.length > 0 && (
          <div className="space-y-3 py-2">
            <div className="space-y-2">
              <Label>Target Workflow</Label>
              <Select value={targetWorkflowId} onValueChange={setTargetWorkflowId}>
                <SelectTrigger><SelectValue placeholder="Select workflow" /></SelectTrigger>
                <SelectContent>{otherWorkflows.map((w) => <SelectItem key={w.id} value={w.id}>{w.name}</SelectItem>)}</SelectContent>
              </Select>
            </div>
            {targetWorkflowSteps.length > 0 && (
              <div className="space-y-2">
                <Label>Target Step</Label>
                <Select value={targetStepId} onValueChange={setTargetStepId}>
                  <SelectTrigger><SelectValue placeholder="Select step" /></SelectTrigger>
                  <SelectContent>{targetWorkflowSteps.map((s) => <SelectItem key={s.id} value={s.id}>{s.name}</SelectItem>)}</SelectContent>
                </Select>
              </div>
            )}
          </div>
        )}
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)} className="cursor-pointer">Cancel</Button>
          {hasTasks && otherWorkflows.length > 0 && (
            <Button type="button" onClick={onMigrateAndDelete} disabled={!targetWorkflowId || !targetStepId || migrateLoading || deleteLoading} className="cursor-pointer">
              {migrateLoading ? 'Migrating...' : 'Migrate & Delete'}
            </Button>
          )}
          <Button type="button" variant="destructive" onClick={onDelete} disabled={deleteLoading || migrateLoading} className="cursor-pointer">
            {hasTasks ? 'Delete Everything' : 'Delete Workflow'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

type StepDeleteDialogProps = {
  open: boolean; onOpenChange: (open: boolean) => void;
  stepTaskCount: number | null; stepsForMigration: WorkflowStep[];
  targetStep: string; setTargetStep: (id: string) => void;
  loading: boolean; onMigrateAndDelete: () => Promise<void>; onDeleteAndTasks: () => Promise<void>;
};

function StepDeleteDialog({ open, onOpenChange, stepTaskCount, stepsForMigration, targetStep, setTargetStep, loading, onMigrateAndDelete, onDeleteAndTasks }: StepDeleteDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete step</DialogTitle>
          <DialogDescription>
            This step has {stepTaskCount} task{stepTaskCount === 1 ? '' : 's'}.
            {stepsForMigration.length > 0 ? ' Choose where to migrate them, or delete the step and its tasks.' : ' Deleting this step will affect these tasks.'}
          </DialogDescription>
        </DialogHeader>
        {stepsForMigration.length > 0 && (
          <div className="space-y-2 py-2">
            <Label>Target Step</Label>
            <Select value={targetStep} onValueChange={setTargetStep}>
              <SelectTrigger><SelectValue placeholder="Select step" /></SelectTrigger>
              <SelectContent>{stepsForMigration.map((s) => <SelectItem key={s.id} value={s.id}>{s.name}</SelectItem>)}</SelectContent>
            </Select>
          </div>
        )}
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)} className="cursor-pointer">Cancel</Button>
          {stepsForMigration.length > 0 && (
            <Button type="button" onClick={onMigrateAndDelete} disabled={!targetStep || loading} className="cursor-pointer">
              {loading ? 'Migrating...' : 'Migrate & Delete Step'}
            </Button>
          )}
          <Button type="button" variant="destructive" onClick={onDeleteAndTasks} disabled={loading} className="cursor-pointer">Delete Step & Tasks</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

type WorkflowDeleteState = {
  deleteOpen: boolean; setDeleteOpen: (v: boolean) => void;
  workflowTaskCount: number | null; setWorkflowTaskCount: (v: number | null) => void;
  workflowDeleteLoading: boolean; setWorkflowDeleteLoading: (v: boolean) => void;
  targetWorkflowId: string; setTargetWorkflowId: (v: string) => void;
  targetWorkflowSteps: WorkflowStep[]; setTargetWorkflowSteps: (v: WorkflowStep[]) => void;
  targetStepId: string; setTargetStepId: (v: string) => void;
  migrateLoading: boolean; setMigrateLoading: (v: boolean) => void;
};

function useWorkflowDeleteState(): WorkflowDeleteState {
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [workflowTaskCount, setWorkflowTaskCount] = useState<number | null>(null);
  const [workflowDeleteLoading, setWorkflowDeleteLoading] = useState(false);
  const [targetWorkflowId, setTargetWorkflowId] = useState<string>('');
  const [targetWorkflowSteps, setTargetWorkflowSteps] = useState<WorkflowStep[]>([]);
  const [targetStepId, setTargetStepId] = useState<string>('');
  const [migrateLoading, setMigrateLoading] = useState(false);
  return { deleteOpen, setDeleteOpen, workflowTaskCount, setWorkflowTaskCount, workflowDeleteLoading, setWorkflowDeleteLoading, targetWorkflowId, setTargetWorkflowId, targetWorkflowSteps, setTargetWorkflowSteps, targetStepId, setTargetStepId, migrateLoading, setMigrateLoading };
}

type StepDeleteState = {
  stepDeleteOpen: boolean; setStepDeleteOpen: (v: boolean) => void;
  stepToDelete: string | null; setStepToDelete: (v: string | null) => void;
  stepTaskCount: number | null; setStepTaskCount: (v: number | null) => void;
  targetStepForMigration: string; setTargetStepForMigration: (v: string) => void;
  stepMigrateLoading: boolean; setStepMigrateLoading: (v: boolean) => void;
};

function useStepDeleteState(): StepDeleteState {
  const [stepDeleteOpen, setStepDeleteOpen] = useState(false);
  const [stepToDelete, setStepToDelete] = useState<string | null>(null);
  const [stepTaskCount, setStepTaskCount] = useState<number | null>(null);
  const [targetStepForMigration, setTargetStepForMigration] = useState<string>('');
  const [stepMigrateLoading, setStepMigrateLoading] = useState(false);
  return { stepDeleteOpen, setStepDeleteOpen, stepToDelete, setStepToDelete, stepTaskCount, setStepTaskCount, targetStepForMigration, setTargetStepForMigration, stepMigrateLoading, setStepMigrateLoading };
}

export function WorkflowCard({ workflow, isWorkflowDirty, initialWorkflowSteps, otherWorkflows = [], onUpdateWorkflow, onDeleteWorkflow, onSaveWorkflow, onWorkflowCreated }: WorkflowCardProps) {
  const { toast } = useToast();
  const [exportOpen, setExportOpen] = useState(false);
  const [exportJson, setExportJson] = useState('');
  const wfDel = useWorkflowDeleteState();
  const stepDel = useStepDeleteState();

  const isNewWorkflow = workflow.id.startsWith('temp-');
  const saveWorkflowRequest = useRequest(onSaveWorkflow);
  const deleteWorkflowRequest = useRequest(onDeleteWorkflow);
  const { workflowSteps, setWorkflowSteps, workflowLoading, refreshWorkflowSteps } = useWorkflowSteps(workflow.id, initialWorkflowSteps, isNewWorkflow, toast);
  const { handleUpdateWorkflowStep, handleAddWorkflowStep, handleRemoveWorkflowStep, handleReorderWorkflowSteps } = useWorkflowStepActions({ workflow, isNewWorkflow, workflowSteps, setWorkflowSteps, refreshWorkflowSteps, setStepToDelete: stepDel.setStepToDelete, setStepTaskCount: stepDel.setStepTaskCount, setTargetStepForMigration: stepDel.setTargetStepForMigration, setStepDeleteOpen: stepDel.setStepDeleteOpen, toast });

  useEffect(() => {
    if (!wfDel.targetWorkflowId) { wfDel.setTargetWorkflowSteps([]); wfDel.setTargetStepId(''); return; }
    let cancelled = false;
    listWorkflowStepsAction(wfDel.targetWorkflowId).then((res) => {
      if (!cancelled) { const steps = res.steps ?? []; wfDel.setTargetWorkflowSteps(steps); wfDel.setTargetStepId(steps.length > 0 ? steps[0].id : ''); }
    }).catch(() => { if (!cancelled) wfDel.setTargetWorkflowSteps([]); });
    return () => { cancelled = true; };
  }, [wfDel.targetWorkflowId]); // eslint-disable-line react-hooks/exhaustive-deps

  const saveNewWorkflowRequest = useRequest(async () => {
    const created = await createWorkflowAction({ workspace_id: workflow.workspace_id, name: workflow.name.trim() || 'New Workflow' });
    for (const step of workflowSteps) { await createWorkflowStepAction({ workflow_id: created.id, name: step.name, position: step.position, color: step.color, prompt: step.prompt, events: step.events, is_start_step: step.is_start_step, allow_manual_move: step.allow_manual_move }); }
    onWorkflowCreated?.(created);
  });
  const activeSaveRequest = isNewWorkflow ? saveNewWorkflowRequest : saveWorkflowRequest;

  const handleSaveWorkflow = async () => { try { if (isNewWorkflow) await saveNewWorkflowRequest.run(); else await saveWorkflowRequest.run(); } catch (error) { toast({ title: 'Failed to save workflow changes', description: error instanceof Error ? error.message : FALLBACK_ERROR_MESSAGE, variant: 'error' }); } };

  const handleDeleteWorkflowClick = async () => {
    if (isNewWorkflow) { wfDel.setWorkflowTaskCount(0); wfDel.setDeleteOpen(true); return; }
    wfDel.setWorkflowDeleteLoading(true);
    try {
      const { task_count } = await getWorkflowTaskCount(workflow.id);
      wfDel.setWorkflowTaskCount(task_count);
      if (task_count > 0 && otherWorkflows.length > 0) wfDel.setTargetWorkflowId(otherWorkflows[0].id);
      wfDel.setDeleteOpen(true);
    } catch (error) { toast({ title: 'Failed to check workflow tasks', description: error instanceof Error ? error.message : FALLBACK_ERROR_MESSAGE, variant: 'error' }); }
    finally { wfDel.setWorkflowDeleteLoading(false); }
  };

  const handleDeleteWorkflow = async () => {
    try { await deleteWorkflowRequest.run(); wfDel.setDeleteOpen(false); }
    catch (error) { toast({ title: 'Failed to delete workflow', description: error instanceof Error ? error.message : FALLBACK_ERROR_MESSAGE, variant: 'error' }); }
  };

  const handleMigrateAndDeleteWorkflow = async () => {
    if (!wfDel.targetWorkflowId || !wfDel.targetStepId) return;
    wfDel.setMigrateLoading(true);
    try { await bulkMoveTasks({ source_workflow_id: workflow.id, target_workflow_id: wfDel.targetWorkflowId, target_step_id: wfDel.targetStepId }); await deleteWorkflowRequest.run(); wfDel.setDeleteOpen(false); }
    catch (error) { toast({ title: 'Failed to migrate tasks', description: error instanceof Error ? error.message : FALLBACK_ERROR_MESSAGE, variant: 'error' }); }
    finally { wfDel.setMigrateLoading(false); }
  };

  const handleMigrateAndDeleteStep = async () => {
    if (!stepDel.stepToDelete || !stepDel.targetStepForMigration) return;
    stepDel.setStepMigrateLoading(true);
    try { await bulkMoveTasks({ source_workflow_id: workflow.id, source_step_id: stepDel.stepToDelete, target_workflow_id: workflow.id, target_step_id: stepDel.targetStepForMigration }); await deleteWorkflowStepAction(stepDel.stepToDelete); await refreshWorkflowSteps(); stepDel.setStepDeleteOpen(false); stepDel.setStepToDelete(null); }
    catch (error) { toast({ title: 'Failed to migrate tasks', description: error instanceof Error ? error.message : FALLBACK_ERROR_MESSAGE, variant: 'error' }); }
    finally { stepDel.setStepMigrateLoading(false); }
  };

  const handleDeleteStepAndTasks = async () => {
    if (!stepDel.stepToDelete) return;
    stepDel.setStepMigrateLoading(true);
    try { await deleteWorkflowStepAction(stepDel.stepToDelete); await refreshWorkflowSteps(); stepDel.setStepDeleteOpen(false); stepDel.setStepToDelete(null); }
    catch (error) { toast({ title: 'Failed to delete step', description: error instanceof Error ? error.message : FALLBACK_ERROR_MESSAGE, variant: 'error' }); }
    finally { stepDel.setStepMigrateLoading(false); }
  };

  const handleExportWorkflow = async () => {
    try { const data = await exportWorkflowAction(workflow.id); setExportJson(JSON.stringify(data, null, 2)); setExportOpen(true); }
    catch (error) { toast({ title: 'Failed to export workflow', description: error instanceof Error ? error.message : FALLBACK_ERROR_MESSAGE, variant: 'error' }); }
  };

  const stepsForStepMigration = stepDel.stepToDelete ? workflowSteps.filter((s) => s.id !== stepDel.stepToDelete) : [];

  return (
    <Card>
      <CardContent className="pt-6">
        <div className="space-y-4">
          <div className="flex items-center justify-between gap-3">
            <div className="space-y-2 flex-1">
              <Label className="flex items-center gap-2"><span>Workflow Name</span>{isWorkflowDirty && <UnsavedChangesBadge />}</Label>
              <div className="flex items-center gap-2">
                <Input value={workflow.name} onChange={(e) => onUpdateWorkflow({ name: e.target.value })} />
                <UnsavedSaveButton isDirty={isWorkflowDirty} isLoading={activeSaveRequest.isLoading} status={activeSaveRequest.status} onClick={handleSaveWorkflow} />
              </div>
            </div>
          </div>
          <div className="space-y-2">
            <Label>Workflow Steps</Label>
            {workflowLoading ? <div className="text-sm text-muted-foreground">Loading workflow steps...</div> : (
              <WorkflowPipelineEditor steps={workflowSteps} onUpdateStep={handleUpdateWorkflowStep} onAddStep={handleAddWorkflowStep} onRemoveStep={handleRemoveWorkflowStep} onReorderSteps={handleReorderWorkflowSteps} />
            )}
          </div>
          <div className="flex justify-end gap-2">
            {!isNewWorkflow && <Button type="button" variant="outline" onClick={handleExportWorkflow} className="cursor-pointer"><IconDownload className="h-4 w-4 mr-2" />Export</Button>}
            <Button type="button" variant="destructive" onClick={handleDeleteWorkflowClick} disabled={deleteWorkflowRequest.isLoading || wfDel.workflowDeleteLoading} className="cursor-pointer"><IconTrash className="h-4 w-4 mr-2" />Delete Workflow</Button>
          </div>
        </div>
      </CardContent>
      <WorkflowDeleteDialog open={wfDel.deleteOpen} onOpenChange={wfDel.setDeleteOpen} workflowTaskCount={wfDel.workflowTaskCount} otherWorkflows={otherWorkflows} targetWorkflowId={wfDel.targetWorkflowId} setTargetWorkflowId={wfDel.setTargetWorkflowId} targetWorkflowSteps={wfDel.targetWorkflowSteps} targetStepId={wfDel.targetStepId} setTargetStepId={wfDel.setTargetStepId} migrateLoading={wfDel.migrateLoading} deleteLoading={deleteWorkflowRequest.isLoading} onDelete={handleDeleteWorkflow} onMigrateAndDelete={handleMigrateAndDeleteWorkflow} />
      <WorkflowExportDialog open={exportOpen} onOpenChange={setExportOpen} title="Export Workflow" json={exportJson} />
      <StepDeleteDialog open={stepDel.stepDeleteOpen} onOpenChange={stepDel.setStepDeleteOpen} stepTaskCount={stepDel.stepTaskCount} stepsForMigration={stepsForStepMigration} targetStep={stepDel.targetStepForMigration} setTargetStep={stepDel.setTargetStepForMigration} loading={stepDel.stepMigrateLoading} onMigrateAndDelete={handleMigrateAndDeleteStep} onDeleteAndTasks={handleDeleteStepAndTasks} />
    </Card>
  );
}
