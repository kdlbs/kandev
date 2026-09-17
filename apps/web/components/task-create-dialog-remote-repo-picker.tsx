"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "@/components/routing/app-link";
import { IconCheck, IconLink } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { Badge } from "@kandev/ui/badge";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Spinner } from "@kandev/ui/spinner";
import { MobilePickerSheet } from "@/components/task/mobile/mobile-picker-sheet";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import type {
  RemoteRepository,
  RemoteRepositoryProvider,
  UseRemoteRepositoriesResult,
} from "@/hooks/domains/integrations/use-remote-repositories";
import { parseGitHubAnyUrl } from "@/hooks/domains/github/use-pr-info-by-url";
import type { TaskRemoteRepoRow } from "@/components/task-create-dialog-types";
import { useTaskCreateDialogPopoverContainer } from "@/hooks/use-task-create-dialog-popover-container";
import {
  RemoteRepositoryProviderIcon,
  RemoteRepoProviderTabs,
} from "@/components/task-create-dialog-remote-repo-provider-tabs";
import { remoteRepositoryMatchesSelection } from "./task-create-dialog-remote-repo-identity";
import {
  looksLikeURL,
  looksLikeSupportedRemoteURL,
} from "@/components/workspace-source-picker/remote-url";
import { Trans, useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";
import type { RemoteRepoChipProps } from "./task-create-dialog-remote-repo-chip";

const TRUNCATE_THRESHOLD = 30;

// --- Repo pill ---------------------------------------------------------------

export function RemoteRepoPill({
  row,
  accessibleRepos,
  selectedRepositoryIdentities,
  onURLChange,
  disabled,
}: {
  row: TaskRemoteRepoRow;
  accessibleRepos: UseRemoteRepositoriesResult;
  selectedRepositoryIdentities: string[];
  onURLChange: RemoteRepoChipProps["onURLChange"];
  disabled?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const usesTouchDrawer = useTouchDrawer();
  const portalContainer = useTaskCreateDialogPopoverContainer();
  const { t } = useTranslation();
  const triggerLabel = useMemo(() => computeTriggerLabel(row), [row]);
  const hasValue = !!row.url;
  const triggerButton = (
    <button
      type="button"
      disabled={disabled}
      onClick={usesTouchDrawer ? () => setOpen(true) : undefined}
      data-testid="remote-repo-chip-trigger"
      className={cn(
        "h-7 inline-flex items-center gap-1.5 rounded-md px-2.5 text-xs bg-transparent",
        usesTouchDrawer && "min-h-11",
        "hover:bg-muted/60 cursor-pointer",
        disabled && "cursor-not-allowed",
        !hasValue && "text-muted-foreground",
      )}
    >
      <RepoTriggerIcon row={row} />
      <span className="truncate max-w-[240px]">{triggerLabel}</span>
    </button>
  );
  const pickerContent = (
    <RemoteRepoPopoverContent
      accessible={accessibleRepos}
      selectedRepositoryIdentities={selectedRepositoryIdentities}
      onPick={(repo) => {
        onURLChange(repo.url, "picker", {
          provider: repo.provider,
          fullName: repo.fullName,
          defaultBranch: repo.defaultBranch,
          ...(repo.provider === "github" ? {} : { remoteUrl: repo.url }),
          ...(repo.providerHost ? { providerHost: repo.providerHost } : {}),
          ...(repo.providerScope ? { providerScope: repo.providerScope } : {}),
          providerRepoId: repo.id,
          providerOwner: repo.owner,
          providerName: repo.name,
        });
        setOpen(false);
      }}
      onPaste={(value) => {
        onURLChange(value, "paste");
        setOpen(false);
      }}
    />
  );
  if (usesTouchDrawer) {
    return (
      <>
        {triggerButton}
        <MobilePickerSheet
          open={open}
          onOpenChange={setOpen}
          title={t("common:repository")}
          contentTestId="remote-repo-popover-content"
        >
          {pickerContent}
        </MobilePickerSheet>
      </>
    );
  }
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{triggerButton}</PopoverTrigger>
      <PopoverContent
        data-testid="remote-repo-popover-content"
        className="w-[380px] max-w-[calc(100vw-2rem)] max-h-[min(420px,calc(100vh-12rem))] overflow-hidden p-0"
        align="start"
        portalContainer={portalContainer}
      >
        {pickerContent}
      </PopoverContent>
    </Popover>
  );
}

function RepoTriggerIcon({ row }: { row: TaskRemoteRepoRow }) {
  if (row.source === "picker" && row.provider) {
    return <RemoteRepositoryProviderIcon provider={row.provider} />;
  }
  return <IconLink className="h-3 w-3 shrink-0 text-muted-foreground" />;
}

export function computeTriggerLabel(row: TaskRemoteRepoRow): string {
  if (!row.url) return t("task:pickOrPasteARepo");
  if (row.source === "picker" && row.fullName) return row.fullName;
  return truncateMiddle(stripScheme(row.url), TRUNCATE_THRESHOLD);
}

function stripScheme(url: string): string {
  return url.replace(/^https?:\/\//, "").replace(/^www\./, "");
}

function truncateMiddle(value: string, max: number): string {
  if (value.length <= max) return value;
  const keep = Math.max(1, Math.floor((max - 1) / 2));
  return `${value.slice(0, keep)}…${value.slice(value.length - keep)}`;
}

// --- Popover content ---------------------------------------------------------

function StagedRemoteUrlHint() {
  return (
    <div className="px-2 pt-1 text-xs text-muted-foreground">
      <Trans i18nKey="task:remoteUrlPressEnter">
        <span className="font-medium text-foreground">Remote URL</span> - press Enter to submit it.
      </Trans>
    </div>
  );
}

function RemoteRepoPopoverContent({
  accessible,
  selectedRepositoryIdentities,
  onPick,
  onPaste,
}: {
  accessible: UseRemoteRepositoriesResult;
  selectedRepositoryIdentities: string[];
  onPick: (repo: RemoteRepository) => void;
  onPaste: (value: string) => void;
}) {
  const { t } = useTranslation();
  const [value, setValue] = useState("");
  const [urlError, setUrlError] = useState<string | null>(null);
  const [activeProvider, setActiveProvider] = useState<RemoteRepositoryProvider | null>(null);
  const { search: triggerSearch } = accessible;
  const matchesURL = accessible.matchesURL ?? looksLikeSupportedRemoteURL;
  useEffect(() => {
    triggerSearch(value);
  }, [value, triggerSearch]);
  const commitURL = (candidate: string) => {
    const trimmed = candidate.trim();
    if (!isSupportedRemoteURL(trimmed, matchesURL)) {
      if (looksLikeURL(trimmed)) {
        setUrlError(t("task:enterRepositoryUrl"));
      }
      return false;
    }
    setUrlError(null);
    onPaste(trimmed);
    return true;
  };
  const visibleUrlError = accessible.unavailable ? null : urlError;
  const hasStagedURL = isSupportedRemoteURL(value.trim(), matchesURL);
  const { showProviderTabs, selectedProvider, visibleRepos } = visibleProviderRepositories(
    accessible,
    activeProvider,
  );
  return (
    <div className="flex flex-col">
      {showProviderTabs && selectedProvider ? (
        <RemoteRepoProviderTabs
          providers={accessible.availableProviders}
          value={selectedProvider}
          onChange={setActiveProvider}
        />
      ) : null}
      <input
        autoFocus
        value={value}
        onChange={(event) => {
          setValue(event.target.value);
          setUrlError(null);
        }}
        onPaste={(event) => {
          const pasted = event.clipboardData.getData("text");
          event.preventDefault();
          setValue(pasted);
        }}
        onKeyDown={(event) => {
          if (
            event.key !== "Enter" ||
            event.defaultPrevented ||
            event.repeat ||
            event.altKey ||
            event.ctrlKey ||
            event.metaKey ||
            event.shiftKey ||
            event.nativeEvent.isComposing ||
            event.keyCode === 229
          )
            return;
          const isURL = looksLikeURL(value.trim());
          if (commitURL(value) || isURL) event.preventDefault();
        }}
        placeholder={t("task:searchRepositoriesOrPasteARemote")}
        aria-label={t("task:searchRepositoriesOrPasteARemote")}
        aria-invalid={visibleUrlError ? true : undefined}
        data-testid="remote-repo-input"
        data-legacy-testid="remote-paste-url-input"
        className={cn(
          "h-11 sm:h-9 mx-2 mt-2 rounded-md px-2 text-xs bg-muted/30 border border-border/60",
          "outline-none focus:bg-muted focus:border-border placeholder:text-muted-foreground",
          visibleUrlError && "border-destructive focus:border-destructive",
        )}
      />
      {hasStagedURL ? <StagedRemoteUrlHint /> : null}
      <PickerList
        accessible={{ ...accessible, repos: visibleRepos }}
        selectedRepositoryIdentities={selectedRepositoryIdentities}
        onPick={onPick}
        urlError={visibleUrlError}
      />
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
function isSupportedRemoteURL(value: string, matchesURL: (url: string) => boolean): boolean {
  return !!parseGitHubAnyUrl(value) || matchesURL(value);
}
function PickerList({
  accessible,
  selectedRepositoryIdentities,
  onPick,
  urlError,
}: {
  accessible: UseRemoteRepositoriesResult;
  selectedRepositoryIdentities: string[];
  onPick: (repo: RemoteRepository) => void;
  urlError: string | null;
}) {
  const { t } = useTranslation();
  const { repos, loading, error } = accessible;
  return (
    <div className="h-56 max-h-[calc(100vh-16rem)] overflow-y-auto p-1">
      {urlError ? (
        <div role="alert" className="px-2 py-3 text-xs text-destructive">
          {urlError}
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
          alreadyAdded={selectedRepositoryIdentities.some((identity) =>
            remoteRepositoryMatchesSelection(repo, identity),
          )}
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
