"use client";

import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { IconAdjustmentsHorizontal } from "@tabler/icons-react";
import { useKanbanDisplaySettings } from "@/hooks/use-kanban-display-settings";
import type { Repository } from "@/lib/types/http";
import type { WorkflowsState } from "@/lib/state/slices";
import type { ComponentProps } from "react";
import { useTranslation } from "react-i18next";

type KanbanDisplayDropdownProps = {
  triggerSize?: ComponentProps<typeof Button>["size"];
  currentPage?: "kanban" | "tasks";
};

/** Returns a catalog key, not copy: `t()` must run at render, never here. */
function getRepositoryPlaceholderKey(
  repositoriesLoading: boolean,
  repositoriesEmpty: boolean,
): string {
  if (repositoriesLoading) return "kanban:loadingRepositories";
  if (repositoriesEmpty) return "kanban:noRepositories";
  return "kanban:selectRepository";
}

function WorkflowSection({
  activeWorkflowId,
  workflows,
  onWorkflowChange,
}: {
  activeWorkflowId: string | null;
  workflows: WorkflowsState["items"];
  onWorkflowChange: (id: string | null) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1.5">
      <DropdownMenuLabel className="px-0 text-foreground">{t("kanban:workflow")}</DropdownMenuLabel>
      <Select
        value={activeWorkflowId ?? "all"}
        onValueChange={(value) => onWorkflowChange(value === "all" ? null : value)}
      >
        <SelectTrigger data-testid="display-workflow-filter" className="w-full border-border">
          <SelectValue placeholder={t("kanban:selectWorkflow")} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">{t("kanban:allWorkflowsTitleCase")}</SelectItem>
          {workflows.map((workflow: WorkflowsState["items"][number]) => (
            <SelectItem key={workflow.id} value={workflow.id}>
              {workflow.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

function RepositorySection({
  repositoryValue,
  repositories,
  repositoriesLoading,
  onRepositoryChange,
}: {
  repositoryValue: string;
  repositories: Repository[];
  repositoriesLoading: boolean;
  onRepositoryChange: (value: string | "all") => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1.5">
      <DropdownMenuLabel className="px-0 text-foreground">
        {t("kanban:repository")}
      </DropdownMenuLabel>
      <Select
        value={repositoryValue}
        onValueChange={(value) => onRepositoryChange(value as string | "all")}
        disabled={repositories.length === 0}
      >
        <SelectTrigger data-testid="display-repository-filter" className="w-full border-border">
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
  );
}

export function KanbanDisplayDropdown({
  triggerSize = "icon",
  currentPage = "kanban",
}: KanbanDisplayDropdownProps) {
  const { t } = useTranslation();
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
  } = useKanbanDisplaySettings();

  const repositoryValue = allRepositoriesSelected ? "all" : (selectedRepositoryId ?? "all");

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="outline"
          size={triggerSize}
          data-testid="display-button"
          className="cursor-pointer"
        >
          <IconAdjustmentsHorizontal className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-[280px] p-3">
        <div className="space-y-3">
          <WorkflowSection
            activeWorkflowId={activeWorkflowId}
            workflows={workflows}
            onWorkflowChange={onWorkflowChange}
          />
          <DropdownMenuSeparator />
          <RepositorySection
            repositoryValue={repositoryValue}
            repositories={repositories}
            repositoriesLoading={repositoriesLoading}
            onRepositoryChange={onRepositoryChange}
          />
          <DropdownMenuSeparator />
          <div className="space-y-1.5">
            <DropdownMenuLabel className="px-0 text-foreground">
              {t("kanban:previewPanel")}
            </DropdownMenuLabel>
            <label className="flex items-center gap-2 cursor-pointer">
              <Checkbox
                checked={enablePreviewOnClick ?? false}
                onCheckedChange={(checked) => {
                  onTogglePreviewOnClick?.(!!checked);
                }}
              />
              <span className="text-sm text-foreground">{t("kanban:openPreviewOnClick")}</span>
            </label>
            <p className="text-xs text-muted-foreground pl-6">
              {t("kanban:whenEnabledClickingATaskOpens")}
            </p>
          </div>
          {currentPage === "tasks" && (
            <>
              <DropdownMenuSeparator />
              <div className="space-y-1.5">
                <DropdownMenuLabel className="px-0 text-foreground">
                  {t("kanban:listRows")}
                </DropdownMenuLabel>
                <label className="flex items-center gap-2 cursor-pointer">
                  <Checkbox
                    checked={tasksListShowDetails}
                    onCheckedChange={(checked) => onToggleTasksListShowDetails(checked === true)}
                  />
                  <span className="text-sm text-foreground">{t("kanban:showTaskDetails")}</span>
                </label>
                <p className="pl-6 text-xs text-muted-foreground">
                  {t("kanban:addRepositoryPullRequestSessionParent")}
                </p>
              </div>
            </>
          )}
        </div>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
