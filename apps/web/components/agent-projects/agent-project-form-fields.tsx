"use client";

import type { FormEvent } from "react";
import { useAppStore } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import { isRemoteBackedProjectRepository } from "@/lib/agent-projects/repositories";
import { Button } from "@kandev/ui/button";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { isSelectableAgentProfile } from "@/lib/state/slices/settings/types";
import type { AgentProject } from "@/lib/types/http-agent-projects";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";

export type ProjectDraft = {
  name: string;
  repositoryIds: string[];
  primaryRepositoryId: string;
  coordinatorProfileId: string;
  economyProfileId: string;
  frontierProfileId: string;
};

export function draftFor(project?: AgentProject): ProjectDraft {
  return {
    name: project?.name ?? "",
    repositoryIds: project?.repository_ids ?? [],
    primaryRepositoryId: project?.primary_repository_id ?? "",
    coordinatorProfileId: project?.coordinator_profile_id ?? "",
    economyProfileId: project?.economy_profile_id ?? "",
    frontierProfileId: project?.frontier_profile_id ?? "",
  };
}

function ProjectSelect({
  label,
  value,
  options,
  unavailableId,
  unavailableOptionLabel,
  mobile,
  onChange,
}: {
  label: string;
  value: string;
  options: Array<{ id: string; label: string }>;
  unavailableId?: string;
  unavailableOptionLabel?: string;
  mobile: boolean;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <label className="flex flex-col gap-1 text-sm">
      <span className="font-medium">{label}</span>
      <select
        className={cn(
          "w-full rounded-md border bg-background px-2 text-sm",
          mobile ? "min-h-11" : "h-8",
        )}
        value={value}
        onChange={(event) => onChange(event.currentTarget.value)}
      >
        <option value="">{t("projects:chooseProfile")}</option>
        {unavailableId && !options.some((option) => option.id === unavailableId) && (
          <option value={unavailableId}>
            {unavailableOptionLabel ?? t("projects:unavailableProfile", { id: unavailableId })}
          </option>
        )}
        {options.map((option) => (
          <option key={option.id} value={option.id}>
            {option.label}
          </option>
        ))}
      </select>
    </label>
  );
}

function projectExecutorData(
  project: AgentProject | undefined,
  executors: AppState["executors"]["items"],
  workspace: AppState["workspaces"]["items"][number] | undefined,
) {
  const defaultExecutor = executors.find((item) => item.id === workspace?.default_executor_id);
  const workspaceDefaultProfile = defaultExecutor?.profiles?.[0];
  const storedProfile = project ? findProjectExecutorProfile(project, executors) : undefined;
  const executorProfile = project ? storedProfile : workspaceDefaultProfile;
  const executor = project ? findExecutorForProfile(executors, storedProfile) : defaultExecutor;
  return {
    executorReady: isReadyProjectExecutor(executorProfile, executor),
    executorName: executorProfile?.name ?? executor?.name ?? "",
    workspaceDefaultExecutorName: workspaceDefaultProfile?.name ?? defaultExecutor?.name ?? "",
    workspaceDefaultExecutorProfileId: workspaceDefaultProfile?.id,
    workspaceDefaultExecutorReady: isReadyProjectExecutor(workspaceDefaultProfile, defaultExecutor),
  };
}

function findProjectExecutorProfile(
  project: AgentProject,
  executors: AppState["executors"]["items"],
) {
  return executors
    .flatMap((executor) => executor.profiles ?? [])
    .find((profile) => profile.id === project.executor_profile_id);
}

function findExecutorForProfile(
  executors: AppState["executors"]["items"],
  profile: ReturnType<typeof findProjectExecutorProfile>,
) {
  return executors.find((executor) => executor.id === profile?.executor_id);
}

function isReadyProjectExecutor(
  profile: { id: string } | undefined,
  executor: { type: string; status: string } | undefined,
) {
  return Boolean(profile && executor?.type === "worktree" && executor.status === "active");
}

export function useProjectFormData(
  open: boolean,
  workspaceId: string | null,
  project?: AgentProject,
) {
  useRepositories(workspaceId, open);
  useSettingsData(open);
  const repositories = useAppStore((state) =>
    workspaceId ? (state.repositories.itemsByWorkspaceId[workspaceId] ?? []) : [],
  );
  const repositoriesLoaded = useAppStore((state) =>
    workspaceId ? (state.repositories.loadedByWorkspaceId[workspaceId] ?? false) : false,
  );
  const profiles = useAppStore((state) => state.agentProfiles.items);
  const executors = useAppStore((state) => state.executors.items);
  const workspace = useAppStore((state) =>
    state.workspaces.items.find((item) => item.id === workspaceId),
  );
  const executorData = projectExecutorData(project, executors, workspace);
  return {
    repositories: repositories.filter(isRemoteBackedProjectRepository),
    repositoriesLoaded,
    profiles: profiles.filter(
      (profile) =>
        (!profile.workspace_id || profile.workspace_id === workspaceId) &&
        profile.enabled !== false &&
        isSelectableAgentProfile(profile),
    ),
    ...executorData,
  };
}

type ProjectFormData = ReturnType<typeof useProjectFormData>;
type RepositoryList = ProjectFormData["repositories"];
type ProfileList = ProjectFormData["profiles"];

function ProjectNameField({
  name,
  mobile,
  onChange,
}: {
  name: string;
  mobile: boolean;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <label className="flex flex-col gap-1 text-sm">
      <span className="font-medium">{t("projects:name")}</span>
      <input
        autoFocus={!mobile}
        required
        maxLength={120}
        className={cn(
          "w-full rounded-md border bg-background px-3 text-sm",
          mobile ? "min-h-11" : "h-8",
        )}
        value={name}
        onChange={(event) => onChange(event.currentTarget.value)}
        data-testid="agent-project-name"
      />
    </label>
  );
}

function ProjectRepositoriesField({
  repositories,
  repositoriesLoaded,
  repositoryIds,
  unavailableRepositoryIds,
  mobile,
  onToggle,
}: {
  repositories: RepositoryList;
  repositoriesLoaded: boolean;
  repositoryIds: string[];
  unavailableRepositoryIds: string[];
  mobile: boolean;
  onToggle: (repositoryId: string) => void;
}) {
  const { t } = useTranslation();
  const empty = repositories.length === 0 && unavailableRepositoryIds.length === 0;
  const rowHeight = mobile ? "min-h-11" : "min-h-8";
  return (
    <fieldset className="space-y-2">
      <legend className="text-sm font-medium">{t("projects:repositories")}</legend>
      {empty ? (
        <p className="text-xs text-muted-foreground">
          {repositoriesLoaded ? t("projects:noRemoteRepositories") : t("common:loading")}
        </p>
      ) : (
        <div className="max-h-40 overflow-y-auto rounded-md border p-1">
          {unavailableRepositoryIds.map((repositoryId) => (
            <label
              key={repositoryId}
              className={cn(
                "flex cursor-pointer items-center gap-2 px-2 text-sm text-muted-foreground",
                rowHeight,
              )}
            >
              <input type="checkbox" checked onChange={() => onToggle(repositoryId)} />
              <span className="truncate">
                {t("projects:unavailableRepository", { id: repositoryId })}
              </span>
            </label>
          ))}
          {repositories.map((repository) => (
            <label
              key={repository.id}
              className={cn("flex cursor-pointer items-center gap-2 px-2 text-sm", rowHeight)}
            >
              <input
                type="checkbox"
                checked={repositoryIds.includes(repository.id)}
                onChange={() => onToggle(repository.id)}
              />
              <span className="truncate">
                {repository.provider_owner
                  ? `${repository.provider_owner}/${repository.provider_name || repository.name}`
                  : repository.name}
              </span>
            </label>
          ))}
        </div>
      )}
    </fieldset>
  );
}

function ProjectPrimaryRepositoryField({
  project,
  repositories,
  draft,
  mobile,
  onChange,
}: {
  project?: AgentProject;
  repositories: RepositoryList;
  draft: ProjectDraft;
  mobile: boolean;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <ProjectSelect
      label={t("projects:primaryRepository")}
      value={draft.primaryRepositoryId}
      unavailableId={project?.primary_repository_id}
      unavailableOptionLabel={t("projects:unavailableRepository", {
        id: project?.primary_repository_id ?? "",
      })}
      options={repositories
        .filter((repository) => draft.repositoryIds.includes(repository.id))
        .map((repository) => ({ id: repository.id, label: repository.name }))}
      mobile={mobile}
      onChange={onChange}
    />
  );
}

function ProjectExecutionField({
  project,
  formData,
  applyDefaultExecutor,
  mobile,
  onApplyDefaultExecutorChange,
}: {
  project?: AgentProject;
  formData: ProjectFormData;
  applyDefaultExecutor: boolean;
  mobile: boolean;
  onApplyDefaultExecutorChange: (value: boolean) => void;
}) {
  const { t } = useTranslation();
  const executionCopy = formData.executorReady
    ? t(project ? "projects:projectExecution" : "projects:workspaceExecution", {
        name: formData.executorName,
      })
    : t(project ? "projects:projectExecutorUnavailable" : "projects:missingExecutor");
  const canApplyDefault =
    Boolean(project) &&
    formData.workspaceDefaultExecutorReady &&
    formData.workspaceDefaultExecutorProfileId !== project?.executor_profile_id;
  return (
    <>
      <div className="space-y-1 rounded-md border px-3 py-2 text-sm">
        <span className="font-medium">{t("projects:execution")}</span>
        <p className="text-muted-foreground">{executionCopy}</p>
      </div>
      {canApplyDefault && (
        <label
          className={cn(
            "flex cursor-pointer items-center gap-2 rounded-md border px-3 text-sm",
            mobile ? "min-h-11" : "min-h-8",
          )}
        >
          <input
            type="checkbox"
            checked={applyDefaultExecutor}
            onChange={(event) => onApplyDefaultExecutorChange(event.currentTarget.checked)}
          />
          <span>
            {t("projects:applyWorkspaceDefaultExecutor", {
              name: formData.workspaceDefaultExecutorName,
            })}
          </span>
        </label>
      )}
    </>
  );
}

function ProjectProfileFields({
  project,
  draft,
  profiles,
  mobile,
  onChange,
}: {
  project?: AgentProject;
  draft: ProjectDraft;
  profiles: ProfileList;
  mobile: boolean;
  onChange: (patch: Partial<ProjectDraft>) => void;
}) {
  const { t } = useTranslation();
  const options = profiles.map((profile) => ({ id: profile.id, label: profile.label }));
  return (
    <>
      <ProjectSelect
        label={t("projects:coordinatorProfile")}
        value={draft.coordinatorProfileId}
        unavailableId={project?.coordinator_profile_id}
        options={options}
        mobile={mobile}
        onChange={(coordinatorProfileId) => onChange({ coordinatorProfileId })}
      />
      <ProjectSelect
        label={t("projects:economyProfile")}
        value={draft.economyProfileId}
        unavailableId={project?.economy_profile_id}
        options={options}
        mobile={mobile}
        onChange={(economyProfileId) => onChange({ economyProfileId })}
      />
      <ProjectSelect
        label={t("projects:frontierProfile")}
        value={draft.frontierProfileId}
        unavailableId={project?.frontier_profile_id}
        options={options}
        mobile={mobile}
        onChange={(frontierProfileId) => onChange({ frontierProfileId })}
      />
    </>
  );
}

function ProjectFormFields({
  project,
  formData,
  draft,
  mobile,
  ready,
  error,
  applyDefaultExecutor,
  onNameChange,
  onToggleRepository,
  onPrimaryRepositoryChange,
  onDraftChange,
  onApplyDefaultExecutorChange,
}: {
  project?: AgentProject;
  formData: ProjectFormData;
  draft: ProjectDraft;
  mobile: boolean;
  ready: boolean;
  error: string | null;
  applyDefaultExecutor: boolean;
  onNameChange: (value: string) => void;
  onToggleRepository: (repositoryId: string) => void;
  onPrimaryRepositoryChange: (value: string) => void;
  onDraftChange: (patch: Partial<ProjectDraft>) => void;
  onApplyDefaultExecutorChange: (value: boolean) => void;
}) {
  const { t } = useTranslation();
  const availableIds = new Set<string>(formData.repositories.map((repository) => repository.id));
  const unavailableIds = draft.repositoryIds.filter((id) => !availableIds.has(id));
  return (
    <div className="min-h-0 flex-1 space-y-4 overflow-y-auto px-4 py-3">
      <ProjectNameField name={draft.name} mobile={mobile} onChange={onNameChange} />
      <ProjectRepositoriesField
        repositories={formData.repositories}
        repositoriesLoaded={formData.repositoriesLoaded}
        repositoryIds={draft.repositoryIds}
        unavailableRepositoryIds={unavailableIds}
        mobile={mobile}
        onToggle={onToggleRepository}
      />
      <ProjectPrimaryRepositoryField
        project={project}
        repositories={formData.repositories}
        draft={draft}
        mobile={mobile}
        onChange={onPrimaryRepositoryChange}
      />
      <ProjectExecutionField
        project={project}
        formData={formData}
        applyDefaultExecutor={applyDefaultExecutor}
        mobile={mobile}
        onApplyDefaultExecutorChange={onApplyDefaultExecutorChange}
      />
      <ProjectProfileFields
        project={project}
        draft={draft}
        profiles={formData.profiles}
        mobile={mobile}
        onChange={onDraftChange}
      />
      {!ready && (
        <p className="text-xs text-muted-foreground">{t("projects:dependenciesRequired")}</p>
      )}
      {error && (
        <p role="alert" className="text-sm text-destructive" data-testid="agent-project-error">
          {error}
        </p>
      )}
    </div>
  );
}

function ProjectFormFooter({
  mobile,
  ready,
  saving,
  project,
  onClose,
}: {
  mobile: boolean;
  ready: boolean;
  saving: boolean;
  project?: AgentProject;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  return (
    <footer
      className={cn(
        "flex shrink-0 gap-2 border-t px-4 py-3",
        mobile
          ? "pb-[calc(0.75rem+env(safe-area-inset-bottom,0px))] flex-col-reverse"
          : "justify-end",
      )}
    >
      <Button
        type="button"
        variant="outline"
        className={mobile ? "min-h-11" : "h-8"}
        onClick={onClose}
      >
        {t("common:cancel")}
      </Button>
      <Button
        type="submit"
        disabled={!ready || saving}
        className={mobile ? "min-h-11" : "h-8"}
        data-testid="agent-project-submit"
      >
        {saving ? t("projects:saving") : t(project ? "common:save" : "projects:createProject")}
      </Button>
    </footer>
  );
}

export function ProjectFormBody({
  onSubmit,
  formData,
  draft,
  mobile,
  ready,
  error,
  saving,
  project,
  applyDefaultExecutor,
  onNameChange,
  onToggleRepository,
  onPrimaryRepositoryChange,
  onDraftChange,
  onApplyDefaultExecutorChange,
  onClose,
}: {
  onSubmit: (event: FormEvent) => void;
  formData: ProjectFormData;
  draft: ProjectDraft;
  mobile: boolean;
  ready: boolean;
  error: string | null;
  saving: boolean;
  project?: AgentProject;
  applyDefaultExecutor: boolean;
  onNameChange: (value: string) => void;
  onToggleRepository: (repositoryId: string) => void;
  onPrimaryRepositoryChange: (value: string) => void;
  onDraftChange: (patch: Partial<ProjectDraft>) => void;
  onApplyDefaultExecutorChange: (value: boolean) => void;
  onClose: () => void;
}) {
  return (
    <form onSubmit={onSubmit} className="flex min-h-0 flex-1 flex-col">
      <ProjectFormFields
        project={project}
        formData={formData}
        draft={draft}
        mobile={mobile}
        ready={ready}
        error={error}
        applyDefaultExecutor={applyDefaultExecutor}
        onNameChange={onNameChange}
        onToggleRepository={onToggleRepository}
        onPrimaryRepositoryChange={onPrimaryRepositoryChange}
        onDraftChange={onDraftChange}
        onApplyDefaultExecutorChange={onApplyDefaultExecutorChange}
      />
      <ProjectFormFooter
        mobile={mobile}
        ready={ready}
        saving={saving}
        project={project}
        onClose={onClose}
      />
    </form>
  );
}
