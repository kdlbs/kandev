"use client";

import { useEffect, useState } from "react";
import { IconCheck } from "@tabler/icons-react";
import { Trans, useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { Badge } from "@kandev/ui/badge";
import { Spinner } from "@kandev/ui/spinner";
import { cn } from "@/lib/utils";
import type {
  RemoteRepository,
  RemoteRepositoryProvider,
  UseRemoteRepositoriesResult,
} from "@/hooks/domains/integrations/use-remote-repositories";
import {
  RemoteRepositoryProviderIcon,
  RemoteRepoProviderTabs,
} from "@/components/task-create-dialog-remote-repo-provider-tabs";

/** Outcome of pressing Enter on typed picker text. */
export type RemoteRepositoryTextSubmitResult =
  | { status: "accepted" }
  | { status: "ignored" }
  | { status: "rejected"; message: string };

export type RemoteRepositoryPickerPanelProps = {
  accessible: UseRemoteRepositoriesResult;
  onPick: (repo: RemoteRepository) => void;
  isRepoSelected: (repo: RemoteRepository) => boolean;
  /** Hides repositories outside the caller's scope. */
  isRepoAllowed?: (repo: RemoteRepository) => boolean;
  /** Whether the typed value can be submitted with Enter (shows `stagedHint`). */
  isTextSubmittable: (value: string) => boolean;
  onSubmitText: (value: string) => RemoteRepositoryTextSubmitResult;
  stagedHint?: React.ReactNode;
  placeholder: string;
};

function isPlainEnter(event: React.KeyboardEvent<HTMLInputElement>): boolean {
  return (
    event.key === "Enter" &&
    !event.defaultPrevented &&
    !event.repeat &&
    !event.altKey &&
    !event.ctrlKey &&
    !event.metaKey &&
    !event.shiftKey &&
    !event.nativeEvent.isComposing &&
    event.keyCode !== 229
  );
}

/**
 * Searchable list of the user's accessible provider repositories. Shared by
 * task creation's remote repository chip and GitHub repository selections.
 */
export function RemoteRepositoryPickerPanel({
  accessible,
  onPick,
  isRepoSelected,
  isRepoAllowed,
  isTextSubmittable,
  onSubmitText,
  stagedHint,
  placeholder,
}: RemoteRepositoryPickerPanelProps) {
  const [value, setValue] = useState("");
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [activeProvider, setActiveProvider] = useState<RemoteRepositoryProvider | null>(null);
  const { search: triggerSearch } = accessible;
  useEffect(() => {
    triggerSearch(value);
  }, [value, triggerSearch]);
  const visibleError = accessible.unavailable ? null : submitError;
  const { showProviderTabs, selectedProvider, visibleRepos } = visibleProviderRepositories(
    accessible,
    activeProvider,
  );
  const allowedRepos = isRepoAllowed ? visibleRepos.filter(isRepoAllowed) : visibleRepos;
  return (
    <div className="flex flex-col">
      <input
        autoFocus
        value={value}
        onChange={(event) => {
          setValue(event.target.value);
          setSubmitError(null);
        }}
        onPaste={(event) => {
          const pasted = event.clipboardData.getData("text");
          event.preventDefault();
          setValue(pasted);
        }}
        onKeyDown={(event) => {
          if (!isPlainEnter(event)) return;
          const result = onSubmitText(value);
          if (result.status === "ignored") return;
          event.preventDefault();
          setSubmitError(result.status === "rejected" ? result.message : null);
        }}
        placeholder={placeholder}
        aria-label={placeholder}
        aria-invalid={visibleError ? true : undefined}
        data-testid="remote-repo-input"
        data-legacy-testid="remote-paste-url-input"
        className={cn(
          "h-11 sm:h-9 mx-2 mt-2 rounded-md px-2 text-xs bg-muted/30 border border-border/60",
          "outline-none focus:bg-muted focus:border-border placeholder:text-muted-foreground",
          visibleError && "border-destructive focus:border-destructive",
        )}
      />
      {stagedHint && isTextSubmittable(value.trim()) ? stagedHint : null}
      <PickerList
        accessible={{ ...accessible, repos: allowedRepos }}
        isRepoSelected={isRepoSelected}
        onPick={onPick}
        submitError={visibleError}
      />
      {showProviderTabs && selectedProvider ? (
        <RemoteRepoProviderTabs
          providers={accessible.availableProviders}
          value={selectedProvider}
          onChange={setActiveProvider}
        />
      ) : null}
    </div>
  );
}

function visibleProviderRepositories(
  accessible: UseRemoteRepositoriesResult,
  activeProvider: RemoteRepositoryProvider | null,
) {
  const showProviderTabs = accessible.availableProviders.length > 1;
  const selectedProvider =
    activeProvider && accessible.availableProviders.includes(activeProvider)
      ? activeProvider
      : accessible.availableProviders[0];
  const visibleRepos = showProviderTabs
    ? accessible.repos.filter((repo) => repo.provider === selectedProvider)
    : accessible.repos;
  return { showProviderTabs, selectedProvider, visibleRepos };
}

function PickerList({
  accessible,
  isRepoSelected,
  onPick,
  submitError,
}: {
  accessible: UseRemoteRepositoriesResult;
  isRepoSelected: (repo: RemoteRepository) => boolean;
  onPick: (repo: RemoteRepository) => void;
  submitError: string | null;
}) {
  const { t } = useTranslation();
  const { repos, loading, error } = accessible;
  return (
    <div className="h-56 max-h-[calc(100vh-16rem)] overflow-y-auto p-1">
      {submitError ? (
        <div role="alert" className="px-2 py-3 text-xs text-destructive">
          {submitError}
        </div>
      ) : null}
      {accessible.unavailable ? <ConnectProvidersBanner /> : null}
      {!accessible.unavailable && loading && repos.length === 0 ? (
        <div
          className="flex items-center gap-2 px-2 py-3 text-xs text-muted-foreground"
          data-testid="remote-repo-picker-loading"
        >
          <Spinner className="size-3" />
          <span>{t("task:loadingRepositories")}</span>
        </div>
      ) : null}
      {!accessible.unavailable && !loading && repos.length === 0 && !error ? (
        <div className="px-2 py-3 text-xs text-muted-foreground">
          {t("task:noRepositoriesFound")}
        </div>
      ) : null}
      {error ? (
        <div className="px-2 py-3 text-xs text-destructive">
          {t("task:couldNotLoadRepositories", { message: error.message })}
        </div>
      ) : null}
      {repos.map((repo) => (
        <RepoOption
          key={`${repo.provider}:${repo.id}`}
          repo={repo}
          alreadyAdded={isRepoSelected(repo)}
          onPick={onPick}
        />
      ))}
    </div>
  );
}

function RepoOption({
  repo,
  alreadyAdded,
  onPick,
}: {
  repo: RemoteRepository;
  alreadyAdded: boolean;
  onPick: (repo: RemoteRepository) => void;
}) {
  const { t } = useTranslation();
  return (
    <button
      type="button"
      onClick={() => onPick(repo)}
      data-testid="remote-repo-option"
      className={cn(
        "flex min-h-11 sm:min-h-8 w-full items-center justify-between gap-2 rounded-sm px-2 py-1.5 text-xs",
        "hover:bg-muted cursor-pointer text-left",
      )}
    >
      <span className="flex min-w-0 items-center gap-2">
        <RemoteRepositoryProviderIcon provider={repo.provider} />
        <span className="truncate">{repo.fullName}</span>
      </span>
      <span className="flex shrink-0 items-center gap-1">
        {alreadyAdded ? <AlreadyAddedMarker /> : null}
        {repo.private ? (
          <Badge variant="outline" className="text-[10px] text-muted-foreground">
            {t("task:private")}
          </Badge>
        ) : null}
      </span>
    </button>
  );
}

function AlreadyAddedMarker() {
  const { t } = useTranslation();
  return (
    <span
      role="img"
      aria-label={t("task:alreadyAdded")}
      data-testid="already-added-repository-marker"
      className="text-primary"
    >
      <IconCheck aria-hidden="true" className="h-4 w-4" />
    </span>
  );
}

function ConnectProvidersBanner() {
  return (
    <div className="px-3 py-3 text-xs text-muted-foreground">
      <Trans i18nKey="task:connectProviderBanner">
        Connect a source control provider in{" "}
        <Link
          href="/settings/integrations"
          className="text-foreground underline underline-offset-2 cursor-pointer"
        >
          Settings
        </Link>{" "}
        to pick from your repositories.
      </Trans>
    </div>
  );
}
