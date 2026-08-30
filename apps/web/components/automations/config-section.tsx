"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Label } from "@kandev/ui/label";
import { SettingsFieldLabel } from "@/components/settings/settings-typography";
import { settingsControlClassName } from "@/components/settings/settings-control";
import { useAppStore } from "@/components/state-provider";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { discoverRepositoriesAction } from "@/app/actions/workspaces";
import type { Executor, ExecutorProfile, LocalRepository, Repository } from "@/lib/types/http";
import type { TaskMode } from "@/lib/types/automation";
import type { TaskRepoRow } from "@/components/task-create-dialog-types";
import { WorkflowSelectorRow } from "@/components/workflow-selector-row";
import { WorkspaceRepoChips } from "@/components/task-create-dialog-workspace-repo-chips";
import { AgentSelector, ExecutorProfileSelector } from "@/components/task-create-dialog-selectors";
import {
  useAgentProfileOptions,
  useExecutorProfileOptions,
} from "@/components/task-create-dialog-options";
import { getMultiRepoExecutorDisabledReason } from "@/components/task-create-dialog-multi-repo-guard";
import { useWorkflows } from "@/hooks/use-workflows";
import { useAllWorkflowSnapshots } from "@/hooks/domains/kanban/use-all-workflow-snapshots";
import {
  pickSelectionFromOptionId,
  resolveExecutorType,
  type RepositorySelection,
} from "./automation-repository-selection";

export type { RepositorySelection } from "./automation-repository-selection";

export function getExecutorItemDisabledReason(
  executorType: string | null | undefined,
  repositorySelections: RepositorySelection[],
): string | null {
  if (repositorySelections.length <= 1) return null;
  return getMultiRepoExecutorDisabledReason(executorType);
}

type ConfigSectionProps = {
  workspaceId: string;
  workflowId: string;
  agentProfileId: string;
  executorProfileId: string;
  taskMode?: TaskMode;
  repositorySelections: RepositorySelection[];
  dirtyFields?: {
    workflowId: boolean;
    agentProfileId: boolean;
    executorProfileId: boolean;
    repositorySelections: boolean;
  };
  onWorkflowChange: (id: string) => void;
  onAgentProfileChange: (id: string) => void;
  onExecutorProfileChange: (id: string) => void;
  onRepositoriesChange: (selections: RepositorySelection[]) => void;
};

const CLEAN_FIELDS = {
  workflowId: false,
  agentProfileId: false,
  executorProfileId: false,
  repositorySelections: false,
};

function useDiscoveredRepositories(workspaceId: string) {
  const [items, setItems] = useState<LocalRepository[]>([]);
  useEffect(() => {
    if (!workspaceId) return;
    let cancelled = false;
    discoverRepositoriesAction(workspaceId)
      .then((response) => {
        if (!cancelled) setItems(response.repositories ?? []);
      })
      .catch(() => {
        if (!cancelled) setItems([]);
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId]);
  return items;
}

function executorProfiles(executors: Executor[]): ExecutorProfile[] {
  return executors.flatMap((executor) =>
    (executor.profiles ?? []).map((profile) => ({
      ...profile,
      executor_type: profile.executor_type ?? executor.type,
      executor_name: profile.executor_name ?? executor.name,
    })),
  );
}

function selectionRows(selections: RepositorySelection[]): TaskRepoRow[] {
  return selections.map((selection, index) => ({
    key: selection.key ?? `automation-repository-${index}`,
    repositoryId: selection.kind === "registered" ? selection.id : undefined,
    localPath: selection.kind === "discovered" ? selection.path : undefined,
    branch: selection.branch ?? (selection.kind === "discovered" ? selection.defaultBranch : ""),
  }));
}

function selectionFromValue(
  key: string,
  value: string,
  repositories: Repository[],
  discovered: LocalRepository[],
): RepositorySelection {
  const picked = pickSelectionFromOptionId(value, repositories, discovered);
  return { ...picked, key, branch: "" };
}

function RepositoryAccess({
  workspaceId,
  repositories,
  discovered,
  selections,
  supportsMultiRepo,
  onChange,
}: {
  workspaceId: string;
  repositories: Repository[];
  discovered: LocalRepository[];
  selections: RepositorySelection[];
  supportsMultiRepo: boolean;
  onChange: (selections: RepositorySelection[]) => void;
}) {
  const { t } = useTranslation();
  const rows = selectionRows(selections);
  const updateSelection = (
    key: string,
    update: (item: RepositorySelection) => RepositorySelection,
  ) => onChange(selections.map((item, index) => (rows[index]?.key === key ? update(item) : item)));
  return (
    <div className="space-y-2 sm:col-span-2">
      <div>
        <h3 className="text-sm font-medium">{t("automations:repositoryModeTitle")}</h3>
        <p className="text-xs text-muted-foreground">
          {t("automations:repositoryModeDescription")}
        </p>
      </div>
      <div className="flex min-w-0 flex-wrap items-center gap-2" data-testid="repository-rows">
        <WorkspaceRepoChips
          rows={rows}
          repositories={repositories}
          discoveredRepositories={discovered}
          workspaceId={workspaceId}
          canAddMore={supportsMultiRepo || rows.length === 0}
          addLabel={t("task:addRepository")}
          allowDuplicateRepositories={false}
          onAdd={() =>
            onChange([
              ...selections,
              { kind: "none", key: `automation-repository-${Date.now()}`, branch: "" },
            ])
          }
          onRemove={(key) => onChange(selections.filter((_, index) => rows[index]?.key !== key))}
          onRowRepositoryChange={(key, value) =>
            updateSelection(key, () => selectionFromValue(key, value, repositories, discovered))
          }
          onRowBranchChange={(key, branch) =>
            updateSelection(key, (item) => ({ ...item, key, branch }))
          }
        />
      </div>
      {rows.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          {t("automations:repositoryModeNoneDescription")}
        </p>
      ) : null}
    </div>
  );
}

export function ConfigSection({
  workspaceId,
  workflowId,
  agentProfileId,
  executorProfileId,
  taskMode = "automation_run",
  repositorySelections,
  dirtyFields = CLEAN_FIELDS,
  onWorkflowChange,
  onAgentProfileChange,
  onExecutorProfileChange,
  onRepositoriesChange,
}: ConfigSectionProps) {
  const { t } = useTranslation();
  useSettingsData(true);
  const { workflows: allWorkflows } = useWorkflows(workspaceId);
  useAllWorkflowSnapshots(workspaceId);
  const workflows = useMemo(
    () => allWorkflows.filter((workflow) => workflow.workspaceId === workspaceId),
    [allWorkflows, workspaceId],
  );
  const snapshots = useAppStore((state) => state.kanbanMulti.snapshots);
  const { repositories } = useRepositories(workspaceId, true);
  const discovered = useDiscoveredRepositories(workspaceId);
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  const executors = useAppStore((state) => state.executors.items);
  const agents = useAgentProfileOptions(agentProfiles);
  const allExecutorProfiles = useMemo(() => executorProfiles(executors), [executors]);
  const disabledReasonFor = useCallback(
    (profile: ExecutorProfile) =>
      getExecutorItemDisabledReason(profile.executor_type, repositorySelections),
    [repositorySelections],
  );
  const executorOptions = useExecutorProfileOptions(allExecutorProfiles, { disabledReasonFor });
  const supportsMultiRepo =
    getMultiRepoExecutorDisabledReason(resolveExecutorType(executors, executorProfileId)) === null;

  return (
    <div className="space-y-3">
      <Label className="text-xs uppercase tracking-wider text-muted-foreground">
        {t("automations:configurationLabel")}
      </Label>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div className="space-y-1.5 sm:col-span-2" data-settings-dirty={dirtyFields.workflowId}>
          <SettingsFieldLabel>{t("automations:workflowLabel")}</SettingsFieldLabel>
          <WorkflowSelectorRow
            workflows={workflows}
            snapshots={snapshots}
            selectedWorkflowId={workflowId || null}
            onWorkflowChange={onWorkflowChange}
            agentProfiles={agentProfiles}
            clearLabel={taskMode === "normal_task" ? undefined : t("automations:noWorkflow")}
            placeholder={
              taskMode === "normal_task"
                ? t("automations:selectWorkflowRequired")
                : t("automations:selectWorkflowOptional")
            }
          />
        </div>
        <div className="space-y-1.5" data-settings-dirty={dirtyFields.agentProfileId}>
          <SettingsFieldLabel>{t("automations:agentProfileLabel")}</SettingsFieldLabel>
          <AgentSelector
            options={agents}
            value={agentProfileId}
            onValueChange={onAgentProfileChange}
            disabled={false}
            placeholder={t("automations:agentProfilePlaceholder")}
            triggerClassName={settingsControlClassName()}
          />
        </div>
        <div className="space-y-1.5" data-settings-dirty={dirtyFields.executorProfileId}>
          <SettingsFieldLabel>{t("automations:executorProfileLabel")}</SettingsFieldLabel>
          <ExecutorProfileSelector
            options={executorOptions}
            value={executorProfileId}
            onValueChange={onExecutorProfileChange}
            disabled={false}
            placeholder={t("automations:executorProfilePlaceholder")}
            triggerClassName={settingsControlClassName()}
          />
        </div>
        <RepositoryAccess
          workspaceId={workspaceId}
          repositories={repositories}
          discovered={discovered}
          selections={repositorySelections}
          supportsMultiRepo={supportsMultiRepo}
          onChange={onRepositoriesChange}
        />
      </div>
    </div>
  );
}
