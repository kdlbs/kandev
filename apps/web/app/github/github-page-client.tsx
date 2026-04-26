"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";
import { IconBrandGithub } from "@tabler/icons-react";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { PageTopbar } from "@/components/page-topbar";
import { useGitHubStatus } from "@/hooks/domains/github/use-github-status";
import type { Repository, Workflow, WorkflowStep } from "@/lib/types/http";
import type { GitHubIssue, GitHubPR } from "@/lib/types/github";
import { PRList } from "@/components/github/my-github/pr-list";
import { IssueList } from "@/components/github/my-github/issue-list";
import {
  PresetsSidebar,
  type SidebarSelection,
} from "@/components/github/my-github/presets-sidebar";
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

type GitHubPageClientProps = {
  workspaceId?: string;
  workflows: Workflow[];
  steps: WorkflowStep[];
  repositories: Repository[];
};

function PageHeader() {
  return (
    <PageTopbar
      title="GitHub"
      subtitle="Pull requests and issues across your repos."
      icon={<IconBrandGithub className="h-4 w-4" />}
    />
  );
}

function NotAuthenticatedNotice() {
  return (
    <Alert>
      <AlertDescription>
        GitHub is not connected. Configure GitHub authentication (gh CLI or a Personal Access Token)
        in{" "}
        <Link href="/settings" className="underline font-medium">
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
}: {
  selection: SidebarSelection;
  items: Array<GitHubPR | GitHubIssue>;
  loading: boolean;
  error: string | null;
  prPresets: TaskPreset[];
  issuePresets: TaskPreset[];
  onStartTask: (payload: LaunchPayload) => void;
}) {
  if (selection.kind === "pr") {
    return (
      <PRList
        items={items as GitHubPR[]}
        loading={loading}
        error={error}
        presets={prPresets}
        onStartTask={onStartTask}
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
  const reposResetKey = `${selection.kind}:${selection.source}:${selection.id}:${committedQuery.trim()}`;
  const knownRepos = useKnownRepos(reposResetKey, pageRepos);
  return useMemo(() => {
    const set = new Set(knownRepos);
    if (repoFilter) set.add(repoFilter);
    return Array.from(set).sort();
  }, [knownRepos, repoFilter]);
}

function useResolvedQueryPresets() {
  const { prPresets: storedPr, issuePresets: storedIssue } = useDefaultQueryPresets();
  const pr = useMemo(() => resolvePresetOptions(storedPr, PR_PRESETS), [storedPr]);
  const issue = useMemo(() => resolvePresetOptions(storedIssue, ISSUE_PRESETS), [storedIssue]);
  return { pr, issue };
}

function useGitHubPageState() {
  const { pr: resolvedPrPresets, issue: resolvedIssuePresets } = useResolvedQueryPresets();

  const [selection, setSelection] = useState<SidebarSelection>(() => ({
    kind: "pr",
    source: "preset",
    id: resolvedPrPresets[0]?.value ?? "",
  }));
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
  } = useSavedPresets();

  const presets = selection.kind === "pr" ? resolvedPrPresets : resolvedIssuePresets;
  const search = useGitHubSearch<GitHubPR | GitHubIssue>(
    selection.kind,
    presets,
    selection.source === "preset" ? selection.id : "",
    committedQuery,
    repoFilter,
  );
  const repoOptions = useRepoOptions(selection, committedQuery, search.items, repoFilter);
  const title = useMemo(
    () => resolveTitle(selection, savedPresets, resolvedPrPresets, resolvedIssuePresets),
    [selection, savedPresets, resolvedPrPresets, resolvedIssuePresets],
  );

  const onSelect = useCallback(
    (s: SidebarSelection) => {
      setSelection(s);
      if (s.source === "saved") {
        const found = savedPresets.find((p) => p.id === s.id);
        setQueryImmediate(found?.customQuery ?? "");
        setRepoFilter(found?.repoFilter ?? "");
        return;
      }
      const presetList = s.kind === "pr" ? resolvedPrPresets : resolvedIssuePresets;
      const preset = presetList.find((p) => p.value === s.id);
      setQueryImmediate(preset?.filter ?? "");
      setRepoFilter("");
    },
    [savedPresets, setQueryImmediate, resolvedPrPresets, resolvedIssuePresets],
  );

  const canSaveCurrent = customQuery.trim().length > 0 || repoFilter.length > 0;
  const suggestedLabel = customQuery.trim() || (repoFilter ? `In ${repoFilter}` : "Saved query");
  const onOpenSaveDialog = () => {
    if (canSaveCurrent) setSaveDialogOpen(true);
  };
  const onConfirmSave = (label: string) => {
    const created = saveSavedPreset({ kind: selection.kind, label, customQuery, repoFilter });
    setSelection({ kind: selection.kind, source: "saved", id: created.id });
  };
  const onDeleteSaved = (id: string) => {
    removeSavedPreset(id);
    if (selection.source === "saved" && selection.id === id) {
      const fallbackPresets = selection.kind === "pr" ? resolvedPrPresets : resolvedIssuePresets;
      setSelection({ kind: selection.kind, source: "preset", id: fallbackPresets[0]?.value ?? "" });
      setQueryImmediate(fallbackPresets[0]?.filter ?? "");
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
  return (
    <div className="flex-1 flex min-h-0">
      <aside className="w-60 border-r overflow-y-auto shrink-0">
        <PresetsSidebar
          selected={selection}
          onSelect={state.onSelect}
          savedPresets={state.savedPresets}
          onDeleteSaved={state.onDeleteSaved}
          canSaveCurrent={state.canSaveCurrent}
          onSaveCurrent={state.onOpenSaveDialog}
          prPresets={state.resolvedPrPresets}
          issuePresets={state.resolvedIssuePresets}
        />
      </aside>
      <main className="flex-1 flex flex-col min-w-0 overflow-hidden">
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
          />
        </div>
        <ResultsPagination
          page={search.page}
          pageSize={search.pageSize}
          total={search.total}
          onPageChange={search.setPage}
        />
      </main>
    </div>
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
  const state = useGitHubPageState();
  const { presets: storedPresets } = useGitHubActionPresets(workspaceId ?? null);
  const prPresets = useMemo(() => resolvePRPresets(storedPresets), [storedPresets]);
  const issuePresets = useMemo(() => resolveIssuePresets(storedPresets), [storedPresets]);

  // Drop the module-level repo accumulator on page unmount so a later visit
  // doesn't inherit a stale set from the previous navigation.
  useEffect(() => resetKnownReposStore, []);

  const onStartTask = useCallback((payload: LaunchPayload) => setLaunchPayload(payload), []);
  const onCloseLaunch = useCallback(() => setLaunchPayload(null), []);
  const authed = !!status?.authenticated;

  return (
    <div className="h-screen w-full flex flex-col bg-background">
      <PageHeader />
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
