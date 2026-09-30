import { primaryTaskRepository, type Task, type Workflow } from "@/lib/types/http";
import type { TaskListFacetValue } from "@/lib/plugins/types";
import { t } from "@/lib/i18n";

export type TaskListStepPreviews = Record<
  string,
  | { status: "loading" | "error" }
  | { status: "success"; steps: Array<{ id: string; title: string; position: number }> }
>;

export type TaskTreeNode = {
  task: Task;
  children: TaskTreeNode[];
  level: number;
};

export type TaskListSection = {
  key: string;
  title: string | null;
  color?: string;
  nodes: TaskTreeNode[];
};

const UNGROUPED_FACET_SECTION_KEY = "facet:host:ungrouped";

function facetValueSectionKey(value: string): string {
  return `facet:value:${value}`;
}

export function buildTaskSections(
  tasks: Task[],
  {
    groupBy,
    workflowMap,
    repoMap,
    facetValues,
    workflows = [],
    workflowStepPreviews = {},
  }: {
    groupBy: string;
    workflowMap: Map<string, string>;
    repoMap: Map<string, string>;
    facetValues: Record<string, readonly TaskListFacetValue[]>;
    workflows?: Workflow[];
    workflowStepPreviews?: TaskListStepPreviews;
  },
): TaskListSection[] {
  const roots = buildTaskTree(tasks);
  if (groupBy.startsWith("facet:")) {
    const grouped = new Map<string, { title: string; color?: string; tasks: Task[] }>();
    for (const task of tasks) {
      const values = facetValues[`${groupBy}:${task.id}`] ?? [];
      const entries = values.length ? values : [{ value: "untagged", label: t("tasks:ungrouped") }];
      for (const value of entries) {
        const key = values.length ? facetValueSectionKey(value.value) : UNGROUPED_FACET_SECTION_KEY;
        const section = grouped.get(key) ?? { title: value.label, color: value.color, tasks: [] };
        section.tasks.push(task);
        grouped.set(key, section);
      }
    }
    return Array.from(grouped.entries())
      .map(([key, section]) => ({
        key,
        title: section.title,
        color: section.color,
        nodes: buildTaskTree(section.tasks),
      }))
      .sort((a, b) =>
        (a.title ?? "").localeCompare(b.title ?? "", undefined, { sensitivity: "base" }),
      );
  }
  if (groupBy === "none") {
    return [{ key: "all", title: null, nodes: roots }];
  }

  if (groupBy === "workflow_step" || groupBy === "state") {
    return buildWorkflowStepSections(roots, workflows, workflowStepPreviews);
  }

  const sections = new Map<string, TaskListSection>();
  for (const node of roots) {
    const { key, title } = groupForTask(node.task, groupBy, workflowMap, repoMap);
    const section = sections.get(key) ?? { key, title, nodes: [] };
    section.nodes.push(node);
    sections.set(key, section);
  }

  return Array.from(sections.values()).sort((a, b) =>
    (a.title ?? "").localeCompare(b.title ?? "", undefined, { sensitivity: "base" }),
  );
}

function buildTaskTree(tasks: Task[]): TaskTreeNode[] {
  const childrenByParent = new Map<string, Task[]>();
  const taskIds = new Set(tasks.map((task) => task.id));
  const roots: Task[] = [];

  for (const task of tasks) {
    if (task.parent_id && taskIds.has(task.parent_id)) {
      const siblings = childrenByParent.get(task.parent_id) ?? [];
      siblings.push(task);
      childrenByParent.set(task.parent_id, siblings);
    } else {
      roots.push(task);
    }
  }

  const visited = new Set<string>();

  const buildNode = (task: Task, level: number): TaskTreeNode | null => {
    if (visited.has(task.id)) return null;
    visited.add(task.id);
    return {
      task,
      level,
      children: (childrenByParent.get(task.id) ?? [])
        .map((child) => buildNode(child, level + 1))
        .filter((node): node is TaskTreeNode => node !== null),
    };
  };

  const nodes = roots
    .map((task) => buildNode(task, 0))
    .filter((node): node is TaskTreeNode => node !== null);
  for (const task of tasks) {
    const node = buildNode(task, 0);
    if (node) nodes.push(node);
  }

  return nodes;
}

function groupForTask(
  task: Task,
  groupBy: string,
  workflowMap: Map<string, string>,
  repoMap: Map<string, string>,
) {
  if (groupBy === "workflow") {
    const title = workflowMap.get(task.workflow_id);
    if (!title) return { key: "workflow:none", title: t("tasks:noWorkflow") };
    return { key: `workflow:${task.workflow_id || "none"}`, title };
  }
  if (groupBy === "repository") {
    const primaryRepo = primaryTaskRepository(task.repositories);
    if (!primaryRepo) return { key: "repository:none", title: t("tasks:noRepository") };
    const repoId = primaryRepo?.repository_id ?? "none";
    const title = repoMap.get(repoId);
    if (!title) return { key: "repository:none", title: t("tasks:noRepository") };
    return { key: `repository:${repoId}`, title };
  }
  return { key: "all", title: null };
}

type StepSection = TaskListSection & {
  workflowId: string;
  workflowOrder: number;
  workflowName: string;
  stepPosition: number;
  noStep: boolean;
};

function buildWorkflowStepSections(
  roots: TaskTreeNode[],
  workflows: Workflow[],
  previews: TaskListStepPreviews,
): TaskListSection[] {
  const workflowMap = new Map(workflows.map((workflow) => [workflow.id, workflow]));
  const multipleWorkflows = new Set(roots.map((node) => node.task.workflow_id)).size > 1;
  const sections = new Map<string, StepSection>();
  for (const node of roots) {
    const task = node.task;
    const noStep = !task.workflow_step_id;
    const key = noStep ? "step:none" : JSON.stringify([task.workflow_id, task.workflow_step_id]);
    const workflow = workflowMap.get(task.workflow_id);
    const preview = previews[task.workflow_id];
    const step = findPreviewStep(preview, task.workflow_step_id);
    const workflowName = workflow?.name ?? t("tasks:noWorkflow");
    const stepTitle = step?.title ?? missingStepTitle(noStep, preview);
    const title =
      multipleWorkflows && !noStep
        ? t("tasks:workflowStepSection", { workflow: workflowName, step: stepTitle })
        : stepTitle;
    const section = sections.get(key) ?? {
      key,
      title,
      nodes: [],
      noStep,
      workflowId: task.workflow_id,
      workflowName,
      workflowOrder: workflow?.sort_order ?? 0,
      stepPosition: step?.position ?? Number.MAX_SAFE_INTEGER,
    };
    section.nodes.push(node);
    sections.set(key, section);
  }
  return [...sections.values()].sort(compareStepSections);
}

function findPreviewStep(preview: TaskListStepPreviews[string] | undefined, stepId: string) {
  if (preview?.status !== "success") return undefined;
  return preview.steps.find((step) => step.id === stepId);
}

function compareStepSections(a: StepSection, b: StepSection): number {
  return (
    Number(a.noStep) - Number(b.noStep) ||
    a.workflowOrder - b.workflowOrder ||
    a.workflowName.localeCompare(b.workflowName) ||
    a.workflowId.localeCompare(b.workflowId) ||
    a.stepPosition - b.stepPosition ||
    (a.title ?? "").localeCompare(b.title ?? "") ||
    a.key.localeCompare(b.key)
  );
}

function missingStepTitle(
  noStep: boolean,
  preview: TaskListStepPreviews[string] | undefined,
): string {
  if (noStep) return t("tasks:noWorkflowStep");
  if (!preview || preview.status === "loading") return t("tasks:loadingWorkflowStep");
  return t("tasks:unknownWorkflowStep");
}
