"use client";

import Link from "@/components/routing/app-link";
import { useCallback, useEffect, useMemo, useState } from "react";
import { IconBrandGitlab, IconMenu2 } from "@tabler/icons-react";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Button } from "@kandev/ui/button";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@kandev/ui/sheet";
import { PageShell } from "@/components/page-shell";
import { fetchGitLabStatus } from "@/lib/api/domains/gitlab-api";
import type { GitLabStatus, Issue, MR } from "@/lib/types/gitlab";
import { MRList } from "@/components/gitlab/my-gitlab/mr-list";
import { IssueList } from "@/components/gitlab/my-gitlab/issue-list";
import type { SidebarSelection } from "@/components/gitlab/my-gitlab/presets-sidebar";
import { PresetsSidebar } from "@/components/gitlab/my-gitlab/presets-sidebar";
import { PresetsScopeBar } from "@/components/gitlab/my-gitlab/presets-scope-bar";
import { resetKnownProjectsStore } from "@/components/gitlab/my-gitlab/use-known-projects";
import { ListToolbar } from "@/components/gitlab/my-gitlab/list-toolbar";
import { useGitLabPageState, type GitLabPageState } from "./use-gitlab-page-state";
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
import { Trans, useTranslation } from "react-i18next";

type GitLabPageClientProps = {
  workspaceId?: string;
  workflows?: Workflow[];
  steps?: WorkflowStep[];
  repositories?: Repository[];
};

function NotConnectedNotice({ reconnect }: { reconnect?: boolean }) {
  const { t } = useTranslation();
  return (
    <Alert>
      <AlertDescription>
        {reconnect
          ? t("gitlab:gitlabCredentialsAreConfiguredButAuthentication")
          : t("gitlab:gitlabIsNotConnectedConfigureGitlab")}
        <Trans i18nKey="gitlab:openSettingsToSeeMrsAndIssues">
          <Link
            href="/settings/integrations/gitlab"
            className="underline font-medium cursor-pointer"
          />
          to see your merge requests and issues.
        </Trans>
      </AlertDescription>
    </Alert>
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
        showMilestoneFilter={state.showMilestoneFilter}
        milestone={state.milestone}
        committedMilestone={state.committedMilestone}
        onMilestoneChange={state.setMilestone}
        onCommitMilestone={state.onCommitMilestone}
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
    </div>
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
  const { t } = useTranslation();
  if (statusLoading) {
    return (
      <div className="p-6 text-sm text-muted-foreground">{t("gitlab:checkingGitlabStatus")}</div>
    );
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
  const { t } = useTranslation();
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
            <SheetTitle>{t("gitlab:filters")}</SheetTitle>
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
  const { t } = useTranslation();
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
    <PageShell
      title="GitLab"
      subtitle={t("gitlab:mergeRequestsAndIssues", { host })}
      icon={<IconBrandGitlab className="h-4 w-4" />}
      scroll="none"
      actions={
        !statusLoading &&
        connected && (
          <Button
            variant="outline"
            size="icon-lg"
            onClick={onOpenMobileSidebar}
            className="h-11 w-11 md:hidden cursor-pointer"
            data-testid="gitlab-mobile-menu-button"
            aria-label={t("gitlab:openGitlabFilters")}
          >
            <IconMenu2 className="h-4 w-4" />
          </Button>
        )
      }
    >
      <div className="flex min-h-0 w-full flex-1 flex-col bg-background">
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
      </div>
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
    </PageShell>
  );
}
