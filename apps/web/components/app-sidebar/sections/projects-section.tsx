"use client";

import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useRouter } from "@/lib/routing/client-router";
import { linkToTask } from "@/lib/links";
import {
  IconBoxMultiple,
  IconChevronDown,
  IconChevronRight,
  IconDots,
  IconPlus,
} from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { useAppStore } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { selectOfficeProjects } from "@/lib/state/slices/office/selectors";
import { useOfficeModeState } from "@/hooks/use-in-office";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import {
  useAgentProjects,
  projectWorkers,
} from "@/hooks/domains/agent-projects/use-agent-projects";
import { useAgentProjectDialogs } from "@/components/agent-projects/agent-project-dialog-provider";
import type { AgentProject } from "@/lib/types/http-agent-projects";
import { cn } from "@/lib/utils";
import { APP_SIDEBAR_SECTION_IDS } from "../app-sidebar-constants";
import { AppSidebarSection } from "../app-sidebar-section";

type ProjectsSectionProps = {
  collapsed: boolean;
  onNavigate?: () => void;
};

type AgentProjectRowProps = {
  project: AgentProject;
  expanded: boolean;
  mobile: boolean;
  onOpenTask: (taskId: string) => void;
  onToggle: () => void;
  onEdit: () => void;
  onArchive: () => void;
  onDelete: () => void;
};

type Navigate = (path: string) => void;

function ProjectsHeaderAction({
  onAdd,
  testId,
  mobile = false,
}: {
  onAdd: () => void;
  testId?: string;
  mobile?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className={cn("h-6 w-6 cursor-pointer", mobile && "h-11 w-11")}
          aria-label={t("sidebar:addProject")}
          onClick={onAdd}
          data-testid={testId}
        >
          <IconPlus className="h-3 w-3 text-muted-foreground/60" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>{t("sidebar:addProject")}</TooltipContent>
    </Tooltip>
  );
}

function AgentProjectRowActions({
  project,
  mobile,
  onEdit,
  onArchive,
  onDelete,
}: Pick<AgentProjectRowProps, "project" | "mobile" | "onEdit" | "onArchive" | "onDelete">) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const touchPending = useRef(false);
  return (
    <DropdownMenu open={open} onOpenChange={setOpen} modal={!mobile}>
      <DropdownMenuTrigger
        asChild
        onPointerDown={(event) => {
          touchPending.current = event.pointerType === "touch";
          if (touchPending.current) event.preventDefault();
        }}
        onPointerCancel={() => {
          touchPending.current = false;
        }}
        onClick={(event) => {
          event.stopPropagation();
          if (!touchPending.current) return;
          touchPending.current = false;
          setOpen((value) => !value);
        }}
      >
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className={cn("shrink-0", mobile ? "h-11 w-11" : "h-7 w-7")}
          aria-label={t("projects:projectActions", { name: project.name })}
        >
          <IconDots className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onClick={onEdit}>{t("projects:editProject")}</DropdownMenuItem>
        <DropdownMenuItem onClick={onArchive}>{t("projects:archive")}</DropdownMenuItem>
        <DropdownMenuItem className="text-destructive" onClick={onDelete}>
          {t("projects:delete")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function AgentProjectWorkerRows({
  project,
  mobile,
  onOpenTask,
}: {
  project: AgentProject;
  mobile: boolean;
  onOpenTask: (taskId: string) => void;
}) {
  const workers = projectWorkers(project);
  return (
    <div
      className="ml-5 space-y-0.5 border-l pl-2"
      data-testid={`agent-project-workers-${project.id}`}
    >
      {workers.map((worker) => (
        <button
          key={worker.id}
          type="button"
          onClick={() => onOpenTask(worker.id)}
          className={cn(
            "flex w-full items-center gap-2 rounded-md px-2 text-left text-[12px] text-muted-foreground hover:bg-muted/60 hover:text-foreground",
            mobile ? "min-h-11" : "min-h-7",
          )}
          data-testid={`agent-project-worker-${worker.id}`}
        >
          <span className="flex-1 truncate">{worker.title}</span>
          <Badge
            variant="outline"
            className="max-w-24 truncate px-1.5 py-0 text-[10px] font-normal"
          >
            {worker.state}
          </Badge>
        </button>
      ))}
    </div>
  );
}

function AgentProjectRow({
  project,
  expanded,
  mobile,
  onOpenTask,
  onToggle,
  onEdit,
  onArchive,
  onDelete,
}: AgentProjectRowProps) {
  const { t } = useTranslation();
  const workers = projectWorkers(project);
  const rowHeight = mobile ? "min-h-11" : "min-h-8";
  const mainTask = project.tasks.find((task) => task.id === project.main_task_id);
  return (
    <div className="space-y-0.5" data-testid={`agent-project-row-${project.id}`}>
      <div
        className={cn("group flex items-center gap-1 rounded-md pr-1 hover:bg-muted/60", rowHeight)}
      >
        {workers.length > 0 ? (
          <button
            type="button"
            className={cn(
              "flex shrink-0 items-center justify-center rounded text-muted-foreground hover:text-foreground",
              mobile ? "min-h-11 min-w-11" : "h-7 w-6",
            )}
            aria-label={t(expanded ? "projects:collapseWorkers" : "projects:expandWorkers", {
              name: project.name,
            })}
            aria-expanded={expanded}
            onClick={onToggle}
            data-testid={`agent-project-expand-${project.id}`}
          >
            {expanded ? (
              <IconChevronDown className="h-4 w-4" />
            ) : (
              <IconChevronRight className="h-4 w-4" />
            )}
          </button>
        ) : (
          <span className={mobile ? "w-11 shrink-0" : "w-6 shrink-0"} />
        )}
        <button
          type="button"
          onClick={() => onOpenTask(project.main_task_id)}
          className={cn(
            "flex min-w-0 flex-1 items-center gap-2 text-left",
            mobile ? "min-h-11" : "min-h-8",
          )}
          data-testid={`agent-project-open-${project.id}`}
        >
          <span className="h-3 w-3 shrink-0 rounded-sm bg-primary/70" />
          <span className="flex-1 truncate text-[13px] font-medium text-foreground/85">
            {project.name}
          </span>
          {mainTask?.state && (
            <Badge
              variant="secondary"
              className="max-w-24 truncate px-1.5 py-0 text-[10px] font-normal"
            >
              {mainTask.state}
            </Badge>
          )}
        </button>
        <AgentProjectRowActions
          project={project}
          mobile={mobile}
          onEdit={onEdit}
          onArchive={onArchive}
          onDelete={onDelete}
        />
      </div>
      {expanded && workers.length > 0 && (
        <AgentProjectWorkerRows project={project} mobile={mobile} onOpenTask={onOpenTask} />
      )}
    </div>
  );
}

function OfficeProjectsSection({ collapsed, onNavigate }: ProjectsSectionProps) {
  const { t } = useTranslation();
  const router = useRouter();
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const touch = isMobile || isFinePointer === false;
  const projects = useAppStore(selectOfficeProjects).filter(
    (project) => project.status !== "archived",
  );
  const navigate: Navigate = (path) => {
    router.push(path);
    onNavigate?.();
  };
  const headerAction = (
    <ProjectsHeaderAction onAdd={() => navigate("/office/projects")} mobile={touch} />
  );
  return (
    <AppSidebarSection
      id={APP_SIDEBAR_SECTION_IDS.projects}
      label={t("sidebar:projects")}
      collapsed={collapsed}
      icon={IconBoxMultiple}
      headerAction={headerAction}
      headerActionVisibility="always"
      defaultExpanded
    >
      {projects.length === 0 ? (
        <p className="px-3 py-2 text-xs text-muted-foreground">{t("sidebar:noProjectsYet")}</p>
      ) : (
        projects.map((project) => (
          <button
            key={project.id}
            type="button"
            onClick={() => navigate(`/office/projects/${project.id}`)}
            className="flex w-full cursor-pointer items-center gap-2.5 rounded-md px-2.5 py-1.5 text-left text-[13px] font-medium text-foreground/80 hover:bg-muted/60"
          >
            <span
              className="h-3 w-3 shrink-0 rounded-sm"
              style={{ backgroundColor: project.color || "#6b7280" }}
            />
            <span className="flex-1 truncate">{project.name}</span>
            {(project.taskCounts?.total ?? 0) > 0 && (
              <Badge
                variant="secondary"
                className="rounded-full px-1.5 py-0 text-[10px] font-normal"
              >
                {project.taskCounts?.total}
              </Badge>
            )}
          </button>
        ))
      )}
    </AppSidebarSection>
  );
}

function AgentProjectRows({
  projects,
  loading,
  loaded,
  error,
  mobile,
  expanded,
  onOpenTask,
  onRefresh,
  onToggle,
  onEdit,
  onArchive,
  onDelete,
}: {
  projects: AgentProject[];
  loading: boolean;
  loaded: boolean;
  error: string | null;
  mobile: boolean;
  expanded: Record<string, boolean>;
  onOpenTask: (taskId: string) => void;
  onRefresh: () => void;
  onToggle: (projectId: string) => void;
  onEdit: (project: AgentProject) => void;
  onArchive: (project: AgentProject) => void;
  onDelete: (project: AgentProject) => void;
}) {
  const { t } = useTranslation();
  if (loading && !loaded) {
    return (
      <p role="status" className="px-3 py-2 text-xs text-muted-foreground">
        {t("common:loading")}
      </p>
    );
  }
  if (error) {
    return (
      <div className="px-3 py-2 text-xs text-destructive">
        <p>{error}</p>
        <Button
          variant="ghost"
          size="sm"
          className={mobile ? "min-h-11" : undefined}
          onClick={onRefresh}
        >
          {t("projects:retry")}
        </Button>
      </div>
    );
  }
  if (projects.length === 0) {
    return <p className="px-3 py-2 text-xs text-muted-foreground">{t("sidebar:noProjectsYet")}</p>;
  }
  return projects.map((project) => (
    <AgentProjectRow
      key={project.id}
      project={project}
      expanded={expanded[project.id] ?? false}
      mobile={mobile}
      onOpenTask={onOpenTask}
      onToggle={() => onToggle(project.id)}
      onEdit={() => onEdit(project)}
      onArchive={() => onArchive(project)}
      onDelete={() => onDelete(project)}
    />
  ));
}

function AgentProjectsSection({ collapsed, onNavigate }: ProjectsSectionProps) {
  const { t } = useTranslation();
  const router = useRouter();
  const { openCreate, editProject, openAction } = useAgentProjectDialogs();
  const projectsEnabled = useFeature("agentProjects");
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  const projects = useAgentProjects(workspaceId, false, projectsEnabled);
  useSettingsData(projectsEnabled);
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const touch = isMobile || isFinePointer === false;
  const navigate: Navigate = (path) => {
    router.push(path);
    onNavigate?.();
  };
  if (!projectsEnabled || !workspaceId) return null;

  const toggleExpanded = (projectId: string) =>
    setExpanded((current) => ({ ...current, [projectId]: !(current[projectId] ?? false) }));
  const headerAction = (
    <ProjectsHeaderAction onAdd={openCreate} testId="agent-project-create-open" mobile={touch} />
  );
  const openTask = (taskId: string) => navigate(linkToTask(taskId));

  return (
    <>
      <AppSidebarSection
        id={APP_SIDEBAR_SECTION_IDS.projects}
        label={t("sidebar:projects")}
        collapsed={collapsed}
        icon={IconBoxMultiple}
        headerAction={headerAction}
        headerActionVisibility="always"
        defaultExpanded={projects.projects.length > 0}
      >
        <AgentProjectRows
          projects={projects.projects}
          loading={projects.loading}
          loaded={projects.loaded}
          error={projects.error}
          mobile={touch}
          expanded={expanded}
          onOpenTask={openTask}
          onRefresh={() => void projects.refresh()}
          onToggle={toggleExpanded}
          onEdit={editProject}
          onArchive={(project) => openAction(project, "archive")}
          onDelete={(project) => openAction(project, "delete")}
        />
      </AppSidebarSection>
    </>
  );
}

export function ProjectsSection({ collapsed, onNavigate }: ProjectsSectionProps) {
  const mode = useOfficeModeState();
  if (mode === "unknown") return null;
  if (mode === "office")
    return <OfficeProjectsSection collapsed={collapsed} onNavigate={onNavigate} />;
  return <AgentProjectsSection collapsed={collapsed} onNavigate={onNavigate} />;
}
