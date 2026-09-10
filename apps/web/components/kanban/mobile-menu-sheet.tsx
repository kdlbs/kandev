"use client";
import { type ReactNode, type RefObject } from "react";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@kandev/ui/sheet";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { Checkbox } from "@kandev/ui/checkbox";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { ToggleGroup, ToggleGroupItem } from "@kandev/ui/toggle-group";
import { IconColumns, IconLayoutKanban, IconList, IconTimeline } from "@tabler/icons-react";
import { MobileWorkspaceActionsSection } from "@/components/app-sidebar/app-sidebar-workspace-actions";
import { AppSidebarWorkspacePicker } from "@/components/app-sidebar/app-sidebar-workspace-picker";
import {
  AppNavSections,
  useAppNavDialogs,
  type AppNavDialogControls,
} from "@/components/navigation/app-nav-sections";
import { TaskSearchInput } from "./task-search-input";
import {
  MobileTasksListOptions,
  type TasksListDisplayOptions,
} from "./mobile-menu-task-list-options";
import { cn } from "@/lib/utils";
import type { Repository } from "@/lib/types/http";
import type { WorkflowsState } from "@/lib/state/slices";
import { useTranslation } from "react-i18next";
import { getRepositoryPlaceholderKey } from "@/lib/kanban/repository-placeholder";
import { useMobileMenuSheetState } from "@/hooks/use-mobile-menu-sheet-state";
import { ColumnsMenu, type ColumnsMenuStep } from "./columns-menu";
import {
  mobileControlClass,
  mobileControlIconClass,
  mobileFieldClass,
  mobileFieldLabelClass,
  mobileSectionClass,
  mobileSectionTitleClass,
} from "./mobile-menu-styles";
import type { TaskListingPage } from "@/lib/task-listing/view-navigation";
export type MobileMenuSheetProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId?: string;
  currentPage?: TaskListingPage;
  searchQuery?: string;
  onSearchChange?: (query: string) => void;
  isSearchLoading?: boolean;
  tasksListOptions?: TasksListDisplayOptions;
};

export type MobileDisplayOptionsProps = {
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
  showRepository: boolean;
  showPreviewPanel: boolean;
  tasksListOptions?: TasksListDisplayOptions;
  /**
   * Column visibility for the workflow the phone board is focused on. Null off
   * the phone kanban, where the lane header owns the control instead.
   */
  columnsSection: MobileColumnsSection | null;
};

export type MobileColumnsSection = {
  workflowId: string;
  workflowName: string;
  steps: ColumnsMenuStep[];
  hiddenStepIds: string[];
  onToggle: (workflowId: string, stepId: string) => void;
  autoHideEmpty: boolean;
  onToggleAutoHide: (workflowId: string) => void;
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
  showRepository,
}: Omit<
  MobileDisplayOptionsProps,
  | "enablePreviewOnClick"
  | "onTogglePreviewOnClick"
  | "tasksListShowDetails"
  | "onToggleTasksListShowDetails"
  | "showTaskDetails"
  | "showPreviewPanel"
  | "tasksListOptions"
  | "columnsSection"
>) {
  const { t } = useTranslation();
  return (
    <>
      {showWorkflow && (
        <div className={mobileFieldClass}>
          <label className={mobileFieldLabelClass}>{t("kanban:workflow")}</label>
          <Select
            value={activeWorkflowId ?? "all"}
            onValueChange={(value) => onWorkflowChange(value === "all" ? null : value)}
          >
            <SelectTrigger className={mobileControlClass}>
              <SelectValue placeholder={t("kanban:allWorkflows")} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t("kanban:allWorkflows")}</SelectItem>
              {workflows.map((workflow: WorkflowsState["items"][number]) => (
                <SelectItem key={workflow.id} value={workflow.id}>
                  {workflow.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}

      {showRepository && (
        <div className={mobileFieldClass}>
          <label className={mobileFieldLabelClass}>{t("kanban:repository")}</label>
          <Select
            value={repositoryValue}
            onValueChange={(value) => onRepositoryChange(value as string | "all")}
            disabled={repositories.length === 0}
          >
            <SelectTrigger className={mobileControlClass}>
              <SelectValue
                placeholder={t(
                  getRepositoryPlaceholderKey(repositoriesLoading, repositories.length === 0),
                )}
              />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t("kanban:allRepositories")}</SelectItem>
              {repositories.map((repo: Repository) => (
                <SelectItem key={repo.id} value={repo.id}>
                  {repo.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}
    </>
  );
}

function MobileDisplayOptions(props: MobileDisplayOptionsProps) {
  const { t } = useTranslation();
  const {
    enablePreviewOnClick,
    onTogglePreviewOnClick,
    tasksListShowDetails,
    onToggleTasksListShowDetails,
    showTaskDetails,
    showPreviewPanel,
    tasksListOptions,
    columnsSection,
    ...selectProps
  } = props;
  return (
    <div className="space-y-4">
      <label className={mobileSectionTitleClass}>{t("kanban:displayOptions")}</label>
      <MobileDisplaySelects {...selectProps} />
      {columnsSection && (
        <div className={mobileFieldClass}>
          <label className={mobileFieldLabelClass}>{t("kanban:columns")}</label>
          <ColumnsMenu {...columnsSection} touchTargets />
        </div>
      )}
      {showPreviewPanel && (
        <div className={mobileFieldClass}>
          <label className={mobileFieldLabelClass}>{t("kanban:previewPanel")}</label>
          <label className="flex h-10 cursor-pointer items-center gap-3 rounded-md px-0 text-sm font-medium">
            <Checkbox
              checked={enablePreviewOnClick ?? false}
              onCheckedChange={(checked) => {
                onTogglePreviewOnClick?.(!!checked);
              }}
            />
            <span className="text-sm">{t("kanban:openPreviewOnClick")}</span>
          </label>
        </div>
      )}
      {showTaskDetails && (
        <div className={mobileFieldClass}>
          <label className={mobileFieldLabelClass}>{t("kanban:listRows")}</label>
          <label className="flex min-h-11 cursor-pointer items-center gap-3 rounded-md px-0 text-sm font-medium">
            <Checkbox
              checked={tasksListShowDetails}
              onCheckedChange={(checked) => onToggleTasksListShowDetails(checked === true)}
            />
            <span>{t("kanban:showTaskDetails")}</span>
          </label>
          <p className="pl-6 text-xs text-muted-foreground">
            {t("kanban:addRepositoryPullRequestSessionParent")}
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
  const { t } = useTranslation();
  if (!onSearchChange) return null;

  return (
    <div className={mobileSectionClass}>
      <label className={mobileSectionTitleClass}>{t("kanban:searchSection")}</label>
      <TaskSearchInput
        value={searchQuery}
        onChange={onSearchChange}
        placeholder={t("kanban:searchTasksPlaceholder")}
        isLoading={isSearchLoading}
        className="w-full [&_[data-slot=input]]:h-10 [&_[data-slot=input]]:pl-9 [&_[data-slot=input]]:pr-9 [&_[data-slot=input]]:text-sm"
      />
    </div>
  );
}

function MobileWorkspaceSection({ onOpenChange }: { onOpenChange: (open: boolean) => void }) {
  const { t } = useTranslation();
  return (
    <div className={mobileSectionClass}>
      <label className={mobileSectionTitleClass}>{t("common:workspace")}</label>
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
  const { t } = useTranslation();
  return (
    <div className={mobileSectionClass}>
      <label className={mobileSectionTitleClass}>{t("kanban:view")}</label>
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
          {t("kanban:kanban")}
        </ToggleGroupItem>
        {showPipeline && (
          <ToggleGroupItem
            value="pipeline"
            className="h-10 min-w-0 flex-1 cursor-pointer gap-2 text-sm data-[state=on]:bg-muted data-[state=on]:text-foreground"
          >
            <IconTimeline className={mobileControlIconClass} />
            {t("kanban:pipeline")}
          </ToggleGroupItem>
        )}
        <ToggleGroupItem
          value="threads"
          className="h-10 min-w-0 flex-1 cursor-pointer gap-2 text-sm data-[state=on]:bg-muted data-[state=on]:text-foreground"
        >
          <IconColumns className={mobileControlIconClass} />
          {t("kanban:threads")}
        </ToggleGroupItem>
        <ToggleGroupItem
          value="list"
          className="h-10 min-w-0 flex-1 cursor-pointer gap-2 text-sm data-[state=on]:bg-muted data-[state=on]:text-foreground"
        >
          <IconList className={mobileControlIconClass} />
          {t("kanban:list")}
        </ToggleGroupItem>
      </ToggleGroup>
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
  const { t } = useTranslation();
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
              <DrawerTitle>{t("kanban:menu")}</DrawerTitle>
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
          <SheetTitle>{t("kanban:menu")}</SheetTitle>
        </SheetHeader>
        {children}
      </SheetContent>
    </Sheet>
  );
}

function MobileMenuContent({
  workspaceId,
  searchQuery,
  onSearchChange,
  isSearchLoading,
  onOpenChange,
  viewValue,
  onViewChange,
  showPipeline,
  displayOptions,
  navControls,
}: Pick<
  MobileMenuSheetProps,
  "workspaceId" | "searchQuery" | "onSearchChange" | "isSearchLoading" | "onOpenChange"
> & {
  viewValue: string;
  onViewChange: (value: string) => void;
  showPipeline: boolean;
  displayOptions: MobileDisplayOptionsProps;
  navControls: AppNavDialogControls;
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
      {/* Home and Tasks are omitted here on purpose: the mobile header's brand
          link is this surface's home affordance and the View toggle above
          switches between Kanban and List. */}
      <AppNavSections
        onNavigate={() => onOpenChange(false)}
        omitSections={["primary"]}
        workspaceActions={<MobileWorkspaceActionsSection workspaceId={workspaceId} />}
        controls={navControls}
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
}: MobileMenuSheetProps) {
  const navControls = useAppNavDialogs(() => onOpenChange(false));
  const { contentRef, isMobile, viewValue, handleViewChange, displayOptions, focusMenu } =
    useMobileMenuSheetState({ open, onOpenChange, workspaceId, currentPage, tasksListOptions });

  return (
    <MobileMenuRender
      isMobile={isMobile}
      open={open}
      onOpenChange={onOpenChange}
      contentRef={contentRef}
      onOpenAutoFocus={focusMenu}
      workspaceId={workspaceId}
      searchQuery={searchQuery}
      onSearchChange={onSearchChange}
      isSearchLoading={isSearchLoading}
      viewValue={viewValue}
      onViewChange={handleViewChange}
      displayOptions={displayOptions}
      navControls={navControls}
    />
  );
}

function MobileMenuRender(
  props: Pick<
    MobileMenuSheetProps,
    "open" | "onOpenChange" | "workspaceId" | "searchQuery" | "onSearchChange" | "isSearchLoading"
  > & {
    isMobile: boolean;
    contentRef: RefObject<HTMLDivElement | null>;
    onOpenAutoFocus: (event: Event) => void;
    viewValue: string;
    onViewChange: (value: string) => void;
    displayOptions: MobileDisplayOptionsProps;
    navControls: AppNavDialogControls;
  },
) {
  const { isMobile, navControls } = props;
  return (
    <>
      <ResponsiveMenuSurface {...props} isMobile={isMobile}>
        <MobileMenuContent {...props} showPipeline={!isMobile} />
      </ResponsiveMenuSurface>
      {navControls.dialogs}
    </>
  );
}
