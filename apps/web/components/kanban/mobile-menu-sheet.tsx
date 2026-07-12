"use client";

import { useRef } from "react";
import { useRouter } from "@/lib/routing/client-router";
import Link from "@/components/routing/app-link";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@kandev/ui/sheet";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { ToggleGroup, ToggleGroupItem } from "@kandev/ui/toggle-group";
import {
  IconAlertTriangle,
  IconLayoutKanban,
  IconList,
  IconSettings,
  IconTimeline,
} from "@tabler/icons-react";
import { AppSidebarWorkspacePicker } from "@/components/app-sidebar/app-sidebar-workspace-picker";
import { TaskSearchInput } from "./task-search-input";
import { useKanbanDisplaySettings } from "@/hooks/use-kanban-display-settings";
import { linkToTasks } from "@/lib/links";
import { cn } from "@/lib/utils";
import type { Repository } from "@/lib/types/http";
import type { WorkflowsState } from "@/lib/state/slices";

type MobileMenuSheetProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId?: string;
  currentPage?: "kanban" | "tasks";
  searchQuery?: string;
  onSearchChange?: (query: string) => void;
  isSearchLoading?: boolean;
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

function getMobileViewValue(currentPage: string, kanbanViewMode: string | null): string {
  if (currentPage === "tasks") return "list";
  if (kanbanViewMode === "graph2") return "pipeline";
  return "kanban";
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
};

function MobileDisplaySelects({
  activeWorkflowId,
  workflows,
  onWorkflowChange,
  repositoryValue,
  repositories,
  repositoriesLoading,
  onRepositoryChange,
}: Omit<MobileDisplayOptionsProps, "enablePreviewOnClick" | "onTogglePreviewOnClick">) {
  return (
    <>
      <div className={mobileFieldClass}>
        <label className={mobileFieldLabelClass}>Workflow</label>
        <Select
          value={activeWorkflowId ?? ""}
          onValueChange={(value) => onWorkflowChange(value || null)}
        >
          <SelectTrigger className={mobileControlClass}>
            <SelectValue placeholder="Select workflow" />
          </SelectTrigger>
          <SelectContent>
            {workflows.map((workflow: WorkflowsState["items"][number]) => (
              <SelectItem key={workflow.id} value={workflow.id}>
                {workflow.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

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
  const { enablePreviewOnClick, onTogglePreviewOnClick, ...selectProps } = props;
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
}: {
  viewValue: string;
  onViewChange: (value: string) => void;
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
        <ToggleGroupItem
          value="pipeline"
          className="h-10 min-w-0 flex-1 cursor-pointer gap-2 text-sm data-[state=on]:bg-muted data-[state=on]:text-foreground"
        >
          <IconTimeline className={mobileControlIconClass} />
          Pipeline
        </ToggleGroupItem>
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
  onOpenChange,
}: {
  showHealthIndicator: boolean;
  onOpenHealthDialog: () => void;
  onOpenChange: (open: boolean) => void;
}) {
  const closeSheet = () => onOpenChange(false);
  const openHealth = () => {
    closeSheet();
    onOpenHealthDialog();
  };

  return (
    <div className="mt-auto flex flex-col gap-3 pt-4 border-t border-border">
      <div className={mobileSectionTitleClass}>Utilities</div>
      <Button
        asChild
        variant="outline"
        className={cn(mobileControlClass, "cursor-pointer justify-start gap-3")}
      >
        <Link href="/settings" onClick={closeSheet}>
          <IconSettings className={mobileControlIconClass} />
          Settings
        </Link>
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

export function MobileMenuSheet({
  open,
  onOpenChange,
  workspaceId,
  currentPage = "kanban",
  searchQuery = "",
  onSearchChange,
  isSearchLoading = false,
  showHealthIndicator,
  onOpenHealthDialog,
}: MobileMenuSheetProps) {
  const contentRef = useRef<HTMLDivElement | null>(null);
  const router = useRouter();
  const {
    workflows,
    activeWorkflowId,
    repositories,
    repositoriesLoading,
    allRepositoriesSelected,
    selectedRepositoryId,
    enablePreviewOnClick,
    onWorkflowChange,
    onRepositoryChange,
    onTogglePreviewOnClick,
    kanbanViewMode,
    onViewModeChange,
  } = useKanbanDisplaySettings();

  const repositoryValue = allRepositoriesSelected ? "all" : (selectedRepositoryId ?? "all");
  const viewValue = getMobileViewValue(currentPage, kanbanViewMode);

  const handleViewChange = (value: string) => {
    if (!value) return;
    if (value === "list") {
      if (currentPage !== "tasks") router.push(linkToTasks(workspaceId));
      onOpenChange(false);
    } else if (value === "kanban") {
      if (currentPage !== "kanban") router.push("/");
      onViewModeChange("");
      onOpenChange(false);
    } else if (value === "pipeline") {
      if (currentPage !== "kanban") router.push("/");
      onViewModeChange("graph2");
      onOpenChange(false);
    }
  };

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        ref={contentRef}
        side="right"
        tabIndex={-1}
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          contentRef.current?.focus({ preventScroll: true });
        }}
        className="w-full sm:max-w-sm overflow-y-auto outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
      >
        <SheetHeader>
          <SheetTitle>Menu</SheetTitle>
        </SheetHeader>
        <div className="flex flex-col gap-6 p-4">
          <MobileSearchSection
            searchQuery={searchQuery}
            onSearchChange={onSearchChange}
            isSearchLoading={isSearchLoading}
          />
          <MobileWorkspaceSection onOpenChange={onOpenChange} />
          <MobileViewSection viewValue={viewValue} onViewChange={handleViewChange} />

          <MobileDisplayOptions
            activeWorkflowId={activeWorkflowId}
            workflows={workflows}
            onWorkflowChange={onWorkflowChange}
            repositoryValue={repositoryValue}
            repositories={repositories}
            repositoriesLoading={repositoriesLoading}
            onRepositoryChange={onRepositoryChange}
            enablePreviewOnClick={enablePreviewOnClick}
            onTogglePreviewOnClick={onTogglePreviewOnClick}
          />

          <MobileUtilityActions
            showHealthIndicator={showHealthIndicator}
            onOpenHealthDialog={onOpenHealthDialog}
            onOpenChange={onOpenChange}
          />
        </div>
      </SheetContent>
    </Sheet>
  );
}
