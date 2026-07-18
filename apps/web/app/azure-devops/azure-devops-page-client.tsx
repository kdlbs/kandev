"use client";

import Link from "@/components/routing/app-link";
import { useCallback, useEffect, useState, type ComponentProps } from "react";
import {
  IconAdjustments,
  IconBrandAzure,
  IconChevronLeft,
  IconChevronRight,
} from "@tabler/icons-react";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Button } from "@kandev/ui/button";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@kandev/ui/sheet";
import { Tabs, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { PageTopbar } from "@/components/page-topbar";
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
  useAzureDevOpsConnection,
  useAzureDevOpsPullRequestFeedback,
  useAzureDevOpsPullRequestSearch,
  useAzureDevOpsWorkItemSearch,
} from "@/hooks/domains/azure-devops/use-azure-devops-browse";
import {
  useAzureDevOpsProjects,
  useAzureDevOpsRepositories,
} from "@/hooks/domains/azure-devops/use-azure-devops-projects";
import type { Repository, Workflow, WorkflowStep } from "@/lib/types/http";
import type { AzureDevOpsPullRequest } from "@/lib/types/azure-devops";

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
  return { filters, update };
}

function useAzureDevOpsPageState(workspaceId?: string) {
  const [mode, setMode] = useState<AzureDevOpsBrowseMode>(WORK_ITEMS_MODE);
  const [mobileFiltersOpen, setMobileFiltersOpen] = useState(false);
  const [launchPayload, setLaunchPayload] = useState<AzureDevOpsLaunchPayload | null>(null);
  const [feedbackOpen, setFeedbackOpen] = useState(false);
  const [skip, setSkip] = useState(0);
  const connection = useAzureDevOpsConnection(workspaceId);
  const projectList = useAzureDevOpsProjects(workspaceId ?? "", !!connection.data?.hasSecret);
  const { filters, update } = useBrowseFilters(
    connection.data?.defaultProjectId || projectList.data[0]?.id,
  );
  const repositoryList = useAzureDevOpsRepositories(workspaceId ?? "", filters.projectId);
  const workItems = useAzureDevOpsWorkItemSearch(workspaceId);
  const pullRequests = useAzureDevOpsPullRequestSearch(workspaceId);
  const feedback = useAzureDevOpsPullRequestFeedback(workspaceId);

  useEffect(() => {
    if (!filters.repositoryId && repositoryList.data[0]?.id) {
      update("repositoryId", repositoryList.data[0].id);
    }
  }, [filters.repositoryId, repositoryList.data, update]);

  const runSearch = useCallback(
    (nextSkip: number = 0) => {
      setMobileFiltersOpen(false);
      if (mode === WORK_ITEMS_MODE) {
        void workItems.search({
          project: filters.projectId,
          wiql: filters.wiql,
          top: filters.top,
        });
        return;
      }
      setSkip(nextSkip);
      void pullRequests.search({
        project: filters.projectId,
        repository: filters.repositoryId,
        status: filters.status === "all" ? undefined : filters.status,
        creator: filters.creator || undefined,
        reviewer: filters.reviewer || undefined,
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

  return {
    mode,
    setMode,
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
    update,
    repositoryList,
    workItems,
    pullRequests,
    feedback,
    runSearch,
    openFeedback,
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
        icon={<IconBrandAzure className="h-4 w-4" />}
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
      <Tabs
        value={state.mode}
        onValueChange={(value) => state.setMode(value as AzureDevOpsBrowseMode)}
        className="border-b px-4 py-2"
      >
        <TabsList>
          <TabsTrigger value={WORK_ITEMS_MODE} className="cursor-pointer">
            Work items
          </TabsTrigger>
          <TabsTrigger value={PULL_REQUESTS_MODE} className="cursor-pointer">
            Pull requests
          </TabsTrigger>
        </TabsList>
      </Tabs>
      <div className="flex min-h-0 flex-1">
        <aside className="hidden w-72 shrink-0 overflow-y-auto border-r p-4 md:block">
          <AzureDevOpsFilters {...filterProps} idSuffix="" />
        </aside>
        <BrowseResults state={state} />
      </div>
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
    </main>
  );
}

export function AzureDevOpsPageClient(props: PageProps) {
  return <AzureDevOpsPageContent key={props.workspaceId ?? ""} {...props} />;
}
