"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { IconFolderPlus, IconPlus, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { MobilePickerSheet } from "@/components/task/mobile/mobile-picker-sheet";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { useTaskCreateDialogPopoverContainer } from "@/hooks/use-task-create-dialog-popover-container";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import type { Branch, LocalRepository, Repository } from "@/lib/types/http";
import {
  useRepositoryCloneSource,
  type RepositoryCloneSourceState,
} from "@/hooks/domains/repositories/use-repository-clone-source";
import type {
  RemoteRepository,
  RemoteRepositoryProvider,
  UseRemoteRepositoriesResult,
} from "@/hooks/domains/integrations/use-remote-repositories";
import { RemoteRepositoryProviderIcon } from "@/components/task-create-dialog-remote-repo-provider-tabs";
import { getRemoteRepositoryProviderLabel } from "@/components/task-create-dialog-remote-repo-provider-tabs";
import {
  looksLikeURL,
  looksLikeSupportedRemoteURL,
} from "@/components/workspace-source-picker/remote-url";
import { parseGitHubAnyUrl } from "@/hooks/domains/github/use-pr-info-by-url";
import { normalizeRepoPath } from "@/components/task-create-dialog-repo-chip-utils";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import {
  RepositorySourceUnavailableNotice,
  useRepositoryPickerSource,
  type RepositorySource,
} from "@/components/task-create-dialog-repository-picker-source";
import {
  LocalChoiceList,
  RemoteChoiceList,
  type RepositoryPickerLocalChoice,
} from "@/components/task-create-dialog-repository-picker-results";

export type LocalRepositoryChoice = {
  repositoryId?: string;
  localPath?: string;
  defaultBranch?: string;
  checkoutSource?: "remote_origin";
  expectedOrigin?: string;
  remoteBranches?: Branch[];
};

export type RepositoryPickerProps = {
  repositories: Repository[];
  discoveredRepositories: LocalRepository[];
  accessible: UseRemoteRepositoriesResult;
  workspaceId?: string | null;
  remoteOriginMode?: boolean;
  /** Workspace or task scope used to remember the last eligible source tab. */
  scopeKey?: string;
  onSelectLocal: (choice: LocalRepositoryChoice) => void;
  onSelectRemote: (repository: RemoteRepository) => void;
  onPasteRemote: (url: string) => void;
  onRefresh: () => void;
  refreshing?: boolean;
  onCreateRepository?: () => void;
};

/**
 * Source picker shared by desktop popovers and the phone repository drawer.
 * The tabs are deliberately outside the search input so source selection stays
 * visible while the user searches or pastes a URL.
 */
export function RepositoryPicker({
  repositories,
  discoveredRepositories,
  accessible,
  workspaceId,
  remoteOriginMode = false,
  scopeKey,
  onSelectLocal,
  onSelectRemote,
  onPasteRemote,
  onRefresh,
  refreshing = false,
  onCreateRepository,
}: RepositoryPickerProps) {
  const mobile = useTouchDrawer();
  const [query, setQuery] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const providerIds = useReadyProviderIds(accessible);
  const { activeSource, unavailableProvider, selectSource } = useRepositoryPickerSource(
    providerIds,
    scopeKey,
  );
  const matchesURL = accessible.matchesURL ?? looksLikeSupportedRemoteURL;
  const { search } = accessible;

  useEffect(() => {
    search(query);
  }, [query, search]);

  const localChoices = useMemo(
    () => filterLocalChoices(repositories, discoveredRepositories, query),
    [discoveredRepositories, query, repositories],
  );
  const cloneSourceCandidates = useMemo(
    () =>
      localChoices.map((choice) => ({
        key: choice.key,
        repositoryId: choice.choice.repositoryId,
        localPath: choice.choice.localPath,
      })),
    [localChoices],
  );
  const cloneSources = useRepositoryCloneSource(
    workspaceId ?? null,
    cloneSourceCandidates,
    Boolean(remoteOriginMode && activeSource === "local"),
  );
  const remoteChoices = useMemo(
    () =>
      accessible.repos.filter(
        (repository) =>
          activeSource !== "local" &&
          repository.provider === activeSource &&
          matchesRepositoryQuery(repository, query),
      ),
    [accessible.repos, activeSource, matchesURL, query],
  );
  const activeError =
    activeSource === "local"
      ? undefined
      : accessible.sourceErrors?.find((entry) => entry.provider === activeSource)?.error;
  const isRefreshing = refreshing || accessible.loading;
  const commitURL = () => commitRemoteURL(query, matchesURL, onPasteRemote, () => setQuery(""));
  return (
    <div className="flex min-w-0 flex-col" data-testid="task-repository-picker">
      {unavailableProvider ? <RepositorySourceUnavailableNotice onRefresh={onRefresh} /> : null}
      <RepositorySourceTabs
        activeSource={activeSource}
        providerIds={providerIds}
        onSelect={selectSource}
      />
      <RepositoryPickerSearch
        activeSource={activeSource}
        query={query}
        inputRef={inputRef}
        matchesURL={matchesURL}
        isRefreshing={isRefreshing}
        onQueryChange={setQuery}
        onCommitURL={commitURL}
        onRefresh={onRefresh}
        onCreateRepository={onCreateRepository}
      />
      <RepositoryPickerResults
        activeSource={activeSource}
        mobile={mobile}
        localChoices={localChoices}
        cloneSourceStates={cloneSources.states}
        remoteOriginMode={remoteOriginMode}
        remoteChoices={remoteChoices}
        loading={accessible.loading}
        error={activeError}
        onSelectLocal={onSelectLocal}
        onSelectRemote={onSelectRemote}
        onRefresh={() => {
          cloneSources.refresh();
          onRefresh();
        }}
      />
    </div>
  );
}

function RepositorySourceTabs({
  activeSource,
  providerIds,
  onSelect,
}: {
  activeSource: RepositorySource;
  providerIds: RemoteRepositoryProvider[];
  onSelect: (source: RepositorySource) => void;
}) {
  const { t } = useTranslation();
  return (
    <div
      role="tablist"
      aria-label={t("task:repositorySources")}
      data-testid="task-repository-source-tabs"
      className="flex min-h-11 shrink-0 gap-0 overflow-x-auto border-b border-border/60 bg-muted/20 p-0 sm:min-h-9"
    >
      <SourceTab
        active={activeSource === "local"}
        label={t("task:repositorySourceLocal")}
        onClick={() => onSelect("local")}
        testId="task-repository-source-local"
      />
      {providerIds.map((provider) => (
        <SourceTab
          key={provider}
          active={activeSource === provider}
          label={getRemoteRepositoryProviderLabel(provider)}
          icon={<RemoteRepositoryProviderIcon provider={provider} />}
          onClick={() => onSelect(provider)}
          testId={`task-repository-source-${provider}`}
        />
      ))}
    </div>
  );
}

function RepositoryPickerSearch({
  activeSource,
  query,
  inputRef,
  matchesURL,
  isRefreshing,
  onQueryChange,
  onCommitURL,
  onRefresh,
  onCreateRepository,
}: {
  activeSource: RepositorySource;
  query: string;
  inputRef: React.RefObject<HTMLInputElement | null>;
  matchesURL: (url: string) => boolean;
  isRefreshing: boolean;
  onQueryChange: (value: string) => void;
  onCommitURL: () => boolean;
  onRefresh: () => void;
  onCreateRepository?: () => void;
}) {
  const { t } = useTranslation();
  const supportsURL = isSupportedRemoteURL(query.trim(), matchesURL);
  return (
    <>
      <div className="flex items-center gap-2 px-2 pt-2">
        <input
          ref={inputRef}
          autoFocus
          value={query}
          onChange={(event) => onQueryChange(event.target.value)}
          onPaste={(event) => {
            const pasted = event.clipboardData.getData("text");
            if (isSupportedRemoteURL(pasted.trim(), matchesURL)) {
              event.preventDefault();
              onQueryChange(pasted);
            }
          }}
          onKeyDown={(event) => {
            if (
              event.key === "Enter" &&
              !event.repeat &&
              !event.altKey &&
              !event.ctrlKey &&
              !event.metaKey &&
              !event.shiftKey &&
              !event.nativeEvent.isComposing &&
              (onCommitURL() || looksLikeURL(query.trim()))
            ) {
              event.preventDefault();
            }
          }}
          placeholder={
            activeSource === "local"
              ? t("task:searchRepositories")
              : t("task:searchRepositoriesOrPasteARemote")
          }
          aria-label={t("task:repositoryPickerSearch")}
          data-testid="task-repository-picker-input"
          className="h-11 min-w-0 flex-1 rounded-md border border-border/60 bg-muted/30 px-2 text-xs outline-none placeholder:text-muted-foreground focus:border-border focus:bg-muted sm:h-9"
        />
        <Button
          type="button"
          variant="ghost"
          size="icon"
          onClick={onRefresh}
          disabled={isRefreshing}
          aria-label={t("task:refreshRepositories")}
          data-testid="task-repository-picker-refresh"
          className="size-11 shrink-0 cursor-pointer sm:size-7"
        >
          <IconRefresh className={cn("size-3.5", isRefreshing && "animate-spin")} />
        </Button>
        {onCreateRepository ? (
          <Button
            type="button"
            variant="ghost"
            size="icon"
            onClick={onCreateRepository}
            aria-label={t("task:createNewRepository")}
            data-testid="create-local-repository-button"
            className="size-11 shrink-0 cursor-pointer sm:size-7"
          >
            <IconFolderPlus className="size-3.5" />
          </Button>
        ) : null}
      </div>
      {supportsURL ? (
        <button
          type="button"
          onClick={onCommitURL}
          data-testid="task-repository-paste-url"
          className="mx-2 mt-1 min-h-11 rounded-md px-2 text-left text-xs text-primary hover:bg-muted sm:min-h-8"
        >
          {t("task:pasteRepositoryUrl")}
        </button>
      ) : null}
    </>
  );
}

function RepositoryPickerResults({
  activeSource,
  mobile,
  localChoices,
  cloneSourceStates,
  remoteOriginMode,
  remoteChoices,
  loading,
  error,
  onSelectLocal,
  onSelectRemote,
  onRefresh,
}: {
  activeSource: RepositorySource;
  mobile: boolean;
  localChoices: Array<{ key: string; label: string; path: string; choice: LocalRepositoryChoice }>;
  cloneSourceStates: Record<string, RepositoryCloneSourceState>;
  remoteOriginMode?: boolean;
  remoteChoices: RemoteRepository[];
  loading: boolean;
  error?: Error;
  onSelectLocal: (choice: LocalRepositoryChoice) => void;
  onSelectRemote: (repository: RemoteRepository) => void;
  onRefresh: () => void;
}) {
  return (
    <div
      className={cn(
        "p-1",
        mobile ? "" : "min-h-[104px] max-h-[min(360px,calc(100vh-16rem))] overflow-y-auto",
      )}
      data-testid="task-repository-picker-results"
    >
      {activeSource === "local" ? (
        <LocalChoiceList
          choices={localChoices}
          cloneSourceStates={cloneSourceStates}
          remoteOriginMode={remoteOriginMode}
          onSelect={onSelectLocal}
        />
      ) : (
        <RemoteChoiceList
          repositories={remoteChoices}
          loading={loading}
          error={error}
          onPick={onSelectRemote}
          onRetry={onRefresh}
        />
      )}
    </div>
  );
}

export type AddRepositoryPickerProps = RepositoryPickerProps & {
  onClose?: () => void;
  onOpen?: () => void;
};

/** Opens the same repository picker as a desktop popover or a phone drawer. */
export function AddRepositoryPicker({ onClose, onOpen, ...props }: AddRepositoryPickerProps) {
  const { t } = useTranslation();
  const mobile = useTouchDrawer();
  const [open, setOpen] = useState(false);
  const portalContainer = useTaskCreateDialogPopoverContainer();
  const close = () => {
    setOpen(false);
    onClose?.();
  };
  const picker = (
    <RepositoryPicker
      {...props}
      onSelectLocal={(choice) => {
        props.onSelectLocal(choice);
        close();
      }}
      onSelectRemote={(repository) => {
        props.onSelectRemote(repository);
        close();
      }}
      onPasteRemote={(url) => {
        props.onPasteRemote(url);
        close();
      }}
      onCreateRepository={
        props.onCreateRepository
          ? () => {
              props.onCreateRepository?.();
              close();
            }
          : undefined
      }
    />
  );
  const trigger = (
    <button
      type="button"
      onClick={() => {
        onOpen?.();
        setOpen(true);
      }}
      aria-label={t("task:addRepository")}
      data-testid="add-repository"
      className="inline-flex min-h-11 items-center justify-center gap-1.5 rounded-md px-2 text-xs text-muted-foreground hover:bg-muted hover:text-foreground sm:min-h-7 sm:px-2 sm:text-[11px]"
    >
      <IconPlus className="size-3.5" />
      <span>{t("task:addRepository")}</span>
    </button>
  );
  if (mobile) {
    return (
      <>
        {trigger}
        <MobilePickerSheet
          open={open}
          onOpenChange={(nextOpen) => (nextOpen ? setOpen(true) : close())}
          title={t("task:addRepository")}
          contentTestId="task-repository-picker-sheet-content"
        >
          {picker}
        </MobilePickerSheet>
      </>
    );
  }
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{trigger}</PopoverTrigger>
      <PopoverContent
        align="start"
        className="w-[400px] max-w-[calc(100vw-2rem)] overflow-hidden p-0"
        data-testid="task-repository-picker-popover"
        portalContainer={portalContainer}
      >
        {picker}
      </PopoverContent>
    </Popover>
  );
}

function useReadyProviderIds(accessible: UseRemoteRepositoriesResult): RemoteRepositoryProvider[] {
  return useMemo(() => {
    if (accessible.providerCatalog) {
      return accessible.providerCatalog
        .filter((entry) => entry.readiness === "ready")
        .map((entry) => entry.provider);
    }
    return accessible.availableProviders;
  }, [accessible.availableProviders, accessible.providerCatalog]);
}

function SourceTab({
  active,
  label,
  icon,
  onClick,
  testId,
}: {
  active: boolean;
  label: string;
  icon?: React.ReactNode;
  onClick: () => void;
  testId: string;
}) {
  return (
    <button
      type="button"
      role="tab"
      aria-selected={active}
      onClick={onClick}
      data-testid={testId}
      className={cn(
        "inline-flex min-h-11 shrink-0 items-center justify-center gap-1.5 border-b-2 px-3 text-xs font-medium whitespace-nowrap cursor-pointer sm:min-h-9",
        active
          ? "border-primary text-foreground"
          : "border-transparent text-muted-foreground hover:bg-muted/60 hover:text-foreground",
      )}
    >
      {icon}
      {label}
    </button>
  );
}

function filterLocalChoices(
  repositories: Repository[],
  discoveredRepositories: LocalRepository[],
  query: string,
): RepositoryPickerLocalChoice[] {
  const needle = query.trim().toLowerCase();
  const localRepositories = repositories.filter(
    (repository) => repository.local_path.trim() !== "",
  );
  const workspacePaths = new Set(
    localRepositories.map((repository) => normalizeRepoPath(repository.local_path)),
  );
  const workspace = localRepositories.map((repository) => ({
    key: `workspace:${repository.id}`,
    label: repository.name,
    path: repository.local_path,
    choice: { repositoryId: repository.id, defaultBranch: repository.default_branch },
  }));
  const discovered = discoveredRepositories
    .filter((repository) => !workspacePaths.has(normalizeRepoPath(repository.path)))
    .map((repository) => ({
      key: `path:${repository.path}`,
      label: repository.name || repository.path,
      path: repository.path,
      choice: { localPath: repository.path, defaultBranch: repository.default_branch },
    }));
  return [...workspace, ...discovered].filter((option) =>
    needle ? `${option.label} ${option.path}`.toLowerCase().includes(needle) : true,
  );
}

function matchesRepositoryQuery(repository: RemoteRepository, query: string): boolean {
  const needle = query.trim().toLowerCase();
  if (!needle) return true;
  return [repository.fullName, repository.url, repository.providerHost]
    .filter(Boolean)
    .some((value) => value!.toLowerCase().includes(needle));
}

function isSupportedRemoteURL(value: string, matchesURL: (url: string) => boolean): boolean {
  return Boolean(value && (parseGitHubAnyUrl(value) || matchesURL(value)));
}

function commitRemoteURL(
  query: string,
  matchesURL: (url: string) => boolean,
  onPasteRemote: (url: string) => void,
  clearQuery: () => void,
): boolean {
  const trimmed = query.trim();
  if (!isSupportedRemoteURL(trimmed, matchesURL)) return false;
  onPasteRemote(trimmed);
  clearQuery();
  return true;
}
