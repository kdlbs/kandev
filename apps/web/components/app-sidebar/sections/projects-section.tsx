"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useRouter } from "@/lib/routing/client-router";
import { linkToTask } from "@/lib/links";
import {
  IconArchive,
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
  useAgentProjectMutations,
  projectWorkers,
} from "@/hooks/domains/agent-projects/use-agent-projects";
import { AgentProjectFormDialog } from "@/components/agent-projects/agent-project-form-dialog";
import { AgentProjectActionDialog } from "@/components/agent-projects/agent-project-action-dialog";
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
  archived?: boolean;
  mobile: boolean;
  onOpenTask: (taskId: string) => void;
  onToggle: () => void;
  onEdit: () => void;
  onArchive: () => void;
  onDelete: () => void;
  onRestore?: () => void;
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
          className={cn("h-5 w-5 cursor-pointer", mobile && "h-11 w-11")}
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
  archived,
  mobile,
  onEdit,
  onArchive,
  onDelete,
  onRestore,
}: Pick<
  AgentProjectRowProps,
  "project" | "archived" | "mobile" | "onEdit" | "onArchive" | "onDelete" | "onRestore"
>) {
  const { t } = useTranslation();
  if (archived && onRestore) {
    return (
      <Button
        type="button"
        size="sm"
        variant="ghost"
        className={cn("shrink-0", mobile ? "min-h-11" : "h-7")}
        onClick={onRestore}
        aria-label={t("projects:restoreProject", { name: project.name })}
      >
        {t("projects:restore")}
      </Button>
    );
  }
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className={cn("shrink-0", mobile ? "h-11 w-11" : "h-7 w-7")}
          aria-label={t("projects:projectActions", { name: project.name })}
          onClick={(event) => event.stopPropagation()}
        >
          <IconDots className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {!archived && (
          <>
            <DropdownMenuItem onClick={onEdit}>{t("projects:editProject")}</DropdownMenuItem>
            <DropdownMenuItem onClick={onArchive}>{t("projects:archive")}</DropdownMenuItem>
          </>
        )}
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
  archived = false,
  mobile,
  onOpenTask,
  onToggle,
  onEdit,
  onArchive,
  onDelete,
  onRestore,
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
          className="flex min-w-0 flex-1 items-center gap-2 text-left"
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
          archived={archived}
          mobile={mobile}
          onEdit={onEdit}
          onArchive={onArchive}
          onDelete={onDelete}
          onRestore={onRestore}
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
  const { isMobile } = useResponsiveBreakpoint();
  const projects = useAppStore(selectOfficeProjects).filter(
    (project) => project.status !== "archived",
  );
  const navigate: Navigate = (path) => {
    router.push(path);
    onNavigate?.();
  };
  const headerAction = (
    <ProjectsHeaderAction onAdd={() => navigate("/office/projects")} mobile={isMobile} />
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

function ArchivedAgentProjects({
  open,
  mobile,
  projects,
  onToggle,
  onOpenTask,
  onDelete,
  onRestore,
}: {
  open: boolean;
  mobile: boolean;
  projects: AgentProject[];
  onToggle: () => void;
  onOpenTask: (taskId: string) => void;
  onDelete: (project: AgentProject) => void;
  onRestore: (projectId: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <button
        type="button"
        className={cn(
          "flex w-full items-center gap-2 rounded-md px-2.5 text-left text-xs text-muted-foreground hover:bg-muted/60 hover:text-foreground",
          mobile ? "min-h-11" : "min-h-8",
        )}
        aria-expanded={open}
        onClick={onToggle}
        data-testid="agent-project-archived-toggle"
      >
        <IconArchive className="h-3.5 w-3.5" />
        <span>{t("projects:archivedProjects")}</span>
        {projects.length > 0 && (
          <Badge variant="secondary" className="ml-auto px-1.5 py-0 text-[10px] font-normal">
            {projects.length}
          </Badge>
        )}
      </button>
      {open &&
        projects.map((project) => (
          <AgentProjectRow
            key={`archived-${project.id}`}
            project={project}
            expanded={false}
            archived
            mobile={mobile}
            onOpenTask={onOpenTask}
            onToggle={() => undefined}
            onEdit={() => undefined}
            onArchive={() => undefined}
            onDelete={() => onDelete(project)}
            onRestore={() => onRestore(project.id)}
          />
        ))}
    </>
  );
}

function AgentProjectDialogs({
  workspaceId,
  createOpen,
  setCreateOpen,
  editingProject,
  setEditingProject,
  actionTarget,
  setActionTarget,
}: {
  workspaceId: string;
  createOpen: boolean;
  setCreateOpen: (open: boolean) => void;
  editingProject: AgentProject | null;
  setEditingProject: (project: AgentProject | null) => void;
  actionTarget: { project: AgentProject; action: "archive" | "delete" } | null;
  setActionTarget: (target: { project: AgentProject; action: "archive" | "delete" } | null) => void;
}) {
  return (
    <>
      <AgentProjectFormDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        workspaceId={workspaceId}
      />
      <AgentProjectFormDialog
        open={Boolean(editingProject)}
        onOpenChange={(open) => {
          if (!open) setEditingProject(null);
        }}
        workspaceId={workspaceId}
        project={editingProject ?? undefined}
      />
      <AgentProjectActionDialog
        open={Boolean(actionTarget)}
        onOpenChange={(open) => {
          if (!open) setActionTarget(null);
        }}
        workspaceId={workspaceId}
        project={actionTarget?.project ?? null}
        action={actionTarget?.action ?? "archive"}
      />
    </>
  );
}

function AgentProjectsSection({ collapsed, onNavigate }: ProjectsSectionProps) {
  const { t } = useTranslation();
  const router = useRouter();
  const projectsEnabled = useFeature("agentProjects");
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const [createOpen, setCreateOpen] = useState(false);
  const [editingProject, setEditingProject] = useState<AgentProject | null>(null);
  const [actionTarget, setActionTarget] = useState<{
    project: AgentProject;
    action: "archive" | "delete";
  } | null>(null);
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  const [archivedOpen, setArchivedOpen] = useState(false);
  const projects = useAgentProjects(workspaceId, false, projectsEnabled);
  const archivedProjects = useAgentProjects(workspaceId, true, projectsEnabled && archivedOpen);
  useSettingsData(projectsEnabled);
  const mutations = useAgentProjectMutations();
  const { isMobile } = useResponsiveBreakpoint();
  const navigate: Navigate = (path) => {
    router.push(path);
    onNavigate?.();
  };
  if (!projectsEnabled || !workspaceId) return null;

  const toggleExpanded = (projectId: string) =>
    setExpanded((current) => ({ ...current, [projectId]: !(current[projectId] ?? false) }));
  const setProjectAction = (project: AgentProject, action: "archive" | "delete") =>
    setActionTarget({ project, action });
  const headerAction = (
    <ProjectsHeaderAction
      onAdd={() => setCreateOpen(true)}
      testId="agent-project-create-open"
      mobile={isMobile}
    />
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
        defaultExpanded
      >
        <AgentProjectRows
          projects={projects.projects}
          loading={projects.loading}
          loaded={projects.loaded}
          error={projects.error}
          mobile={isMobile}
          expanded={expanded}
          onOpenTask={openTask}
          onRefresh={() => void projects.refresh()}
          onToggle={toggleExpanded}
          onEdit={setEditingProject}
          onArchive={(project) => setProjectAction(project, "archive")}
          onDelete={(project) => setProjectAction(project, "delete")}
        />
        <ArchivedAgentProjects
          open={archivedOpen}
          mobile={isMobile}
          projects={archivedProjects.projects}
          onToggle={() => setArchivedOpen((current) => !current)}
          onOpenTask={openTask}
          onDelete={(project) => setProjectAction(project, "delete")}
          onRestore={(projectId) => void mutations.restore(workspaceId, projectId)}
        />
      </AppSidebarSection>
      <AgentProjectDialogs
        workspaceId={workspaceId}
        createOpen={createOpen}
        setCreateOpen={setCreateOpen}
        editingProject={editingProject}
        setEditingProject={setEditingProject}
        actionTarget={actionTarget}
        setActionTarget={setActionTarget}
      />
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
