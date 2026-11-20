"use client";

import { useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { t as translate } from "@/lib/i18n";
import { useRouter } from "@/lib/routing/client-router";
import { IconArrowsShuffle } from "@tabler/icons-react";
import { SettingsSection } from "@/components/settings/settings-section";
import {
  WorkflowList,
  type WorkflowSavedParams,
} from "@/app/settings/workspace/workspace-workflows-list";
import { WorkflowSectionActions } from "@/components/settings/workflow-section-actions";
import { WorkflowSyncSection } from "@/components/settings/workflow-sync-section";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { useToast } from "@/components/toast-provider";
import { useWorkflowSettings } from "@/hooks/domains/settings/use-workflow-settings";
import {
  deleteWorkflowAction,
  exportAllWorkflowsAction,
  reorderWorkflowsAction,
} from "@/app/actions/workspaces";
import {
  agentProfileId as toAgentProfileId,
  type Workflow,
  type Workspace,
  type WorkflowTemplate,
} from "@/lib/types/http";
import { WorkflowDialogs } from "@/app/settings/workspace/workspace-workflows-dialogs";
import { useWorkflowCreation } from "@/app/settings/workspace/use-workflow-creation";
import { useWorkflowImport } from "@/app/settings/workspace/use-workflow-import";
import { WorkspaceNotFoundCard } from "@/app/settings/workspace/workspace-not-found-card";

type WorkspaceWorkflowsClientProps = {
  workspace: Workspace | null;
  workflows: Workflow[];
  workflowTemplates: WorkflowTemplate[];
  /** The dedicated Improve Kandev workspace is configuration-immutable. */
  isImproveWorkspace?: boolean;
};

const TEMP_WORKFLOW_PREFIX = "temp-workflow-";

type WorkflowActionsArgs = {
  workspace: Workspace | null;
  workflowItems: Workflow[];
  savedWorkflowItems: Workflow[];
  workflowTemplates: WorkflowTemplate[];
  setWorkflowItems: React.Dispatch<React.SetStateAction<Workflow[]>>;
  setSavedWorkflowItems: React.Dispatch<React.SetStateAction<Workflow[]>>;
};

function useWorkflowImportExport(
  workspace: Workspace | null,
  workflowItems: Workflow[],
  router: ReturnType<typeof useRouter>,
  toast: ReturnType<typeof useToast>["toast"],
) {
  const [isExportDialogOpen, setIsExportDialogOpen] = useState(false);
  const [exportYaml, setExportYaml] = useState("");

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
        title: translate("workflows:failedToExportWorkflows"),
        description: error instanceof Error ? error.message : translate("common:requestFailed"),
        variant: "error",
      });
    }
  };

  return {
    isExportDialogOpen,
    setIsExportDialogOpen,
    exportYaml,
    handleExportAll,
  };
}

export function hasNewerWorkflowMetadata(current: Workflow, savedFrom: Workflow) {
  return (
    current.name !== savedFrom.name ||
    (current.description ?? "") !== (savedFrom.description ?? "") ||
    (current.prompt ?? "") !== (savedFrom.prompt ?? "") ||
    (current.agent_profile_id ?? "") !== (savedFrom.agent_profile_id ?? "")
  );
}

function mergeSavedWorkflow(current: Workflow, submitted: Workflow, saved: Workflow): Workflow {
  if (!hasNewerWorkflowMetadata(current, submitted)) return saved;
  return { ...current, id: saved.id, workflow_template_id: saved.workflow_template_id };
}

function useWorkflowActions({
  workspace,
  workflowItems,
  savedWorkflowItems,
  workflowTemplates,
  setWorkflowItems,
  setSavedWorkflowItems,
}: WorkflowActionsArgs) {
  const finalizedWorkflowIdsRef = useRef(new Map<string, string>());
  const creation = useWorkflowCreation({
    workspace,
    workflowItems,
    workflowTemplates,
    setWorkflowItems,
  });

  const handleUpdateWorkflow = (
    workflowId: string,
    updates: {
      name?: string;
      description?: string;
      prompt?: string;
      agent_profile_id?: string;
    },
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
    if (!workflowId.startsWith(TEMP_WORKFLOW_PREFIX)) await deleteWorkflowAction(workflowId);
    creation.forgetInitialSteps(workflowId);
    finalizedWorkflowIdsRef.current.delete(workflowId);
    setWorkflowItems((prev) => prev.filter((wf) => wf.id !== workflowId));
    setSavedWorkflowItems((prev) => prev.filter((wf) => wf.id !== workflowId));
  };

  const handleWorkflowSaved = ({
    clientWorkflow,
    submittedWorkflow,
    savedWorkflow,
    currentSteps,
    finalizeIdentity,
  }: WorkflowSavedParams) => {
    const isNew = clientWorkflow.id.startsWith(TEMP_WORKFLOW_PREFIX);
    if (isNew && !finalizeIdentity) return;
    if (isNew) finalizedWorkflowIdsRef.current.set(clientWorkflow.id, savedWorkflow.id);
    setWorkflowItems((prev) =>
      prev.map((item) =>
        item.id === clientWorkflow.id
          ? mergeSavedWorkflow(item, submittedWorkflow, savedWorkflow)
          : item,
      ),
    );
    setSavedWorkflowItems((previous) =>
      alignSavedWorkflowsToDraftOrder(
        workflowItems,
        [...previous.filter((item) => item.id !== savedWorkflow.id), savedWorkflow],
        finalizedWorkflowIdsRef.current,
      ),
    );
    if (isNew) {
      creation.remapInitialSteps(clientWorkflow.id, savedWorkflow.id, currentSteps);
    }
  };

  const handleDiscardWorkflow = (workflowId: string) => {
    creation.forgetInitialSteps(workflowId);
    finalizedWorkflowIdsRef.current.delete(workflowId);
    if (workflowId.startsWith(TEMP_WORKFLOW_PREFIX)) {
      setWorkflowItems((previous) => previous.filter((item) => item.id !== workflowId));
      return;
    }
    const saved = savedWorkflowItems.find((item) => item.id === workflowId);
    if (saved) {
      setWorkflowItems((previous) =>
        previous.map((item) => (item.id === workflowId ? saved : item)),
      );
    }
  };

  return {
    ...creation,
    locallyFinalizedWorkflowIds: new Set(finalizedWorkflowIdsRef.current.values()),
    handleUpdateWorkflow,
    handleDeleteWorkflow,
    handleWorkflowSaved,
    handleDiscardWorkflow,
  };
}

export function alignSavedWorkflowsToDraftOrder(
  drafts: Workflow[],
  saved: Workflow[],
  finalizedWorkflowIds: ReadonlyMap<string, string>,
): Workflow[] {
  const savedById = new Map<string, Workflow>(saved.map((workflow) => [workflow.id, workflow]));
  return drafts.flatMap((draft) => {
    const persistedId = finalizedWorkflowIds.get(draft.id) ?? draft.id;
    const baseline = savedById.get(persistedId);
    return baseline ? [baseline] : [];
  });
}

export function WorkspaceWorkflowsClient({
  workspace,
  workflows,
  workflowTemplates,
  isImproveWorkspace = false,
}: WorkspaceWorkflowsClientProps) {
  const { t } = useTranslation();
  const page = useWorkspaceWorkflowsPage(workspace, workflows, workflowTemplates);
  const [syncDialogOpen, setSyncDialogOpen] = useState(false);

  if (!workspace)
    return <WorkspaceNotFoundCard onBack={() => page.router.push("/settings/workspaces")} />;

  return (
    <div className="space-y-8">
      {/* No section header — see the Repositories tab: the section below is
          already named, marked and described. */}
      <SettingsSection
        divided
        framed={false}
        icon={<IconArrowsShuffle className="h-5 w-5" />}
        title={t("workflows:workflows")}
        // The read-only note is the section's description here rather than a
        // heading above it: the duplicated heading is gone, and this workspace
        // being immutable is the one thing the static description omits.
        description={
          isImproveWorkspace
            ? t("workflows:workflowsReadOnlyImprove")
            : t("workflows:workflowsSectionDescription")
        }
        action={
          isImproveWorkspace ? undefined : (
            <WorkflowSectionActions
              onExport={page.handleExportAll}
              onImport={() => page.setIsImportDialogOpen(true)}
              onAdd={page.handleOpenAddWorkflowDialog}
              onGitHubSync={() => setSyncDialogOpen(true)}
            />
          )
        }
      >
        {/* The unframed section content starts at its divider; pt-4 keeps the
            first block (sync status or the list view control) off the line. */}
        <div className="pt-4">
          {!isImproveWorkspace && (
            <WorkflowSyncSection
              workspaceId={workspace.id}
              dialogOpen={syncDialogOpen}
              onDialogOpenChange={setSyncDialogOpen}
            />
          )}
          <WorkflowList
            workflowItems={page.workflowItems}
            savedWorkflowItems={page.savedWorkflowItems}
            orderDirtyIds={page.workflowOrderDirtyIds}
            initialStepsByWorkflowId={page.initialStepsByWorkflowId}
            locallyFinalizedWorkflowIds={page.locallyFinalizedWorkflowIds}
            isWorkflowDirty={page.isWorkflowDirty}
            isImproveWorkspace={isImproveWorkspace}
            onUpdate={page.handleUpdateWorkflow}
            onDelete={page.handleDeleteWorkflow}
            onDuplicate={page.handleDuplicateWorkflow}
            onWorkflowSaved={page.handleWorkflowSaved}
            onDiscard={page.handleDiscardWorkflow}
            onReorder={page.handleReorderWorkflows}
          />
        </div>
      </SettingsSection>
      <WorkflowDialogs page={page} />
    </div>
  );
}

type WorkflowOrderDraftArgs = {
  workspace: Workspace | null;
  workflowItems: Workflow[];
  savedWorkflowItems: Workflow[];
  setWorkflowItems: React.Dispatch<React.SetStateAction<Workflow[]>>;
  setSavedWorkflowItems: React.Dispatch<React.SetStateAction<Workflow[]>>;
  idMappings: React.RefObject<Map<string, string>>;
};

export function getWorkflowOrderDirtyIds(
  workflows: Workflow[],
  savedWorkflows: Workflow[],
): ReadonlySet<string> {
  const savedPositions = new Map(
    savedWorkflows.map((workflow, position) => [workflow.id, position]),
  );
  return new Set(
    workflows.flatMap((workflow, position) =>
      savedPositions.get(workflow.id) === position ? [] : [workflow.id],
    ),
  );
}

function useWorkflowOrderDraft({
  workspace,
  workflowItems,
  savedWorkflowItems,
  setWorkflowItems,
  setSavedWorkflowItems,
  idMappings,
}: WorkflowOrderDraftArgs) {
  const currentOrder = workflowItems.map((workflow) => workflow.id);
  const savedOrder = savedWorkflowItems.map((workflow) => workflow.id);
  const dirtyIds = getWorkflowOrderDirtyIds(workflowItems, savedWorkflowItems);
  useSettingsSaveContributor({
    id: `workflow-order:${workspace?.id ?? "missing"}`,
    order: 1000,
    revision: currentOrder.join("\0"),
    isDirty: currentOrder.join("\0") !== savedOrder.join("\0"),
    save: async () => {
      if (!workspace) return;
      const persistedOrder = currentOrder.map((id) => idMappings.current.get(id) ?? id);
      if (persistedOrder.some((id) => id.startsWith(TEMP_WORKFLOW_PREFIX))) {
        throw new Error(translate("workflows:saveWorkflowsBeforeOrdering"));
      }
      if (persistedOrder.length > 0) {
        await reorderWorkflowsAction(workspace.id, persistedOrder);
      }
      setSavedWorkflowItems((previous) => sortWorkflows(previous, persistedOrder));
    },
    discard: () => setWorkflowItems(savedWorkflowItems),
  });
  return dirtyIds;
}

function sortWorkflows(workflows: Workflow[], order: string[]): Workflow[] {
  const positions = new Map(order.map((id, position) => [id, position]));
  return [...workflows].sort(
    (left, right) =>
      (positions.get(left.id) ?? Number.MAX_SAFE_INTEGER) -
      (positions.get(right.id) ?? Number.MAX_SAFE_INTEGER),
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
  const {
    workflowItems,
    setWorkflowItems,
    savedWorkflowItems,
    setSavedWorkflowItems,
    isWorkflowDirty,
  } = useWorkflowSettings(kanbanWorkflows, workspace?.id);
  const workflowIdMappingsRef = useRef(new Map<string, string>());

  const importExport = useWorkflowImportExport(workspace, workflowItems, router, toast);
  const importState = useWorkflowImport({ workspace, router, toast });
  const actions = useWorkflowActions({
    workspace,
    workflowItems,
    savedWorkflowItems,
    workflowTemplates,
    setWorkflowItems,
    setSavedWorkflowItems,
  });

  const handleReorderWorkflows = (reordered: Workflow[]) => {
    setWorkflowItems(reordered);
  };

  const handleWorkflowSaved = (params: WorkflowSavedParams) => {
    if (params.clientWorkflow.id !== params.savedWorkflow.id) {
      workflowIdMappingsRef.current.set(params.clientWorkflow.id, params.savedWorkflow.id);
    }
    actions.handleWorkflowSaved(params);
  };

  const workflowOrderDirtyIds = useWorkflowOrderDraft({
    workspace,
    workflowItems,
    savedWorkflowItems,
    setWorkflowItems,
    setSavedWorkflowItems,
    idMappings: workflowIdMappingsRef,
  });

  return {
    router,
    workflowItems,
    savedWorkflowItems,
    workflowOrderDirtyIds,
    isWorkflowDirty,
    ...importExport,
    ...importState,
    ...actions,
    handleWorkflowSaved,
    handleReorderWorkflows,
    workflowTemplates,
  };
}
