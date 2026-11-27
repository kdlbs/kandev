"use client";

import { useCallback, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { SettingsFieldLabel } from "@/components/settings/settings-typography";
import { settingsControlClassName } from "@/components/settings/settings-control";
import { useAppStore } from "@/components/state-provider";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useSecrets } from "@/hooks/domains/settings/use-secrets";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { useRepositoryDiscovery } from "@/hooks/domains/workspace/use-repository-discovery";
import type { Executor, ExecutorProfile, LocalRepository, Repository } from "@/lib/types/http";
import type { Automation, PersonalCredentialBindingInput, TaskMode } from "@/lib/types/automation";
import type { TaskRepoRow } from "@/components/task-create-dialog-types";
import {
  AutomationCredentialErrorMessage,
  type AutomationCredentialError,
} from "./automation-credential-error";
import { WorkflowSelectorRow } from "@/components/workflow-selector-row";
import { WorkspaceRepoChips } from "@/components/task-create-dialog-workspace-repo-chips";
import { AgentSelector, ExecutorProfileSelector } from "@/components/task-create-dialog-selectors";
import {
  useAgentProfileOptions,
  useExecutorProfileOptions,
} from "@/components/task-create-dialog-options";
import { getMultiRepoExecutorDisabledReason } from "@/components/task-create-dialog-multi-repo-guard";
import {
  PersonalCredentialFields,
  WorkspaceCredentialChoices,
} from "./automation-credential-choice-controls";
import { useWorkflows } from "@/hooks/use-workflows";
import { useAllWorkflowSnapshots } from "@/hooks/domains/kanban/use-all-workflow-snapshots";
import {
  pickSelectionFromOptionId,
  resolveExecutorType,
  type RepositorySelection,
} from "./automation-repository-selection";

export type { RepositorySelection } from "./automation-repository-selection";
export { supportsPersonalCredentialChoices } from "./automation-credential-eligibility";

export function getExecutorItemDisabledReason(
  executorType: string | null | undefined,
  repositorySelections: RepositorySelection[],
): string | null {
  if (repositorySelections.length <= 1) return null;
  return getMultiRepoExecutorDisabledReason(executorType);
}

type ConfigSectionProps = {
  workspaceId: string;
  automation: Automation | null;
  onAutomationChange: (automation: Automation) => void;
  agentProfileId: string;
  executorProfileId: string;
  credentialChoicesEligible: boolean;
  repositorySelections: RepositorySelection[];
  dirtyFields?: {
    agentProfileId: boolean;
    executorProfileId: boolean;
    repositorySelections: boolean;
  };
  onAgentProfileChange: (id: string) => void;
  onExecutorProfileChange: (id: string) => void;
  onRepositoriesChange: (selections: RepositorySelection[]) => void;
  credentialError?: AutomationCredentialError | null;
  personalCredentialBindings: PersonalCredentialBindingInput[];
  onPersonalCredentialBindingsChange: (bindings: PersonalCredentialBindingInput[]) => void;
};

type CredentialProfileProps = {
  profileId: string;
  dirty: boolean;
  credentialError: AutomationCredentialError | null | undefined;
  fields: { key: string; secretId: string }[];
  bindings: PersonalCredentialBindingInput[];
  choices: NonNullable<Automation["credential_choices"]>;
  secrets: ReturnType<typeof useSecrets>["items"];
  secretsLoaded: boolean;
  canEdit: boolean;
  eligible: boolean;
  onChange: (bindings: PersonalCredentialBindingInput[]) => void;
};

const CLEAN_FIELDS = {
  agentProfileId: false,
  executorProfileId: false,
  repositorySelections: false,
};

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
  automation,
  onAutomationChange,
  repositories,
  discovered,
  selections,
  supportsMultiRepo,
  onChange,
  credentialError,
  isWorkspaceOwner,
}: {
  workspaceId: string;
  automation: Automation | null;
  onAutomationChange: (automation: Automation) => void;
  repositories: Repository[];
  discovered: LocalRepository[];
  selections: RepositorySelection[];
  supportsMultiRepo: boolean;
  onChange: (selections: RepositorySelection[]) => void;
  credentialError?: AutomationCredentialError | null;
  isWorkspaceOwner: boolean;
}) {
  const { t } = useTranslation();
  const rows = selectionRows(selections);
  const updateSelection = (
    key: string,
    update: (item: RepositorySelection) => RepositorySelection,
  ) => onChange(selections.map((item, index) => (rows[index]?.key === key ? update(item) : item)));
  return (
    <div className="space-y-2 sm:col-span-2" data-credential-field="repositorySelections">
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
          showDiscoveryControls
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
      {credentialError?.target === "repositorySelections" ? (
        <AutomationCredentialErrorMessage
          error={credentialError}
          id="automation-repository-credential-error"
        />
      ) : null}
      <WorkspaceCredentialChoices
        automation={automation}
        repositories={repositories}
        selectedRepositoryIds={selections.flatMap((selection) =>
          selection.kind === "registered" ? [selection.id] : [],
        )}
        isWorkspaceOwner={isWorkspaceOwner}
        onAutomationChange={onAutomationChange}
      />
    </div>
  );
}

function credentialFieldsForProfile(
  profileId: string,
  profileFields: { key: string; secret_id?: string | null }[],
  bindings: PersonalCredentialBindingInput[],
  choices: Automation["credential_choices"],
) {
  const fields = new Map<string, string>();
  for (const field of profileFields) {
    if (field.secret_id) fields.set(field.key, field.secret_id);
  }
  for (const binding of bindings) {
    if (binding.profile_id === profileId && !fields.has(binding.key)) fields.set(binding.key, "");
  }
  for (const choice of choices ?? []) {
    if (choice.kind === "personal" && choice.profile_id === profileId && !fields.has(choice.key)) {
      fields.set(choice.key, "");
    }
  }
  return [...fields].map(([key, secretId]) => ({ key, secretId }));
}

type ConfigSectionDataInput = Pick<
  ConfigSectionProps,
  | "workspaceId"
  | "automation"
  | "agentProfileId"
  | "executorProfileId"
  | "credentialChoicesEligible"
  | "personalCredentialBindings"
  | "repositorySelections"
>;

function useConfigSectionData({
  workspaceId,
  automation,
  agentProfileId,
  executorProfileId,
  credentialChoicesEligible,
  personalCredentialBindings,
  repositorySelections,
}: ConfigSectionDataInput) {
  useSettingsData(true);
  const { repositories } = useRepositories(workspaceId, true);
  const discovered = useRepositoryDiscovery(workspaceId, true).repositories;
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  const settingsAgents = useAppStore((state) => state.settingsAgents.items);
  const userId = useAppStore((state) => state.auth.user?.id);
  const workspace = useAppStore((state) =>
    state.workspaces.items.find((item) => item.id === workspaceId),
  );
  const isWorkspaceOwner = workspace?.viewer_role === "owner" || workspace?.owner_id === userId;
  const globalSecrets = useSecrets("global");
  const executors = useAppStore((state) => state.executors.items);
  const agents = useAgentProfileOptions(agentProfiles);
  const allExecutorProfiles = useMemo(() => executorProfiles(executors), [executors]);
  const agentProfileFields =
    settingsAgents
      .flatMap((agent) => agent.profiles)
      .find((profile) => profile.id === agentProfileId)?.envVars ?? [];
  const selectedExecutorProfile = allExecutorProfiles.find(
    (profile) => profile.id === executorProfileId,
  );
  const choices = automation?.credential_choices;
  const agentCredentialFields = useMemo(
    () =>
      credentialFieldsForProfile(
        agentProfileId,
        agentProfileFields,
        personalCredentialBindings,
        choices,
      ),
    [agentProfileFields, agentProfileId, choices, personalCredentialBindings],
  );
  const executorCredentialFields = useMemo(
    () =>
      credentialFieldsForProfile(
        executorProfileId,
        selectedExecutorProfile?.env_vars ?? [],
        personalCredentialBindings,
        choices,
      ),
    [choices, executorProfileId, personalCredentialBindings, selectedExecutorProfile],
  );
  const disabledReasonFor = useCallback(
    (profile: ExecutorProfile) =>
      getExecutorItemDisabledReason(profile.executor_type, repositorySelections),
    [repositorySelections],
  );
  const executorOptions = useExecutorProfileOptions(allExecutorProfiles, { disabledReasonFor });
  const supportsMultiRepo =
    getMultiRepoExecutorDisabledReason(resolveExecutorType(executors, executorProfileId)) === null;

  return {
    repositories,
    discovered,
    globalSecrets,
    agents,
    executorOptions,
    supportsMultiRepo,
    agentCredentialFields,
    executorCredentialFields,
    credentialChoices: choices ?? [],
    canEditCredentialChoices:
      (automation?.can_manage_credential_choices ?? true) && credentialChoicesEligible,
    isWorkspaceOwner,
  };
}

function ProfileCredentialControls({
  target,
  profileId,
  credentialError,
  fields,
  bindings,
  choices,
  secrets,
  secretsLoaded,
  canEdit,
  eligible,
  onChange,
}: Omit<CredentialProfileProps, "dirty"> & { target: "agentProfileId" | "executorProfileId" }) {
  const errorId = `automation-${target === "agentProfileId" ? "agent" : "executor"}-credential-error`;
  return (
    <>
      {credentialError?.target === target ? (
        <AutomationCredentialErrorMessage error={credentialError} id={errorId} />
      ) : null}
      {eligible ? (
        <PersonalCredentialFields
          profileId={profileId}
          fields={fields}
          bindings={bindings}
          choices={choices}
          secrets={secrets}
          loaded={secretsLoaded}
          canEdit={canEdit}
          onChange={onChange}
        />
      ) : null}
    </>
  );
}

export function WorkflowField({
  workspaceId,
  workflowId,
  taskMode,
  isDirty = false,
  onWorkflowChange,
}: {
  workspaceId: string;
  workflowId: string;
  taskMode: TaskMode;
  isDirty?: boolean;
  onWorkflowChange: (id: string) => void;
}) {
  const { t } = useTranslation();
  const { workflows: allWorkflows } = useWorkflows(workspaceId);
  useAllWorkflowSnapshots(workspaceId);
  const workflows = useMemo(
    () => allWorkflows.filter((workflow) => workflow.workspaceId === workspaceId),
    [allWorkflows, workspaceId],
  );
  const snapshots = useAppStore((state) => state.kanbanMulti.snapshots);
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  const required = taskMode === "normal_task";
  return (
    <div className="space-y-1.5" data-settings-dirty={isDirty}>
      <SettingsFieldLabel>{t("automations:workflowLabel")}</SettingsFieldLabel>
      <WorkflowSelectorRow
        workflows={workflows}
        snapshots={snapshots}
        selectedWorkflowId={workflowId || null}
        onWorkflowChange={onWorkflowChange}
        agentProfiles={agentProfiles}
        clearLabel={required ? undefined : t("automations:noWorkflow")}
        placeholder={
          required
            ? t("automations:selectWorkflowRequired")
            : t("automations:selectWorkflowOptional")
        }
      />
      {required ? (
        <p className="text-xs text-muted-foreground">{t("automations:normalTaskWorkflowHelp")}</p>
      ) : null}
    </div>
  );
}

function AgentCredentialProfile({
  dirty,
  profileId,
  options,
  onValueChange,
  ...credentials
}: CredentialProfileProps & {
  options: ReturnType<typeof useAgentProfileOptions>;
  onValueChange: (id: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1.5" data-settings-dirty={dirty} data-credential-field="agentProfileId">
      <SettingsFieldLabel>{t("automations:agentProfileLabel")}</SettingsFieldLabel>
      <AgentSelector
        options={options}
        value={profileId}
        onValueChange={onValueChange}
        disabled={false}
        placeholder={t("automations:agentProfilePlaceholder")}
        triggerClassName={settingsControlClassName()}
        ariaDescribedBy={
          credentials.credentialError?.target === "agentProfileId"
            ? "automation-agent-credential-error"
            : undefined
        }
      />
      <ProfileCredentialControls target="agentProfileId" profileId={profileId} {...credentials} />
    </div>
  );
}

function ExecutorCredentialProfile({
  dirty,
  profileId,
  options,
  onValueChange,
  ...credentials
}: CredentialProfileProps & {
  options: ReturnType<typeof useExecutorProfileOptions>;
  onValueChange: (id: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div
      className="space-y-1.5"
      data-settings-dirty={dirty}
      data-credential-field="executorProfileId"
    >
      <SettingsFieldLabel>{t("automations:executorProfileLabel")}</SettingsFieldLabel>
      <ExecutorProfileSelector
        options={options}
        value={profileId}
        onValueChange={onValueChange}
        disabled={false}
        placeholder={t("automations:executorProfilePlaceholder")}
        triggerClassName={settingsControlClassName()}
        ariaDescribedBy={
          credentials.credentialError?.target === "executorProfileId"
            ? "automation-executor-credential-error"
            : undefined
        }
      />
      <ProfileCredentialControls
        target="executorProfileId"
        profileId={profileId}
        {...credentials}
      />
    </div>
  );
}

function ConfigSectionGrid({
  workspaceId,
  automation,
  onAutomationChange,
  agentProfileId,
  executorProfileId,
  credentialChoicesEligible,
  repositorySelections,
  dirtyFields = CLEAN_FIELDS,
  onAgentProfileChange,
  onExecutorProfileChange,
  onRepositoriesChange,
  credentialError,
  personalCredentialBindings,
  onPersonalCredentialBindingsChange,
  data,
}: ConfigSectionProps & { data: ReturnType<typeof useConfigSectionData> }) {
  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
      <AgentCredentialProfile
        dirty={dirtyFields.agentProfileId}
        profileId={agentProfileId}
        options={data.agents}
        onValueChange={onAgentProfileChange}
        credentialError={credentialError}
        fields={data.agentCredentialFields}
        bindings={personalCredentialBindings}
        choices={data.credentialChoices}
        secrets={data.globalSecrets.items}
        secretsLoaded={data.globalSecrets.loaded}
        canEdit={data.canEditCredentialChoices}
        eligible={credentialChoicesEligible}
        onChange={onPersonalCredentialBindingsChange}
      />
      <ExecutorCredentialProfile
        dirty={dirtyFields.executorProfileId}
        profileId={executorProfileId}
        options={data.executorOptions}
        onValueChange={onExecutorProfileChange}
        credentialError={credentialError}
        fields={data.executorCredentialFields}
        bindings={personalCredentialBindings}
        choices={data.credentialChoices}
        secrets={data.globalSecrets.items}
        secretsLoaded={data.globalSecrets.loaded}
        canEdit={data.canEditCredentialChoices}
        eligible={credentialChoicesEligible}
        onChange={onPersonalCredentialBindingsChange}
      />
      <RepositoryAccess
        workspaceId={workspaceId}
        automation={automation}
        onAutomationChange={onAutomationChange}
        repositories={data.repositories}
        discovered={data.discovered}
        selections={repositorySelections}
        supportsMultiRepo={data.supportsMultiRepo}
        credentialError={credentialError}
        isWorkspaceOwner={data.isWorkspaceOwner}
        onChange={onRepositoriesChange}
      />
    </div>
  );
}

export function ConfigSection({
  workspaceId,
  automation,
  onAutomationChange,
  agentProfileId,
  executorProfileId,
  credentialChoicesEligible,
  repositorySelections,
  dirtyFields = CLEAN_FIELDS,
  onAgentProfileChange,
  onExecutorProfileChange,
  onRepositoriesChange,
  credentialError,
  personalCredentialBindings,
  onPersonalCredentialBindingsChange,
}: ConfigSectionProps) {
  const data = useConfigSectionData({
    workspaceId,
    automation,
    agentProfileId,
    executorProfileId,
    credentialChoicesEligible,
    personalCredentialBindings,
    repositorySelections,
  });
  const updatePersonalCredentialBindings = useCallback(
    (bindings: PersonalCredentialBindingInput[]) => {
      if (data.canEditCredentialChoices) onPersonalCredentialBindingsChange(bindings);
    },
    [data.canEditCredentialChoices, onPersonalCredentialBindingsChange],
  );
  return (
    <ConfigSectionGrid
      workspaceId={workspaceId}
      automation={automation}
      onAutomationChange={onAutomationChange}
      agentProfileId={agentProfileId}
      executorProfileId={executorProfileId}
      credentialChoicesEligible={credentialChoicesEligible}
      repositorySelections={repositorySelections}
      dirtyFields={dirtyFields}
      onAgentProfileChange={onAgentProfileChange}
      onExecutorProfileChange={onExecutorProfileChange}
      onRepositoriesChange={onRepositoriesChange}
      credentialError={credentialError}
      personalCredentialBindings={personalCredentialBindings}
      onPersonalCredentialBindingsChange={updatePersonalCredentialBindings}
      data={data}
    />
  );
}
