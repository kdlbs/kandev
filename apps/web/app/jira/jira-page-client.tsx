"use client";

import Link from "@/components/routing/app-link";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { IconTicket } from "@tabler/icons-react";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { PageTopbar } from "@/components/page-topbar";
import { listJiraProjects, searchJiraTickets } from "@/lib/api/domains/jira-api";
import type { JiraProject, JiraTicket } from "@/lib/types/jira";
import type { Workflow, WorkflowStep } from "@/lib/types/http";
import { jiraConfigQueryOptions } from "@/lib/query/query-options";
import { TicketRow } from "@/components/jira/my-jira/ticket-row";
import { useJiraSearch } from "@/components/jira/my-jira/use-jira-search";
import { JiraErrorMessage } from "@/components/jira/jira-ticket-common";
import { JiraTicketDialog } from "@/components/jira/jira-ticket-dialog";
import {
  QuickTaskLauncher,
  type JiraLaunchPayload,
} from "@/components/jira/my-jira/quick-task-launcher";
import {
  DEFAULT_FILTERS,
  filtersToJql,
  type FilterState,
} from "@/components/jira/my-jira/filter-model";
import { DEFAULT_VIEW, useSavedViews } from "@/components/jira/my-jira/use-saved-views";
import {
  reconcileStatuses,
  useProjectStatuses,
} from "@/components/jira/my-jira/use-project-statuses";
import { ListToolbar } from "@/components/jira/my-jira/list-toolbar";
import { FilterBar, hasActiveFilters } from "@/components/jira/my-jira/filter-bar";
import { ResultsPagination } from "@/components/jira/my-jira/results-pagination";
import { JqlEditor } from "@/components/jira/my-jira/jql-editor";
import { useJiraTaskPresets } from "@/components/jira/my-jira/use-task-presets";
import type { JiraTaskPreset } from "@/components/jira/my-jira/presets";

type JiraPageClientProps = {
  workspaceId?: string;
  workflows: Workflow[];
  steps: WorkflowStep[];
};

function NotConfiguredNotice() {
  return (
    <div className="p-6 max-w-2xl">
      <Alert>
        <AlertDescription>
          Jira is not configured.{" "}
          <Link href="/settings/integrations/jira" className="underline font-medium cursor-pointer">
            Configure Jira
          </Link>{" "}
          to see your tickets here.
        </AlertDescription>
      </Alert>
    </div>
  );
}

async function loadUserProjects(workspaceId: string): Promise<JiraProject[]> {
  const [{ projects: all }, search] = await Promise.all([
    listJiraProjects({ workspaceId }),
    searchJiraTickets(
      {
        jql: "(assignee = currentUser() OR reporter = currentUser()) ORDER BY updated DESC",
        maxResults: 100,
      },
      { workspaceId },
    ),
  ]);
  const userKeys = new Set(search.tickets.map((t) => t.projectKey));
  return (all ?? []).filter((p) => userKeys.has(p.key));
}

function useJiraPageData(workspaceId?: string) {
  const [projects, setProjects] = useState<JiraProject[]>([]);
  const [projectsLoaded, setProjectsLoaded] = useState(false);
  const configQuery = useQuery({
    ...jiraConfigQueryOptions(workspaceId),
    enabled: Boolean(workspaceId),
  });
  const configured = Boolean(configQuery.data?.hasSecret);
  const defaultProjectKey = configQuery.data?.defaultProjectKey ?? "";

  useEffect(() => {
    let cancelled = false;
    async function load() {
      if (!workspaceId || !configured) {
        setProjects([]);
        setProjectsLoaded(true);
        return;
      }
      try {
        setProjectsLoaded(false);
        const list = await loadUserProjects(workspaceId);
        if (!cancelled) setProjects(list);
      } catch {
        // Non-fatal: pill will just show empty list. Users can still filter by other dims.
      } finally {
        if (!cancelled) setProjectsLoaded(true);
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, [workspaceId, configured]);

  const loaded = !workspaceId || (configQuery.isFetched && (!configured || projectsLoaded));
  return { loaded, configured, projects, defaultProjectKey };
}

// initialFilters seeds the ticket list with the workspace's default project
// (issue #1588 follow-up) so opening /jira lands on that project pre-selected.
// An empty defaultProjectKey keeps the historical "no project" default.
function initialFilters(defaultProjectKey: string): FilterState {
  const key = defaultProjectKey.trim();
  if (!key) return DEFAULT_VIEW.filters;
  return { ...DEFAULT_VIEW.filters, projectKeys: [key] };
}

function TicketResults({
  items,
  loading,
  error,
  presets,
  onStartTask,
  onOpen,
}: {
  items: JiraTicket[];
  loading: boolean;
  error: string | null;
  presets: JiraTaskPreset[];
  onStartTask: (ticket: JiraTicket, preset: JiraTaskPreset) => void;
  onOpen: (ticket: JiraTicket) => void;
}) {
  if (error) {
    return (
      <div className="flex justify-center py-16">
        <JiraErrorMessage error={error} />
      </div>
    );
  }
  if (!loading && items.length === 0) {
    return <div className="text-sm text-muted-foreground py-8 text-center">No tickets found.</div>;
  }
  return (
    <div>
      {items.map((t) => (
        <TicketRow
          key={t.key}
          ticket={t}
          presets={presets}
          onStartTask={onStartTask}
          onOpen={onOpen}
        />
      ))}
    </div>
  );
}

type AuthenticatedViewProps = {
  workspaceId: string | undefined;
  projects: JiraProject[];
  defaultProjectKey: string;
  presets: JiraTaskPreset[];
  onStartTask: (ticket: JiraTicket, preset: JiraTaskPreset) => void;
  onOpenTicket: (ticket: JiraTicket) => void;
};

function AuthenticatedView({
  workspaceId,
  projects,
  defaultProjectKey,
  presets,
  onStartTask,
  onOpenTicket,
}: AuthenticatedViewProps) {
  const state = useFilterState(defaultProjectKey);
  const search = useJiraSearch(workspaceId ?? null, state.effectiveJql);
  const { options: statusOptions, loaded: statusesLoaded } = useProjectStatuses(
    state.filters.projectKeys,
    workspaceId,
  );

  // When the available status union changes (project selection changed, or
  // statuses finished loading), drop any selected statuses that are no longer
  // offered so the JQL never references a status absent from the selection.
  // Gate on statusesLoaded so a saved view's statuses aren't stripped on the
  // first render, before useProjectStatuses has fetched the current project's
  // statuses (options is still [] until then).
  const { filters, updateFilters } = state;
  useEffect(() => {
    if (!statusesLoaded) return;
    const reconciled = reconcileStatuses(filters.statuses, statusOptions);
    if (reconciled !== filters.statuses) {
      updateFilters({ ...filters, statuses: reconciled });
    }
  }, [statusesLoaded, statusOptions, filters, updateFilters]);

  return (
    <>
      <ListToolbar
        searchText={state.filters.searchText}
        onSearchChange={(searchText) => state.updateFilters({ ...state.filters, searchText })}
        views={state.views}
        activeViewId={state.activeViewId}
        onSelectView={state.selectView}
        onDeleteView={state.deleteView}
        onSaveView={state.saveCurrentAsView}
        count={search.items.length}
        loading={search.loading}
        sort={state.filters.sort}
        onSortChange={(sort) => state.updateFilters({ ...state.filters, sort })}
        onRefresh={search.refresh}
        showJqlEditor={state.showJqlEditor}
        onToggleJqlEditor={() => state.setShowJqlEditor(!state.showJqlEditor)}
      />
      {state.showJqlEditor && (
        <JqlEditor
          composedJql={state.composedJql}
          customJql={state.customJql}
          onApply={state.applyCustomJql}
          onReset={state.resetCustomJql}
        />
      )}
      {state.customJql === null && (
        <FilterBar
          filters={state.filters}
          onChange={state.updateFilters}
          projects={projects}
          statusOptions={statusOptions}
          hasActiveFilters={hasActiveFilters(state.filters)}
          onClear={() => state.updateFilters(DEFAULT_FILTERS)}
        />
      )}
      <main className="flex-1 overflow-auto px-6 py-2">
        <TicketResults
          items={search.items}
          loading={search.loading}
          error={search.error}
          presets={presets}
          onStartTask={onStartTask}
          onOpen={onOpenTicket}
        />
      </main>
      <ResultsPagination
        page={search.page}
        pageSize={search.pageSize}
        itemCount={search.items.length}
        isLast={search.isLast}
        onNext={search.goNext}
        onPrev={search.goPrev}
      />
    </>
  );
}

function useFilterState(defaultProjectKey: string) {
  const savedViews = useSavedViews();
  const [filters, setFilters] = useState<FilterState>(() => initialFilters(defaultProjectKey));
  const [activeViewId, setActiveViewId] = useState<string | null>(DEFAULT_VIEW.id);
  const [customJql, setCustomJql] = useState<string | null>(null);
  const [showJqlEditor, setShowJqlEditor] = useState(false);

  const composedJql = useMemo(() => filtersToJql(filters), [filters]);
  const effectiveJql = customJql ?? composedJql;

  const updateFilters = useCallback((next: FilterState) => {
    setFilters(next);
    setActiveViewId(null);
    setCustomJql(null);
  }, []);

  const selectView = useCallback(
    (id: string) => {
      const v = savedViews.views.find((x) => x.id === id);
      if (!v) return;
      setFilters(v.filters);
      setActiveViewId(id);
      const savedCustomJql = v.customJql ?? null;
      setCustomJql(savedCustomJql);
      // Auto-open the JQL editor when restoring a JQL-backed view so the user
      // can see what's running.
      if (savedCustomJql !== null) setShowJqlEditor(true);
    },
    [savedViews.views],
  );

  const saveCurrentAsView = useCallback(
    (name: string) => {
      const view = savedViews.save(name, filters, customJql);
      setActiveViewId(view.id);
    },
    [savedViews, filters, customJql],
  );

  const resetCustomJql = useCallback(() => setCustomJql(null), []);

  return {
    filters,
    updateFilters,
    views: savedViews.views,
    activeViewId,
    selectView,
    deleteView: savedViews.remove,
    saveCurrentAsView,
    composedJql,
    customJql,
    effectiveJql,
    applyCustomJql: setCustomJql,
    resetCustomJql,
    showJqlEditor,
    setShowJqlEditor,
  };
}

export function JiraPageClient({ workspaceId, workflows, steps }: JiraPageClientProps) {
  const { loaded, configured, projects, defaultProjectKey } = useJiraPageData(workspaceId);
  const { taskPresets } = useJiraTaskPresets();
  const [launchPayload, setLaunchPayload] = useState<JiraLaunchPayload | null>(null);
  const [openTicket, setOpenTicket] = useState<JiraTicket | null>(null);

  const onStartTask = useCallback((ticket: JiraTicket, preset: JiraTaskPreset) => {
    setLaunchPayload({ ticket, preset });
  }, []);
  const onCloseLaunch = useCallback(() => setLaunchPayload(null), []);
  const onOpenTicket = useCallback((ticket: JiraTicket) => setOpenTicket(ticket), []);

  return (
    <div className="h-screen w-full flex flex-col bg-background">
      <PageTopbar
        title="Jira"
        subtitle="Tickets across your Atlassian projects."
        icon={<IconTicket className="h-4 w-4" />}
      />
      {!loaded && <div className="p-6 text-sm text-muted-foreground">Checking Jira status…</div>}
      {loaded && !configured && <NotConfiguredNotice />}
      {loaded && configured && (
        <AuthenticatedView
          workspaceId={workspaceId}
          projects={projects}
          defaultProjectKey={defaultProjectKey}
          presets={taskPresets}
          onStartTask={onStartTask}
          onOpenTicket={onOpenTicket}
        />
      )}
      <QuickTaskLauncher
        workspaceId={workspaceId ?? null}
        workflows={workflows}
        steps={steps}
        payload={launchPayload}
        onClose={onCloseLaunch}
      />
      <JiraTicketDialog
        open={!!openTicket}
        onOpenChange={(v) => {
          if (!v) setOpenTicket(null);
        }}
        workspaceId={workspaceId}
        ticketKey={openTicket?.key}
        initialTicket={openTicket}
        presets={taskPresets}
        onStartTask={(ticket, preset) => {
          setOpenTicket(null);
          onStartTask(ticket, preset);
        }}
      />
    </div>
  );
}
