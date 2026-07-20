"use client";

import Link from "@/components/routing/app-link";
import { useCallback, useEffect, useState, type ComponentProps } from "react";
import { IconAdjustments, IconChevronLeft, IconChevronRight } from "@tabler/icons-react";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Button } from "@kandev/ui/button";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@kandev/ui/sheet";
import { PageTopbar } from "@/components/page-topbar";
import { AzureDevOpsIcon } from "@/components/icons/azure-devops-icon";
import {
  AzureDevOpsFilters,
  type AzureDevOpsBrowseMode,
  type AzureDevOpsFiltersState,
} from "@/components/azure-devops/azure-devops-filters";
import { AzureDevOpsFeedbackDialog } from "@/components/azure-devops/azure-devops-feedback-dialog";
import {
  AzureDevOpsPullRequestResults,
  AzureDevOpsWorkItemResults,
} from "@/components/azure-devops/azure-devops-results";
import {
  AzureDevOpsTaskLauncher,
  type AzureDevOpsLaunchPayload,
} from "@/components/azure-devops/azure-devops-task-launcher";
import {
  AzureDevOpsScopeBar,
  type AzureDevOpsScopeSelection,
} from "@/components/azure-devops/azure-devops-scope-bar";
import { AzureDevOpsSaveViewDialog } from "@/components/azure-devops/azure-devops-save-view-dialog";
import { presetsForKind } from "@/components/azure-devops/azure-devops-presets";
import {
  useAzureDevOpsConnection,
  useAzureDevOpsPullRequestFeedback,
  useAzureDevOpsPullRequestSearch,
  useAzureDevOpsWorkItemSearch,
} from "@/hooks/domains/azure-devops/use-azure-devops-browse";
import {
  useAzureDevOpsProjects,
  useAzureDevOpsRepositories,
} from "@/hooks/domains/azure-devops/use-azure-devops-projects";
import { useAzureDevOpsSavedViews } from "@/hooks/domains/azure-devops/use-azure-devops-saved-views";
import type { Repository, Workflow, WorkflowStep } from "@/lib/types/http";
import type { AzureDevOpsPullRequest, AzureDevOpsSavedView } from "@/lib/types/azure-devops";

const PAGE_SIZE = 25;
const WORK_ITEMS_MODE: AzureDevOpsBrowseMode = "work-items";
const PULL_REQUESTS_MODE: AzureDevOpsBrowseMode = "pull-requests";
const DEFAULT_WIQL =
  "SELECT [System.Id] FROM WorkItems WHERE [System.TeamProject] = @project ORDER BY [System.ChangedDate] DESC";

const DEFAULT_FILTERS: AzureDevOpsFiltersState = {
  projectId: "",
  repositoryId: "",
  wiql: DEFAULT_WIQL,
  top: 50,
  status: "active",
  creator: "",
  reviewer: "",
};

type PageProps = {
  workspaceId?: string;
  workflows: Workflow[];
  steps: WorkflowStep[];
  repositories: Repository[];
};

function NotConfigured({ workspaceId }: { workspaceId?: string }) {
  const href = workspaceId
    ? `/settings/workspace/${encodeURIComponent(workspaceId)}/integrations/azure-devops`
    : "/settings/integrations/azure-devops";
  return (
    <div className="max-w-2xl p-6">
      <Alert>
        <AlertDescription>
          Azure DevOps is not connected for this workspace.{" "}
          <Link href={href} className="cursor-pointer font-medium underline">
            Configure Azure DevOps
          </Link>
        </AlertDescription>
      </Alert>
    </div>
  );
}

function ResultHeader({
  mode,
  workItemCount,
  pullRequestCount,
}: {
  mode: AzureDevOpsBrowseMode;
  workItemCount: number;
  pullRequestCount: number;
}) {
  const count = mode === WORK_ITEMS_MODE ? workItemCount : pullRequestCount;
  return (
    <div className="flex min-h-12 items-center justify-between border-b px-4">
      <h2 className="text-sm font-semibold">
        {mode === WORK_ITEMS_MODE ? "Work items" : "Pull requests"}
      </h2>
      <span className="text-xs text-muted-foreground">{count} results</span>
    </div>
  );
}

function PullRequestPagination({
  skip,
  count,
  loading,
  onPage,
}: {
  skip: number;
  count: number;
  loading: boolean;
  onPage: (skip: number) => void;
}) {
  if (skip === 0 && count < PAGE_SIZE) return null;
  return (
    <div className="flex items-center justify-between border-t px-4 py-2">
      <span className="text-xs text-muted-foreground">
        {count === 0 ? 0 : skip + 1}-{skip + count}
      </span>
      <div className="flex gap-1">
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          onClick={() => onPage(Math.max(0, skip - PAGE_SIZE))}
          disabled={loading || skip === 0}
          className="cursor-pointer"
          aria-label="Previous pull request page"
        >
          <IconChevronLeft className="h-4 w-4" />
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          onClick={() => onPage(skip + PAGE_SIZE)}
          disabled={loading || count < PAGE_SIZE}
          className="cursor-pointer"
          aria-label="Next pull request page"
        >
          <IconChevronRight className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}

function useBrowseFilters(defaultProjectId?: string) {
  const [filters, setFilters] = useState(DEFAULT_FILTERS);
  useEffect(() => {
    if (defaultProjectId) {
      setFilters((current) =>
        current.projectId ? current : { ...current, projectId: defaultProjectId },
      );
    }
  }, [defaultProjectId]);
  const update = useCallback(
    <K extends keyof AzureDevOpsFiltersState>(key: K, value: AzureDevOpsFiltersState[K]) =>
      setFilters((current) => ({
        ...current,
        [key]: value,
        ...(key === "projectId" ? { repositoryId: "" } : {}),
      })),
    [],
  );
  const replace = useCallback((next: AzureDevOpsFiltersState) => setFilters(next), []);
  return { filters, update, replace };
}

function modeForKind(kind: AzureDevOpsScopeSelection["kind"]): AzureDevOpsBrowseMode {
  return kind === "work_item" ? WORK_ITEMS_MODE : PULL_REQUESTS_MODE;
}

function kindForMode(mode: AzureDevOpsBrowseMode): AzureDevOpsScopeSelection["kind"] {
  return mode === WORK_ITEMS_MODE ? "work_item" : "pull_request";
}

function filtersFromSavedView(
  current: AzureDevOpsFiltersState,
  view: AzureDevOpsSavedView,
): AzureDevOpsFiltersState {
  return {
    ...current,
    projectId: view.projectId,
    repositoryId: view.repositoryId ?? "",
    wiql: view.wiql ?? current.wiql,
    top: view.top ?? 50,
    status: view.status ?? "active",
    creator: view.creator ?? "",
    reviewer: view.reviewer ?? "",
  };
}

function filtersForScope(
  current: AzureDevOpsFiltersState,
  selection: AzureDevOpsScopeSelection,
  views: AzureDevOpsSavedView[],
): AzureDevOpsFiltersState | undefined {
  if (selection.source === "saved") {
    const view = views.find((candidate) => candidate.id === selection.id);
    return view ? filtersFromSavedView(current, view) : undefined;
  }
  const preset = presetsForKind(selection.kind).find(
    (candidate) => candidate.value === selection.id,
  );
  return preset ? { ...current, ...preset.filters } : undefined;
}

function useDefaultAzureRepository(
  repositoryId: string,
  repositories: Array<{ id: string }>,
  update: (key: "repositoryId", value: string) => void,
) {
  useEffect(() => {
    if (!repositoryId && repositories[0]?.id) update("repositoryId", repositories[0].id);
  }, [repositories, repositoryId, update]);
}

type RunAzureSearch = (
  nextSkip?: number,
  override?: { mode: AzureDevOpsBrowseMode; filters: AzureDevOpsFiltersState },
) => void;

function useAzureScopeControls({
  filters,
  replace,
  runSearch,
  views,
  setMode,
  setSkip,
}: {
  filters: AzureDevOpsFiltersState;
  replace: (next: AzureDevOpsFiltersState) => void;
  runSearch: RunAzureSearch;
  views: AzureDevOpsSavedView[];
  setMode: (mode: AzureDevOpsBrowseMode) => void;
  setSkip: (skip: number) => void;
}) {
  const [selection, setSelection] = useState<AzureDevOpsScopeSelection>({
    kind: "work_item",
    source: "preset",
    id: "recent",
  });
  const selectScope = useCallback(
    (next: AzureDevOpsScopeSelection) => {
      setSelection(next);
      const nextMode = modeForKind(next.kind);
      setMode(nextMode);
      setSkip(0);
      const nextFilters = filtersForScope(filters, next, views);
      if (nextFilters) {
        replace(nextFilters);
        runSearch(0, { mode: nextMode, filters: nextFilters });
      }
    },
    [filters, replace, runSearch, setMode, setSkip, views],
  );
  const markCustom = useCallback(
    () => setSelection((current) => ({ ...current, source: "preset", id: "" })),
    [],
  );
  return { selection, setSelection, selectScope, markCustom };
}

function useAzureSavedViewControls({
  mode,
  filters,
  savedViews,
  setSelection,
}: {
  mode: AzureDevOpsBrowseMode;
  filters: AzureDevOpsFiltersState;
  savedViews: ReturnType<typeof useAzureDevOpsSavedViews>;
  setSelection: (selection: AzureDevOpsScopeSelection) => void;
}) {
  const [saveViewOpen, setSaveViewOpen] = useState(false);
  const canSaveCurrent = Boolean(
    filters.projectId && (mode === WORK_ITEMS_MODE ? filters.wiql.trim() : filters.repositoryId),
  );
  const saveCurrentView = useCallback(
    async (label: string) => {
      const view: AzureDevOpsSavedView = {
        id: `az_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`,
        kind: kindForMode(mode),
        label,
        projectId: filters.projectId,
        repositoryId: mode === PULL_REQUESTS_MODE ? filters.repositoryId : undefined,
        wiql: mode === WORK_ITEMS_MODE ? filters.wiql : undefined,
        top: mode === WORK_ITEMS_MODE ? filters.top : undefined,
        status: mode === PULL_REQUESTS_MODE ? filters.status : undefined,
        creator: mode === PULL_REQUESTS_MODE ? filters.creator : undefined,
        reviewer: mode === PULL_REQUESTS_MODE ? filters.reviewer : undefined,
        createdAt: new Date().toISOString(),
      };
      await savedViews.replace([...savedViews.views, view]);
      setSelection({ kind: view.kind, source: "saved", id: view.id });
    },
    [filters, mode, savedViews, setSelection],
  );
  return { saveViewOpen, setSaveViewOpen, canSaveCurrent, saveCurrentView };
}

function useAzureDevOpsPageState(workspaceId?: string) {
  const [mode, setMode] = useState<AzureDevOpsBrowseMode>(WORK_ITEMS_MODE);
  const [mobileFiltersOpen, setMobileFiltersOpen] = useState(false);
  const [launchPayload, setLaunchPayload] = useState<AzureDevOpsLaunchPayload | null>(null);
  const [feedbackOpen, setFeedbackOpen] = useState(false);
  const [skip, setSkip] = useState(0);
  const connection = useAzureDevOpsConnection(workspaceId);
  const projectList = useAzureDevOpsProjects(workspaceId ?? "", !!connection.data?.hasSecret);
  const { filters, update, replace } = useBrowseFilters(
    connection.data?.defaultProjectId || projectList.data[0]?.id,
  );
  const repositoryList = useAzureDevOpsRepositories(workspaceId ?? "", filters.projectId);
  const workItems = useAzureDevOpsWorkItemSearch(workspaceId);
  const pullRequests = useAzureDevOpsPullRequestSearch(workspaceId);
  const feedback = useAzureDevOpsPullRequestFeedback(workspaceId);
  const savedViews = useAzureDevOpsSavedViews(workspaceId);

  useDefaultAzureRepository(filters.repositoryId, repositoryList.data, update);

  const runSearch = useCallback(
    (
      nextSkip: number = 0,
      override?: { mode: AzureDevOpsBrowseMode; filters: AzureDevOpsFiltersState },
    ) => {
      setMobileFiltersOpen(false);
      const searchMode = override?.mode ?? mode;
      const searchFilters = override?.filters ?? filters;
      if (searchMode === WORK_ITEMS_MODE) {
        void workItems.search({
          project: searchFilters.projectId,
          wiql: searchFilters.wiql,
          top: searchFilters.top,
        });
        return;
      }
      setSkip(nextSkip);
      void pullRequests.search({
        project: searchFilters.projectId,
        repository: searchFilters.repositoryId,
        status: searchFilters.status === "all" ? undefined : searchFilters.status,
        creator: searchFilters.creator || undefined,
        reviewer: searchFilters.reviewer || undefined,
        skip: nextSkip,
        top: PAGE_SIZE,
      });
    },
    [filters, mode, pullRequests, workItems],
  );

  const openFeedback = (pullRequest: AzureDevOpsPullRequest) => {
    setFeedbackOpen(true);
    void feedback.load(pullRequest);
  };

  const scope = useAzureScopeControls({
    filters,
    replace,
    runSearch,
    views: savedViews.views,
    setMode,
    setSkip,
  });
  const savedViewControls = useAzureSavedViewControls({
    mode,
    filters,
    savedViews,
    setSelection: scope.setSelection,
  });

  const updateFromUI = useCallback(
    <K extends keyof AzureDevOpsFiltersState>(key: K, value: AzureDevOpsFiltersState[K]) => {
      scope.markCustom();
      update(key, value);
    },
    [scope, update],
  );

  return {
    mode,
    selection: scope.selection,
    selectScope: scope.selectScope,
    saveViewOpen: savedViewControls.saveViewOpen,
    setSaveViewOpen: savedViewControls.setSaveViewOpen,
    mobileFiltersOpen,
    setMobileFiltersOpen,
    launchPayload,
    setLaunchPayload,
    feedbackOpen,
    setFeedbackOpen,
    skip,
    connection,
    projectList,
    filters,
    update: updateFromUI,
    repositoryList,
    workItems,
    pullRequests,
    feedback,
    runSearch,
    openFeedback,
    savedViews,
    canSaveCurrent: savedViewControls.canSaveCurrent,
    saveCurrentView: savedViewControls.saveCurrentView,
  };
}

type PageState = ReturnType<typeof useAzureDevOpsPageState>;

function BrowseResults({ state }: { state: PageState }) {
  return (
    <section className="flex min-w-0 flex-1 flex-col overflow-hidden">
      <ResultHeader
        mode={state.mode}
        workItemCount={state.workItems.data.length}
        pullRequestCount={state.pullRequests.data.length}
      />
      <div className="min-h-0 flex-1 overflow-y-auto">
        {state.mode === WORK_ITEMS_MODE ? (
          <AzureDevOpsWorkItemResults
            items={state.workItems.data}
            loading={state.workItems.loading}
            error={state.workItems.error}
            onStartTask={(item) => state.setLaunchPayload({ kind: "work-item", item })}
          />
        ) : (
          <AzureDevOpsPullRequestResults
            items={state.pullRequests.data}
            loading={state.pullRequests.loading}
            error={state.pullRequests.error}
            onFeedback={state.openFeedback}
            onStartTask={(pullRequest) =>
              state.setLaunchPayload({ kind: "pull-request", pullRequest })
            }
          />
        )}
      </div>
      {state.mode === PULL_REQUESTS_MODE && (
        <PullRequestPagination
          skip={state.skip}
          count={state.pullRequests.data.length}
          loading={state.pullRequests.loading}
          onPage={state.runSearch}
        />
      )}
    </section>
  );
}

function MobileFilters({
  state,
  filterProps,
}: {
  state: PageState;
  filterProps: Omit<ComponentProps<typeof AzureDevOpsFilters>, "idSuffix">;
}) {
  return (
    <Sheet open={state.mobileFiltersOpen} onOpenChange={state.setMobileFiltersOpen}>
      <SheetContent side="left" className="w-80 max-w-[90vw] overflow-y-auto">
        <SheetHeader className="mb-5 text-left">
          <SheetTitle>Azure DevOps filters</SheetTitle>
        </SheetHeader>
        <AzureDevOpsFilters {...filterProps} idSuffix="-mobile" />
      </SheetContent>
    </Sheet>
  );
}

function AzureDevOpsPageContent({ workspaceId, workflows, steps, repositories }: PageProps) {
  const state = useAzureDevOpsPageState(workspaceId);

  if (state.connection.loading) return null;
  if (!state.connection.data?.hasSecret) return <NotConfigured workspaceId={workspaceId} />;

  const searchLoading =
    state.mode === WORK_ITEMS_MODE ? state.workItems.loading : state.pullRequests.loading;
  const filterProps = {
    mode: state.mode,
    filters: state.filters,
    projects: state.projectList.data,
    repositories: state.repositoryList.data,
    loading: searchLoading,
    onChange: state.update,
    onSearch: () => state.runSearch(0),
  };

  return (
    <main className="flex min-h-0 flex-1 flex-col overflow-hidden">
      <PageTopbar
        title="Azure DevOps"
        subtitle={`${state.connection.data.organizationUrl} · Boards and Repos`}
        icon={<AzureDevOpsIcon className="h-4 w-4" />}
        actions={
          <Button
            type="button"
            variant="outline"
            size="icon-lg"
            onClick={() => state.setMobileFiltersOpen(true)}
            className="cursor-pointer md:hidden"
            aria-label="Open Azure DevOps filters"
            data-testid="azure-devops-mobile-filter-button"
          >
            <IconAdjustments className="h-4 w-4" />
          </Button>
        }
      />
      <AzureDevOpsScopeBar
        selected={state.selection}
        onSelect={state.selectScope}
        savedViews={state.savedViews.views}
        onDeleteSaved={(id) => void state.savedViews.remove(id)}
        canSaveCurrent={state.canSaveCurrent}
        onSaveCurrent={() => state.setSaveViewOpen(true)}
      />
      <div className="hidden border-b px-4 py-3 md:block">
        <AzureDevOpsFilters {...filterProps} idSuffix="" compact />
      </div>
      <BrowseResults state={state} />
      <MobileFilters state={state} filterProps={filterProps} />
      <AzureDevOpsFeedbackDialog
        open={state.feedbackOpen}
        loading={state.feedback.loading}
        error={state.feedback.error}
        feedback={state.feedback.data}
        onOpenChange={(open) => {
          state.setFeedbackOpen(open);
          if (!open) state.feedback.clear();
        }}
      />
      <AzureDevOpsTaskLauncher
        workspaceId={workspaceId}
        workflows={workflows}
        steps={steps}
        repositories={repositories}
        payload={state.launchPayload}
        onClose={() => state.setLaunchPayload(null)}
      />
      <AzureDevOpsSaveViewDialog
        open={state.saveViewOpen}
        kind={state.selection.kind}
        onOpenChange={state.setSaveViewOpen}
        onSave={state.saveCurrentView}
      />
    </main>
  );
}

export function AzureDevOpsPageClient(props: PageProps) {
  return <AzureDevOpsPageContent key={props.workspaceId ?? ""} {...props} />;
}
