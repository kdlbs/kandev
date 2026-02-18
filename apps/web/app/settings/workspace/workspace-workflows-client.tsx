'use client';

import { useMemo, useRef, useState } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { IconDownload, IconLayoutColumns, IconPlus, IconUpload } from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import { Card, CardContent } from '@kandev/ui/card';
import { Separator } from '@kandev/ui/separator';
import { Label } from '@kandev/ui/label';
import { Input } from '@kandev/ui/input';
import { RadioGroup, RadioGroupItem } from '@kandev/ui/radio-group';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@kandev/ui/dialog';
import { Textarea } from '@kandev/ui/textarea';
import { SettingsSection } from '@/components/settings/settings-section';
import { WorkflowCard } from '@/components/settings/workflow-card';
import { WorkflowExportDialog } from '@/components/settings/workflow-export-dialog';
import { useToast } from '@/components/toast-provider';
import { useWorkflowSettings } from '@/hooks/domains/settings/use-workflow-settings';
import { cn, generateUUID } from '@/lib/utils';
import {
  deleteWorkflowAction,
  updateWorkflowAction,
  exportAllWorkflowsAction,
  importWorkflowsAction,
} from '@/app/actions/workspaces';
import type { Workflow, StepDefinition, WorkflowStep, Workspace, WorkflowTemplate, WorkflowExportData } from '@/lib/types/http';

type ImportWorkflowsDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  importJson: string;
  onImportJsonChange: (value: string) => void;
  onFileUpload: (e: React.ChangeEvent<HTMLInputElement>) => void;
  fileInputRef: React.RefObject<HTMLInputElement | null>;
  onImport: () => void;
  importLoading: boolean;
};

function ImportWorkflowsDialog({
  open,
  onOpenChange,
  importJson,
  onImportJsonChange,
  onFileUpload,
  fileInputRef,
  onImport,
  importLoading,
}: ImportWorkflowsDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Import Workflows</DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label>Upload JSON file</Label>
            <input
              ref={fileInputRef}
              type="file"
              accept=".json"
              onChange={onFileUpload}
              className="block w-full text-sm text-muted-foreground file:mr-4 file:py-2 file:px-4 file:rounded file:border-0 file:text-sm file:font-medium file:bg-primary file:text-primary-foreground file:cursor-pointer cursor-pointer"
            />
          </div>
          <div className="space-y-2">
            <Label>Or paste JSON</Label>
            <Textarea
              placeholder='{"version": 1, "type": "kandev_workflow", "workflows": [...]}'
              value={importJson}
              onChange={(e) => onImportJsonChange(e.target.value)}
              className="font-mono text-xs max-h-96 overflow-y-auto"
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} className="cursor-pointer">
            Cancel
          </Button>
          <Button onClick={onImport} disabled={!importJson.trim() || importLoading} className="cursor-pointer">
            {importLoading ? 'Importing...' : 'Import'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function TemplateRadioItem({ template, isSelected }: { template: WorkflowTemplate; isSelected: boolean }) {
  return (
    <label htmlFor={template.id} className={cn('flex items-start gap-3 p-3 rounded-lg border cursor-pointer transition-colors', isSelected ? 'border-primary bg-primary/5' : 'border-border hover:border-primary/50')}>
      <RadioGroupItem value={template.id} id={template.id} className="mt-0.5" />
      <div className="flex flex-col gap-1.5 min-w-0">
        <span className="font-medium">{template.name}</span>
        {template.description && <span className="text-sm text-muted-foreground">{template.description}</span>}
        {template.default_steps && template.default_steps.length > 0 && (
          <div className="flex items-center gap-1.5 flex-wrap mt-0.5">
            {template.default_steps.map((step, i) => (
              <div key={i} className="flex items-center gap-1">
                {i > 0 && <span className="text-muted-foreground/40 text-xs">&rarr;</span>}
                <div className="flex items-center gap-1 text-xs text-muted-foreground">
                  <div className={cn('w-2 h-2 rounded-full', step.color ?? 'bg-slate-500')} />
                  {step.name}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </label>
  );
}

type CreateWorkflowDialogProps = {
  open: boolean; onOpenChange: (open: boolean) => void;
  workflowName: string; onWorkflowNameChange: (value: string) => void;
  selectedTemplateId: string | null; onSelectedTemplateChange: (value: string | null) => void;
  workflowTemplates: WorkflowTemplate[]; onCreate: () => void;
};

function CreateWorkflowDialog({ open, onOpenChange, workflowName, onWorkflowNameChange, selectedTemplateId, onSelectedTemplateChange, workflowTemplates, onCreate }: CreateWorkflowDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:w-[900px] sm:max-w-none">
        <DialogHeader><DialogTitle>Add Workflow</DialogTitle></DialogHeader>
        <div className="space-y-6">
          <div className="space-y-2">
            <Label htmlFor="workflowName">Name</Label>
            <Input id="workflowName" placeholder="My Project Workflow" value={workflowName} onChange={(e) => onWorkflowNameChange(e.target.value)} />
          </div>
          {workflowTemplates.length > 0 && (
            <div className="space-y-2">
              <Label>Template</Label>
              <RadioGroup value={selectedTemplateId ?? 'custom'} onValueChange={(v) => onSelectedTemplateChange(v === 'custom' ? null : v)}>
                <div className="grid gap-3">
                  {workflowTemplates.map((template) => <TemplateRadioItem key={template.id} template={template} isSelected={selectedTemplateId === template.id} />)}
                  <label htmlFor="custom" className={cn('flex items-start gap-3 p-3 rounded-lg border cursor-pointer transition-colors', selectedTemplateId === null ? 'border-primary bg-primary/5' : 'border-border hover:border-primary/50')}>
                    <RadioGroupItem value="custom" id="custom" className="mt-0.5" />
                    <div className="flex flex-col gap-1.5"><span className="font-medium">Custom</span><span className="text-sm text-muted-foreground">Create your own agentic workflow from scratch.</span></div>
                  </label>
                </div>
              </RadioGroup>
            </div>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} className="cursor-pointer">Cancel</Button>
          <Button onClick={onCreate} className="cursor-pointer">Add Workflow</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

type WorkspaceWorkflowsClientProps = {
  workspace: Workspace | null;
  workflows: Workflow[];
  workflowTemplates: WorkflowTemplate[];
};

const DEFAULT_CUSTOM_STEPS: StepDefinition[] = [
  { name: 'Todo', position: 0, color: 'bg-slate-500' },
  { name: 'In Progress', position: 1, color: 'bg-blue-500' },
  { name: 'Review', position: 2, color: 'bg-purple-500' },
  { name: 'Done', position: 3, color: 'bg-green-500' },
];

type WorkflowActionsArgs = {
  workspace: Workspace | null;
  workflowItems: Workflow[];
  workflowTemplates: WorkflowTemplate[];
  setWorkflowItems: React.Dispatch<React.SetStateAction<Workflow[]>>;
  setSavedWorkflowItems: React.Dispatch<React.SetStateAction<Workflow[]>>;
  router: ReturnType<typeof useRouter>;
};

function buildWorkflowSteps(workflow: Workflow, definitions: StepDefinition[]): WorkflowStep[] {
  return definitions.map((step, index) => ({
    id: `temp-step-${workflow.id}-${index}`,
    workflow_id: workflow.id,
    name: step.name,
    position: step.position ?? index,
    color: step.color ?? 'bg-slate-500',
    prompt: step.prompt,
    events: step.events,
    is_start_step: step.is_start_step,
    allow_manual_move: true,
    created_at: '',
    updated_at: '',
  }));
}

function useWorkflowImportExport(workspace: Workspace | null, router: ReturnType<typeof useRouter>, toast: ReturnType<typeof useToast>['toast']) {
  const [isExportDialogOpen, setIsExportDialogOpen] = useState(false);
  const [exportJson, setExportJson] = useState('');
  const [isImportDialogOpen, setIsImportDialogOpen] = useState(false);
  const [importJson, setImportJson] = useState('');
  const [importLoading, setImportLoading] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const handleExportAll = async () => {
    if (!workspace) return;
    try {
      const data = await exportAllWorkflowsAction(workspace.id);
      setExportJson(JSON.stringify(data, null, 2));
      setIsExportDialogOpen(true);
    } catch (error) {
      toast({ title: 'Failed to export workflows', description: error instanceof Error ? error.message : 'Request failed', variant: 'error' });
    }
  };

  const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = (event) => { setImportJson(event.target?.result as string); };
    reader.readAsText(file);
    e.target.value = '';
  };

  const handleImport = async () => {
    if (!workspace || !importJson.trim()) return;
    setImportLoading(true);
    try {
      const data = JSON.parse(importJson.trim()) as WorkflowExportData;
      const result = await importWorkflowsAction(workspace.id, data);
      const created = result.created ?? [];
      const skipped = result.skipped ?? [];
      const parts: string[] = [];
      if (created.length > 0) parts.push(`Created: ${created.join(', ')}`);
      if (skipped.length > 0) parts.push(`Skipped (already exist): ${skipped.join(', ')}`);
      toast({ title: 'Import complete', description: parts.join('. ') });
      setIsImportDialogOpen(false);
      setImportJson('');
      if (created.length > 0) router.refresh();
    } catch (error) {
      toast({ title: 'Failed to import workflows', description: error instanceof Error ? error.message : 'Invalid JSON', variant: 'error' });
    } finally { setImportLoading(false); }
  };

  return { isExportDialogOpen, setIsExportDialogOpen, exportJson, isImportDialogOpen, setIsImportDialogOpen, importJson, setImportJson, importLoading, fileInputRef, handleExportAll, handleFileUpload, handleImport };
}

function useWorkflowActions({ workspace, workflowItems, workflowTemplates, setWorkflowItems, setSavedWorkflowItems, router }: WorkflowActionsArgs) {
  const [isAddWorkflowDialogOpen, setIsAddWorkflowDialogOpen] = useState(false);
  const [newWorkflowName, setNewWorkflowName] = useState('');
  const [selectedTemplateId, setSelectedTemplateId] = useState<string | null>(null);

  const handleOpenAddWorkflowDialog = () => {
    setNewWorkflowName('');
    setSelectedTemplateId(workflowTemplates.length > 0 ? workflowTemplates[0].id : null);
    setIsAddWorkflowDialogOpen(true);
  };

  const handleCreateWorkflow = () => {
    if (!workspace) return;
    const templateName = selectedTemplateId ? workflowTemplates.find((t) => t.id === selectedTemplateId)?.name ?? 'New Workflow' : 'New Workflow';
    const draftWorkflow: Workflow = { id: `temp-${generateUUID()}`, workspace_id: workspace.id, name: newWorkflowName.trim() || templateName, description: '', workflow_template_id: selectedTemplateId, created_at: '', updated_at: '' };
    setWorkflowItems((prev) => [draftWorkflow, ...prev]);
    setIsAddWorkflowDialogOpen(false);
  };

  const handleUpdateWorkflow = (workflowId: string, updates: { name?: string; description?: string }) => {
    setWorkflowItems((prev) => prev.map((wf) => (wf.id === workflowId ? { ...wf, ...updates } : wf)));
  };

  const handleDeleteWorkflow = async (workflowId: string) => {
    if (workflowId.startsWith('temp-')) { setWorkflowItems((prev) => prev.filter((wf) => wf.id !== workflowId)); return; }
    await deleteWorkflowAction(workflowId);
    setWorkflowItems((prev) => prev.filter((wf) => wf.id !== workflowId));
    setSavedWorkflowItems((prev) => prev.filter((wf) => wf.id !== workflowId));
  };

  const handleWorkflowCreated = (tempId: string, created: Workflow) => {
    setWorkflowItems((prev) => prev.map((item) => (item.id === tempId ? created : item)));
    setSavedWorkflowItems((prev) => [{ ...created }, ...prev]);
    router.refresh();
  };

  const handleSaveWorkflow = async (workflowId: string) => {
    const workflow = workflowItems.find((item) => item.id === workflowId);
    if (!workflow) return;
    const updates: { name?: string; description?: string } = {};
    if (workflow.name.trim()) updates.name = workflow.name.trim();
    if (Object.keys(updates).length) await updateWorkflowAction(workflowId, updates);
    setWorkflowItems((prev) => prev.map((item) => (item.id === workflowId ? { ...item, ...updates } : item)));
    setSavedWorkflowItems((prev) => prev.some((item) => item.id === workflowId) ? prev.map((item) => item.id === workflowId ? { ...workflow, ...updates } : item) : [...prev, { ...workflow, ...updates }]);
  };

  return { isAddWorkflowDialogOpen, setIsAddWorkflowDialogOpen, newWorkflowName, setNewWorkflowName, selectedTemplateId, setSelectedTemplateId, handleOpenAddWorkflowDialog, handleCreateWorkflow, handleUpdateWorkflow, handleDeleteWorkflow, handleWorkflowCreated, handleSaveWorkflow };
}

type WorkflowSectionActionsProps = { onExport: () => void; onImport: () => void; onAdd: () => void };

function WorkflowSectionActions({ onExport, onImport, onAdd }: WorkflowSectionActionsProps) {
  return (
    <div className="flex gap-2">
      <Button size="sm" variant="outline" onClick={onExport} className="cursor-pointer"><IconDownload className="h-4 w-4 mr-2" />Export All</Button>
      <Button size="sm" variant="outline" onClick={onImport} className="cursor-pointer"><IconUpload className="h-4 w-4 mr-2" />Import</Button>
      <Button size="sm" onClick={onAdd} className="cursor-pointer"><IconPlus className="h-4 w-4 mr-2" />Add Workflow</Button>
    </div>
  );
}

type WorkflowListProps = { workflowItems: Workflow[]; templateStepsById: Map<string, StepDefinition[]>; isWorkflowDirty: (wf: Workflow) => boolean; onUpdate: (id: string, u: { name?: string; description?: string }) => void; onDelete: (id: string) => void; onSave: (id: string) => void; onCreated: (id: string, wf: Workflow) => void };

function WorkflowList({ workflowItems, templateStepsById, isWorkflowDirty, onUpdate, onDelete, onSave, onCreated }: WorkflowListProps) {
  return (
    <div className="grid gap-3">
      {workflowItems.map((workflow) => {
        const isTempWorkflow = workflow.id.startsWith('temp-');
        const templateSteps = isTempWorkflow && workflow.workflow_template_id ? templateStepsById.get(workflow.workflow_template_id) ?? [] : DEFAULT_CUSTOM_STEPS;
        const initialWorkflowSteps = isTempWorkflow && templateSteps.length ? buildWorkflowSteps(workflow, templateSteps) : undefined;
        return (
          <WorkflowCard key={workflow.id} workflow={workflow} isWorkflowDirty={isWorkflowDirty(workflow)} initialWorkflowSteps={initialWorkflowSteps}
            otherWorkflows={workflowItems.filter((w) => w.id !== workflow.id && !w.id.startsWith('temp-'))}
            onUpdateWorkflow={(updates) => onUpdate(workflow.id, updates)} onDeleteWorkflow={async () => { await onDelete(workflow.id); }}
            onSaveWorkflow={async () => { await onSave(workflow.id); }} onWorkflowCreated={(created) => onCreated(workflow.id, created)}
          />
        );
      })}
    </div>
  );
}

export function WorkspaceWorkflowsClient({ workspace, workflows, workflowTemplates }: WorkspaceWorkflowsClientProps) {
  const router = useRouter();
  const { toast } = useToast();
  const { workflowItems, setWorkflowItems, setSavedWorkflowItems, isWorkflowDirty } = useWorkflowSettings(workflows);

  const importExport = useWorkflowImportExport(workspace, router, toast);
  const { isExportDialogOpen, setIsExportDialogOpen, exportJson, isImportDialogOpen, setIsImportDialogOpen, importJson, setImportJson, importLoading, fileInputRef, handleExportAll, handleFileUpload, handleImport } = importExport;

  const actions = useWorkflowActions({ workspace, workflowItems, workflowTemplates, setWorkflowItems, setSavedWorkflowItems, router });
  const { isAddWorkflowDialogOpen, setIsAddWorkflowDialogOpen, newWorkflowName, setNewWorkflowName, selectedTemplateId, setSelectedTemplateId, handleOpenAddWorkflowDialog, handleCreateWorkflow, handleUpdateWorkflow, handleDeleteWorkflow, handleWorkflowCreated, handleSaveWorkflow } = actions;

  const templateStepsById = useMemo(() => new Map(workflowTemplates.map((t) => [t.id, t.default_steps ?? []])), [workflowTemplates]);

  if (!workspace) {
    return (
      <div><Card><CardContent className="py-12 text-center">
        <p className="text-muted-foreground">Workspace not found</p>
        <Button className="mt-4" onClick={() => router.push('/settings/workspace')}>Back to Workspaces</Button>
      </CardContent></Card></div>
    );
  }

  return (
    <div className="space-y-8">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2 className="text-2xl font-bold">{workspace.name}</h2>
          <p className="text-sm text-muted-foreground mt-1">Manage workflows for this workspace.</p>
        </div>
        <Button asChild variant="outline" size="sm"><Link href={`/settings/workspace/${workspace.id}`}>Workspace settings</Link></Button>
      </div>
      <Separator />
      <SettingsSection icon={<IconLayoutColumns className="h-5 w-5" />} title="Workflows"
        description="Create autonomous pipelines with automated transitions or manual workflows where you move tasks yourself"
        action={<WorkflowSectionActions onExport={handleExportAll} onImport={() => setIsImportDialogOpen(true)} onAdd={handleOpenAddWorkflowDialog} />}
      >
        <WorkflowList workflowItems={workflowItems} templateStepsById={templateStepsById} isWorkflowDirty={isWorkflowDirty}
          onUpdate={handleUpdateWorkflow} onDelete={handleDeleteWorkflow} onSave={handleSaveWorkflow} onCreated={handleWorkflowCreated}
        />
      </SettingsSection>
      <WorkflowExportDialog open={isExportDialogOpen} onOpenChange={setIsExportDialogOpen} title="Export Workflows" json={exportJson} />
      <ImportWorkflowsDialog open={isImportDialogOpen} onOpenChange={setIsImportDialogOpen} importJson={importJson} onImportJsonChange={setImportJson}
        onFileUpload={handleFileUpload} fileInputRef={fileInputRef} onImport={handleImport} importLoading={importLoading}
      />
      <CreateWorkflowDialog open={isAddWorkflowDialogOpen} onOpenChange={setIsAddWorkflowDialogOpen} workflowName={newWorkflowName}
        onWorkflowNameChange={setNewWorkflowName} selectedTemplateId={selectedTemplateId} onSelectedTemplateChange={setSelectedTemplateId}
        workflowTemplates={workflowTemplates} onCreate={handleCreateWorkflow}
      />
    </div>
  );
}
