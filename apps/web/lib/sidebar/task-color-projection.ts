import type { Repository } from "@/lib/types/http";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import type { SidebarTaskColorAutomation } from "@/lib/task-color-automation-settings";
import {
  repositoryIdentityForSavedRepository,
  repositoryIdentityForTaskRepository,
  type TaskRepositoryRuleIdentity,
} from "./repository-rule-identity";
import {
  resolveAutomaticTaskColor,
  type AutomaticTaskColorResult,
  type TaskColorFacts,
} from "./task-color-rules";

type ProjectedTask = KanbanState["tasks"][number] & { _workflowId?: string };

export type TaskColorProjectionContext = {
  settings: SidebarTaskColorAutomation;
  workspaceId?: string;
  repositoriesById: ReadonlyMap<string, Repository>;
  stepColorById: ReadonlyMap<string, string>;
};

export type SidebarTaskColorDisplayContext = Pick<
  TaskColorProjectionContext,
  "workspaceId" | "repositoriesById" | "stepColorById"
> & {
  settings?: SidebarTaskColorAutomation;
  manualColor?: string | null;
};

export type SidebarTaskColorDisplay = {
  facts: TaskColorFacts;
  automaticColor: AutomaticTaskColorResult | null;
  effectiveColorToken: string | null;
};

export function taskColorFacts(
  task: ProjectedTask,
  context: Pick<TaskColorProjectionContext, "workspaceId" | "repositoriesById" | "stepColorById">,
): TaskColorFacts {
  const workspaceId = task.workspaceId ?? context.workspaceId;
  const workflowId = task._workflowId ?? task.workflowId;
  return {
    workspaceId,
    workflowId,
    workflowStepId: task.workflowStepId,
    workflowStepColor: context.stepColorById.get(task.workflowStepId),
    state: task.state,
    priority: task.priority,
    origin: task.origin,
    primaryExecutorProfileId: task.primaryExecutorProfileId ?? undefined,
    repositories: taskRepositoryIdentities(task, workspaceId, context.repositoriesById),
  };
}

export function taskColorProjection(
  task: ProjectedTask,
  context: TaskColorProjectionContext,
): AutomaticTaskColorResult | null {
  return resolveAutomaticTaskColor(context.settings, taskColorFacts(task, context));
}

export function sidebarTaskColorDisplay(
  task: ProjectedTask,
  context: SidebarTaskColorDisplayContext,
): SidebarTaskColorDisplay {
  const facts = taskColorFacts(task, context);
  const automaticColor = context.settings
    ? resolveAutomaticTaskColor(context.settings, facts)
    : null;
  return {
    facts,
    automaticColor,
    effectiveColorToken: automaticColor?.color.token ?? context.manualColor ?? null,
  };
}

function taskRepositoryIdentities(
  task: ProjectedTask,
  workspaceId: string | undefined,
  repositoriesById: ReadonlyMap<string, Repository>,
): TaskRepositoryRuleIdentity[] {
  const links =
    task.repositories ?? (task.repositoryId ? [{ repository_id: task.repositoryId }] : []);
  return links.map((link) => {
    const saved = repositoriesById.get(link.repository_id);
    return saved
      ? repositoryIdentityForSavedRepository(saved)
      : repositoryIdentityForTaskRepository({
          repository_id: link.repository_id,
          workspace_id: workspaceId,
        });
  });
}
