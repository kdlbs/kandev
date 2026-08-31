"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { SidebarTaskColorDimension } from "@/lib/task-color-automation-settings";
import type { ExecutorProfile, TaskOrigin, TaskPriority, TaskState } from "@/lib/types/http";

export type TaskColorRuleOption = {
  key: string;
  value: unknown;
  label: string;
  secondaryLabel?: string;
  color?: string;
  available: boolean;
};

export type TaskColorRuleOptionMap = Record<SidebarTaskColorDimension, TaskColorRuleOption[]>;

type Snapshots = AppState["kanbanMulti"]["snapshots"];
type Workflow = AppState["workflows"]["items"][number];
type ExecutorProfileOption = Pick<ExecutorProfile, "id" | "name">;

const TASK_STATES: readonly TaskState[] = [
  "CREATED",
  "SCHEDULING",
  "TODO",
  "IN_PROGRESS",
  "REVIEW",
  "BLOCKED",
  "WAITING_FOR_INPUT",
  "COMPLETED",
  "FAILED",
  "CANCELLED",
];

const PRIORITIES: readonly TaskPriority[] = ["critical", "high", "medium", "low"];

const ORIGINS: readonly { value: TaskOrigin | "kanban"; labelKey: string }[] = [
  { value: "manual", labelKey: "task:automaticColorOriginManual" },
  { value: "agent_created", labelKey: "task:automaticColorOriginAgentCreated" },
  { value: "routine", labelKey: "task:automaticColorOriginRoutine" },
  { value: "onboarding", labelKey: "task:automaticColorOriginOnboarding" },
  { value: "automation_run", labelKey: "task:automaticColorOriginAutomationRun" },
  { value: "automation_task", labelKey: "task:automaticColorOriginAutomationTask" },
  { value: "kanban", labelKey: "task:automaticColorOriginKanban" },
];

const DIMENSION_LABEL_KEYS: Record<SidebarTaskColorDimension, string> = {
  workflow_step: "task:automaticColorDimensionWorkflowStep",
  repository: "task:automaticColorDimensionRepository",
  workflow: "task:automaticColorDimensionWorkflow",
  executor_profile: "task:automaticColorDimensionExecutorProfile",
  task_state: "task:automaticColorDimensionTaskState",
  priority: "task:automaticColorDimensionPriority",
  origin: "task:automaticColorDimensionOrigin",
};

const TASK_STATE_LABEL_KEYS: Record<TaskState, string> = {
  CREATED: "common:taskStateCreated",
  SCHEDULING: "common:taskStateScheduling",
  TODO: "common:taskStateTodo",
  IN_PROGRESS: "common:taskStateInProgress",
  REVIEW: "common:taskStateReview",
  BLOCKED: "common:taskStateBlocked",
  WAITING_FOR_INPUT: "common:taskStateWaitingForInput",
  COMPLETED: "common:taskStateCompleted",
  FAILED: "common:taskStateFailed",
  CANCELLED: "common:taskStateCancelled",
};

const PRIORITY_LABEL_KEYS: Record<TaskPriority, string> = {
  critical: "task:priorityCritical",
  high: "task:priorityHigh",
  medium: "task:priorityMedium",
  low: "task:priorityLow",
};

export function taskColorRuleOptionKey(value: unknown): string {
  return JSON.stringify(value);
}

export function buildTaskColorRuleOptions(
  sources: {
    snapshots: Snapshots;
    workflows: readonly Workflow[];
    executorProfiles: readonly ExecutorProfileOption[];
    activeWorkspaceId?: string | null;
  },
  translate: (key: string) => string,
): TaskColorRuleOptionMap {
  const workflowOptions: TaskColorRuleOption[] = [];
  const workflowStepOptions: TaskColorRuleOption[] = [];
  const workflowsById = new Map(sources.workflows.map((workflow) => [workflow.id, workflow]));

  for (const workflow of sources.workflows) {
    workflowOptions.push({
      key: taskColorRuleOptionKey({ workspace_id: workflow.workspaceId, workflow_id: workflow.id }),
      value: { workspace_id: workflow.workspaceId, workflow_id: workflow.id },
      label: workflow.name || workflow.id,
      available: true,
    });
  }

  for (const [snapshotId, snapshot] of Object.entries(sources.snapshots)) {
    const workflow = workflowsById.get(snapshotId);
    const workspaceId = workflow?.workspaceId ?? sources.activeWorkspaceId ?? undefined;
    if (!workspaceId) continue;
    if (!workflow) {
      workflowOptions.push({
        key: taskColorRuleOptionKey({
          workspace_id: workspaceId,
          workflow_id: snapshot.workflowId,
        }),
        value: { workspace_id: workspaceId, workflow_id: snapshot.workflowId },
        label: snapshot.workflowName || snapshot.workflowId,
        available: true,
      });
    }
    for (const step of [...snapshot.steps].sort((a, b) => a.position - b.position)) {
      const value = { workspace_id: workspaceId, step_id: step.id };
      if (workflowStepOptions.some((option) => option.key === taskColorRuleOptionKey(value)))
        continue;
      workflowStepOptions.push({
        key: taskColorRuleOptionKey(value),
        value,
        label: step.title || step.id,
        secondaryLabel: workflow?.name ?? snapshot.workflowName,
        color: step.color,
        available: true,
      });
    }
  }

  workflowOptions.sort((a, b) => a.label.localeCompare(b.label));
  workflowStepOptions.sort((a, b) =>
    `${a.secondaryLabel ?? ""}/${a.label}`.localeCompare(`${b.secondaryLabel ?? ""}/${b.label}`),
  );

  const executorProfileOptions = sources.executorProfiles
    .map((profile) => ({
      key: taskColorRuleOptionKey(profile.id),
      value: profile.id,
      label: profile.name || profile.id,
      available: true,
    }))
    .sort((a, b) => a.label.localeCompare(b.label));

  const taskStateOptions = TASK_STATES.map((value) => ({
    key: taskColorRuleOptionKey(value),
    value,
    label: translate(TASK_STATE_LABEL_KEYS[value]),
    available: true,
  }));
  const priorityOptions = PRIORITIES.map((value) => ({
    key: taskColorRuleOptionKey(value),
    value,
    label: translate(PRIORITY_LABEL_KEYS[value]),
    available: true,
  }));
  const originOptions = ORIGINS.map(({ value, labelKey }) => ({
    key: taskColorRuleOptionKey(value),
    value,
    label: translate(labelKey),
    available: true,
  }));

  return {
    workflow_step: workflowStepOptions,
    repository: [],
    workflow: workflowOptions,
    executor_profile: executorProfileOptions,
    task_state: taskStateOptions,
    priority: priorityOptions,
    origin: originOptions,
  };
}

export function useTaskColorRuleOptions(): TaskColorRuleOptionMap {
  const snapshots = useAppStore((state) => state.kanbanMulti.snapshots);
  const workflows = useAppStore((state) => state.workflows.items);
  const executors = useAppStore((state) => state.executors.items);
  const executorProfiles = useMemo(
    () => executors.flatMap((executor) => executor.profiles ?? []),
    [executors],
  );
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const { t, i18n } = useTranslation();

  return useMemo(
    () =>
      buildTaskColorRuleOptions(
        { snapshots, workflows, executorProfiles, activeWorkspaceId },
        (key) => t(key),
      ),
    [activeWorkspaceId, executorProfiles, i18n.language, snapshots, t, workflows],
  );
}

export function taskColorDimensionLabel(
  dimension: SidebarTaskColorDimension,
  translate: (key: string) => string,
): string {
  return translate(DIMENSION_LABEL_KEYS[dimension]);
}

export function taskColorDimensionLabelKey(dimension: SidebarTaskColorDimension): string {
  return DIMENSION_LABEL_KEYS[dimension];
}
