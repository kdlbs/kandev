import type { TaskSwitcherItem } from "@/components/task/task-switcher-types";
import type { SidebarTaskColorAutomation } from "@/lib/task-color-automation-settings";
import type { TaskColor } from "@/lib/task-colors";
import { manualTaskColorPresentation, resolveTaskItemColor } from "@/lib/task-color-presentation";
import { resolveAutomaticTaskColor, type TaskColorFacts } from "./task-color-rules";

export type SidebarColorRankingSettings = {
  automation: SidebarTaskColorAutomation;
  manualColors: Record<string, TaskColor | null>;
};

export function sidebarTaskColorFacts(task: TaskSwitcherItem): TaskColorFacts {
  return {
    workspaceId: task.workspaceId,
    workflowId: task.workflowId,
    workflowStepId: task.workflowStepId,
    workflowStepColor: task.workflowStepColor,
    state: task.state,
    priority: task.priority,
    origin: task.origin,
    primaryExecutorProfileId: task.primaryExecutorProfileId,
    repositories: task.repositoryRuleIdentities ?? [],
  };
}

export function effectiveSidebarColorToken(
  task: TaskSwitcherItem,
  settings: SidebarColorRankingSettings,
): string | null {
  const automatic = resolveAutomaticTaskColor(
    settings.automation,
    sidebarTaskColorFacts(task),
  )?.color;
  const manual = manualTaskColorPresentation(settings.manualColors[task.id] ?? null);
  return resolveTaskItemColor(automatic, manual)?.token ?? null;
}

export function sidebarColorMatches(token: string | null | undefined, preferred: string): boolean {
  return token === preferred;
}
