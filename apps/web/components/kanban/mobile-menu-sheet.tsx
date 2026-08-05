"use client";
import { type ReactNode, type RefObject, useRef, useState } from "react";
import { useRouter } from "@/lib/routing/client-router";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@kandev/ui/sheet";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { ToggleGroup, ToggleGroupItem } from "@kandev/ui/toggle-group";
import {
  IconAlertTriangle,
  IconActivity,
  IconLayoutKanban,
  IconList,
  IconStethoscope,
  IconTimeline,
} from "@tabler/icons-react";
import { AppSidebarWorkspacePicker } from "@/components/app-sidebar/app-sidebar-workspace-picker";
import { MobileIntegrationsSection } from "@/components/integrations/integrations-menu";
import { DestinationRows } from "@/components/navigation/destination-rows";
import { MobilePluginNavSection } from "@/components/plugins/mobile-plugin-nav-section";
import { useStaticDestinations } from "@/hooks/use-app-destinations";
import { MOBILE_MENU_UTILITY_SECTIONS } from "@/lib/navigation/surface-policy";
import { TaskSearchInput } from "./task-search-input";
import {
  MobileTasksListOptions,
  type TasksListDisplayOptions,
} from "./mobile-menu-task-list-options";
import { useKanbanDisplaySettings } from "@/hooks/use-kanban-display-settings";
import { linkToTask, linkToTaskOverview, linkToTasks } from "@/lib/links";
import { cn } from "@/lib/utils";
import type { Repository, Task } from "@/lib/types/http";
import type { WorkflowsState } from "@/lib/state/slices";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useAppStatusDrawer } from "@/components/app-status-bar/app-status-surface-provider";
import { useConnectionIssueCopy } from "@/components/app-status-bar/connection-status-item";
import { ImproveKandevDialog } from "@/components/improve-kandev-dialog";
type MobileMenuSheetProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId?: string;
  currentPage?: "kanban" | "tasks";
  searchQuery?: string;
  onSearchChange?: (query: string) => void;
  isSearchLoading?: boolean;
  tasksListOptions?: TasksListDisplayOptions;
  showHealthIndicator: boolean;
  onOpenHealthDialog: () => void;
};

const mobileSectionClass = "space-y-2";
const mobileSectionTitleClass = "text-sm font-medium";
const mobileFieldClass = "space-y-1.5";
const mobileFieldLabelClass = "text-xs font-medium text-muted-foreground";
const mobileControlClass = "h-10 w-full px-3 text-sm";
const mobileControlIconClass = "h-4 w-4 shrink-0";

function getRepositoryPlaceholder(loading: boolean, empty: boolean): string {
  if (loading) return "Loading repositories...";
  if (empty) return "No repositories";
  return "Select repository";
}

type MobileDisplayOptionsProps = {
  activeWorkflowId: string | null;
  workflows: WorkflowsState["items"];
  onWorkflowChange: (id: string | null) => void;
  repositoryValue: string;
  repositories: Repository[];
  repositoriesLoading: boolean;
  onRepositoryChange: (value: string | "all") => void;
  enablePreviewOnClick: boolean | undefined;
  onTogglePreviewOnClick: ((checked: boolean) => void) | undefined;
  tasksListShowDetails: boolean;
  onToggleTasksListShowDetails: (checked: boolean) => void;
  showTaskDetails: boolean;
  showWorkflow: boolean;
  tasksListOptions?: TasksListDisplayOptions;
};

function MobileDisplaySelects({
  activeWorkflowId,
  workflows,
  onWorkflowChange,
  repositoryValue,
  repositories,
  repositoriesLoading,
  onRepositoryChange,
  showWorkflow,
}: Omit<
  MobileDisplayOptionsProps,
  | "enablePreviewOnClick"
  | "onTogglePreviewOnClick"
  | "tasksListShowDetails"
  | "onToggleTasksListShowDetails"
  | "showTaskDetails"
  | "tasksListOptions"
>) {
  return (
    <>
      {showWorkflow && (
        <div className={mobileFieldClass}>
          <label className={mobileFieldLabelClass}>Workflow</label>
          <Select
            value={activeWorkflowId ?? "all"}
            onValueChange={(value) => onWorkflowChange(value === "all" ? null : value)}
          >
            <SelectTrigger className={mobileControlClass}>
              <SelectValue placeholder="All workflows" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All workflows</SelectItem>
              {workflows.map((workflow: WorkflowsState["items"][number]) => (
                <SelectItem key={workflow.id} value={workflow.id}>
                  {workflow.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}

      <div className={mobileFieldClass}>
        <label className={mobileFieldLabelClass}>Repository</label>
        <Select
          value={repositoryValue}
          onValueChange={(value) => onRepositoryChange(value as string | "all")}
          disabled={repositories.length === 0}
        >
          <SelectTrigger className={mobileControlClass}>
            <SelectValue
              placeholder={getRepositoryPlaceholder(repositoriesLoading, repositories.length === 0)}
            />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All repositories</SelectItem>
            {repositories.map((repo: Repository) => (
              <SelectItem key={repo.id} value={repo.id}>
                {repo.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
    </>
  );
}

function MobileDisplayOptions(props: MobileDisplayOptionsProps) {
  const {
    enablePreviewOnClick,
    onTogglePreviewOnClick,
    tasksListShowDetails,
    onToggleTasksListShowDetails,
    showTaskDetails,
    tasksListOptions,
    ...selectProps
  } = props;
  return (
    <div className="space-y-4">
      <label className={mobileSectionTitleClass}>Display Options</label>
      <MobileDisplaySelects {...selectProps} />
      <div className={mobileFieldClass}>
        <label className={mobileFieldLabelClass}>Preview Panel</label>
        <label className="flex h-10 cursor-pointer items-center gap-3 rounded-md px-0 text-sm font-medium">
          <Checkbox
            checked={enablePreviewOnClick ?? false}
            onCheckedChange={(checked) => {
              onTogglePreviewOnClick?.(!!checked);
            }}
          />
          <span className="text-sm">Open preview on click</span>
        </label>
      </div>
      {showTaskDetails && (
        <div className={mobileFieldClass}>
          <label className={mobileFieldLabelClass}>List rows</label>
          <label className="flex min-h-11 cursor-pointer items-center gap-3 rounded-md px-0 text-sm font-medium">
            <Checkbox
              checked={tasksListShowDetails}
              onCheckedChange={(checked) => onToggleTasksListShowDetails(checked === true)}
            />
            <span>Show task details</span>
          </label>
          <p className="pl-6 text-xs text-muted-foreground">
            Add repository, pull request, session, parent, and review context to List rows.
          </p>
        </div>
      )}
      {tasksListOptions && <MobileTasksListOptions options={tasksListOptions} />}
    </div>
  );
}

function MobileSearchSection({
  searchQuery,
  onSearchChange,
  isSearchLoading,
}: {
  searchQuery: string;
  onSearchChange?: (query: string) => void;
  isSearchLoading: boolean;
}) {
  if (!onSearchChange) return null;

  return (
    <div className={mobileSectionClass}>
      <label className={mobileSectionTitleClass}>Search</label>
      <TaskSearchInput
        value={searchQuery}
        onChange={onSearchChange}
        placeholder="Search tasks..."
        isLoading={isSearchLoading}
        className="w-full [&_[data-slot=input]]:h-10 [&_[data-slot=input]]:pl-9 [&_[data-slot=input]]:pr-9 [&_[data-slot=input]]:text-sm"
      />
    </div>
  );
}

function MobileWorkspaceSection({ onOpenChange }: { onOpenChange: (open: boolean) => void }) {
  return (
    <div className={mobileSectionClass}>
      <label className={mobileSectionTitleClass}>Workspace</label>
      <AppSidebarWorkspacePicker
        modal={false}
        onActionComplete={() => onOpenChange(false)}
        triggerClassName={cn("flex-none", mobileControlClass)}
        triggerTestId="mobile-workspace-trigger"
        chevronTestId="mobile-workspace-trigger-chevron"
        itemTestIdPrefix="mobile-workspace-item"
        contentClassName="w-80 max-w-[calc(100vw-2rem)]"
      />
    </div>
  );
}

function MobileViewSection({
  viewValue,
  onViewChange,
  showPipeline,
}: {
  viewValue: string;
  onViewChange: (value: string) => void;
  showPipeline: boolean;
}) {
  return (
    <div className={mobileSectionClass}>
      <label className={mobileSectionTitleClass}>View</label>
      <ToggleGroup
        type="single"
        value={viewValue}
        onValueChange={onViewChange}
        variant="outline"
        className="w-full justify-start"
      >
        <ToggleGroupItem
          value="kanban"
          className="h-10 min-w-0 flex-1 cursor-pointer gap-2 text-sm data-[state=on]:bg-muted data-[state=on]:text-foreground"
        >
          <IconLayoutKanban className={mobileControlIconClass} />
          Kanban
        </ToggleGroupItem>
        {showPipeline && (
          <ToggleGroupItem
            value="pipeline"
            className="h-10 min-w-0 flex-1 cursor-pointer gap-2 text-sm data-[state=on]:bg-muted data-[state=on]:text-foreground"
          >
            <IconTimeline className={mobileControlIconClass} />
            Pipeline
          </ToggleGroupItem>
        )}
        <ToggleGroupItem
          value="list"
          className="h-10 min-w-0 flex-1 cursor-pointer gap-2 text-sm data-[state=on]:bg-muted data-[state=on]:text-foreground"
        >
          <IconList className={mobileControlIconClass} />
          List
        </ToggleGroupItem>
      </ToggleGroup>
    </div>
  );
}

function MobileUtilityActions({
  showHealthIndicator,
  onOpenHealthDialog,
  onOpenImproveKandev,
  onOpenChange,
}: {
  showHealthIndicator: boolean;
  onOpenHealthDialog: () => void;
  onOpenImproveKandev: () => void;
  onOpenChange: (open: boolean) => void;
}) {
  const { enabled: statusDrawerEnabled, issueSeverity, openStatusDrawer } = useAppStatusDrawer();
  const issueDetails = useConnectionIssueCopy(issueSeverity);
  // Stats, Settings, and anything else added to the manifest's insight/utility
  // sections. Before the manifest, this block hardcoded a Settings link and
  // Stats had no phone entry point at all.
  const destinations = useStaticDestinations("mobileMenu", MOBILE_MENU_UTILITY_SECTIONS);
  const closeSheet = () => onOpenChange(false);
  const openHealth = () => {
    closeSheet();
    onOpenHealthDialog();
  };
  const openStatus = () => {
    closeSheet();
    requestAnimationFrame(openStatusDrawer);
  };

  return (
    <div className="mt-auto flex flex-col gap-3 pt-4 border-t border-border">
      <div className={mobileSectionTitleClass}>Utilities</div>
      {statusDrawerEnabled && (
        <Button
          type="button"
          variant="outline"
          className={cn(
            "relative h-11 w-full cursor-pointer justify-start gap-3 px-3 text-sm",
            issueSeverity === "lost" && "border-destructive/40 text-destructive",
            issueSeverity === "unstable" && "border-amber-500/40 text-amber-500",
          )}
          onClick={openStatus}
          data-testid="mobile-home-status-button"
          aria-label={issueDetails?.description}
          data-connection-severity={issueSeverity === "none" ? undefined : issueSeverity}
        >
          <IconActivity className={mobileControlIconClass} />
          Status
          {issueDetails && (
            <span
              className={cn("ml-auto size-2 rounded-full", issueDetails.dotClass)}
              aria-hidden="true"
            />
          )}
        </Button>
      )}
      <DestinationRows
        destinations={destinations}
        onNavigate={closeSheet}
        className="gap-3 px-3 text-sm"
      />
      <Button
        type="button"
        variant="outline"
        className="h-11 w-full cursor-pointer justify-start gap-3 px-3 text-sm"
        onClick={onOpenImproveKandev}
        data-testid="mobile-improve-kandev-button"
      >
        <IconStethoscope className={mobileControlIconClass} />
        Improve Kandev
      </Button>
      {showHealthIndicator && (
        <Button
          type="button"
          variant="outline"
          className={cn(mobileControlClass, "cursor-pointer justify-start gap-3")}
          onClick={openHealth}
        >
          <IconAlertTriangle className={cn(mobileControlIconClass, "text-warning")} />
          Health issues
        </Button>
      )}
    </div>
  );
}

function ResponsiveMenuSurface({
  isMobile,
  open,
  onOpenChange,
  contentRef,
  onOpenAutoFocus,
  children,
}: {
  isMobile: boolean;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  contentRef: RefObject<HTMLDivElement | null>;
  onOpenAutoFocus: (event: Event) => void;
  children: ReactNode;
}) {
  if (isMobile) {
    return (
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerContent
          ref={contentRef}
          tabIndex={-1}
          onOpenAutoFocus={onOpenAutoFocus}
          className="h-[calc(100dvh-16px-env(safe-area-inset-bottom,0px))] !max-h-[calc(100dvh-16px-env(safe-area-inset-bottom,0px))] outline-none"
        >
          <div
            data-testid="mobile-home-menu-card"
            className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl bg-background shadow-2xl shadow-black/20"
          >
            <DrawerHeader className="shrink-0 border-b border-border/70 pb-3 text-left">
              <DrawerTitle>Menu</DrawerTitle>
            </DrawerHeader>
            <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain pb-[env(safe-area-inset-bottom,0px)]">
              {children}
            </div>
          </div>
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        ref={contentRef}
        side="right"
        tabIndex={-1}
        onOpenAutoFocus={onOpenAutoFocus}
        className="w-full overflow-y-auto outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 sm:max-w-sm"
      >
        <SheetHeader>
          <SheetTitle>Menu</SheetTitle>
        </SheetHeader>
        {children}
      </SheetContent>
    </Sheet>
  );
}

function MobileMenuContent({
  searchQuery,
  onSearchChange,
  isSearchLoading,
  onOpenChange,
  viewValue,
  onViewChange,
  showPipeline,
  displayOptions,
  showHealthIndicator,
  onOpenHealthDialog,
  onOpenImproveKandev,
}: Pick<
  MobileMenuSheetProps,
  | "searchQuery"
  | "onSearchChange"
  | "isSearchLoading"
  | "onOpenChange"
  | "showHealthIndicator"
  | "onOpenHealthDialog"
> & {
  viewValue: string;
  onViewChange: (value: string) => void;
  showPipeline: boolean;
  displayOptions: MobileDisplayOptionsProps;
  onOpenImproveKandev: () => void;
}) {
  return (
    <div className="flex min-h-full flex-col gap-6 p-4">
      <MobileSearchSection
        searchQuery={searchQuery ?? ""}
        onSearchChange={onSearchChange}
        isSearchLoading={isSearchLoading ?? false}
      />
      <MobileWorkspaceSection onOpenChange={onOpenChange} />
      <MobileViewSection
        viewValue={viewValue}
        onViewChange={onViewChange}
        showPipeline={showPipeline}
      />
      <MobileDisplayOptions {...displayOptions} />
      <MobilePluginNavSection onNavigate={() => onOpenChange(false)} />
      <MobileIntegrationsSection onNavigate={() => onOpenChange(false)} />
      <MobileUtilityActions
        showHealthIndicator={showHealthIndicator}
        onOpenHealthDialog={onOpenHealthDialog}
        onOpenImproveKandev={onOpenImproveKandev}
        onOpenChange={onOpenChange}
      />
    </div>
  );
}

export function MobileMenuSheet({
  open,
  onOpenChange,
  workspaceId,
  currentPage = "kanban",
  searchQuery = "",
  onSearchChange,
  isSearchLoading = false,
  tasksListOptions,
  showHealthIndicator,
  onOpenHealthDialog,
}: MobileMenuSheetProps) {
  const contentRef = useRef<HTMLDivElement | null>(null);
  const router = useRouter();
  const [improveOpen, setImproveOpen] = useState(false);
  const { isMobile } = useResponsiveBreakpoint();
  const {
    workflows,
    activeWorkflowId,
    repositories,
    repositoriesLoading,
    allRepositoriesSelected,
    selectedRepositoryId,
    enablePreviewOnClick,
    tasksListShowDetails,
    onWorkflowChange,
    onRepositoryChange,
    onTogglePreviewOnClick,
    onToggleTasksListShowDetails,
    effectiveTaskListingView,
    onViewModeChange,
  } = useKanbanDisplaySettings();
  const repositoryValue = allRepositoriesSelected ? "all" : (selectedRepositoryId ?? "all");
  const viewValue = currentPage === "tasks" ? "list" : effectiveTaskListingView;
  const handleViewChange = (value: string) => {
    if (!value) return;
    if (value === "list") {
      onViewModeChange("list");
      if (currentPage !== "tasks") router.push(linkToTasks(workspaceId));
      onOpenChange(false);
    } else if (value === "kanban") {
      onViewModeChange("kanban");
      if (currentPage !== "kanban")
        router.push(linkToTaskOverview({ workspaceId, workflowId: activeWorkflowId ?? undefined }));
      onOpenChange(false);
    } else if (value === "pipeline" && !isMobile) {
      onViewModeChange("pipeline");
      if (currentPage !== "kanban")
        router.push(linkToTaskOverview({ workspaceId, workflowId: activeWorkflowId ?? undefined }));
      onOpenChange(false);
    }
  };
  const displayOptions = {
    activeWorkflowId,
    workflows,
    onWorkflowChange,
    repositoryValue,
    repositories,
    repositoriesLoading,
    onRepositoryChange,
    enablePreviewOnClick,
    onTogglePreviewOnClick,
    tasksListShowDetails,
    onToggleTasksListShowDetails,
    showTaskDetails: currentPage === "tasks",
    showWorkflow: !isMobile || currentPage !== "kanban",
    tasksListOptions: isMobile && currentPage === "tasks" ? tasksListOptions : undefined,
  };
  const focusMenu = (event: Event) => {
    event.preventDefault();
    contentRef.current?.focus({ preventScroll: true });
  };
  const openImproveKandev = () => {
    onOpenChange(false);
    requestAnimationFrame(() => setImproveOpen(true));
  };

  return (
    <MobileMenuRender
      isMobile={isMobile}
      open={open}
      onOpenChange={onOpenChange}
      contentRef={contentRef}
      onOpenAutoFocus={focusMenu}
      searchQuery={searchQuery}
      onSearchChange={onSearchChange}
      isSearchLoading={isSearchLoading}
      viewValue={viewValue}
      onViewChange={handleViewChange}
      displayOptions={displayOptions}
      showHealthIndicator={showHealthIndicator}
      onOpenHealthDialog={onOpenHealthDialog}
      onOpenImproveKandev={openImproveKandev}
      improveOpen={improveOpen}
      onImproveOpenChange={setImproveOpen}
      workspaceId={workspaceId ?? null}
      onTaskCreated={(task) => router.push(linkToTask(task.id))}
    />
  );
}

function MobileMenuRender(
  props: Pick<
    MobileMenuSheetProps,
    | "open"
    | "onOpenChange"
    | "searchQuery"
    | "onSearchChange"
    | "isSearchLoading"
    | "showHealthIndicator"
    | "onOpenHealthDialog"
  > & {
    isMobile: boolean;
    contentRef: RefObject<HTMLDivElement | null>;
    onOpenAutoFocus: (event: Event) => void;
    viewValue: string;
    onViewChange: (value: string) => void;
    displayOptions: MobileDisplayOptionsProps;
    onOpenImproveKandev: () => void;
    improveOpen: boolean;
    onImproveOpenChange: (open: boolean) => void;
    workspaceId: string | null;
    onTaskCreated: (task: Task) => void;
  },
) {
  const { isMobile, improveOpen, onImproveOpenChange, workspaceId, onTaskCreated } = props;
  return (
    <>
      <ResponsiveMenuSurface {...props} isMobile={isMobile}>
        <MobileMenuContent {...props} showPipeline={!isMobile} />
      </ResponsiveMenuSurface>
      <ImproveKandevDialog
        open={improveOpen}
        onOpenChange={onImproveOpenChange}
        workspaceId={workspaceId}
        onSuccess={onTaskCreated}
      />
    </>
  );
}
