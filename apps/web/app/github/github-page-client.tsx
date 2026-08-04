"use client";

import Link from "@/components/routing/app-link";
import { useCallback, useEffect, useMemo, useRef, useState, type MutableRefObject } from "react";
import { IconBrandGithub, IconMenu2 } from "@tabler/icons-react";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Button } from "@kandev/ui/button";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@kandev/ui/sheet";
import { PageShell } from "@/components/page-shell";
import { useGitHubStatus } from "@/hooks/domains/github/use-github-status";
import { usePRKeyToTasks } from "@/hooks/domains/github/use-pr-key-to-tasks";
import { useIssueKeyToTasks } from "@/hooks/domains/github/use-issue-key-to-tasks";
import type { Repository, Workflow, WorkflowStep } from "@/lib/types/http";
import type { GitHubIssue, GitHubPR, TaskIssueLink, TaskPR } from "@/lib/types/github";
import { PRList } from "@/components/github/my-github/pr-list";
import { IssueList } from "@/components/github/my-github/issue-list";
import {
  PresetsSidebar,
  type SidebarSelection,
} from "@/components/github/my-github/presets-sidebar";
import { PresetsScopeBar } from "@/components/github/my-github/presets-scope-bar";
import {
  PR_PRESETS,
  ISSUE_PRESETS,
  type PresetOption,
} from "@/components/github/my-github/search-bar";
import { useGitHubSearch } from "@/components/github/my-github/use-github-search";
import {
  useDefaultQueryPresets,
  resolvePresetOptions,
} from "@/components/github/my-github/use-default-query-presets";
import type { SavedPreset } from "@/components/github/my-github/use-saved-presets";
import { useSavedPresetActions } from "@/components/github/my-github/use-saved-preset-actions";
import { useKnownRepos, resetKnownReposStore } from "@/components/github/my-github/use-known-repos";
import { useCommittedQuery } from "@/components/github/my-github/use-committed-query";
import { ListToolbar } from "@/components/github/my-github/list-toolbar";
import { ResultsPagination } from "@/components/github/my-github/results-pagination";
import { SavePresetDialog } from "@/components/github/my-github/save-preset-dialog";
import {
  QuickTaskLauncher,
  type LaunchPayload,
  type TaskPreset,
} from "@/components/github/my-github/quick-task-launcher";
import {
  resolvePRPresets,
  resolveIssuePresets,
} from "@/components/github/my-github/action-presets";
import { useGitHubActionPresets } from "@/hooks/domains/github/use-github-action-presets";
import { useAllWorkflowSnapshots } from "@/hooks/domains/kanban/use-all-workflow-snapshots";
import { hasGitHubPersonalActor } from "@/lib/github-auth";
import { Trans, useTranslation } from "react-i18next";

type GitHubPageClientProps = {
  workspaceId?: string;
  workflows: Workflow[];
  steps: WorkflowStep[];
  repositories: Repository[];
};

function NotAuthenticatedNotice({
  workspaceId,
  personalRequired,
}: {
  workspaceId?: string;
  personalRequired: boolean;
}) {
  const { t } = useTranslation();
  const settingsHref = workspaceId
    ? `/settings/workspace/${workspaceId}/integrations/github`
    : "/settings/integrations/github";
  return (
    <Alert>
      <AlertDescription>
        {personalRequired
          ? t("github:connectYourPersonalGithubIdentityTo")
          : t("github:githubIsNotConnectedConfigureWorkspace")}{" "}
        <Trans i18nKey="github:openSettingsToSeePrsAndIssues">
          <Link href={settingsHref} className="underline font-medium cursor-pointer" />
          to see your pull requests and issues.
        </Trans>
      </AlertDescription>
    </Alert>
  );
}

function NoWorkspaceNotice() {
  const { t } = useTranslation();
  return (
    <div className="px-6 py-3 border-b shrink-0">
      <Alert>
        <AlertDescription>{t("github:noWorkspaceConfiguredCreateAWorkspace")}</AlertDescription>
      </Alert>
    </div>
  );
}

function hasGitHubUserIdentity(status: ReturnType<typeof useGitHubStatus>["status"]) {
  return hasGitHubPersonalActor(status);
}

function resolveTitle(
  selection: SidebarSelection,
  saved: SavedPreset[],
  prPresets: PresetOption[],
  issuePresets: PresetOption[],
  t: (key: string, values?: Record<string, unknown>) => string,
): string {
  if (selection.source === "saved") {
    return saved.find((p) => p.id === selection.id)?.label ?? t("github:savedQueryFallback");
  }
  const presets = selection.kind === "pr" ? prPresets : issuePresets;
  return (
    presets.find((p) => p.value === selection.id)?.label ??
    (selection.kind === "pr" ? t("github:titlePullRequests") : t("github:titleIssues"))
  );
}

function ResultsList({
  workspaceId,
  selection,
  items,
  loading,
  error,
  prPresets,
  issuePresets,
  onStartTask,
  prKeyToTasks,
  issueKeyToTasks,
}: {
  workspaceId: string | null;
  selection: SidebarSelection;
  items: Array<GitHubPR | GitHubIssue>;
  loading: boolean;
  error: string | null;
  prPresets: TaskPreset[];
  issuePresets: TaskPreset[];
  onStartTask: (payload: LaunchPayload) => void;
  prKeyToTasks: Map<string, TaskPR[]>;
  issueKeyToTasks: Map<string, TaskIssueLink[]>;
}) {
  if (selection.kind === "pr") {
    return (
      <PRList
        workspaceId={workspaceId}
        items={items as GitHubPR[]}
        loading={loading}
        error={error}
        presets={prPresets}
        onStartTask={onStartTask}
        prKeyToTasks={prKeyToTasks}
      />
    );
  }
  return (
    <IssueList
      items={items as GitHubIssue[]}
      loading={loading}
      error={error}
      presets={issuePresets}
      onStartTask={onStartTask}
      issueKeyToTasks={issueKeyToTasks}
    />
  );
}

type GitHubPageState = ReturnType<typeof useGitHubPageState>;

function useRepoOptions(
  workspaceId: string | null,
  selection: SidebarSelection,
  committedQuery: string,
  items: Array<GitHubPR | GitHubIssue>,
  repoFilter: string,
): string[] {
  const pageRepos = useMemo(
    () =>
      items
        .filter((it) => it.repo_owner && it.repo_name)
        .map((it) => `${it.repo_owner}/${it.repo_name}`),
    [items],
  );
  // Reset the accumulator whenever the query context changes (preset, saved,
  // custom query). Repo filter is deliberately excluded so narrowing doesn't
  // reset — that's the whole point of the accumulator.
  const reposResetKey = `${workspaceId ?? "global"}:${selection.kind}:${selection.source}:${selection.id}:${committedQuery.trim()}`;
  const knownRepos = useKnownRepos(reposResetKey, pageRepos);
  return useMemo(() => {
    const set = new Set(knownRepos);
    if (repoFilter) set.add(repoFilter);
    return Array.from(set).sort();
  }, [knownRepos, repoFilter]);
}

function useResolvedQueryPresets(workspaceId: string | null = null) {
  const { prPresets: storedPr, issuePresets: storedIssue } = useDefaultQueryPresets(workspaceId);
  const pr = useMemo(() => resolvePresetOptions(storedPr, PR_PRESETS), [storedPr]);
  const issue = useMemo(() => resolvePresetOptions(storedIssue, ISSUE_PRESETS), [storedIssue]);
  return { pr, issue };
}

function useInitialSidebarSelection(
  workspaceId: string | null,
  resolvedPrPresets: PresetOption[],
  autoResetSearchRef: MutableRefObject<boolean>,
  setQueryImmediate: (query: string) => void,
  setRepoFilter: (repo: string) => void,
) {
  const userSelectedRef = useRef(false);
  const [selection, setSelection] = useState<SidebarSelection>(() => ({
    kind: "pr",
    source: "preset",
    id: resolvedPrPresets[0]?.value ?? "",
  }));

  useEffect(() => {
    userSelectedRef.current = false;
    autoResetSearchRef.current = true;
  }, [workspaceId]);

  useEffect(() => {
    if (userSelectedRef.current || !autoResetSearchRef.current) return;
    const first = resolvedPrPresets[0];
    setSelection({ kind: "pr", source: "preset", id: first?.value ?? "" });
    setQueryImmediate(first?.filter ?? "");
    setRepoFilter("");
  }, [workspaceId, resolvedPrPresets, autoResetSearchRef, setQueryImmediate, setRepoFilter]);

  const setUserSelection = useCallback((next: SidebarSelection) => {
    userSelectedRef.current = true;
    setSelection(next);
  }, []);

  return { selection, setProgrammaticSelection: setSelection, setUserSelection };
}

function useSidebarSelectionHandler({
  savedPresets,
  resolvedPrPresets,
  resolvedIssuePresets,
  setQueryImmediate,
  setRepoFilter,
  setUserSelection,
  markSearchInteracted,
}: {
  savedPresets: SavedPreset[];
  resolvedPrPresets: PresetOption[];
  resolvedIssuePresets: PresetOption[];
  setQueryImmediate: (query: string) => void;
  setRepoFilter: (repo: string) => void;
  setUserSelection: (next: SidebarSelection) => void;
  markSearchInteracted: () => void;
}) {
  return useCallback(
    (s: SidebarSelection) => {
      markSearchInteracted();
      setUserSelection(s);
      if (s.source === "saved") {
        const found = savedPresets.find((p) => p.id === s.id);
        setQueryImmediate(found?.customQuery ?? "");
        setRepoFilter(found?.repoFilter ?? "");
        return;
      }
      const preset = (s.kind === "pr" ? resolvedPrPresets : resolvedIssuePresets).find(
        (p) => p.value === s.id,
      );
      setQueryImmediate(preset?.filter ?? "");
      setRepoFilter("");
    },
    [
      savedPresets,
      setQueryImmediate,
      resolvedPrPresets,
      resolvedIssuePresets,
      setUserSelection,
      markSearchInteracted,
    ],
  );
}

function useSearchInteractionControls(
  setCustomQueryRaw: (query: string) => void,
  setRepoFilterRaw: (repo: string) => void,
) {
  const autoResetSearchRef = useRef(true);
  const markSearchInteracted = useCallback(() => {
    autoResetSearchRef.current = false;
  }, []);
  const setCustomQuery = useCallback(
    (query: string) => {
      markSearchInteracted();
      setCustomQueryRaw(query);
    },
    [markSearchInteracted, setCustomQueryRaw],
  );
  const setRepoFilter = useCallback(
    (repo: string) => {
      markSearchInteracted();
      setRepoFilterRaw(repo);
    },
    [markSearchInteracted, setRepoFilterRaw],
  );
  return { autoResetSearchRef, markSearchInteracted, setCustomQuery, setRepoFilter };
}

function useGitHubPageState(workspaceId: string | null, enabled: boolean) {
  const { t } = useTranslation();
  const { pr: resolvedPrPresets, issue: resolvedIssuePresets } =
    useResolvedQueryPresets(workspaceId);
  const {
    draft: customQuery,
    committed: committedQuery,
    setDraft: setCustomQueryRaw,
    setImmediate: setQueryImmediate,
    commit: commitCustomQuery,
  } = useCommittedQuery(resolvedPrPresets[0]?.filter ?? "");
  const [repoFilter, setRepoFilterRaw] = useState("");
  const [saveDialogOpen, setSaveDialogOpen] = useState(false);
  const { autoResetSearchRef, markSearchInteracted, setCustomQuery, setRepoFilter } =
    useSearchInteractionControls(setCustomQueryRaw, setRepoFilterRaw);
  const { selection, setProgrammaticSelection, setUserSelection } = useInitialSidebarSelection(
    workspaceId,
    resolvedPrPresets,
    autoResetSearchRef,
    setQueryImmediate,
    setRepoFilterRaw,
  );
  const { savedPresets, onConfirmSave, onDeleteSaved } = useSavedPresetActions({
    workspaceId,
    selection,
    customQuery,
    resolvedPrPresets,
    resolvedIssuePresets,
    setProgrammaticSelection,
    setQueryImmediate,
    setRepoFilter: setRepoFilterRaw,
  });

  const presets = selection.kind === "pr" ? resolvedPrPresets : resolvedIssuePresets;
  const search = useGitHubSearch<GitHubPR | GitHubIssue>({
    enabled,
    kind: selection.kind,
    presets,
    preset: selection.source === "preset" ? selection.id : "",
    customQuery: committedQuery,
    repoFilter,
    workspaceId,
  });
  const repoOptions = useRepoOptions(
    workspaceId,
    selection,
    committedQuery,
    search.items,
    repoFilter,
  );
  const title = useMemo(
    () => resolveTitle(selection, savedPresets, resolvedPrPresets, resolvedIssuePresets, t),
    [selection, savedPresets, resolvedPrPresets, resolvedIssuePresets, t],
  );

  const onSelect = useSidebarSelectionHandler({
    savedPresets,
    resolvedPrPresets,
    resolvedIssuePresets,
    setQueryImmediate,
    setRepoFilter: setRepoFilterRaw,
    setUserSelection,
    markSearchInteracted,
  });

  const canSaveCurrent = customQuery.trim().length > 0 || repoFilter.length > 0;
  const suggestedLabel = customQuery.trim() || (repoFilter ? `In ${repoFilter}` : "Saved query");
  const onOpenSaveDialog = () => canSaveCurrent && setSaveDialogOpen(true);

  return {
    selection,
    customQuery,
    committedQuery,
    setCustomQuery,
    commitCustomQuery,
    repoFilter,
    setRepoFilter,
    savedPresets,
    search,
    repoOptions,
    title,
    onSelect,
    canSaveCurrent,
    suggestedLabel,
    saveDialogOpen,
    setSaveDialogOpen,
    onOpenSaveDialog,
    onConfirmSave,
    onDeleteSaved,
    resolvedPrPresets,
    resolvedIssuePresets,
  };
}

function AuthenticatedLayout({
  workspaceId,
  state,
  prPresets,
  issuePresets,
  onStartTask,
}: {
  workspaceId: string | undefined;
  state: GitHubPageState;
  prPresets: TaskPreset[];
  issuePresets: TaskPreset[];
  onStartTask: (payload: LaunchPayload) => void;
}) {
  const { selection, search, repoOptions, title } = state;
  const prKeyToTasks = usePRKeyToTasks(workspaceId ?? null);
  const issueKeyToTasks = useIssueKeyToTasks(workspaceId ?? null);
  useAllWorkflowSnapshots(workspaceId ?? null);
  return (
    // Not a <main>: AppShell owns that landmark, one per page.
    <div className="flex-1 flex flex-col min-w-0 overflow-hidden">
      <PresetsScopeBar
        className="hidden md:flex"
        selected={selection}
        onSelect={state.onSelect}
        savedPresets={state.savedPresets}
        onDeleteSaved={state.onDeleteSaved}
        canSaveCurrent={state.canSaveCurrent}
        onSaveCurrent={state.onOpenSaveDialog}
        prPresets={state.resolvedPrPresets}
        issuePresets={state.resolvedIssuePresets}
      />
      <ListToolbar
        title={title}
        count={search.total}
        loading={search.loading}
        lastFetchedAt={search.lastFetchedAt}
        customQuery={state.customQuery}
        committedQuery={state.committedQuery}
        onCustomQueryChange={state.setCustomQuery}
        onCommitCustomQuery={state.commitCustomQuery}
        repoFilter={state.repoFilter}
        onRepoFilterChange={state.setRepoFilter}
        repoOptions={repoOptions}
        onRefresh={search.refresh}
      />
      {!workspaceId && <NoWorkspaceNotice />}
      <div className="flex-1 overflow-auto px-6 py-4">
        <ResultsList
          workspaceId={workspaceId ?? null}
          selection={selection}
          items={search.items}
          loading={search.loading}
          error={search.error}
          prPresets={prPresets}
          issuePresets={issuePresets}
          onStartTask={onStartTask}
          prKeyToTasks={prKeyToTasks}
          issueKeyToTasks={issueKeyToTasks}
        />
      </div>
      <ResultsPagination
        page={search.page}
        pageSize={search.pageSize}
        total={search.total}
        onPageChange={search.setPage}
      />
    </div>
  );
}

export function GitHubPageClient({
  workspaceId,
  workflows,
  steps,
  repositories,
}: GitHubPageClientProps) {
  const { t } = useTranslation();
  const { status, loaded } = useGitHubStatus(workspaceId ?? null);
  const authed = hasGitHubUserIdentity(status);
  const [launchPayload, setLaunchPayload] = useState<LaunchPayload | null>(null);
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false);
  const state = useGitHubPageState(workspaceId ?? null, authed);
  const { presets: storedPresets } = useGitHubActionPresets(workspaceId ?? null);
  const prPresets = useMemo(() => resolvePRPresets(storedPresets), [storedPresets]);
  const issuePresets = useMemo(() => resolveIssuePresets(storedPresets), [storedPresets]);

  // Drop the module-level repo accumulator on page unmount so a later visit
  // doesn't inherit a stale set from the previous navigation.
  useEffect(() => resetKnownReposStore, []);

  const onStartTask = useCallback((payload: LaunchPayload) => setLaunchPayload(payload), []);
  const onCloseLaunch = useCallback(() => setLaunchPayload(null), []);
  const onOpenMobileSidebar = useCallback(() => setMobileSidebarOpen(true), []);
  // Close the mobile sheet after any sidebar selection. KindToggle clicks also
  // route through onSelect — closing on every selection is acceptable UX since
  // the user always wants to see the list after picking a kind or preset.
  // No useCallback: `state` is a fresh object every render, so memoizing
  // these handlers would be deceptive — they'd still be new refs each pass.
  const onMobileSidebarSelect = (s: Parameters<typeof state.onSelect>[0]) => {
    state.onSelect(s);
    setMobileSidebarOpen(false);
  };
  const onMobileSaveCurrent = () => {
    setMobileSidebarOpen(false);
    state.onOpenSaveDialog();
  };

  return (
    <PageShell
      title="GitHub"
      subtitle={t("github:pullRequestsAndIssuesAcrossYour")}
      icon={<IconBrandGithub className="h-4 w-4" />}
      scroll="none"
      actions={
        loaded &&
        authed && (
          <Button
            variant="outline"
            size="icon-lg"
            onClick={onOpenMobileSidebar}
            className="md:hidden cursor-pointer"
            data-testid="github-mobile-menu-button"
            aria-label={t("github:openGithubFilters")}
          >
            <IconMenu2 className="h-4 w-4" />
          </Button>
        )
      }
    >
      <div className="flex min-h-0 w-full flex-1 flex-col bg-background">
        {!loaded && (
          <div className="p-6 text-sm text-muted-foreground">{t("github:checkingGithubStatus")}</div>
        )}
        {loaded && !authed && (
          <div className="p-6 max-w-2xl">
            <NotAuthenticatedNotice
              workspaceId={workspaceId}
              personalRequired={status?.automation?.source === "github_app_installation"}
            />
          </div>
        )}
        {loaded && authed && (
          <AuthenticatedLayout
            workspaceId={workspaceId}
            state={state}
            prPresets={prPresets}
            issuePresets={issuePresets}
            onStartTask={onStartTask}
          />
        )}
      </div>
      <Sheet open={mobileSidebarOpen} onOpenChange={setMobileSidebarOpen}>
        <SheetContent
          side="right"
          className="w-full sm:max-w-sm overflow-y-auto p-0"
          data-testid="github-mobile-sidebar"
        >
          <SheetHeader className="px-4 pt-4 pb-2">
            <SheetTitle>{t("github:filters")}</SheetTitle>
          </SheetHeader>
          <PresetsSidebar
            selected={state.selection}
            onSelect={onMobileSidebarSelect}
            savedPresets={state.savedPresets}
            onDeleteSaved={state.onDeleteSaved}
            canSaveCurrent={state.canSaveCurrent}
            onSaveCurrent={onMobileSaveCurrent}
            prPresets={state.resolvedPrPresets}
            issuePresets={state.resolvedIssuePresets}
          />
        </SheetContent>
      </Sheet>
      <QuickTaskLauncher
        workspaceId={workspaceId ?? null}
        workflows={workflows}
        steps={steps}
        repositories={repositories}
        payload={launchPayload}
        onClose={onCloseLaunch}
      />
      <SavePresetDialog
        open={state.saveDialogOpen}
        onOpenChange={state.setSaveDialogOpen}
        kind={state.selection.kind}
        customQuery={state.customQuery}
        repoFilter={state.repoFilter}
        repoOptions={state.repoOptions}
        suggestedLabel={state.suggestedLabel}
        onSave={state.onConfirmSave}
      />
    </PageShell>
  );
}
