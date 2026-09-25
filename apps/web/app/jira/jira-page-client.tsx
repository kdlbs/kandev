"use client";

import Link from "@/components/routing/app-link";
import { useCallback, useEffect, useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import { IconTicket } from "@tabler/icons-react";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { PageShell } from "@/components/page-shell";
import { getJiraConfig, listJiraProjects, searchJiraTickets } from "@/lib/api/domains/jira-api";
import type { JiraProject, JiraStatus, JiraTicket } from "@/lib/types/jira";
import type { Workflow, WorkflowStep } from "@/lib/types/http";
import { TicketRow } from "@/components/jira/my-jira/ticket-row";
import { useJiraSearch } from "@/components/jira/my-jira/use-jira-search";
import { JiraErrorMessage } from "@/components/jira/jira-ticket-common";
import { JiraTicketDialog } from "@/components/jira/jira-ticket-dialog";
import {
  QuickTaskLauncher,
  type JiraLaunchPayload,
} from "@/components/jira/my-jira/quick-task-launcher";
import { DEFAULT_FILTERS } from "@/components/jira/my-jira/filter-model";
import { useJiraFilterState } from "@/components/jira/my-jira/use-jira-filter-state";
import {
  reconcileStatusesForQuery,
  useProjectStatuses,
} from "@/components/jira/my-jira/use-project-statuses";
import { ListToolbar } from "@/components/jira/my-jira/list-toolbar";
import { FilterBar, hasActiveFilters } from "@/components/jira/my-jira/filter-bar";
import { ResultsPagination } from "@/components/jira/my-jira/results-pagination";
import { JqlEditor } from "@/components/jira/my-jira/jql-editor";
import { useJiraTaskPresets } from "@/components/jira/my-jira/use-task-presets";
import type { JiraTaskPreset } from "@/components/jira/my-jira/presets";
import { useToast } from "@/components/toast-provider";

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
          <Trans i18nKey="jira:notConfiguredNotice">
            Jira is not configured.{" "}
            <Link
              href="/settings/integrations/jira"
              className="underline font-medium cursor-pointer"
            />{" "}
            to see your tickets here.
          </Trans>
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
  const [loaded, setLoaded] = useState(false);
  const [configured, setConfigured] = useState(false);
  const [projects, setProjects] = useState<JiraProject[]>([]);
  const [defaultProjectKey, setDefaultProjectKey] = useState("");

  useEffect(() => {
    let cancelled = false;
    async function load() {
      if (!workspaceId) {
        setLoaded(true);
        return;
      }
      try {
        const cfg = await getJiraConfig({ workspaceId });
        if (cancelled) return;
        const ok = !!cfg && cfg.hasSecret;
        setConfigured(ok);
        setDefaultProjectKey(cfg?.defaultProjectKey ?? "");
        if (ok) {
          void loadUserProjects(workspaceId)
            .then((list) => {
              if (!cancelled) setProjects(list);
            })
            .catch(() => {
              // Non-fatal: the project pill can stay empty while users search other fields.
            });
        }
      } finally {
        if (!cancelled) setLoaded(true);
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, [workspaceId]);

  return { loaded, configured, projects, defaultProjectKey };
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
  const { t } = useTranslation();
  if (error) {
    return (
      <div className="flex justify-center py-16">
        <JiraErrorMessage error={error} />
      </div>
    );
  }
  if (!loading && items.length === 0) {
    return (
      <div className="text-sm text-muted-foreground py-8 text-center">
        {t("jira:noTicketsFound")}
      </div>
    );
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

type JiraFilterState = ReturnType<typeof useJiraFilterState>;
type JiraSearchState = ReturnType<typeof useJiraSearch>;

function AuthenticatedJiraContent({
  state,
  search,
  statusOptions,
  projects,
  presets,
  onStartTask,
  onOpenTicket,
}: {
  state: JiraFilterState;
  search: JiraSearchState;
  statusOptions: JiraStatus[];
  projects: JiraProject[];
  presets: JiraTaskPreset[];
  onStartTask: AuthenticatedViewProps["onStartTask"];
  onOpenTicket: AuthenticatedViewProps["onOpenTicket"];
}) {
  const { t } = useTranslation();
  const { toast } = useToast();

  return (
    <>
      <ListToolbar
        searchText={state.filters.searchText}
        onSearchChange={(searchText) => state.updateFilters({ ...state.filters, searchText })}
        views={state.views}
        activeViewId={state.activeViewId}
        defaultViewId={state.defaultViewId}
        viewsReady={state.viewsReady}
        defaultMutationPending={state.defaultMutationPending}
        viewMutationPending={state.viewMutationPending}
        onSelectView={state.selectView}
        onSetDefaultView={(id) => {
          void state.setDefaultView(id).catch(() => {
            toast({ description: t("jira:defaultViewSaveFailed"), variant: "error" });
          });
        }}
        onDeleteView={(id) => {
          void state.deleteView(id).catch(() => {
            toast({ description: t("jira:defaultViewDeleteFailed"), variant: "error" });
          });
        }}
        onSaveView={(name) => {
          void state.saveCurrentAsView(name).catch((error: unknown) => {
            const message = error instanceof Error ? error.message : String(error);
            toast({
              description: t("jira:saveFailed", { error: message }),
              variant: "error",
            });
          });
        }}
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
      {/* Not a <main>: AppShell owns that landmark, one per page. */}
      <div className="flex-1 overflow-auto px-6 py-2">
        <TicketResults
          items={search.items}
          loading={search.loading}
          error={search.error}
          presets={presets}
          onStartTask={onStartTask}
          onOpen={onOpenTicket}
        />
      </div>
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

function AuthenticatedView({
  workspaceId,
  projects,
  defaultProjectKey,
  presets,
  onStartTask,
  onOpenTicket,
}: AuthenticatedViewProps) {
  const state = useJiraFilterState(defaultProjectKey);
  const search = useJiraSearch(
    workspaceId ?? null,
    state.effectiveJql,
    state.initialSelectionResolved,
  );
  const { options: statusOptions, loaded: statusesLoaded } = useProjectStatuses(
    state.filters.projectKeys,
    workspaceId,
  );

  // When the available status union changes (project selection changed, or
  // statuses finished loading), drop unavailable selections from structured
  // filters. Saved custom JQL remains the exact query the user chose.
  // Gate on statusesLoaded so a saved view's statuses aren't stripped on the
  // first render, before useProjectStatuses has fetched the current project's
  // statuses (options is still [] until then).
  const { filters, updateFilters } = state;
  useEffect(() => {
    const reconciled = reconcileStatusesForQuery(
      statusesLoaded,
      state.customJql,
      filters.statuses,
      statusOptions,
    );
    if (reconciled !== filters.statuses) {
      updateFilters({ ...filters, statuses: reconciled });
    }
  }, [statusesLoaded, statusOptions, filters, state.customJql, updateFilters]);

  return (
    <AuthenticatedJiraContent
      state={state}
      search={search}
      statusOptions={statusOptions}
      projects={projects}
      presets={presets}
      onStartTask={onStartTask}
      onOpenTicket={onOpenTicket}
    />
  );
}

export function JiraPageClient({ workspaceId, workflows, steps }: JiraPageClientProps) {
  const { t } = useTranslation();
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
    <PageShell
      title="Jira"
      subtitle={t("jira:ticketsAcrossYourAtlassianProjects")}
      icon={<IconTicket className="h-4 w-4" />}
      scroll="none"
    >
      <div className="flex min-h-0 w-full flex-1 flex-col bg-background">
        {!loaded && (
          <div className="p-6 text-sm text-muted-foreground">{t("jira:checkingJiraStatus")}</div>
        )}
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
      </div>
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
    </PageShell>
  );
}
