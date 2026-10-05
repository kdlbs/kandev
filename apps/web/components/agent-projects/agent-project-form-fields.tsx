"use client";

import type { FormEvent } from "react";
import { useCallback } from "react";
import { IconChevronDown } from "@tabler/icons-react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import { isRemoteBackedProjectRepository } from "@/lib/agent-projects/repositories";
import type { RemoteRepository } from "@/hooks/domains/integrations/use-remote-repositories";
import { registerRemoteRepositorySelectionAction } from "@/app/actions/workspaces";
import { PENDING_PROJECT_REPOSITORY_PREFIX } from "@/lib/agent-projects/repository-resolution";

import { Button } from "@kandev/ui/button";
import {
  ProjectCoordinatorField,
  ProjectExecutionField,
  ProjectNameField,
  ProjectPrimaryRepositoryField,
  ProjectPromptField,
  ProjectRepositoriesField,
  ProjectWorkerProfileFields,
} from "./agent-project-form-field-controls";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { isSelectableAgentProfile } from "@/lib/state/slices/settings/types";
import type { AgentProject } from "@/lib/types/http-agent-projects";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";

export type ProjectDraft = {
  name: string;
  initialPrompt: string;
  repositoryIds: string[];
  remoteRepositories: RemoteRepository[];
  primaryRepositoryId: string;
  coordinatorProfileId: string;
  economyProfileId: string;
  economyProfileInherited: boolean;
  frontierProfileId: string;
  frontierProfileInherited: boolean;
};

export function draftFor(project?: AgentProject): ProjectDraft {
  return {
    name: project?.name ?? "",
    initialPrompt: "",
    repositoryIds: project?.repository_ids ?? [],
    remoteRepositories: [],
    primaryRepositoryId: project?.primary_repository_id ?? "",
    coordinatorProfileId: project?.coordinator_profile_id ?? "",
    economyProfileId: project?.economy_profile_id ?? "",
    economyProfileInherited: !project,
    frontierProfileId: project?.frontier_profile_id ?? "",
    frontierProfileInherited: !project,
  };
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
  const { t } = useTranslation();
  useRepositories(workspaceId, open);
  useSettingsData(open);
  const storeApi = useAppStoreApi();
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
  const createRemoteRepository = useCallback(
    async (remote: RemoteRepository) => {
      if (!workspaceId) throw new Error(t("projects:workspaceUnavailable"));
      const created = await registerRemoteRepositorySelectionAction(workspaceId, {
        remote_url: remote.url,
        provider: remote.provider,
        provider_host: remote.providerHost,
        provider_scope: remote.providerScope,
        provider_repo_id: remote.id,
        provider_owner: remote.owner,
        provider_name: remote.name,
        default_branch: remote.defaultBranch,
      });
      storeApi.getState().upsertRepository(workspaceId, created);
      return created;
    },
    [storeApi, t, workspaceId],
  );
  return {
    repositories: repositories.filter(isRemoteBackedProjectRepository),
    repositoriesLoaded,
    profiles: profiles.filter(
      (profile) =>
        (!profile.workspace_id || profile.workspace_id === workspaceId) &&
        profile.enabled !== false &&
        profile.cli_passthrough !== true &&
        isSelectableAgentProfile(profile),
    ),
    createRemoteRepository,
    ...executorData,
  };
}

export type ProjectFormData = ReturnType<typeof useProjectFormData>;
type ProjectFormFieldsProps = {
  workspaceId: string;
  project?: AgentProject;
  formData: ProjectFormData;
  draft: ProjectDraft;
  createdProject?: AgentProject;
  mobile: boolean;
  ready: boolean;
  error: string | null;
  saving: boolean;
  createRetryLocked: boolean;
  promptLocked: boolean;
  applyDefaultExecutor: boolean;
  onNameChange: (value: string) => void;
  onPromptChange: (value: string) => void;
  onToggleRepository: (repositoryId: string) => void;
  onSelectRemoteRepository: (repository: RemoteRepository) => void;
  onPrimaryRepositoryChange: (value: string) => void;
  onDraftChange: (patch: Partial<ProjectDraft>) => void;
  onApplyDefaultExecutorChange: (value: boolean) => void;
};

function ProjectRepositoryAndProfileFields({
  formProps,
  unavailableRepositoryIds,
  fieldsDisabled,
}: {
  formProps: Pick<
    ProjectFormFieldsProps,
    | "workspaceId"
    | "project"
    | "formData"
    | "draft"
    | "mobile"
    | "promptLocked"
    | "onNameChange"
    | "onPromptChange"
    | "onToggleRepository"
    | "onSelectRemoteRepository"
    | "onPrimaryRepositoryChange"
    | "onDraftChange"
  >;
  unavailableRepositoryIds: string[];
  fieldsDisabled: boolean;
}) {
  const {
    workspaceId,
    project,
    formData,
    draft,
    mobile,
    promptLocked,
    onNameChange,
    onPromptChange,
    onToggleRepository,
    onSelectRemoteRepository,
    onPrimaryRepositoryChange,
    onDraftChange,
  } = formProps;
  return (
    <>
      <ProjectRepositoriesField
        workspaceId={workspaceId}
        repositories={formData.repositories}
        repositoriesLoaded={formData.repositoriesLoaded}
        repositoryIds={draft.repositoryIds}
        remoteRepositories={draft.remoteRepositories}
        unavailableRepositoryIds={unavailableRepositoryIds}
        mobile={mobile}
        disabled={fieldsDisabled}
        onToggle={onToggleRepository}
        onSelectRemote={onSelectRemoteRepository}
      />
      <ProjectNameField
        name={draft.name}
        mobile={mobile}
        disabled={fieldsDisabled}
        onChange={onNameChange}
      />
      {!project && (
        <ProjectPromptField
          prompt={draft.initialPrompt}
          mobile={mobile}
          disabled={promptLocked || fieldsDisabled}
          onChange={onPromptChange}
        />
      )}
      <div className="grid gap-3 md:grid-cols-2">
        <ProjectPrimaryRepositoryField
          project={project}
          repositories={formData.repositories}
          remoteRepositories={draft.remoteRepositories}
          draft={draft}
          mobile={mobile}
          disabled={fieldsDisabled}
          onChange={onPrimaryRepositoryChange}
        />
        <ProjectCoordinatorField
          project={project}
          draft={draft}
          profiles={formData.profiles}
          mobile={mobile}
          disabled={fieldsDisabled}
          onChange={onDraftChange}
        />
      </div>
    </>
  );
}

function ProjectFormFields({
  workspaceId,
  project,
  formData,
  draft,
  createdProject,
  mobile,
  ready,
  error,
  saving,
  createRetryLocked,
  promptLocked,
  applyDefaultExecutor,
  onNameChange,
  onPromptChange,
  onToggleRepository,
  onSelectRemoteRepository,
  onPrimaryRepositoryChange,
  onDraftChange,
  onApplyDefaultExecutorChange,
}: ProjectFormFieldsProps) {
  const { t } = useTranslation();
  const availableIds = new Set<string>(formData.repositories.map((repository) => repository.id));
  const unavailableIds = draft.repositoryIds.filter(
    (id) => !id.startsWith(PENDING_PROJECT_REPOSITORY_PREFIX) && !availableIds.has(id),
  );
  const fieldsDisabled = Boolean(createdProject || saving || createRetryLocked);
  const displayError =
    createdProject && error
      ? t("projects:startFailed", { project: createdProject.name, message: error })
      : error;
  return (
    <div
      className={cn(
        "min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain pb-3",
        mobile && "px-4 pt-3",
      )}
    >
      <ProjectRepositoryAndProfileFields
        formProps={{
          workspaceId,
          project,
          formData,
          draft,
          mobile,
          promptLocked,
          onNameChange,
          onPromptChange,
          onToggleRepository,
          onSelectRemoteRepository,
          onPrimaryRepositoryChange,
          onDraftChange,
        }}
        unavailableRepositoryIds={unavailableIds}
        fieldsDisabled={fieldsDisabled}
      />
      <details className="group rounded-md border px-3 py-2" data-testid="agent-project-advanced">
        <summary className="flex min-h-7 cursor-pointer list-none items-center justify-between text-sm font-medium marker:hidden max-md:min-h-11 [@media(pointer:coarse)]:min-h-11">
          <span>{t("projects:advancedSettings")}</span>
          <IconChevronDown
            className="ml-2 size-4 shrink-0 opacity-60 transition-transform group-open:rotate-180"
            aria-hidden="true"
          />
        </summary>
        <div className="space-y-4 pt-3">
          <div className="grid gap-3 md:grid-cols-2">
            <ProjectWorkerProfileFields
              project={project}
              draft={draft}
              profiles={formData.profiles}
              mobile={mobile}
              disabled={fieldsDisabled}
              onChange={onDraftChange}
            />
          </div>
          <ProjectExecutionField
            project={project}
            formData={formData}
            applyDefaultExecutor={applyDefaultExecutor}
            disabled={fieldsDisabled}
            onApplyDefaultExecutorChange={onApplyDefaultExecutorChange}
          />
        </div>
      </details>
      {!ready && (
        <p className="text-xs text-muted-foreground">{t("projects:dependenciesRequired")}</p>
      )}
      {displayError && (
        <p role="alert" className="text-sm text-destructive" data-testid="agent-project-error">
          {displayError}
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
  createdProject,
  hasPrompt,
  onClose,
  onRetryStart,
  onOpenCoordinator,
}: {
  mobile: boolean;
  ready: boolean;
  saving: boolean;
  project?: AgentProject;
  createdProject?: AgentProject;
  hasPrompt: boolean;
  onClose: () => void;
  onRetryStart: () => void;
  onOpenCoordinator: () => void;
}) {
  const { t } = useTranslation();
  let submitLabel: "projects:createProject" | "common:save" | "projects:createAndStart" =
    "projects:createProject";
  if (project) submitLabel = "common:save";
  else if (hasPrompt) submitLabel = "projects:createAndStart";
  const submitText = saving ? t("projects:saving") : t(submitLabel);
  return (
    <footer
      className={cn(
        "flex shrink-0 gap-2 border-t pt-3",
        mobile ? "px-4 pb-[calc(0.75rem+env(safe-area-inset-bottom,0px))] flex-col" : "justify-end",
      )}
    >
      <Button
        type="button"
        variant="outline"
        className={mobile ? "min-h-11" : undefined}
        onClick={onClose}
      >
        {t("common:cancel")}
      </Button>
      {createdProject ? (
        <>
          <Button
            type="button"
            variant="outline"
            disabled={saving}
            className={mobile ? "min-h-11" : undefined}
            onClick={onOpenCoordinator}
            data-testid="agent-project-open-coordinator"
          >
            {t("projects:openCoordinator")}
          </Button>
          <Button
            type="button"
            disabled={saving}
            className={mobile ? "min-h-11" : undefined}
            onClick={onRetryStart}
            data-testid="agent-project-retry-start"
          >
            {saving ? t("projects:saving") : t("projects:retryStart")}
          </Button>
        </>
      ) : (
        <Button
          type="submit"
          disabled={!ready || saving}
          className={mobile ? "min-h-11" : undefined}
          data-testid="agent-project-submit"
        >
          {submitText}
        </Button>
      )}
    </footer>
  );
}

export function ProjectFormBody({
  workspaceId,
  onSubmit,
  formData,
  draft,
  mobile,
  ready,
  error,
  saving,
  project,
  createdProject,
  hasPrompt,
  createRetryLocked,
  promptLocked,
  applyDefaultExecutor,
  onNameChange,
  onPromptChange,
  onToggleRepository,
  onSelectRemoteRepository,
  onPrimaryRepositoryChange,
  onDraftChange,
  onApplyDefaultExecutorChange,
  onClose,
  onRetryStart,
  onOpenCoordinator,
}: {
  workspaceId: string;
  onSubmit: (event: FormEvent) => void;
  formData: ProjectFormData;
  draft: ProjectDraft;
  mobile: boolean;
  ready: boolean;
  error: string | null;
  saving: boolean;
  project?: AgentProject;
  createdProject?: AgentProject;
  hasPrompt: boolean;
  createRetryLocked: boolean;
  promptLocked: boolean;
  applyDefaultExecutor: boolean;
  onNameChange: (value: string) => void;
  onPromptChange: (value: string) => void;
  onToggleRepository: (repositoryId: string) => void;
  onSelectRemoteRepository: (repository: RemoteRepository) => void;
  onPrimaryRepositoryChange: (value: string) => void;
  onDraftChange: (patch: Partial<ProjectDraft>) => void;
  onApplyDefaultExecutorChange: (value: boolean) => void;
  onClose: () => void;
  onRetryStart: () => void;
  onOpenCoordinator: () => void;
}) {
  return (
    <form onSubmit={onSubmit} className="flex min-h-0 flex-1 flex-col">
      <ProjectFormFields
        workspaceId={workspaceId}
        project={project}
        formData={formData}
        draft={draft}
        createdProject={createdProject}
        mobile={mobile}
        ready={ready}
        error={error}
        saving={saving}
        createRetryLocked={createRetryLocked}
        promptLocked={promptLocked}
        applyDefaultExecutor={applyDefaultExecutor}
        onNameChange={onNameChange}
        onPromptChange={onPromptChange}
        onToggleRepository={onToggleRepository}
        onSelectRemoteRepository={onSelectRemoteRepository}
        onPrimaryRepositoryChange={onPrimaryRepositoryChange}
        onDraftChange={onDraftChange}
        onApplyDefaultExecutorChange={onApplyDefaultExecutorChange}
      />
      <ProjectFormFooter
        mobile={mobile}
        ready={ready}
        saving={saving}
        project={project}
        createdProject={createdProject}
        hasPrompt={hasPrompt}
        onClose={onClose}
        onRetryStart={onRetryStart}
        onOpenCoordinator={onOpenCoordinator}
      />
    </form>
  );
}
