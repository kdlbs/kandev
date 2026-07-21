"use client";

import Link from "@/components/routing/app-link";
import { useCallback, useEffect, useMemo, useState } from "react";
import { IconBrandGitlab, IconMenu2 } from "@tabler/icons-react";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Button } from "@kandev/ui/button";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@kandev/ui/sheet";
import { PageTopbar } from "@/components/page-topbar";
import { fetchGitLabStatus } from "@/lib/api/domains/gitlab-api";
import type { GitLabStatus, Issue, MR } from "@/lib/types/gitlab";
import { MRList } from "@/components/gitlab/my-gitlab/mr-list";
import { IssueList } from "@/components/gitlab/my-gitlab/issue-list";
import {
  PresetsSidebar,
  type SidebarSelection,
} from "@/components/gitlab/my-gitlab/presets-sidebar";
import { PresetsScopeBar } from "@/components/gitlab/my-gitlab/presets-scope-bar";
import { MR_PRESETS, ISSUE_PRESETS } from "@/components/gitlab/my-gitlab/presets";
import { useGitLabSearch } from "@/components/gitlab/my-gitlab/use-gitlab-search";
import { useSavedPresets, type SavedPreset } from "@/components/gitlab/my-gitlab/use-saved-presets";
import {
  useKnownProjects,
  resetKnownProjectsStore,
} from "@/components/gitlab/my-gitlab/use-known-projects";
import { useCommittedQuery } from "@/components/gitlab/my-gitlab/use-committed-query";
import { ListToolbar } from "@/components/gitlab/my-gitlab/list-toolbar";
import { ResultsPagination } from "@/components/gitlab/my-gitlab/results-pagination";
import { SavePresetDialog } from "@/components/gitlab/my-gitlab/save-preset-dialog";
import { useMRKeyToTasks } from "@/hooks/domains/gitlab/use-mr-key-to-tasks";
import { useGitLabActionPresets } from "@/hooks/domains/gitlab/use-gitlab-action-presets";
import { useAllWorkflowSnapshots } from "@/hooks/domains/kanban/use-all-workflow-snapshots";
import type { Repository, Workflow, WorkflowStep } from "@/lib/types/http";
import {
  QuickTaskLauncher,
  type GitLabLaunchPayload,
  type GitLabTaskPreset,
} from "@/components/gitlab/my-gitlab/quick-task-launcher";
import { toGitLabTaskPreset } from "@/components/gitlab/my-gitlab/task-presets";
import { useAppStore } from "@/components/state-provider";

type GitLabPageClientProps = {
  workspaceId?: string;
  workflows?: Workflow[];
  steps?: WorkflowStep[];
  repositories?: Repository[];
};

function PageHeader({
  host,
  onOpenMobileSidebar,
}: {
  host: string;
  onOpenMobileSidebar?: () => void;
}) {
  return (
    <PageTopbar
      title="GitLab"
      subtitle={`${host} · merge requests and issues`}
      icon={<IconBrandGitlab className="h-4 w-4" />}
      actions={
        onOpenMobileSidebar && (
          <Button
            variant="outline"
            size="icon-lg"
            onClick={onOpenMobileSidebar}
            className="h-11 w-11 md:hidden cursor-pointer"
            data-testid="gitlab-mobile-menu-button"
            aria-label="Open GitLab filters"
          >
            <IconMenu2 className="h-4 w-4" />
          </Button>
        )
      }
    />
  );
}

function NotConnectedNotice({ reconnect }: { reconnect?: boolean }) {
  return (
    <Alert>
      <AlertDescription>
        {reconnect
          ? "GitLab credentials are configured, but authentication failed. Reconnect in "
          : "GitLab is not connected. Configure GitLab authentication in "}
        <Link href="/settings/integrations/gitlab" className="underline font-medium cursor-pointer">
          Settings → GitLab
        </Link>{" "}
        to see your merge requests and issues.
      </AlertDescription>
    </Alert>
  );
}

function resolveTitle(selection: SidebarSelection, saved: SavedPreset[]): string {
  if (selection.source === "saved") {
    return saved.find((p) => p.id === selection.id)?.label ?? "Saved query";
  }
  const presets = selection.kind === "mr" ? MR_PRESETS : ISSUE_PRESETS;
  return (
    presets.find((p) => p.value === selection.id)?.label ??
    (selection.kind === "mr" ? "Merge requests" : "Issues")
  );
}

function ResultsList({
  selection,
  items,
  loading,
  error,
  mrPresets,
  issuePresets,
  onStartTask,
  mrKeyToTasks,
  workspaceId,
  host,
}: {
  selection: SidebarSelection;
  items: Array<MR | Issue>;
  loading: boolean;
  error: string | null;
  mrPresets: GitLabTaskPreset[];
  issuePresets: GitLabTaskPreset[];
  onStartTask: (payload: GitLabLaunchPayload) => void;
  mrKeyToTasks: ReturnType<typeof useMRKeyToTasks>;
  workspaceId?: string;
  host: string;
}) {
  if (selection.kind === "mr") {
    return (
      <MRList
        items={items as MR[]}
        loading={loading}
        error={error}
        presets={mrPresets}
        onStartTask={onStartTask}
        mrKeyToTasks={mrKeyToTasks}
      />
    );
  }
  return (
    <IssueList
      items={items as Issue[]}
      loading={loading}
      error={error}
      presets={issuePresets}
      onStartTask={onStartTask}
      workspaceId={workspaceId}
      host={host}
    />
  );
}

type GitLabPageState = ReturnType<typeof useGitLabPageState>;

function useProjectOptions(
  selection: SidebarSelection,
  committedQuery: string,
  items: Array<MR | Issue>,
  projectFilter: string,
): string[] {
  const pageProjects = useMemo(
    () => items.filter((it) => !!it.project_path).map((it) => it.project_path),
    [items],
  );
  const resetKey = `${selection.kind}:${selection.source}:${selection.id}:${committedQuery.trim()}`;
  const knownProjects = useKnownProjects(resetKey, pageProjects);
  return useMemo(() => {
    const set = new Set(knownProjects);
    if (projectFilter) set.add(projectFilter);
    return Array.from(set).sort();
  }, [knownProjects, projectFilter]);
}

function useGitLabPageState(searchEnabled: boolean, workspaceId?: string) {
  const [selection, setSelection] = useState<SidebarSelection>(() => ({
    kind: "mr",
    source: "preset",
    id: MR_PRESETS[0]?.value ?? "",
  }));
  const {
    draft: customQuery,
    committed: committedQuery,
    setDraft: setCustomQuery,
    setImmediate: setQueryImmediate,
    commit: commitCustomQuery,
  } = useCommittedQuery("");
  const [projectFilter, setProjectFilter] = useState("");
  const [saveDialogOpen, setSaveDialogOpen] = useState(false);
  const {
    presets: savedPresets,
    save: saveSavedPreset,
    remove: removeSavedPreset,
  } = useSavedPresets();

  const presets = selection.kind === "mr" ? MR_PRESETS : ISSUE_PRESETS;
  const search = useGitLabSearch({
    workspaceId: workspaceId ?? "",
    kind: selection.kind,
    presets,
    preset: selection.source === "preset" ? selection.id : "",
    customQuery: committedQuery,
    projectFilter,
    enabled: searchEnabled && Boolean(workspaceId),
  });
  const projectOptions = useProjectOptions(
    selection,
    committedQuery,
    search.rawItems,
    projectFilter,
  );
  const title = useMemo(() => resolveTitle(selection, savedPresets), [selection, savedPresets]);

  const onSelect = useCallback(
    (s: SidebarSelection) => {
      setSelection(s);
      if (s.source === "saved") {
        const found = savedPresets.find((p) => p.id === s.id);
        setQueryImmediate(found?.customQuery ?? "");
        setProjectFilter(found?.projectFilter ?? "");
        return;
      }
      setQueryImmediate("");
      setProjectFilter("");
    },
    [savedPresets, setQueryImmediate],
  );

  // Use committedQuery (not the unflushed draft) so the saved preset always
  // matches what is currently displayed in the list.
  const canSaveCurrent = committedQuery.trim().length > 0 || projectFilter.length > 0;
  const suggestedLabel =
    committedQuery.trim() || (projectFilter ? `In ${projectFilter}` : "Saved query");
  const onOpenSaveDialog = () => {
    if (canSaveCurrent) setSaveDialogOpen(true);
  };
  const onConfirmSave = (label: string) => {
    const created = saveSavedPreset({
      kind: selection.kind,
      label,
      customQuery: committedQuery,
      projectFilter,
    });
    setSelection({ kind: selection.kind, source: "saved", id: created.id });
  };
  const onDeleteSaved = (id: string) => {
    removeSavedPreset(id);
    if (selection.source === "saved" && selection.id === id) {
      const fallbackPresets = selection.kind === "mr" ? MR_PRESETS : ISSUE_PRESETS;
      setSelection({ kind: selection.kind, source: "preset", id: fallbackPresets[0]?.value ?? "" });
      setQueryImmediate("");
      setProjectFilter("");
    }
  };

  return {
    selection,
    customQuery,
    committedQuery,
    setCustomQuery,
    commitCustomQuery,
    projectFilter,
    setProjectFilter,
    savedPresets,
    search,
    projectOptions,
    title,
    onSelect,
    canSaveCurrent,
    suggestedLabel,
    saveDialogOpen,
    setSaveDialogOpen,
    onOpenSaveDialog,
    onConfirmSave,
    onDeleteSaved,
  };
}

function AuthenticatedLayout({
  workspaceId,
  state,
  mrPresets,
  issuePresets,
  onStartTask,
  host,
}: {
  workspaceId?: string;
  state: GitLabPageState;
  mrPresets: GitLabTaskPreset[];
  issuePresets: GitLabTaskPreset[];
  onStartTask: (payload: GitLabLaunchPayload) => void;
  host: string;
}) {
  const { selection, search, projectOptions, title } = state;
  const mrKeyToTasks = useMRKeyToTasks(workspaceId ?? null);
  useAllWorkflowSnapshots(workspaceId ?? null);
  // When the user narrows by project, the toolbar badge shows how many of the
  // current page actually match (so it never reads "47" while three rows
  // render). Pagination still uses search.total so the user can navigate to
  // later pages that may contain more matches.
  const displayedCount = state.projectFilter ? search.items.length : search.total;
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
      />
      <ListToolbar
        title={title}
        count={displayedCount}
        loading={search.loading}
        lastFetchedAt={search.lastFetchedAt}
        customQuery={state.customQuery}
        committedQuery={state.committedQuery}
        onCustomQueryChange={state.setCustomQuery}
        onCommitCustomQuery={state.commitCustomQuery}
        projectFilter={state.projectFilter}
        onProjectFilterChange={state.setProjectFilter}
        projectOptions={projectOptions}
        onRefresh={search.refresh}
      />
      <div className="flex-1 overflow-auto px-6 py-4">
        <ResultsList
          selection={selection}
          items={search.items}
          loading={search.loading}
          error={search.error}
          mrPresets={mrPresets}
          issuePresets={issuePresets}
          onStartTask={onStartTask}
          mrKeyToTasks={mrKeyToTasks}
          workspaceId={workspaceId}
          host={host}
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

function useGitLabStatusFetch(workspaceId: string | undefined, enabled: boolean) {
  const [result, setResult] = useState<{ workspaceId?: string; status: GitLabStatus | null }>({
    workspaceId,
    status: null,
  });
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!enabled || !workspaceId) {
      setLoading(true);
      return;
    }
    let cancelled = false;
    setLoading(true);
    fetchGitLabStatus({ cache: "no-store", workspaceId })
      .then((s) => {
        if (!cancelled) setResult({ workspaceId, status: s });
      })
      .catch(() => {
        if (!cancelled) setResult({ workspaceId, status: null });
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [enabled, workspaceId]);

  const current = enabled && result.workspaceId === workspaceId;
  return { status: current ? result.status : null, loading: !current || loading };
}

function GitLabPageBody({
  statusLoading,
  connected,
  reconnect,
  workspaceId,
  state,
  mrPresets,
  issuePresets,
  onStartTask,
  host,
}: {
  statusLoading: boolean;
  connected: boolean;
  reconnect: boolean;
  workspaceId?: string;
  state: GitLabPageState;
  mrPresets: GitLabTaskPreset[];
  issuePresets: GitLabTaskPreset[];
  onStartTask: (payload: GitLabLaunchPayload) => void;
  host: string;
}) {
  if (statusLoading) {
    return <div className="p-6 text-sm text-muted-foreground">Checking GitLab status…</div>;
  }
  if (!connected) {
    return (
      <div className="p-6 max-w-2xl">
        <NotConnectedNotice reconnect={reconnect} />
      </div>
    );
  }
  return (
    <AuthenticatedLayout
      workspaceId={workspaceId}
      state={state}
      mrPresets={mrPresets}
      issuePresets={issuePresets}
      onStartTask={onStartTask}
      host={host}
    />
  );
}

function GitLabPageOverlays({
  state,
  mobileSidebarOpen,
  setMobileSidebarOpen,
  workspaceId,
  workflows,
  steps,
  repositories,
  configuredHost,
  launchPayload,
  setLaunchPayload,
}: {
  state: GitLabPageState;
  mobileSidebarOpen: boolean;
  setMobileSidebarOpen: (open: boolean) => void;
  workspaceId?: string;
  workflows: Workflow[];
  steps: WorkflowStep[];
  repositories: Repository[];
  configuredHost: string;
  launchPayload: GitLabLaunchPayload | null;
  setLaunchPayload: (payload: GitLabLaunchPayload | null) => void;
}) {
  const onMobileSidebarSelect = (selection: Parameters<typeof state.onSelect>[0]) => {
    state.onSelect(selection);
    setMobileSidebarOpen(false);
  };
  const onMobileSaveCurrent = () => {
    setMobileSidebarOpen(false);
    state.onOpenSaveDialog();
  };
  return (
    <>
      <Sheet open={mobileSidebarOpen} onOpenChange={setMobileSidebarOpen}>
        <SheetContent
          side="right"
          className="w-full sm:max-w-sm overflow-y-auto p-0"
          data-testid="gitlab-mobile-sidebar"
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
          />
        </SheetContent>
      </Sheet>
      <SavePresetDialog
        open={state.saveDialogOpen}
        onOpenChange={state.setSaveDialogOpen}
        kind={state.selection.kind}
        customQuery={state.committedQuery}
        projectFilter={state.projectFilter}
        suggestedLabel={state.suggestedLabel}
        onSave={state.onConfirmSave}
      />
      <QuickTaskLauncher
        workspaceId={workspaceId ?? null}
        configuredHost={configuredHost}
        workflows={workflows}
        steps={steps}
        repositories={repositories}
        payload={launchPayload}
        onClose={() => setLaunchPayload(null)}
      />
    </>
  );
}

function useGitLabWorkspaceScope(serverWorkspaceId?: string) {
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const workspaceId = activeWorkspaceId ?? serverWorkspaceId;
  const switching = Boolean(serverWorkspaceId && workspaceId && serverWorkspaceId !== workspaceId);
  const { status, loading } = useGitLabStatusFetch(workspaceId, !switching);
  useEffect(() => {
    if (switching) window.location.reload();
  }, [switching]);
  return {
    workspaceId,
    switching,
    status,
    statusLoading: switching || loading,
    connected: Boolean(status?.authenticated),
    reconnect: Boolean(status?.token_configured && !status.authenticated),
  };
}

function useGitLabTaskPresets(workspaceId: string | undefined, switching: boolean) {
  const { presets } = useGitLabActionPresets(switching ? null : workspaceId);
  const mrPresets = useMemo(() => (presets?.mr ?? []).map(toGitLabTaskPreset), [presets?.mr]);
  const issuePresets = useMemo(
    () => (presets?.issue ?? []).map(toGitLabTaskPreset),
    [presets?.issue],
  );
  return { mrPresets, issuePresets };
}

export function GitLabPageClient({
  workspaceId,
  workflows = [],
  steps = [],
  repositories = [],
}: GitLabPageClientProps = {}) {
  const scope = useGitLabWorkspaceScope(workspaceId);
  const { status, statusLoading, connected, reconnect } = scope;
  const host = status?.host ?? "https://gitlab.com";
  const state = useGitLabPageState(!statusLoading && connected, scope.workspaceId);
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false);
  const [launchPayload, setLaunchPayload] = useState<GitLabLaunchPayload | null>(null);
  const { mrPresets, issuePresets } = useGitLabTaskPresets(scope.workspaceId, scope.switching);

  useEffect(() => resetKnownProjectsStore, []);

  const onOpenMobileSidebar = useCallback(() => setMobileSidebarOpen(true), []);

  return (
    <div className="h-screen w-full flex flex-col bg-background">
      <PageHeader
        host={host}
        onOpenMobileSidebar={!statusLoading && connected ? onOpenMobileSidebar : undefined}
      />
      <GitLabPageBody
        statusLoading={statusLoading}
        connected={connected}
        reconnect={reconnect}
        workspaceId={scope.workspaceId}
        state={state}
        mrPresets={mrPresets}
        issuePresets={issuePresets}
        onStartTask={setLaunchPayload}
        host={host}
      />
      <GitLabPageOverlays
        state={state}
        mobileSidebarOpen={mobileSidebarOpen}
        setMobileSidebarOpen={setMobileSidebarOpen}
        workspaceId={scope.workspaceId}
        workflows={workflows}
        steps={steps}
        repositories={repositories}
        configuredHost={host}
        launchPayload={launchPayload}
        setLaunchPayload={setLaunchPayload}
      />
    </div>
  );
}
