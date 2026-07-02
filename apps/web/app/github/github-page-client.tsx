"use client";

import Link from "@/components/routing/app-link";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { IconBrandGithub, IconMenu2 } from "@tabler/icons-react";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Button } from "@kandev/ui/button";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@kandev/ui/sheet";
import { PageTopbar } from "@/components/page-topbar";
import { useGitHubStatus } from "@/hooks/domains/github/use-github-status";
import { usePRKeyToTasks } from "@/hooks/domains/github/use-pr-key-to-tasks";
import type { Repository, Workflow, WorkflowStep } from "@/lib/types/http";
import type { GitHubIssue, GitHubPR, TaskPR } from "@/lib/types/github";
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
import { useSavedPresets, type SavedPreset } from "@/components/github/my-github/use-saved-presets";
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

type GitHubPageClientProps = {
  workspaceId?: string;
  workflows: Workflow[];
  steps: WorkflowStep[];
  repositories: Repository[];
};

function PageHeader({ onOpenMobileSidebar }: { onOpenMobileSidebar?: () => void }) {
  return (
    <PageTopbar
      title="GitHub"
      subtitle="Pull requests and issues across your repos."
      icon={<IconBrandGithub className="h-4 w-4" />}
      actions={
        onOpenMobileSidebar && (
          <Button
            variant="outline"
            size="icon-lg"
            onClick={onOpenMobileSidebar}
            className="md:hidden cursor-pointer"
            data-testid="github-mobile-menu-button"
            aria-label="Open GitHub filters"
          >
            <IconMenu2 className="h-4 w-4" />
          </Button>
        )
      }
    />
  );
}

function NotAuthenticatedNotice() {
  return (
    <Alert>
      <AlertDescription>
        GitHub is not connected. Configure GitHub authentication (gh CLI or a Personal Access Token)
        in{" "}
        <Link href="/settings" className="underline font-medium cursor-pointer">
          Settings → GitHub
        </Link>{" "}
        to see your pull requests and issues.
      </AlertDescription>
    </Alert>
  );
}

function NoWorkspaceNotice() {
  return (
    <div className="px-6 py-3 border-b shrink-0">
      <Alert>
        <AlertDescription>
          No workspace configured. Create a workspace first to start tasks from PRs/issues.
        </AlertDescription>
      </Alert>
    </div>
  );
}

function resolveTitle(
  selection: SidebarSelection,
  saved: SavedPreset[],
  prPresets: PresetOption[],
  issuePresets: PresetOption[],
): string {
  if (selection.source === "saved") {
    return saved.find((p) => p.id === selection.id)?.label ?? "Saved query";
  }
  const presets = selection.kind === "pr" ? prPresets : issuePresets;
  return (
    presets.find((p) => p.value === selection.id)?.label ??
    (selection.kind === "pr" ? "Pull requests" : "Issues")
  );
}

function ResultsList({
  selection,
  items,
  loading,
  error,
  prPresets,
  issuePresets,
  onStartTask,
  prKeyToTasks,
}: {
  selection: SidebarSelection;
  items: Array<GitHubPR | GitHubIssue>;
  loading: boolean;
  error: string | null;
  prPresets: TaskPreset[];
  issuePresets: TaskPreset[];
  onStartTask: (payload: LaunchPayload) => void;
  prKeyToTasks: Map<string, TaskPR[]>;
}) {
  if (selection.kind === "pr") {
    return (
      <PRList
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
  }, [workspaceId]);

  useEffect(() => {
    if (userSelectedRef.current) return;
    const first = resolvedPrPresets[0];
    setSelection({ kind: "pr", source: "preset", id: first?.value ?? "" });
    setQueryImmediate(first?.filter ?? "");
    setRepoFilter("");
  }, [workspaceId, resolvedPrPresets, setQueryImmediate, setRepoFilter]);

  const setUserSelection = useCallback((next: SidebarSelection) => {
    userSelectedRef.current = true;
    setSelection(next);
  }, []);

  return { selection, setProgrammaticSelection: setSelection, setUserSelection };
}

function firstPresetSelection(
  kind: SidebarSelection["kind"],
  pr: PresetOption[],
  issue: PresetOption[],
) {
  const preset = (kind === "pr" ? pr : issue)[0];
  return {
    selection: { kind, source: "preset", id: preset?.value ?? "" } as SidebarSelection,
    filter: preset?.filter ?? "",
  };
}

function useSidebarSelectionHandler({
  savedPresets,
  resolvedPrPresets,
  resolvedIssuePresets,
  setQueryImmediate,
  setRepoFilter,
  setUserSelection,
}: {
  savedPresets: SavedPreset[];
  resolvedPrPresets: PresetOption[];
  resolvedIssuePresets: PresetOption[];
  setQueryImmediate: (query: string) => void;
  setRepoFilter: (repo: string) => void;
  setUserSelection: (next: SidebarSelection) => void;
}) {
  return useCallback(
    (s: SidebarSelection) => {
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
    [savedPresets, setQueryImmediate, resolvedPrPresets, resolvedIssuePresets, setUserSelection],
  );
}

function useGitHubPageState(workspaceId: string | null) {
  const { pr: resolvedPrPresets, issue: resolvedIssuePresets } =
    useResolvedQueryPresets(workspaceId);
  const {
    draft: customQuery,
    committed: committedQuery,
    setDraft: setCustomQuery,
    setImmediate: setQueryImmediate,
    commit: commitCustomQuery,
  } = useCommittedQuery(resolvedPrPresets[0]?.filter ?? "");
  const [repoFilter, setRepoFilter] = useState("");
  const [saveDialogOpen, setSaveDialogOpen] = useState(false);
  const {
    presets: savedPresets,
    save: saveSavedPreset,
    remove: removeSavedPreset,
  } = useSavedPresets(workspaceId);
  const { selection, setProgrammaticSelection, setUserSelection } = useInitialSidebarSelection(
    workspaceId,
    resolvedPrPresets,
    setQueryImmediate,
    setRepoFilter,
  );

  const presets = selection.kind === "pr" ? resolvedPrPresets : resolvedIssuePresets;
  const search = useGitHubSearch<GitHubPR | GitHubIssue>({
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
    () => resolveTitle(selection, savedPresets, resolvedPrPresets, resolvedIssuePresets),
    [selection, savedPresets, resolvedPrPresets, resolvedIssuePresets],
  );

  const onSelect = useSidebarSelectionHandler({
    savedPresets,
    resolvedPrPresets,
    resolvedIssuePresets,
    setQueryImmediate,
    setRepoFilter,
    setUserSelection,
  });

  const canSaveCurrent = customQuery.trim().length > 0 || repoFilter.length > 0;
  const suggestedLabel = customQuery.trim() || (repoFilter ? `In ${repoFilter}` : "Saved query");
  const onOpenSaveDialog = () => canSaveCurrent && setSaveDialogOpen(true);
  const onConfirmSave = (label: string) => {
    const created = saveSavedPreset({ kind: selection.kind, label, customQuery, repoFilter });
    if (!created) return;
    setProgrammaticSelection({ kind: selection.kind, source: "saved", id: created.id });
  };
  const onDeleteSaved = (id: string) => {
    removeSavedPreset(id);
    if (selection.source === "saved" && selection.id === id) {
      const fallback = firstPresetSelection(
        selection.kind,
        resolvedPrPresets,
        resolvedIssuePresets,
      );
      setProgrammaticSelection(fallback.selection);
      setQueryImmediate(fallback.filter);
      setRepoFilter("");
    }
  };

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
  useAllWorkflowSnapshots(workspaceId ?? null);
  return (
    <main className="flex-1 flex flex-col min-w-0 overflow-hidden">
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
          selection={selection}
          items={search.items}
          loading={search.loading}
          error={search.error}
          prPresets={prPresets}
          issuePresets={issuePresets}
          onStartTask={onStartTask}
          prKeyToTasks={prKeyToTasks}
        />
      </div>
      <ResultsPagination
        page={search.page}
        pageSize={search.pageSize}
        total={search.total}
        onPageChange={search.setPage}
      />
    </main>
  );
}

export function GitHubPageClient({
  workspaceId,
  workflows,
  steps,
  repositories,
}: GitHubPageClientProps) {
  const { status, loaded } = useGitHubStatus();
  const [launchPayload, setLaunchPayload] = useState<LaunchPayload | null>(null);
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false);
  const state = useGitHubPageState(workspaceId ?? null);
  const { presets: storedPresets } = useGitHubActionPresets(workspaceId ?? null);
  const prPresets = useMemo(() => resolvePRPresets(storedPresets), [storedPresets]);
  const issuePresets = useMemo(() => resolveIssuePresets(storedPresets), [storedPresets]);

  // Drop the module-level repo accumulator on page unmount so a later visit
  // doesn't inherit a stale set from the previous navigation.
  useEffect(() => resetKnownReposStore, []);

  const onStartTask = useCallback((payload: LaunchPayload) => setLaunchPayload(payload), []);
  const onCloseLaunch = useCallback(() => setLaunchPayload(null), []);
  const authed = !!status?.authenticated;
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
    <div className="h-screen w-full flex flex-col bg-background">
      <PageHeader onOpenMobileSidebar={loaded && authed ? onOpenMobileSidebar : undefined} />
      {!loaded && <div className="p-6 text-sm text-muted-foreground">Checking GitHub status…</div>}
      {loaded && !authed && (
        <div className="p-6 max-w-2xl">
          <NotAuthenticatedNotice />
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
      <Sheet open={mobileSidebarOpen} onOpenChange={setMobileSidebarOpen}>
        <SheetContent
          side="right"
          className="w-full sm:max-w-sm overflow-y-auto p-0"
          data-testid="github-mobile-sidebar"
        >
          <SheetHeader className="px-4 pt-4 pb-2">
            <SheetTitle>Filters</SheetTitle>
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
        suggestedLabel={state.suggestedLabel}
        onSave={state.onConfirmSave}
      />
    </div>
  );
}
