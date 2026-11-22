"use client";

import { useEffect, useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { Badge } from "@kandev/ui/badge";
import { cn } from "@/lib/utils";
import {
  AlreadyAddedMarker,
  isPlainEnter,
  movePickerFocus,
  PickerLoadingStatus,
  PickerSearchRow,
} from "@/components/picker-controls";
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
  /** Hides submit errors while the connect-provider banner is shown. */
  hideSubmitErrorWhenUnavailable?: boolean;
};

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
  hideSubmitErrorWhenUnavailable = false,
}: RemoteRepositoryPickerPanelProps) {
  const { t } = useTranslation();
  const [value, setValue] = useState("");
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [activeProvider, setActiveProvider] = useState<RemoteRepositoryProvider | null>(null);
  const { search: triggerSearch } = accessible;
  useEffect(() => {
    triggerSearch(value);
  }, [value, triggerSearch]);
  const visibleError =
    hideSubmitErrorWhenUnavailable && accessible.unavailable ? null : submitError;
  const { showProviderTabs, selectedProvider, visibleRepos } = visibleProviderRepositories(
    accessible,
    activeProvider,
  );
  const allowedRepos = isRepoAllowed ? visibleRepos.filter(isRepoAllowed) : visibleRepos;
  return (
    <div className="flex flex-col" onKeyDown={movePickerFocus}>
      <PickerSearchRow
        value={value}
        onChange={(next) => {
          setValue(next);
          setSubmitError(null);
        }}
        onPaste={(event) => {
          const pasted = event.clipboardData.getData("text");
          event.preventDefault();
          setValue(pasted);
        }}
        onKeyDown={(event) => {
          if (!isPlainEnter(event)) return;
          // Enter belongs to the picker: never let it reach a surrounding form or dialog.
          event.preventDefault();
          const result = onSubmitText(value);
          if (result.status === "ignored") return;
          setSubmitError(result.status === "rejected" ? result.message : null);
        }}
        placeholder={placeholder}
        invalid={Boolean(visibleError)}
        testId="remote-repo-input"
        legacyTestId="remote-paste-url-input"
        refresh={
          accessible.refresh
            ? {
                onRefresh: accessible.refresh,
                refreshing: accessible.loading,
                label: t("task:refreshRepositories"),
                testId: "remote-repo-refresh",
              }
            : undefined
        }
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
        <PickerLoadingStatus
          label={t("task:loadingRepositories")}
          testId="remote-repo-picker-loading"
        />
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
      data-picker-option=""
      className={cn(
        "flex min-h-8 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11 w-full items-center justify-between gap-2 rounded-sm px-2 py-1.5 text-xs",
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
