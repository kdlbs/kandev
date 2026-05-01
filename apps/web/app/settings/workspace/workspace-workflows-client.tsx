"use client";

import { useMemo, useRef, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  IconDownload,
  IconGripVertical,
  IconArrowsShuffle,
  IconPlus,
  IconUpload,
} from "@tabler/icons-react";
import {
  DndContext,
  closestCenter,
  type DragEndEvent,
  PointerSensor,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import {
  SortableContext,
  verticalListSortingStrategy,
  arrayMove,
  useSortable,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
import { SettingsSection } from "@/components/settings/settings-section";
import { WorkflowCard } from "@/components/settings/workflow-card";
import { WorkflowExportDialog } from "@/components/settings/workflow-export-dialog";
import { useToast } from "@/components/toast-provider";
import { useWorkflowSettings } from "@/hooks/domains/settings/use-workflow-settings";
import { generateUUID } from "@/lib/utils";
import {
  deleteWorkflowAction,
  updateWorkflowAction,
  exportAllWorkflowsAction,
  importWorkflowsAction,
  reorderWorkflowsAction,
} from "@/app/actions/workspaces";
import type {
  Workflow,
  StepDefinition,
  WorkflowStep,
  Workspace,
  WorkflowTemplate,
} from "@/lib/types/http";
import {
  CreateWorkflowDialog,
  ImportWorkflowsDialog,
} from "@/app/settings/workspace/workspace-workflows-dialogs";
import { WorkspaceNotFoundCard } from "@/app/settings/workspace/workspace-not-found-card";

type WorkspaceWorkflowsClientProps = {
  workspace: Workspace | null;
  workflows: Workflow[];
  workflowTemplates: WorkflowTemplate[];
};

const DEFAULT_CUSTOM_STEPS: StepDefinition[] = [
  { name: "Todo", position: 0, color: "bg-slate-500" },
  { name: "In Progress", position: 1, color: "bg-blue-500" },
  { name: "Review", position: 2, color: "bg-purple-500" },
  { name: "Done", position: 3, color: "bg-green-500" },
];

type WorkflowActionsArgs = {
  workspace: Workspace | null;
  workflowItems: Workflow[];
  workflowTemplates: WorkflowTemplate[];
  setWorkflowItems: React.Dispatch<React.SetStateAction<Workflow[]>>;
  setSavedWorkflowItems: React.Dispatch<React.SetStateAction<Workflow[]>>;
};

function buildWorkflowSteps(workflow: Workflow, definitions: StepDefinition[]): WorkflowStep[] {
  return definitions.map((step, index) => ({
    id: `temp-step-${workflow.id}-${index}`,
    workflow_id: workflow.id,
    name: step.name,
    position: step.position ?? index,
    color: step.color ?? "bg-slate-500",
    prompt: step.prompt,
    events: step.events,
    is_start_step: step.is_start_step,
    show_in_command_panel: step.show_in_command_panel,
    allow_manual_move: true,
    created_at: "",
    updated_at: "",
  }));
}

function useWorkflowImportExport(
  workspace: Workspace | null,
  router: ReturnType<typeof useRouter>,
  toast: ReturnType<typeof useToast>["toast"],
) {
  const [isExportDialogOpen, setIsExportDialogOpen] = useState(false);
  const [exportYaml, setExportYaml] = useState("");
  const [isImportDialogOpen, setIsImportDialogOpen] = useState(false);
  const [importYaml, setImportYaml] = useState("");
  const [importLoading, setImportLoading] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const handleExportAll = async () => {
    if (!workspace) return;
    try {
      const yamlText = await exportAllWorkflowsAction(workspace.id);
      setExportYaml(yamlText);
      setIsExportDialogOpen(true);
    } catch (error) {
      toast({
        title: "Failed to export workflows",
        description: error instanceof Error ? error.message : "Request failed",
        variant: "error",
      });
    }
  };

  const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = (event) => {
      setImportYaml(event.target?.result as string);
    };
    reader.readAsText(file);
    e.target.value = "";
  };

  const handleImport = async () => {
    if (!workspace || !importYaml.trim()) return;
    setImportLoading(true);
    try {
      const result = await importWorkflowsAction(workspace.id, importYaml.trim());
      const created = result.created ?? [];
      const skipped = result.skipped ?? [];
      const parts: string[] = [];
      if (created.length > 0) parts.push(`Created: ${created.join(", ")}`);
      if (skipped.length > 0) parts.push(`Skipped (already exist): ${skipped.join(", ")}`);
      toast({ title: "Import complete", description: parts.join(". ") });
      setIsImportDialogOpen(false);
      setImportYaml("");
      if (created.length > 0) router.refresh();
    } catch (error) {
      toast({
        title: "Failed to import workflows",
        description: error instanceof Error ? error.message : "Invalid YAML",
        variant: "error",
      });
    } finally {
      setImportLoading(false);
    }
  };

  return {
    isExportDialogOpen,
    setIsExportDialogOpen,
    exportYaml,
    isImportDialogOpen,
    setIsImportDialogOpen,
    importYaml,
    setImportYaml,
    importLoading,
    fileInputRef,
    handleExportAll,
    handleFileUpload,
    handleImport,
  };
}

function useWorkflowActions({
  workspace,
  workflowItems,
  workflowTemplates,
  setWorkflowItems,
  setSavedWorkflowItems,
}: WorkflowActionsArgs) {
  const [isAddWorkflowDialogOpen, setIsAddWorkflowDialogOpen] = useState(false);
  const [newWorkflowName, setNewWorkflowName] = useState("");
  const [selectedTemplateId, setSelectedTemplateId] = useState<string | null>(null);

  const handleOpenAddWorkflowDialog = () => {
    setNewWorkflowName("");
    setSelectedTemplateId(workflowTemplates.length > 0 ? workflowTemplates[0].id : null);
    setIsAddWorkflowDialogOpen(true);
  };

  const handleCreateWorkflow = () => {
    if (!workspace) return;
    const templateName = selectedTemplateId
      ? (workflowTemplates.find((t) => t.id === selectedTemplateId)?.name ?? "New Workflow")
      : "New Workflow";
    const draftWorkflow: Workflow = {
      id: `temp-${generateUUID()}`,
      workspace_id: workspace.id,
      name: newWorkflowName.trim() || templateName,
      description: "",
      workflow_template_id: selectedTemplateId,
      created_at: "",
      updated_at: "",
    };
    setWorkflowItems((prev) => [draftWorkflow, ...prev]);
    setIsAddWorkflowDialogOpen(false);
  };

  const handleUpdateWorkflow = (
    workflowId: string,
    updates: { name?: string; description?: string; agent_profile_id?: string },
  ) => {
    setWorkflowItems((prev) =>
      prev.map((wf) => (wf.id === workflowId ? { ...wf, ...updates } : wf)),
    );
  };

  const handleDeleteWorkflow = async (workflowId: string) => {
    if (workflowId.startsWith("temp-")) {
      setWorkflowItems((prev) => prev.filter((wf) => wf.id !== workflowId));
      return;
    }
    await deleteWorkflowAction(workflowId);
    setWorkflowItems((prev) => prev.filter((wf) => wf.id !== workflowId));
    setSavedWorkflowItems((prev) => prev.filter((wf) => wf.id !== workflowId));
  };

  const handleWorkflowCreated = (tempId: string, created: Workflow) => {
    setWorkflowItems((prev) => prev.map((item) => (item.id === tempId ? created : item)));
    setSavedWorkflowItems((prev) => [{ ...created }, ...prev]);
    // Note: No router.refresh() needed. Local state already has the correct workflow,
    // and SSR will fetch fresh data on next navigation.
  };

  const handleSaveWorkflow = async (workflowId: string) => {
    const workflow = workflowItems.find((item) => item.id === workflowId);
    if (!workflow) return;
    const updates: { name?: string; description?: string; agent_profile_id?: string } = {};
    if (workflow.name.trim()) updates.name = workflow.name.trim();
    updates.agent_profile_id = workflow.agent_profile_id ?? "";
    if (Object.keys(updates).length) await updateWorkflowAction(workflowId, updates);
    setWorkflowItems((prev) =>
      prev.map((item) => (item.id === workflowId ? { ...item, ...updates } : item)),
    );
    setSavedWorkflowItems((prev) =>
      prev.some((item) => item.id === workflowId)
        ? prev.map((item) => (item.id === workflowId ? { ...workflow, ...updates } : item))
        : [...prev, { ...workflow, ...updates }],
    );
  };

  return {
    isAddWorkflowDialogOpen,
    setIsAddWorkflowDialogOpen,
    newWorkflowName,
    setNewWorkflowName,
    selectedTemplateId,
    setSelectedTemplateId,
    handleOpenAddWorkflowDialog,
    handleCreateWorkflow,
    handleUpdateWorkflow,
    handleDeleteWorkflow,
    handleWorkflowCreated,
    handleSaveWorkflow,
  };
}

type WorkflowSectionActionsProps = {
  onExport: () => void;
  onImport: () => void;
  onAdd: () => void;
};

function WorkflowSectionActions({ onExport, onImport, onAdd }: WorkflowSectionActionsProps) {
  return (
    <div className="flex gap-2">
      <Button size="sm" variant="outline" onClick={onExport} className="cursor-pointer">
        <IconDownload className="h-4 w-4 mr-2" />
        Export All
      </Button>
      <Button size="sm" variant="outline" onClick={onImport} className="cursor-pointer">
        <IconUpload className="h-4 w-4 mr-2" />
        Import
      </Button>
      <Button
        size="sm"
        onClick={onAdd}
        className="cursor-pointer"
        data-testid="add-workflow-button"
      >
        <IconPlus className="h-4 w-4 mr-2" />
        Add Workflow
      </Button>
    </div>
  );
}

type WorkflowListProps = {
  workflowItems: Workflow[];
  workspaceId: string;
  templateStepsById: Map<string, StepDefinition[]>;
  isWorkflowDirty: (wf: Workflow) => boolean;
  onUpdate: (
    id: string,
    u: { name?: string; description?: string; agent_profile_id?: string },
  ) => void;
  onDelete: (id: string) => void;
  onSave: (id: string) => void;
  onCreated: (id: string, wf: Workflow) => void;
  onReorder: (items: Workflow[]) => void;
};

function SortableWorkflowItem({
  workflow,
  children,
}: {
  workflow: Workflow;
  children: React.ReactNode;
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: workflow.id,
  });
  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
  };
  return (
    <div ref={setNodeRef} style={style} className="relative min-w-0">
      <div
        className="absolute left-0 top-6 -ml-8 flex items-center cursor-grab active:cursor-grabbing z-10"
        data-testid={`workflow-drag-handle-${workflow.id}`}
        {...attributes}
        {...listeners}
      >
        <IconGripVertical className="h-5 w-5 text-muted-foreground" />
      </div>
      {children}
    </div>
  );
}

function WorkflowList({
  workflowItems,
  workspaceId,
  templateStepsById,
  isWorkflowDirty,
  onUpdate,
  onDelete,
  onSave,
  onCreated,
  onReorder,
}: WorkflowListProps) {
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 8 } }));

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (!over || active.id === over.id) return;
    const oldIndex = workflowItems.findIndex((wf) => wf.id === active.id);
    const newIndex = workflowItems.findIndex((wf) => wf.id === over.id);
    if (oldIndex === -1 || newIndex === -1) return;
    const reordered = arrayMove(workflowItems, oldIndex, newIndex);
    onReorder(reordered);
    // Persist to backend (skip temp workflows)
    const persistedIds = reordered.filter((wf) => !wf.id.startsWith("temp-")).map((wf) => wf.id);
    if (persistedIds.length > 0) {
      reorderWorkflowsAction(workspaceId, persistedIds).catch(() => {});
    }
  };

  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
      <SortableContext
        items={workflowItems.map((wf) => wf.id)}
        strategy={verticalListSortingStrategy}
      >
        <div className="grid min-w-0 gap-3 pl-8">
          {workflowItems.map((workflow) => {
            const isTempWorkflow = workflow.id.startsWith("temp-");
            const templateSteps =
              isTempWorkflow && workflow.workflow_template_id
                ? (templateStepsById.get(workflow.workflow_template_id) ?? [])
                : DEFAULT_CUSTOM_STEPS;
            const initialWorkflowSteps =
              isTempWorkflow && templateSteps.length
                ? buildWorkflowSteps(workflow, templateSteps)
                : undefined;
            return (
              <SortableWorkflowItem key={workflow.id} workflow={workflow}>
                <WorkflowCard
                  workflow={workflow}
                  isWorkflowDirty={isWorkflowDirty(workflow)}
                  initialWorkflowSteps={initialWorkflowSteps}
                  templateStepCount={isTempWorkflow ? templateSteps.length : 0}
                  otherWorkflows={workflowItems.filter(
                    (w) => w.id !== workflow.id && !w.id.startsWith("temp-"),
                  )}
                  onUpdateWorkflow={(updates) => onUpdate(workflow.id, updates)}
                  onDeleteWorkflow={async () => {
                    await onDelete(workflow.id);
                  }}
                  onSaveWorkflow={async () => {
                    await onSave(workflow.id);
                  }}
                  onWorkflowCreated={(created) => onCreated(workflow.id, created)}
                />
              </SortableWorkflowItem>
            );
          })}
        </div>
      </SortableContext>
    </DndContext>
  );
}

function WorkflowDialogs({ page }: { page: ReturnType<typeof useWorkspaceWorkflowsPage> }) {
  return (
    <>
      <WorkflowExportDialog
        open={page.isExportDialogOpen}
        onOpenChange={page.setIsExportDialogOpen}
        title="Export Workflows"
        content={page.exportYaml}
      />
      <ImportWorkflowsDialog
        open={page.isImportDialogOpen}
        onOpenChange={page.setIsImportDialogOpen}
        importYaml={page.importYaml}
        onImportYamlChange={page.setImportYaml}
        onFileUpload={page.handleFileUpload}
        fileInputRef={page.fileInputRef}
        onImport={page.handleImport}
        importLoading={page.importLoading}
      />
      <CreateWorkflowDialog
        open={page.isAddWorkflowDialogOpen}
        onOpenChange={page.setIsAddWorkflowDialogOpen}
        workflowName={page.newWorkflowName}
        onWorkflowNameChange={page.setNewWorkflowName}
        selectedTemplateId={page.selectedTemplateId}
        onSelectedTemplateChange={page.setSelectedTemplateId}
        workflowTemplates={page.workflowTemplates}
        onCreate={page.handleCreateWorkflow}
      />
    </>
  );
}

export function WorkspaceWorkflowsClient({
  workspace,
  workflows,
  workflowTemplates,
}: WorkspaceWorkflowsClientProps) {
  const page = useWorkspaceWorkflowsPage(workspace, workflows, workflowTemplates);

  if (!workspace)
    return <WorkspaceNotFoundCard onBack={() => page.router.push("/settings/workspace")} />;

  return (
    <div className="space-y-8">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2 className="text-2xl font-bold">{workspace.name}</h2>
          <p className="text-sm text-muted-foreground mt-1">Manage workflows for this workspace.</p>
        </div>
        <Button asChild variant="outline" size="sm">
          <Link href={`/settings/workspace/${workspace.id}`}>Workspace settings</Link>
        </Button>
      </div>
      <Separator />
      <SettingsSection
        icon={<IconArrowsShuffle className="h-5 w-5" />}
        title="Workflows"
        description="Create autonomous pipelines with automated transitions or manual workflows where you move tasks yourself"
        action={
          <WorkflowSectionActions
            onExport={page.handleExportAll}
            onImport={() => page.setIsImportDialogOpen(true)}
            onAdd={page.handleOpenAddWorkflowDialog}
          />
        }
      >
        <WorkflowList
          workflowItems={page.workflowItems}
          workspaceId={workspace.id}
          templateStepsById={page.templateStepsById}
          isWorkflowDirty={page.isWorkflowDirty}
          onUpdate={page.handleUpdateWorkflow}
          onDelete={page.handleDeleteWorkflow}
          onSave={page.handleSaveWorkflow}
          onCreated={page.handleWorkflowCreated}
          onReorder={page.handleReorderWorkflows}
        />
      </SettingsSection>
      <WorkflowDialogs page={page} />
    </div>
  );
}

function useWorkspaceWorkflowsPage(
  workspace: Workspace | null,
  workflows: Workflow[],
  workflowTemplates: WorkflowTemplate[],
) {
  const router = useRouter();
  const { toast } = useToast();
  const { workflowItems, setWorkflowItems, setSavedWorkflowItems, isWorkflowDirty } =
    useWorkflowSettings(workflows, workspace?.id);

  const importExport = useWorkflowImportExport(workspace, router, toast);
  const {
    isExportDialogOpen,
    setIsExportDialogOpen,
    exportYaml,
    isImportDialogOpen,
    setIsImportDialogOpen,
    importYaml,
    setImportYaml,
    importLoading,
    fileInputRef,
    handleExportAll,
    handleFileUpload,
    handleImport,
  } = importExport;

  const actions = useWorkflowActions({
    workspace,
    workflowItems,
    workflowTemplates,
    setWorkflowItems,
    setSavedWorkflowItems,
  });
  const {
    isAddWorkflowDialogOpen,
    setIsAddWorkflowDialogOpen,
    newWorkflowName,
    setNewWorkflowName,
    selectedTemplateId,
    setSelectedTemplateId,
    handleOpenAddWorkflowDialog,
    handleCreateWorkflow,
    handleUpdateWorkflow,
    handleDeleteWorkflow,
    handleWorkflowCreated,
    handleSaveWorkflow,
  } = actions;

  const handleReorderWorkflows = (reordered: Workflow[]) => {
    setWorkflowItems(reordered);
    setSavedWorkflowItems((prev) => {
      const orderMap = new Map(reordered.map((wf, i) => [wf.id, i]));
      return [...prev].sort((a, b) => (orderMap.get(a.id) ?? 0) - (orderMap.get(b.id) ?? 0));
    });
  };

  const templateStepsById = useMemo(
    () => new Map(workflowTemplates.map((t) => [t.id, t.default_steps ?? []])),
    [workflowTemplates],
  );
  return {
    router,
    workflowItems,
    isWorkflowDirty,
    isExportDialogOpen,
    setIsExportDialogOpen,
    exportYaml,
    isImportDialogOpen,
    setIsImportDialogOpen,
    importYaml,
    setImportYaml,
    importLoading,
    fileInputRef,
    handleExportAll,
    handleFileUpload,
    handleImport,
    isAddWorkflowDialogOpen,
    setIsAddWorkflowDialogOpen,
    newWorkflowName,
    setNewWorkflowName,
    selectedTemplateId,
    setSelectedTemplateId,
    handleOpenAddWorkflowDialog,
    handleCreateWorkflow,
    handleUpdateWorkflow,
    handleDeleteWorkflow,
    handleWorkflowCreated,
    handleSaveWorkflow,
    handleReorderWorkflows,
    workflowTemplates,
    templateStepsById,
  };
}
