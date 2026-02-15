'use client';

import { useMemo, useState } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { IconLayoutColumns, IconPlus } from '@tabler/icons-react';
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
import { SettingsSection } from '@/components/settings/settings-section';
import { WorkflowCard } from '@/components/settings/workflow-card';
import { cn, generateUUID } from '@/lib/utils';
import {
  deleteWorkflowAction,
  updateWorkflowAction,
} from '@/app/actions/workspaces';
import type { Workflow, StepDefinition, WorkflowStep, Workspace, WorkflowTemplate } from '@/lib/types/http';

type WorkspaceWorkflowsClientProps = {
  workspace: Workspace | null;
  workflows: Workflow[];
  workflowTemplates: WorkflowTemplate[];
};

export function WorkspaceWorkflowsClient({
  workspace,
  workflows,
  workflowTemplates,
}: WorkspaceWorkflowsClientProps) {
  const router = useRouter();
  const [workflowItems, setWorkflowItems] = useState<Workflow[]>(workflows);
  const [savedWorkflowItems, setSavedWorkflowItems] = useState<Workflow[]>(workflows);

  // Dialog state for creating a new workflow
  const [isAddWorkflowDialogOpen, setIsAddWorkflowDialogOpen] = useState(false);
  const [newWorkflowName, setNewWorkflowName] = useState('');
  const [selectedTemplateId, setSelectedTemplateId] = useState<string | null>(null);

  const templateStepsById = useMemo(() => {
    return new Map(
      workflowTemplates.map((template) => [template.id, template.default_steps ?? []])
    );
  }, [workflowTemplates]);

  const defaultCustomSteps = useMemo<StepDefinition[]>(
    () => [
      { name: 'Todo', position: 0, color: 'bg-slate-500' },
      { name: 'In Progress', position: 1, color: 'bg-blue-500' },
      { name: 'Review', position: 2, color: 'bg-purple-500' },
      { name: 'Done', position: 3, color: 'bg-green-500' },
    ],
    []
  );

  const buildWorkflowSteps = (workflow: Workflow, definitions: StepDefinition[]): WorkflowStep[] =>
    definitions.map((step, index) => ({
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

  const savedWorkflowsById = useMemo(() => {
    return new Map(savedWorkflowItems.map((workflow) => [workflow.id, workflow]));
  }, [savedWorkflowItems]);

  const isWorkflowDirty = (workflow: Workflow) => {
    const saved = savedWorkflowsById.get(workflow.id);
    if (!saved) return true;
    if (workflow.name !== saved.name || workflow.description !== saved.description) return true;
    return false;
  };

  const handleOpenAddWorkflowDialog = () => {
    setNewWorkflowName('');
    setSelectedTemplateId(workflowTemplates.length > 0 ? workflowTemplates[0].id : null);
    setIsAddWorkflowDialogOpen(true);
  };

  const handleCreateWorkflow = () => {
    if (!workspace) return;

    const draftWorkflow: Workflow = {
      id: `temp-${generateUUID()}`,
      workspace_id: workspace.id,
      name: newWorkflowName.trim() || (selectedTemplateId ? workflowTemplates.find((t) => t.id === selectedTemplateId)?.name ?? 'New Workflow' : 'New Workflow'),
      description: '',
      workflow_template_id: selectedTemplateId,
      created_at: '',
      updated_at: '',
    };

    setWorkflowItems((prev) => [draftWorkflow, ...prev]);
    setIsAddWorkflowDialogOpen(false);
  };

  const handleUpdateWorkflow = (workflowId: string, updates: { name?: string; description?: string }) => {
    setWorkflowItems((prev) =>
      prev.map((workflow) => (workflow.id === workflowId ? { ...workflow, ...updates } : workflow))
    );
  };

  const handleDeleteWorkflow = async (workflowId: string) => {
    if (workflowId.startsWith('temp-')) {
      setWorkflowItems((prev) => prev.filter((workflow) => workflow.id !== workflowId));
      return;
    }
    await deleteWorkflowAction(workflowId);
    setWorkflowItems((prev) => prev.filter((workflow) => workflow.id !== workflowId));
    setSavedWorkflowItems((prev) => prev.filter((workflow) => workflow.id !== workflowId));
  };

  const handleWorkflowCreated = (tempId: string, created: Workflow) => {
    setWorkflowItems((prev) => prev.map((item) => (item.id === tempId ? created : item)));
    setSavedWorkflowItems((prev) => [{ ...created }, ...prev]);
    router.refresh();
  };

  const handleSaveWorkflow = async (workflowId: string) => {
    const workflow = workflowItems.find((item) => item.id === workflowId);
    if (!workflow) return;
    // For existing workflows, only update workflow name/description
    const updates: { name?: string; description?: string } = {};
    if (workflow.name.trim()) {
      updates.name = workflow.name.trim();
    }
    if (Object.keys(updates).length) {
      await updateWorkflowAction(workflowId, updates);
    }
    setWorkflowItems((prev) =>
      prev.map((item) => (item.id === workflowId ? { ...item, ...updates } : item))
    );
    setSavedWorkflowItems((prev) =>
      prev.some((item) => item.id === workflowId)
        ? prev.map((item) =>
          item.id === workflowId ? { ...workflow, ...updates } : item
        )
        : [...prev, { ...workflow, ...updates }]
    );
  };

  if (!workspace) {
    return (
      <div>
        <Card>
          <CardContent className="py-12 text-center">
            <p className="text-muted-foreground">Workspace not found</p>
            <Button className="mt-4" onClick={() => router.push('/settings/workspace')}>
              Back to Workspaces
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="space-y-8">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2 className="text-2xl font-bold">{workspace.name}</h2>
          <p className="text-sm text-muted-foreground mt-1">
            Manage workflows for this workspace.
          </p>
        </div>
        <Button asChild variant="outline" size="sm">
          <Link href={`/settings/workspace/${workspace.id}`}>Workspace settings</Link>
        </Button>
      </div>

      <Separator />

      <SettingsSection
        icon={<IconLayoutColumns className="h-5 w-5" />}
        title="Workflows"
        description="Create autonomous pipelines with automated transitions or manual workflows where you move tasks yourself"
        action={
          <Button size="sm" onClick={handleOpenAddWorkflowDialog} className="cursor-pointer">
            <IconPlus className="h-4 w-4 mr-2" />
            Add Workflow
          </Button>
        }
      >
        <div className="grid gap-3">
          {workflowItems.map((workflow) => {
            const isTempWorkflow = workflow.id.startsWith('temp-');
            const templateSteps =
              isTempWorkflow && workflow.workflow_template_id
                ? templateStepsById.get(workflow.workflow_template_id) ?? []
                : defaultCustomSteps;
            const initialWorkflowSteps =
              isTempWorkflow && templateSteps.length
                ? buildWorkflowSteps(workflow, templateSteps)
                : undefined;
            return (
              <WorkflowCard
                key={workflow.id}
                workflow={workflow}
                isWorkflowDirty={isWorkflowDirty(workflow)}
                initialWorkflowSteps={initialWorkflowSteps}
                otherWorkflows={workflowItems.filter(
                  (w) => w.id !== workflow.id && !w.id.startsWith('temp-')
                )}
                onUpdateWorkflow={(updates) => handleUpdateWorkflow(workflow.id, updates)}
                onDeleteWorkflow={() => handleDeleteWorkflow(workflow.id)}
                onSaveWorkflow={() => handleSaveWorkflow(workflow.id)}
                onWorkflowCreated={(created) => handleWorkflowCreated(workflow.id, created)}
              />
            );
          })}
        </div>
      </SettingsSection>

      {/* Create Workflow Dialog */}
      <Dialog open={isAddWorkflowDialogOpen} onOpenChange={setIsAddWorkflowDialogOpen}>
        <DialogContent className="sm:w-[900px] sm:max-w-none">
          <DialogHeader>
            <DialogTitle>Add Workflow</DialogTitle>
          </DialogHeader>

          <div className="space-y-6">
            <div className="space-y-2">
              <Label htmlFor="workflowName">Name</Label>
              <Input
                id="workflowName"
                placeholder="My Project Workflow"
                value={newWorkflowName}
                onChange={(e) => setNewWorkflowName(e.target.value)}
              />
            </div>

            {workflowTemplates.length > 0 && (
              <div className="space-y-2">
                <Label>Template</Label>
                <RadioGroup
                  value={selectedTemplateId ?? 'custom'}
                  onValueChange={(value) =>
                    setSelectedTemplateId(value === 'custom' ? null : value)
                  }
                >
                  <div className="grid gap-3">
                    {workflowTemplates.map((template) => (
                      <label
                        key={template.id}
                        htmlFor={template.id}
                        className={cn(
                          'flex items-start gap-3 p-3 rounded-lg border cursor-pointer transition-colors',
                          selectedTemplateId === template.id
                            ? 'border-primary bg-primary/5'
                            : 'border-border hover:border-primary/50',
                        )}
                      >
                        <RadioGroupItem value={template.id} id={template.id} className="mt-0.5" />
                        <div className="flex flex-col gap-1.5 min-w-0">
                          <span className="font-medium">{template.name}</span>
                          {template.description && (
                            <span className="text-sm text-muted-foreground">
                              {template.description}
                            </span>
                          )}
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
                    ))}
                    <label
                      htmlFor="custom"
                      className={cn(
                        'flex items-start gap-3 p-3 rounded-lg border cursor-pointer transition-colors',
                        selectedTemplateId === null
                          ? 'border-primary bg-primary/5'
                          : 'border-border hover:border-primary/50',
                      )}
                    >
                      <RadioGroupItem value="custom" id="custom" className="mt-0.5" />
                      <div className="flex flex-col gap-1.5">
                        <span className="font-medium">Custom</span>
                        <span className="text-sm text-muted-foreground">
                          Create your own agentic workflow from scratch.
                        </span>
                      </div>
                    </label>
                  </div>
                </RadioGroup>
              </div>
            )}

            {/* Workflow preview intentionally omitted from the modal */}
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setIsAddWorkflowDialogOpen(false)} className="cursor-pointer">
              Cancel
            </Button>
            <Button onClick={handleCreateWorkflow} className="cursor-pointer">Add Workflow</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
