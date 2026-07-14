"use client";

import { useMemo, useRef, useState } from "react";
import Link from "@/components/routing/app-link";
import { useRouter } from "@/lib/routing/client-router";
import { IconGripVertical, IconArrowsShuffle } from "@tabler/icons-react";
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
import { WorkflowSectionActions } from "@/components/settings/workflow-section-actions";
import { WorkflowSyncSection } from "@/components/settings/workflow-sync-section";
import { WorkflowExportDialog } from "@/components/settings/workflow-export-dialog";
import { useToast } from "@/components/toast-provider";
import { useWorkflowSettings } from "@/hooks/domains/settings/use-workflow-settings";
import {
  deleteWorkflowAction,
  updateWorkflowAction,
  exportAllWorkflowsAction,
  importWorkflowsAction,
  reorderWorkflowsAction,
} from "@/app/actions/workspaces";
import {
  agentProfileId as toAgentProfileId,
  type Workflow,
  type WorkflowStep,
  type Workspace,
  type WorkflowTemplate,
} from "@/lib/types/http";
import {
  CreateWorkflowDialog,
  ImportWorkflowsDialog,
} from "@/app/settings/workspace/workspace-workflows-dialogs";
import { useWorkflowCreation } from "@/app/settings/workspace/use-workflow-creation";
import { WorkspaceNotFoundCard } from "@/app/settings/workspace/workspace-not-found-card";

type WorkspaceWorkflowsClientProps = {
  workspace: Workspace | null;
  workflows: Workflow[];
  workflowTemplates: WorkflowTemplate[];
};

type WorkflowActionsArgs = {
  workspace: Workspace | null;
  workflowItems: Workflow[];
  workflowTemplates: WorkflowTemplate[];
  setWorkflowItems: React.Dispatch<React.SetStateAction<Workflow[]>>;
  setSavedWorkflowItems: React.Dispatch<React.SetStateAction<Workflow[]>>;
  toast: ReturnType<typeof useToast>["toast"];
};

function useWorkflowImportExport(
  workspace: Workspace | null,
  workflowItems: Workflow[],
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
      // Export only the workflows shown in this settings view (kanban-only —
      // office workflows are filtered out upstream).
      // Workflow import/export is kanban-only by design (ADR-0004).
      const exportIds = workflowItems.map((wf) => wf.id);
      const yamlText = await exportAllWorkflowsAction(workspace.id, exportIds);
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

function brandWorkflowUpdates(updates: {
  name?: string;
  description?: string;
  agent_profile_id?: string;
}): Partial<Workflow> {
  return {
    ...(updates.name !== undefined ? { name: updates.name } : {}),
    ...(updates.description !== undefined ? { description: updates.description } : {}),
    ...(updates.agent_profile_id !== undefined
      ? { agent_profile_id: toAgentProfileId(updates.agent_profile_id) }
      : {}),
  };
}

function hasNewerWorkflowMetadata(current: Workflow, savedFrom: Workflow) {
  return (
    current.name !== savedFrom.name ||
    current.description !== savedFrom.description ||
    current.agent_profile_id !== savedFrom.agent_profile_id
  );
}

function useWorkflowActions({
  workspace,
  workflowItems,
  workflowTemplates,
  setWorkflowItems,
  setSavedWorkflowItems,
  toast,
}: WorkflowActionsArgs) {
  const creation = useWorkflowCreation({
    workspace,
    workflowTemplates,
    setWorkflowItems,
    setSavedWorkflowItems,
    toast,
  });

  const handleUpdateWorkflow = (
    workflowId: string,
    updates: { name?: string; description?: string; agent_profile_id?: string },
  ) => {
    setWorkflowItems((prev) =>
      prev.map((wf) =>
        wf.id === workflowId
          ? {
              ...wf,
              ...updates,
              agent_profile_id:
                updates.agent_profile_id !== undefined
                  ? toAgentProfileId(updates.agent_profile_id)
                  : wf.agent_profile_id,
            }
          : wf,
      ),
    );
  };

  const handleDeleteWorkflow = async (workflowId: string) => {
    await deleteWorkflowAction(workflowId);
    creation.forgetInitialSteps(workflowId);
    setWorkflowItems((prev) => prev.filter((wf) => wf.id !== workflowId));
    setSavedWorkflowItems((prev) => prev.filter((wf) => wf.id !== workflowId));
  };

  const handleSaveWorkflow = async (workflowId: string) => {
    const workflow = workflowItems.find((item) => item.id === workflowId);
    if (!workflow) return;
    const updates: { name?: string; description?: string; agent_profile_id?: string } = {};
    if (workflow.name.trim()) updates.name = workflow.name.trim();
    updates.agent_profile_id = workflow.agent_profile_id ?? "";
    if (Object.keys(updates).length) await updateWorkflowAction(workflowId, updates);
    const branded = brandWorkflowUpdates(updates);
    const savedSnapshot = { ...workflow, ...branded };
    setWorkflowItems((prev) =>
      prev.map((item) =>
        item.id === workflowId && !hasNewerWorkflowMetadata(item, workflow)
          ? { ...item, ...branded }
          : item,
      ),
    );
    setSavedWorkflowItems((prev) =>
      prev.some((item) => item.id === workflowId)
        ? prev.map((item) => (item.id === workflowId ? savedSnapshot : item))
        : [...prev, savedSnapshot],
    );
  };

  return {
    ...creation,
    handleUpdateWorkflow,
    handleDeleteWorkflow,
    handleSaveWorkflow,
  };
}

type WorkflowListProps = {
  workflowItems: Workflow[];
  workspaceId: string;
  initialStepsByWorkflowId: Map<string, WorkflowStep[]>;
  isWorkflowDirty: (wf: Workflow) => boolean;
  onUpdate: (
    id: string,
    u: { name?: string; description?: string; agent_profile_id?: string },
  ) => void;
  onDelete: (id: string) => void;
  onSave: (id: string) => void;
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
        className="absolute left-0 top-6 -ml-6 flex items-center cursor-grab active:cursor-grabbing z-10 sm:-ml-8"
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
  initialStepsByWorkflowId,
  isWorkflowDirty,
  onUpdate,
  onDelete,
  onSave,
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
    const workflowIds = reordered.map((wf) => wf.id);
    if (workflowIds.length > 0) {
      reorderWorkflowsAction(workspaceId, workflowIds).catch(() => {});
    }
  };

  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
      <SortableContext
        items={workflowItems.map((wf) => wf.id)}
        strategy={verticalListSortingStrategy}
      >
        <div className="grid min-w-0 gap-3 pl-6 sm:pl-8">
          {workflowItems.map((workflow) => (
            <SortableWorkflowItem key={workflow.id} workflow={workflow}>
              <WorkflowCard
                workflow={workflow}
                isWorkflowDirty={isWorkflowDirty(workflow)}
                initialWorkflowSteps={initialStepsByWorkflowId.get(workflow.id)}
                otherWorkflows={workflowItems.filter((w) => w.id !== workflow.id)}
                onUpdateWorkflow={(updates) => onUpdate(workflow.id, updates)}
                onDeleteWorkflow={async () => {
                  await onDelete(workflow.id);
                }}
                onSaveWorkflow={async () => {
                  await onSave(workflow.id);
                }}
              />
            </SortableWorkflowItem>
          ))}
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
        createLoading={page.createWorkflowLoading}
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
  const [syncDialogOpen, setSyncDialogOpen] = useState(false);

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
            onGitHubSync={() => setSyncDialogOpen(true)}
          />
        }
      >
        <WorkflowSyncSection
          workspaceId={workspace.id}
          dialogOpen={syncDialogOpen}
          onDialogOpenChange={setSyncDialogOpen}
        />
        <WorkflowList
          workflowItems={page.workflowItems}
          workspaceId={workspace.id}
          initialStepsByWorkflowId={page.initialStepsByWorkflowId}
          isWorkflowDirty={page.isWorkflowDirty}
          onUpdate={page.handleUpdateWorkflow}
          onDelete={page.handleDeleteWorkflow}
          onSave={page.handleSaveWorkflow}
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
  // Workflow settings is kanban-only by design (ADR-0004): office-style
  // workflows are managed from the Office surface and are not importable /
  // exportable here, so we keep them out of this view entirely.
  const kanbanWorkflows = useMemo(
    () => workflows.filter((wf) => wf.style !== "office"),
    [workflows],
  );
  const { workflowItems, setWorkflowItems, setSavedWorkflowItems, isWorkflowDirty } =
    useWorkflowSettings(kanbanWorkflows, workspace?.id);

  const importExport = useWorkflowImportExport(workspace, workflowItems, router, toast);
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
    toast,
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
    createWorkflowLoading,
    initialStepsByWorkflowId,
    handleUpdateWorkflow,
    handleDeleteWorkflow,
    handleSaveWorkflow,
  } = actions;

  const handleReorderWorkflows = (reordered: Workflow[]) => {
    setWorkflowItems(reordered);
    setSavedWorkflowItems((prev) => {
      const orderMap = new Map(reordered.map((wf, i) => [wf.id, i]));
      return [...prev].sort((a, b) => (orderMap.get(a.id) ?? 0) - (orderMap.get(b.id) ?? 0));
    });
  };

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
    createWorkflowLoading,
    initialStepsByWorkflowId,
    handleUpdateWorkflow,
    handleDeleteWorkflow,
    handleSaveWorkflow,
    handleReorderWorkflows,
    workflowTemplates,
  };
}
