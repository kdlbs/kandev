"use client";

import Link from "next/link";
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
            className="md:hidden cursor-pointer"
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

function NotConnectedNotice() {
  return (
    <Alert>
      <AlertDescription>
        GitLab is not connected. Configure GitLab authentication in{" "}
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
}: {
  selection: SidebarSelection;
  items: Array<MR | Issue>;
  loading: boolean;
  error: string | null;
}) {
  if (selection.kind === "mr") {
    return <MRList items={items as MR[]} loading={loading} error={error} />;
  }
  return <IssueList items={items as Issue[]} loading={loading} error={error} />;
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

function useGitLabPageState() {
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
  const search = useGitLabSearch(
    selection.kind,
    presets,
    selection.source === "preset" ? selection.id : "",
    committedQuery,
    projectFilter,
  );
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

function AuthenticatedLayout({ state }: { state: GitLabPageState }) {
  const { selection, search, projectOptions, title } = state;
  // When the user narrows by project, the toolbar badge shows how many of the
  // current page actually match (so it never reads "47" while three rows
  // render). Pagination still uses search.total so the user can navigate to
  // later pages that may contain more matches.
  const displayedCount = state.projectFilter ? search.items.length : search.total;
  return (
    <div className="flex-1 flex min-h-0">
      <aside
        className="hidden md:flex md:flex-col w-60 border-r overflow-y-auto shrink-0"
        data-testid="gitlab-presets-sidebar-inline"
      >
        <PresetsSidebar
          selected={selection}
          onSelect={state.onSelect}
          savedPresets={state.savedPresets}
          onDeleteSaved={state.onDeleteSaved}
          canSaveCurrent={state.canSaveCurrent}
          onSaveCurrent={state.onOpenSaveDialog}
        />
      </aside>
      <main className="flex-1 flex flex-col min-w-0 overflow-hidden">
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

function useGitLabStatusFetch() {
  const [status, setStatus] = useState<GitLabStatus | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    fetchGitLabStatus({ cache: "no-store" })
      .then((s) => {
        if (!cancelled) setStatus(s);
      })
      .catch(() => {
        if (!cancelled) setStatus(null);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  return { status, loading };
}

export function GitLabPageClient(_props: { workspaceId?: string } = {}) {
  const { status, loading: statusLoading } = useGitLabStatusFetch();
  const state = useGitLabPageState();
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false);

  useEffect(() => resetKnownProjectsStore, []);

  const connected = !!(status?.authenticated || status?.token_configured);
  const host = status?.host ?? "https://gitlab.com";

  const onOpenMobileSidebar = useCallback(() => setMobileSidebarOpen(true), []);
  const { onSelect, onOpenSaveDialog } = state;
  const onMobileSidebarSelect = useCallback(
    (s: Parameters<typeof onSelect>[0]) => {
      onSelect(s);
      setMobileSidebarOpen(false);
    },
    [onSelect],
  );
  const onMobileSaveCurrent = useCallback(() => {
    setMobileSidebarOpen(false);
    onOpenSaveDialog();
  }, [onOpenSaveDialog]);

  return (
    <div className="h-screen w-full flex flex-col bg-background">
      <PageHeader
        host={host}
        onOpenMobileSidebar={!statusLoading && connected ? onOpenMobileSidebar : undefined}
      />
      {statusLoading && (
        <div className="p-6 text-sm text-muted-foreground">Checking GitLab status…</div>
      )}
      {!statusLoading && !connected && (
        <div className="p-6 max-w-2xl">
          <NotConnectedNotice />
        </div>
      )}
      {!statusLoading && connected && <AuthenticatedLayout state={state} />}
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
    </div>
  );
}
