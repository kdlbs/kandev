"use client";

import { useState } from "react";
import { IconInfoCircle, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import type { RemoteRepository } from "@/hooks/domains/integrations/use-remote-repositories";
import { AgentProjectRepositoryPicker } from "./agent-project-repository-picker";
import { projectRemoteSelectionId } from "@/lib/agent-projects/repository-resolution";
import {
  normalizeRemoteRepositoryURL,
  remoteRepositorySelectionIdentity,
} from "@/components/task-create-dialog-remote-repo-identity";
import type { AgentProject } from "@/lib/types/http-agent-projects";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import type { ProjectDraft, ProjectFormData } from "./agent-project-form-fields";

type RepositoryList = ProjectFormData["repositories"];
type ProfileList = ProjectFormData["profiles"];

function ProjectSelect({
  label,
  value,
  options,
  unavailableId,
  unavailableOptionLabel,
  placeholder,
  testId,
  mobile,
  disabled,
  onChange,
}: {
  label: string;
  value: string;
  options: Array<{ id: string; label: string }>;
  unavailableId?: string;
  unavailableOptionLabel?: string;
  placeholder?: string;
  testId?: string;
  mobile: boolean;
  disabled?: boolean;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <label className="flex min-w-0 flex-1 flex-col gap-1 text-sm">
      <span className="font-medium">{label}</span>
      <select
        disabled={disabled}
        className={cn(
          "w-full rounded-md border bg-background px-2 text-sm",
          controlSizingClassName("standard"),
          mobile && "min-h-11",
        )}
        value={value}
        onChange={(event) => onChange(event.currentTarget.value)}
        data-testid={testId}
      >
        <option value="">{placeholder ?? t("projects:chooseProfile")}</option>
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

export function ProjectHelpButton({
  label,
  description,
  testId,
}: {
  label: string;
  description: string;
  testId: string;
}) {
  const usesTouchDrawer = useTouchDrawer();
  const [drawerOpen, setDrawerOpen] = useState(false);
  const trigger = (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      className={cn(controlSizingClassName("icon"), "shrink-0 text-muted-foreground")}
      aria-label={label}
      aria-haspopup={usesTouchDrawer ? "dialog" : undefined}
      aria-expanded={usesTouchDrawer ? drawerOpen : undefined}
      data-testid={testId}
    >
      <IconInfoCircle className="size-4" aria-hidden="true" />
    </Button>
  );
  if (usesTouchDrawer) {
    return (
      <Drawer open={drawerOpen} onOpenChange={setDrawerOpen}>
        <DrawerTrigger asChild>{trigger}</DrawerTrigger>
        <DrawerContent data-testid={`${testId}-drawer`}>
          <DrawerHeader>
            <DrawerTitle>{label}</DrawerTitle>
            <DrawerDescription>{description}</DrawerDescription>
          </DrawerHeader>
        </DrawerContent>
      </Drawer>
    );
  }
  return (
    <Tooltip>
      <TooltipTrigger asChild>{trigger}</TooltipTrigger>
      <TooltipContent className="max-w-xs text-xs leading-relaxed">{description}</TooltipContent>
    </Tooltip>
  );
}

export function ProjectNameField({
  name,
  mobile,
  disabled,
  onChange,
}: {
  name: string;
  mobile: boolean;
  disabled: boolean;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-1 text-sm">
      <label htmlFor="agent-project-name" className="font-medium">
        {t("projects:name")}
      </label>
      <input
        id="agent-project-name"
        autoFocus={!mobile}
        disabled={disabled}
        required
        maxLength={120}
        className={cn(
          "w-full rounded-md border bg-background px-3 text-sm",
          controlSizingClassName("standard"),
          mobile && "min-h-11",
        )}
        value={name}
        onChange={(event) => onChange(event.currentTarget.value)}
        data-testid="agent-project-name"
      />
    </div>
  );
}

export function ProjectPromptField({
  prompt,
  mobile,
  disabled,
  onChange,
}: {
  prompt: string;
  mobile: boolean;
  disabled: boolean;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <label className="flex flex-col gap-1 text-sm">
      <span className="font-medium">{t("projects:initialPrompt")}</span>
      <textarea
        value={prompt}
        disabled={disabled}
        onChange={(event) => onChange(event.currentTarget.value)}
        placeholder={t("projects:initialPromptPlaceholder")}
        className={cn(
          "min-h-32 w-full resize-y rounded-md border bg-background px-3 py-2 text-sm",
          mobile && "min-h-36",
        )}
        data-testid="agent-project-initial-prompt"
      />
    </label>
  );
}

export function ProjectRepositoriesField({
  workspaceId,
  repositories,
  repositoriesLoaded,
  repositoryIds,
  remoteRepositories,
  unavailableRepositoryIds,
  mobile,
  disabled,
  onToggle,
  onSelectRemote,
}: {
  workspaceId: string;
  repositories: RepositoryList;
  repositoriesLoaded: boolean;
  repositoryIds: string[];
  remoteRepositories: RemoteRepository[];
  unavailableRepositoryIds: string[];
  mobile: boolean;
  disabled: boolean;
  onToggle: (repositoryId: string) => void;
  onSelectRemote: (repository: RemoteRepository) => void;
}) {
  const { t } = useTranslation();
  const selectedRemote = remoteRepositories.filter((repository) =>
    repositoryIds.includes(projectRemoteSelectionId(repository)),
  );
  const selectedExisting = repositories.filter((repository) =>
    repositoryIds.includes(repository.id),
  );
  const selectedIdentities = [
    ...selectedRemote.map((repository) => remoteRepositorySelectionIdentity(repository)),
    ...selectedExisting.flatMap((repository) => {
      const identity = remoteRepositorySelectionIdentity({
        provider: repository.provider,
        id: repository.provider_repo_id || repository.id,
        providerHost: repository.provider_host,
        providerScope: repository.provider_scope,
        url: repository.remote_url,
      });
      return [
        identity,
        ...(repository.remote_url
          ? [`url:${normalizeRemoteRepositoryURL(repository.remote_url)}`]
          : []),
      ];
    }),
  ];
  return (
    <fieldset className="space-y-2" disabled={disabled}>
      <legend className="text-sm font-medium">{t("projects:repositories")}</legend>
      {repositories.length === 0 && unavailableRepositoryIds.length === 0 && !repositoriesLoaded ? (
        <p className="text-xs text-muted-foreground">{t("common:loading")}</p>
      ) : null}
      <div className="flex flex-wrap items-center gap-2">
        {unavailableRepositoryIds.map((repositoryId) => (
          <ProjectRepositoryChip
            key={repositoryId}
            label={t("projects:unavailableRepository", { id: repositoryId })}
            removeLabel={t("projects:removeRepository", { name: repositoryId })}
            disabled={disabled}
            mobile={mobile}
            testId="agent-project-unavailable-repository"
            onRemove={() => onToggle(repositoryId)}
          />
        ))}
        {selectedExisting.map((repository) => (
          <ProjectRepositoryChip
            key={repository.id}
            label={
              repository.provider_owner
                ? `${repository.provider_owner}/${repository.provider_name || repository.name}`
                : repository.name
            }
            removeLabel={t("projects:removeRepository", { name: repository.name })}
            disabled={disabled}
            mobile={mobile}
            onRemove={() => onToggle(repository.id)}
          />
        ))}
        <AgentProjectRepositoryPicker
          workspaceId={workspaceId}
          selected={selectedRemote}
          selectedIdentities={selectedIdentities}
          existingRepositories={repositories.filter(
            (repository) => !repositoryIds.includes(repository.id),
          )}
          selectedRepositoryIds={repositoryIds}
          disabled={disabled}
          onSelect={onSelectRemote}
          onSelectExisting={(repositoryId) => onToggle(repositoryId)}
          onRemove={(repository) => onToggle(projectRemoteSelectionId(repository))}
        />
      </div>
    </fieldset>
  );
}

function ProjectRepositoryChip({
  label,
  removeLabel,
  disabled,
  mobile,
  testId,
  onRemove,
}: {
  label: string;
  removeLabel: string;
  disabled: boolean;
  mobile: boolean;
  testId?: string;
  onRemove: () => void;
}) {
  return (
    <span
      className="inline-flex min-w-0 max-w-full items-center gap-1 rounded-md border bg-background px-2 text-sm"
      data-testid={testId ?? "agent-project-repository-chip"}
    >
      <span className="max-w-[15rem] truncate">{label}</span>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        disabled={disabled}
        aria-label={removeLabel}
        className={cn(controlSizingClassName("icon"), "shrink-0 p-0", mobile && "min-w-11")}
        onClick={onRemove}
        data-testid="agent-project-remove-repository"
      >
        <IconX className="size-3.5" aria-hidden="true" />
      </Button>
    </span>
  );
}

export function ProjectPrimaryRepositoryField({
  project,
  repositories,
  remoteRepositories,
  draft,
  mobile,
  disabled,
  onChange,
}: {
  project?: AgentProject;
  repositories: RepositoryList;
  remoteRepositories: RemoteRepository[];
  draft: ProjectDraft;
  mobile: boolean;
  disabled: boolean;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <ProjectSelect
      label={t("projects:primaryRepository")}
      placeholder={t("kanban:selectRepository")}
      value={draft.primaryRepositoryId}
      unavailableId={project?.primary_repository_id}
      unavailableOptionLabel={t("projects:unavailableRepository", {
        id: project?.primary_repository_id ?? "",
      })}
      options={[
        ...repositories
          .filter((repository) => draft.repositoryIds.includes(repository.id))
          .map((repository) => ({
            id: repository.id,
            label: repository.provider_owner
              ? `${repository.provider_owner}/${repository.provider_name || repository.name}`
              : repository.name,
          })),
        ...remoteRepositories
          .filter((repository) =>
            draft.repositoryIds.includes(projectRemoteSelectionId(repository)),
          )
          .map((repository) => ({
            id: projectRemoteSelectionId(repository),
            label: repository.fullName,
          })),
      ]}
      mobile={mobile}
      disabled={disabled}
      onChange={onChange}
    />
  );
}

export function ProjectExecutionField({
  project,
  formData,
  applyDefaultExecutor,
  disabled,
  onApplyDefaultExecutorChange,
}: {
  project?: AgentProject;
  formData: ProjectFormData;
  applyDefaultExecutor: boolean;
  disabled: boolean;
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
            "min-h-8 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11",
          )}
        >
          <input
            type="checkbox"
            checked={applyDefaultExecutor}
            disabled={disabled}
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

export function ProjectCoordinatorField({
  project,
  draft,
  profiles,
  mobile,
  disabled,
  onChange,
}: {
  project?: AgentProject;
  draft: ProjectDraft;
  profiles: ProfileList;
  mobile: boolean;
  disabled: boolean;
  onChange: (patch: Partial<ProjectDraft>) => void;
}) {
  const { t } = useTranslation();
  const options = profiles.map((profile) => ({ id: profile.id, label: profile.label }));
  return (
    <div className="flex min-w-0 items-end gap-1">
      <ProjectSelect
        label={t("projects:coordinatorProfile")}
        value={draft.coordinatorProfileId}
        unavailableId={project?.coordinator_profile_id}
        options={options}
        mobile={mobile}
        disabled={disabled}
        onChange={(coordinatorProfileId) => onChange({ coordinatorProfileId })}
      />
      <ProjectHelpButton
        label={t("projects:profileHelpLabel")}
        description={t("projects:coordinatorProfileHelp")}
        testId="agent-project-coordinator-help"
      />
    </div>
  );
}

const SAME_AS_COORDINATOR = "__same_as_coordinator__";

export function ProjectWorkerProfileFields({
  project,
  draft,
  profiles,
  mobile,
  disabled,
  onChange,
}: {
  project?: AgentProject;
  draft: ProjectDraft;
  profiles: ProfileList;
  mobile: boolean;
  disabled: boolean;
  onChange: (patch: Partial<ProjectDraft>) => void;
}) {
  const { t } = useTranslation();
  const options = profiles.map((profile) => ({ id: profile.id, label: profile.label }));
  const workerOptions = [
    { id: SAME_AS_COORDINATOR, label: t("projects:sameAsCoordinator") },
    ...options,
  ];
  return (
    <>
      <div className="flex min-w-0 items-end gap-1">
        <ProjectSelect
          label={t("projects:economyProfile")}
          value={draft.economyProfileInherited ? SAME_AS_COORDINATOR : draft.economyProfileId}
          unavailableId={project?.economy_profile_id}
          options={workerOptions}
          testId="agent-project-economy-profile-select"
          mobile={mobile}
          disabled={disabled}
          onChange={(economyProfileId) =>
            onChange(
              economyProfileId === SAME_AS_COORDINATOR
                ? { economyProfileInherited: true }
                : { economyProfileId, economyProfileInherited: false },
            )
          }
        />
        <ProjectHelpButton
          label={t("projects:profileHelpLabel")}
          description={t("projects:economyProfileHelp")}
          testId="agent-project-economy-help"
        />
      </div>
      <div className="flex min-w-0 items-end gap-1">
        <ProjectSelect
          label={t("projects:frontierProfile")}
          value={draft.frontierProfileInherited ? SAME_AS_COORDINATOR : draft.frontierProfileId}
          unavailableId={project?.frontier_profile_id}
          options={workerOptions}
          testId="agent-project-frontier-profile-select"
          mobile={mobile}
          disabled={disabled}
          onChange={(frontierProfileId) =>
            onChange(
              frontierProfileId === SAME_AS_COORDINATOR
                ? { frontierProfileInherited: true }
                : { frontierProfileId, frontierProfileInherited: false },
            )
          }
        />
        <ProjectHelpButton
          label={t("projects:profileHelpLabel")}
          description={t("projects:frontierProfileHelp")}
          testId="agent-project-frontier-help"
        />
      </div>
    </>
  );
}
