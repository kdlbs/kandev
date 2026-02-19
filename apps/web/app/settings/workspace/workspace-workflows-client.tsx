"use client";

import { useMemo, useRef, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { IconDownload, IconLayoutColumns, IconPlus, IconUpload } from "@tabler/icons-react";
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
} from "@/app/actions/workspaces";
import type {
  Workflow,
  StepDefinition,
  WorkflowStep,
  Workspace,
  WorkflowTemplate,
  WorkflowExportData,
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
  router: ReturnType<typeof useRouter>;
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
  const [exportJson, setExportJson] = useState("");
  const [isImportDialogOpen, setIsImportDialogOpen] = useState(false);
  const [importJson, setImportJson] = useState("");
  const [importLoading, setImportLoading] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const handleExportAll = async () => {
    if (!workspace) return;
    try {
      const data = await exportAllWorkflowsAction(workspace.id);
      setExportJson(JSON.stringify(data, null, 2));
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
      setImportJson(event.target?.result as string);
    };
    reader.readAsText(file);
    e.target.value = "";
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
      if (created.length > 0) parts.push(`Created: ${created.join(", ")}`);
      if (skipped.length > 0) parts.push(`Skipped (already exist): ${skipped.join(", ")}`);
      toast({ title: "Import complete", description: parts.join(". ") });
      setIsImportDialogOpen(false);
      setImportJson("");
      if (created.length > 0) router.refresh();
    } catch (error) {
      toast({
        title: "Failed to import workflows",
        description: error instanceof Error ? error.message : "Invalid JSON",
        variant: "error",
      });
    } finally {
      setImportLoading(false);
    }
  };

  return {
    isExportDialogOpen,
    setIsExportDialogOpen,
    exportJson,
    isImportDialogOpen,
    setIsImportDialogOpen,
    importJson,
    setImportJson,
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
  router,
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
    updates: { name?: string; description?: string },
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
    router.refresh();
  };

  const handleSaveWorkflow = async (workflowId: string) => {
    const workflow = workflowItems.find((item) => item.id === workflowId);
    if (!workflow) return;
    const updates: { name?: string; description?: string } = {};
    if (workflow.name.trim()) updates.name = workflow.name.trim();
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
      <Button size="sm" onClick={onAdd} className="cursor-pointer">
        <IconPlus className="h-4 w-4 mr-2" />
        Add Workflow
      </Button>
    </div>
  );
}

type WorkflowListProps = {
  workflowItems: Workflow[];
  templateStepsById: Map<string, StepDefinition[]>;
  isWorkflowDirty: (wf: Workflow) => boolean;
  onUpdate: (id: string, u: { name?: string; description?: string }) => void;
  onDelete: (id: string) => void;
  onSave: (id: string) => void;
  onCreated: (id: string, wf: Workflow) => void;
};

function WorkflowList({
  workflowItems,
  templateStepsById,
  isWorkflowDirty,
  onUpdate,
  onDelete,
  onSave,
  onCreated,
}: WorkflowListProps) {
  return (
    <div className="grid gap-3">
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
          <WorkflowCard
            key={workflow.id}
            workflow={workflow}
            isWorkflowDirty={isWorkflowDirty(workflow)}
            initialWorkflowSteps={initialWorkflowSteps}
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
        );
      })}
    </div>
  );
}

export function WorkspaceWorkflowsClient({
  workspace,
  workflows,
  workflowTemplates,
}: WorkspaceWorkflowsClientProps) {
  const page = useWorkspaceWorkflowsPage(workspace, workflows, workflowTemplates);
  const {
    router,
    workflowItems,
    isWorkflowDirty,
    isExportDialogOpen,
    setIsExportDialogOpen,
    exportJson,
    isImportDialogOpen,
    setIsImportDialogOpen,
    importJson,
    setImportJson,
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
    templateStepsById,
  } = page;

  if (!workspace) return <WorkspaceNotFoundCard onBack={() => router.push("/settings/workspace")} />;

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
        icon={<IconLayoutColumns className="h-5 w-5" />}
        title="Workflows"
        description="Create autonomous pipelines with automated transitions or manual workflows where you move tasks yourself"
        action={
          <WorkflowSectionActions
            onExport={handleExportAll}
            onImport={() => setIsImportDialogOpen(true)}
            onAdd={handleOpenAddWorkflowDialog}
          />
        }
      >
        <WorkflowList
          workflowItems={workflowItems}
          templateStepsById={templateStepsById}
          isWorkflowDirty={isWorkflowDirty}
          onUpdate={handleUpdateWorkflow}
          onDelete={handleDeleteWorkflow}
          onSave={handleSaveWorkflow}
          onCreated={handleWorkflowCreated}
        />
      </SettingsSection>
      <WorkflowExportDialog
        open={isExportDialogOpen}
        onOpenChange={setIsExportDialogOpen}
        title="Export Workflows"
        json={exportJson}
      />
      <ImportWorkflowsDialog
        open={isImportDialogOpen}
        onOpenChange={setIsImportDialogOpen}
        importJson={importJson}
        onImportJsonChange={setImportJson}
        onFileUpload={handleFileUpload}
        fileInputRef={fileInputRef}
        onImport={handleImport}
        importLoading={importLoading}
      />
      <CreateWorkflowDialog
        open={isAddWorkflowDialogOpen}
        onOpenChange={setIsAddWorkflowDialogOpen}
        workflowName={newWorkflowName}
        onWorkflowNameChange={setNewWorkflowName}
        selectedTemplateId={selectedTemplateId}
        onSelectedTemplateChange={setSelectedTemplateId}
        workflowTemplates={workflowTemplates}
        onCreate={handleCreateWorkflow}
      />
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
    useWorkflowSettings(workflows);

  const importExport = useWorkflowImportExport(workspace, router, toast);
  const {
    isExportDialogOpen,
    setIsExportDialogOpen,
    exportJson,
    isImportDialogOpen,
    setIsImportDialogOpen,
    importJson,
    setImportJson,
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
    router,
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

  const templateStepsById = useMemo(
    () => new Map(workflowTemplates.map((t) => [t.id, t.default_steps ?? []])),
    [workflowTemplates],
  );
  return (
    {
      router,
      workflowItems,
      isWorkflowDirty,
      isExportDialogOpen,
      setIsExportDialogOpen,
      exportJson,
      isImportDialogOpen,
      setIsImportDialogOpen,
      importJson,
      setImportJson,
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
      templateStepsById,
    }
  );
}
