"use client";

import { useEffect, useRef, useState } from "react";
import { IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { inspectRemoteRepositorySelectionAction } from "@/app/actions/workspaces";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { RemoteRepoPickerContent } from "@/components/task-create-dialog-remote-repo-chip";
import { RemoteRepositoryProviderIcon } from "@/components/task-create-dialog-remote-repo-provider-tabs";
import {
  builtInRepositoryProviderForURL,
  useRemoteRepositories,
} from "@/hooks/domains/integrations/use-remote-repositories";
import type {
  RemoteRepository,
  UseRemoteRepositoriesResult,
} from "@/hooks/domains/integrations/use-remote-repositories";
import type { Repository } from "@/lib/types/http";
import { normalizeRemoteRepositoryURL } from "@/components/task-create-dialog-remote-repo-identity";
import { pluginRegistry } from "@/lib/plugins/registry";
import { repositoryProviderMatchesURL } from "@/lib/plugins/repository-provider-url-resolution";
import { projectRemoteRepositoryKey } from "@/lib/agent-projects/repository-resolution";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";

type RepositoryPickerProps = {
  workspaceId: string;
  selected: RemoteRepository[];
  selectedIdentities: string[];
  existingRepositories: Repository[];
  selectedRepositoryIds: string[];
  disabled: boolean;
  onSelect: (repository: RemoteRepository) => void;
  onSelectExisting: (repositoryId: string) => void;
  onRemove: (repository: RemoteRepository) => void;
};

async function inspectPluginRepositoryURL(workspaceId: string, value: string, signal: AbortSignal) {
  const candidates = pluginRegistry
    .getRepositoryProviders()
    .filter((candidate) => repositoryProviderMatchesURL(candidate, value));
  const attempts = await Promise.allSettled(
    candidates.map((candidate) =>
      inspectRemoteRepositorySelectionAction(
        workspaceId,
        { remote_url: value, provider: candidate.id },
        signal,
      ),
    ),
  );
  const matches = attempts.flatMap((attempt) =>
    attempt.status === "fulfilled" ? [attempt.value] : [],
  );
  return matches.length === 1 ? matches[0] : undefined;
}

async function inspectPastedRepositoryURL(
  workspaceId: string,
  value: string,
  accessible: UseRemoteRepositoriesResult,
  signal: AbortSignal,
) {
  const normalized = normalizeRemoteRepositoryURL(value);
  const listed = (accessible.allRepos ?? accessible.repos).find(
    (repository) => normalizeRemoteRepositoryURL(repository.url) === normalized,
  );
  const provider =
    listed?.provider ?? builtInRepositoryProviderForURL(value, accessible.gitLabHost);
  if (!provider) return inspectPluginRepositoryURL(workspaceId, value, signal);
  const hints = listed ? repositorySelectionHints(listed) : {};
  return inspectRemoteRepositorySelectionAction(
    workspaceId,
    { remote_url: value, provider, ...hints },
    signal,
  );
}

function repositorySelectionHints(repository: RemoteRepository) {
  return {
    provider_host: repository.providerHost,
    provider_scope: repository.providerScope,
    provider_repo_id: repository.id,
    provider_owner: repository.owner,
    provider_name: repository.name,
    default_branch: repository.defaultBranch || undefined,
  };
}

function remoteRepositoryFromSelection(
  verified: Awaited<ReturnType<typeof inspectRemoteRepositorySelectionAction>>,
): RemoteRepository {
  return {
    provider: verified.provider,
    id: verified.provider_repo_id,
    owner: verified.provider_owner,
    name: verified.provider_name,
    fullName: `${verified.provider_owner}/${verified.provider_name}`,
    url: verified.remote_url,
    providerHost: verified.provider_host,
    providerScope: verified.provider_scope,
    defaultBranch: verified.default_branch,
    private: true,
  };
}

function usePastedRepositoryInspection(
  workspaceId: string,
  accessible: UseRemoteRepositoriesResult,
  onSelect: (repository: RemoteRepository) => void,
  onClose: () => void,
) {
  const { t } = useTranslation();
  const [resolvingURL, setResolvingURL] = useState(false);
  const [resolutionError, setResolutionError] = useState<string | null>(null);
  const generation = useRef(0);
  const inspectionController = useRef<AbortController | null>(null);
  const invalidate = () => {
    generation.current += 1;
    inspectionController.current?.abort();
    inspectionController.current = null;
  };
  const cancel = () => {
    invalidate();
    setResolvingURL(false);
  };
  useEffect(() => {
    invalidate();
    setResolvingURL(false);
    setResolutionError(null);
    return invalidate;
  }, [workspaceId]);
  const resolve = async (value: string) => {
    const token = ++generation.current;
    inspectionController.current?.abort();
    const controller = new AbortController();
    inspectionController.current = controller;
    setResolvingURL(true);
    setResolutionError(null);
    try {
      const verified = await inspectPastedRepositoryURL(
        workspaceId,
        value,
        accessible,
        controller.signal,
      );
      if (!isInspectionCurrent(token, generation, controller)) return;
      if (!verified) {
        setResolutionError(t("projects:repositoryUrlUnavailable"));
        return;
      }
      onSelect(remoteRepositoryFromSelection(verified));
      onClose();
    } catch {
      if (isInspectionCurrent(token, generation, controller)) {
        setResolutionError(t("projects:repositoryUrlInspectionFailed"));
      }
    } finally {
      if (token === generation.current) {
        inspectionController.current = null;
        setResolvingURL(false);
      }
    }
  };
  return {
    resolvingURL,
    resolutionError,
    resolve,
    invalidate,
    cancel,
    clearError: () => setResolutionError(null),
  };
}

function isInspectionCurrent(
  token: number,
  generation: { current: number },
  controller: AbortController,
) {
  return token === generation.current && !controller.signal.aborted;
}

function SelectedRepositoryChips({
  selected,
  disabled,
  onRemove,
}: Pick<RepositoryPickerProps, "selected" | "disabled" | "onRemove">) {
  const { t } = useTranslation();
  return selected.map((repository) => (
    <span
      key={projectRemoteRepositoryKey(repository)}
      className="inline-flex min-w-0 max-w-full items-center gap-1 rounded-md border bg-background px-2 text-sm"
      data-testid="agent-project-repository-chip"
    >
      <RemoteRepositoryProviderIcon provider={repository.provider} />
      <span className="max-w-[15rem] truncate">{repository.fullName}</span>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        disabled={disabled}
        aria-label={t("projects:removeRepository", { name: repository.fullName })}
        className={cn(controlSizingClassName("icon"), "shrink-0")}
        onClick={() => onRemove(repository)}
        data-testid="agent-project-remove-repository"
      >
        <IconX className="size-3.5" />
      </Button>
    </span>
  ));
}

function ExistingRepositoryOptions({
  existingRepositories,
  selectedRepositoryIds,
  disabled,
  onSelectExisting,
  onClose,
}: Pick<
  RepositoryPickerProps,
  "existingRepositories" | "selectedRepositoryIds" | "disabled" | "onSelectExisting"
> & { onClose: () => void }) {
  const { t } = useTranslation();
  if (existingRepositories.length === 0) return null;
  return (
    <div className="border-b p-2" data-testid="agent-project-existing-repositories">
      <p className="px-2 pb-1 text-xs font-medium text-muted-foreground">
        {t("projects:configuredRepositories")}
      </p>
      <div className="max-h-24 overflow-y-auto">
        {existingRepositories.map((repository) => (
          <button
            key={repository.id}
            type="button"
            disabled={disabled || selectedRepositoryIds.includes(repository.id)}
            className="flex min-h-7 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11 w-full items-center gap-2 rounded-sm px-2 text-left text-xs hover:bg-muted disabled:opacity-50"
            onClick={() => {
              onSelectExisting(repository.id);
              onClose();
            }}
            data-testid="agent-project-existing-repository-option"
          >
            <RemoteRepositoryProviderIcon provider={repository.provider} />
            <span className="truncate">
              {repository.provider_owner
                ? `${repository.provider_owner}/${repository.provider_name || repository.name}`
                : repository.name}
            </span>
          </button>
        ))}
      </div>
    </div>
  );
}

function RepositoryPickerPopover({
  open,
  resolvingURL,
  disabled,
  accessible,
  existingRepositories,
  selectedRepositoryIds,
  selectedIdentities,
  onOpenChange,
  onSelect,
  onSelectExisting,
  onPaste,
}: {
  open: boolean;
  resolvingURL: boolean;
  disabled: boolean;
  accessible: UseRemoteRepositoriesResult;
  existingRepositories: Repository[];
  selectedRepositoryIds: string[];
  selectedIdentities: string[];
  onOpenChange: (open: boolean) => void;
  onSelect: (repository: RemoteRepository) => void;
  onSelectExisting: (repositoryId: string) => void;
  onPaste: (value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          disabled={disabled || resolvingURL}
          className={controlSizingClassName("standard")}
          data-testid="agent-project-add-repository"
        >
          {resolvingURL ? t("projects:resolvingRepository") : t("projects:addRepository")}
        </Button>
      </PopoverTrigger>
      <PopoverContent
        className="w-[420px] max-w-[calc(100vw-2rem)] overflow-hidden p-0"
        align="start"
      >
        <PickerContents
          accessible={accessible}
          existingRepositories={existingRepositories}
          selectedRepositoryIds={selectedRepositoryIds}
          disabled={disabled}
          selectedIdentities={selectedIdentities}
          onOpenChange={onOpenChange}
          onSelect={onSelect}
          onSelectExisting={onSelectExisting}
          onPaste={onPaste}
        />
      </PopoverContent>
    </Popover>
  );
}

function PickerContents({
  accessible,
  existingRepositories,
  selectedRepositoryIds,
  disabled,
  selectedIdentities,
  onOpenChange,
  onSelect,
  onSelectExisting,
  onPaste,
}: {
  accessible: UseRemoteRepositoriesResult;
  existingRepositories: Repository[];
  selectedRepositoryIds: string[];
  disabled: boolean;
  selectedIdentities: string[];
  onOpenChange: (open: boolean) => void;
  onSelect: (repository: RemoteRepository) => void;
  onSelectExisting: (repositoryId: string) => void;
  onPaste: (value: string) => void;
}) {
  return (
    <>
      <ExistingRepositoryOptions
        existingRepositories={existingRepositories}
        selectedRepositoryIds={selectedRepositoryIds}
        disabled={disabled}
        onSelectExisting={onSelectExisting}
        onClose={() => onOpenChange(false)}
      />
      <RemoteRepoPickerContent
        accessible={accessible}
        selectedRepositoryIdentities={selectedIdentities}
        onPick={onSelect}
        onPaste={onPaste}
      />
    </>
  );
}

export function AgentProjectRepositoryPicker(props: RepositoryPickerProps) {
  const {
    workspaceId,
    selected,
    selectedIdentities,
    existingRepositories,
    selectedRepositoryIds,
    disabled,
    onSelect,
    onSelectExisting,
    onRemove,
  } = props;
  const accessible = useRemoteRepositories(workspaceId);
  const [open, setOpen] = useState(false);
  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) inspection.cancel();
    setOpen(nextOpen);
  };
  const inspection = usePastedRepositoryInspection(workspaceId, accessible, onSelect, () =>
    handleOpenChange(false),
  );
  const select = (repository: RemoteRepository) => {
    inspection.cancel();
    inspection.clearError();
    onSelect(repository);
    setOpen(false);
  };
  return (
    <div className="space-y-2" data-testid="agent-project-repositories">
      <div className="flex flex-wrap items-center gap-2">
        <SelectedRepositoryChips selected={selected} disabled={disabled} onRemove={onRemove} />
        <RepositoryPickerPopover
          open={open}
          resolvingURL={inspection.resolvingURL}
          disabled={disabled}
          accessible={accessible}
          existingRepositories={existingRepositories}
          selectedRepositoryIds={selectedRepositoryIds}
          selectedIdentities={selectedIdentities}
          onOpenChange={handleOpenChange}
          onSelect={select}
          onSelectExisting={onSelectExisting}
          onPaste={(value) => void inspection.resolve(value)}
        />
      </div>
      {inspection.resolutionError ? (
        <p className="text-xs text-destructive" role="alert">
          {inspection.resolutionError}
        </p>
      ) : null}
    </div>
  );
}
