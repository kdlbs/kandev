import type { Task, Workflow, WorkflowStep, WorkflowTemplate } from "@/lib/types/http";
import type { WorkflowSyncConfig } from "@/lib/types/workflow-sync";
import type { DemoHttpResponse } from "./protocol";
import { DEMO_IDS, demoSteps, demoSupportSteps, demoWorkflows } from "./scenario";
import { DEMO_WORKFLOW_TEMPLATES, findDemoWorkflowTemplate } from "./workflow-templates";
import { routeDemoWorkflowSync } from "./workflow-sync-runtime";
import {
  exportWorkflowYaml,
  parseWorkflowImport,
  type PortableWorkflow,
} from "./workflow-transfer";

export type DemoWorkflowRouteContext = {
  path: string;
  method: string;
  input: Record<string, unknown>;
  rawBody?: string;
  searchParams: URLSearchParams;
};

export type DemoWorkflowRuntimeSnapshot = {
  workflows: Workflow[];
  steps: WorkflowStep[];
  nextWorkflow: number;
  nextStep: number;
  syncConfig: WorkflowSyncConfig | null;
};

type RuntimeOptions = {
  snapshot?: DemoWorkflowRuntimeSnapshot;
  getTasks: () => Task[];
  onChange: (snapshot: DemoWorkflowRuntimeSnapshot) => void;
  notify: (action: string, payload: unknown) => void;
};

const NOW = "2026-07-18T12:00:00.000Z";
const WORKFLOW_NOT_FOUND = "Workflow not found";
const WORKFLOW_READ_ONLY =
  "workflow is managed by GitHub sync and is read-only; edit its definition in the synced repository";

export function createDemoWorkflowRuntime(options: RuntimeOptions) {
  const state = options.snapshot
    ? structuredClone(options.snapshot)
    : initialWorkflowRuntimeSnapshot();

  const changed = (action?: string, payload?: unknown) => {
    options.onChange(structuredClone(state));
    if (action) options.notify(action, payload);
  };

  return {
    route(context: DemoWorkflowRouteContext): DemoHttpResponse | null {
      const common = routeWorkflowReads(context, state, options.getTasks);
      if (common) return common;
      const mutation = routeWorkflowMutations(context, state, options.getTasks, changed);
      if (mutation) return mutation;
      const step = routeStepRequests(context, state, options.getTasks, changed);
      if (step) return step;
      const transfer = routeWorkflowTransfer(context, state, changed);
      if (transfer) return transfer;
      return routeDemoWorkflowSync(context, state, changed);
    },
    snapshot: () => structuredClone(state),
  };
}

export function initialWorkflowRuntimeSnapshot(): DemoWorkflowRuntimeSnapshot {
  return {
    workflows: structuredClone(demoWorkflows),
    steps: structuredClone([...demoSteps, ...demoSupportSteps]),
    nextWorkflow: 1,
    nextStep: 1,
    syncConfig: null,
  };
}

function routeWorkflowReads(
  { path, method }: DemoWorkflowRouteContext,
  state: DemoWorkflowRuntimeSnapshot,
  getTasks: () => Task[],
) {
  if (method !== "GET") return null;
  if (path === "/api/v1/workflows" || /^\/api\/v1\/workspaces\/[^/]+\/workflows$/.test(path)) {
    return ok({ workflows: orderedWorkflows(state), total: state.workflows.length });
  }
  if (["/api/v1/workflow/templates", "/api/v1/workflow-templates"].includes(path)) {
    return ok({
      templates: DEMO_WORKFLOW_TEMPLATES,
      total: DEMO_WORKFLOW_TEMPLATES.length,
    });
  }
  const templateMatch = path.match(/^\/api\/v1\/workflow\/templates\/([^/]+)$/);
  if (templateMatch) {
    const template = findDemoWorkflowTemplate(templateMatch[1]);
    return template ? ok(template) : error("Template not found", 404);
  }
  const workflowMatch = path.match(/^\/api\/v1\/workflows\/([^/]+)$/);
  if (workflowMatch) {
    const workflow = findWorkflow(state, workflowMatch[1]);
    return workflow ? ok(workflow) : error(WORKFLOW_NOT_FOUND, 404);
  }
  const snapshotMatch = path.match(/^\/api\/v1\/workflows\/([^/]+)\/snapshot$/);
  if (snapshotMatch) return workflowSnapshot(state, snapshotMatch[1], getTasks());
  return null;
}

function routeWorkflowMutations(
  { path, method, input }: DemoWorkflowRouteContext,
  state: DemoWorkflowRuntimeSnapshot,
  getTasks: () => Task[],
  changed: (action?: string, payload?: unknown) => void,
) {
  if (path === "/api/v1/workflows" && method === "POST") {
    if (!stringValue(input.workspace_id) || !stringValue(input.name)) {
      return error("workspace_id and name are required", 400);
    }
    const workflow = createWorkflow(state, input);
    changed("workflow.created", workflow);
    return ok(workflow, 201);
  }
  const workflowMatch = path.match(/^\/api\/v1\/workflows\/([^/]+)$/);
  if (workflowMatch && ["PATCH", "DELETE"].includes(method)) {
    const workflow = findWorkflow(state, workflowMatch[1]);
    if (!workflow) return error(WORKFLOW_NOT_FOUND, 404);
    if (workflow.source === "github") return error(WORKFLOW_READ_ONLY, 409);
    if (method === "DELETE") {
      state.workflows = state.workflows.filter((item) => item.id !== workflow.id);
      state.steps = state.steps.filter((step) => step.workflow_id !== workflow.id);
      removeTasks(getTasks(), (task) => task.workflow_id === workflow.id);
      changed("workflow.deleted", { id: workflow.id, workspace_id: workflow.workspace_id });
      return ok({ success: true });
    }
    updateWorkflow(workflow, input);
    changed("workflow.updated", workflow);
    return ok(workflow);
  }
  const reorderMatch = path.match(/^\/api\/v1\/workspaces\/([^/]+)\/workflows\/reorder$/);
  if (reorderMatch && method === "PUT") {
    const ids = stringArray(input.workflow_ids);
    if (!ids.length) return error("workflow_ids is required", 400);
    reorderWorkflows(state, ids);
    changed();
    return ok({ success: true });
  }
  return null;
}

function routeStepRequests(
  context: DemoWorkflowRouteContext,
  state: DemoWorkflowRuntimeSnapshot,
  getTasks: () => Task[],
  changed: (action?: string, payload?: unknown) => void,
) {
  return (
    routeStepCollections(context, state, changed) ??
    routeTaskCountsAndMoves(context, getTasks, changed) ??
    routeIndividualStep(context, state, getTasks, changed)
  );
}

function routeStepCollections(
  context: DemoWorkflowRouteContext,
  state: DemoWorkflowRuntimeSnapshot,
  changed: (action?: string, payload?: unknown) => void,
) {
  const { path, method, input } = context;
  const listMatch = path.match(/^\/api\/v1\/workflows\/([^/]+)\/workflow\/steps$/);
  if (listMatch && method === "GET") return stepList(state, listMatch[1]);
  if (listMatch && method === "POST") {
    const template = findDemoWorkflowTemplate(stringValue(input.template_id));
    const workflow = findWorkflow(state, listMatch[1]);
    if (!workflow) return error(WORKFLOW_NOT_FOUND, 404);
    if (!template) return error("Template not found", 404);
    if (workflow.source === "github") return error(WORKFLOW_READ_ONLY, 409);
    state.steps.push(...stepsFromTemplate(state, workflow.id, template));
    changed();
    return ok({ success: true }, 201);
  }
  if (/^\/api\/v1\/workspaces\/[^/]+\/workflow-steps$/.test(path) && method === "GET") {
    return ok({ steps: orderedSteps(state.steps), total: state.steps.length });
  }
  if (path === "/api/v1/workflow/steps" && method === "POST") {
    return createStepResponse(state, input, changed);
  }
  const reorderMatch = path.match(/^\/api\/v1\/workflows\/([^/]+)\/workflow\/steps\/reorder$/);
  if (reorderMatch && method === "PUT") {
    return reorderStepsResponse(state, reorderMatch[1], input, changed);
  }
  return null;
}

function routeTaskCountsAndMoves(
  { path, method, input }: DemoWorkflowRouteContext,
  getTasks: () => Task[],
  changed: (action?: string, payload?: unknown) => void,
) {
  const countMatch = path.match(/^\/api\/v1\/workflow\/steps\/([^/]+)\/task-count$/);
  if (countMatch && method === "GET") {
    return ok({
      task_count: getTasks().filter((task) => task.workflow_step_id === countMatch[1]).length,
    });
  }
  const workflowCount = path.match(/^\/api\/v1\/workflows\/([^/]+)\/task-count$/);
  if (workflowCount && method === "GET") {
    return ok({
      task_count: getTasks().filter((task) => task.workflow_id === workflowCount[1]).length,
    });
  }
  if (path === "/api/v1/tasks/bulk-move" && method === "POST") {
    return bulkMoveTasks(input, getTasks(), changed);
  }
  return null;
}

function routeIndividualStep(
  { path, method, input }: DemoWorkflowRouteContext,
  state: DemoWorkflowRuntimeSnapshot,
  getTasks: () => Task[],
  changed: (action?: string, payload?: unknown) => void,
) {
  const stepMatch = path.match(/^\/api\/v1\/workflow\/steps\/([^/]+)$/);
  if (!stepMatch) return null;
  const step = state.steps.find((item) => item.id === stepMatch[1]);
  if (!step) return error("Step not found", 404);
  if (method === "GET") return ok(step);
  if (workflowIsReadOnly(state, step.workflow_id)) return error(WORKFLOW_READ_ONLY, 409);
  if (method === "PUT") return updateStepResponse(state, step, input, changed);
  if (method === "DELETE") {
    state.steps = state.steps.filter((item) => item.id !== step.id);
    normalizeStepPositions(state, step.workflow_id);
    removeTasks(getTasks(), (task) => task.workflow_step_id === step.id);
    changed("workflow.step.deleted", { step });
    return ok({ success: true });
  }
  return null;
}

function routeWorkflowTransfer(
  { path, method, rawBody, searchParams }: DemoWorkflowRouteContext,
  state: DemoWorkflowRuntimeSnapshot,
  changed: (action?: string, payload?: unknown) => void,
) {
  const singleExport = path.match(/^\/api\/v1\/workflows\/([^/]+)\/export$/);
  if (singleExport && method === "GET") {
    const workflow = findWorkflow(state, singleExport[1]);
    return workflow
      ? yamlResponse(exportWorkflowYaml([workflow], (id) => stepsFor(state, id)))
      : error(WORKFLOW_NOT_FOUND, 404);
  }
  if (/^\/api\/v1\/workspaces\/[^/]+\/workflows\/export$/.test(path) && method === "GET") {
    const ids = searchParams.has("ids")
      ? new Set((searchParams.get("ids") ?? "").split(",").filter(Boolean))
      : null;
    const workflows = orderedWorkflows(state).filter((workflow) => !ids || ids.has(workflow.id));
    return yamlResponse(exportWorkflowYaml(workflows, (id) => stepsFor(state, id)));
  }
  if (!/^\/api\/v1\/workspaces\/[^/]+\/workflows\/import$/.test(path) || method !== "POST") {
    return null;
  }
  const imported = parseWorkflowImport(rawBody ?? "");
  if (!imported) return error("Invalid YAML: expected a Kandev workflow export", 400);
  const created: string[] = [];
  const skipped: string[] = [];
  for (const portable of imported) {
    if (state.workflows.some((workflow) => workflow.name === portable.name)) {
      skipped.push(portable.name);
      continue;
    }
    const workflow = importPortableWorkflow(state, portable);
    created.push(portable.name);
    changed("workflow.created", workflow);
  }
  return ok({ created, skipped });
}

function createWorkflow(state: DemoWorkflowRuntimeSnapshot, input: Record<string, unknown>) {
  const now = new Date().toISOString();
  const workflow: Workflow = {
    id: `demo-workflow-created-${state.nextWorkflow++}` as Workflow["id"],
    workspace_id: stringValue(input.workspace_id) as Workflow["workspace_id"],
    name: stringValue(input.name),
    description: optionalString(input.description),
    workflow_template_id: optionalString(input.workflow_template_id),
    sort_order: state.workflows.length,
    style: "kanban",
    created_at: now,
    updated_at: now,
  };
  state.workflows.push(workflow);
  const template = findDemoWorkflowTemplate(workflow.workflow_template_id ?? "");
  if (template) state.steps.push(...stepsFromTemplate(state, workflow.id, template));
  return workflow;
}

function updateWorkflow(workflow: Workflow, input: Record<string, unknown>) {
  if (typeof input.name === "string") workflow.name = input.name;
  if (typeof input.description === "string") workflow.description = input.description;
  if (typeof input.agent_profile_id === "string") {
    workflow.agent_profile_id = input.agent_profile_id as Workflow["agent_profile_id"];
  }
  workflow.updated_at = new Date().toISOString();
}

function createStepResponse(
  state: DemoWorkflowRuntimeSnapshot,
  input: Record<string, unknown>,
  changed: (action?: string, payload?: unknown) => void,
) {
  const workflowId = stringValue(input.workflow_id);
  if (!workflowId || !stringValue(input.name))
    return error("workflow_id and name are required", 400);
  if (!findWorkflow(state, workflowId)) return error(WORKFLOW_NOT_FOUND, 404);
  if (workflowIsReadOnly(state, workflowId)) return error(WORKFLOW_READ_ONLY, 409);
  const now = new Date().toISOString();
  const step: WorkflowStep = {
    id: `demo-step-created-${state.nextStep++}`,
    workflow_id: workflowId as WorkflowStep["workflow_id"],
    name: stringValue(input.name),
    position: numberValue(input.position, stepsFor(state, workflowId).length),
    color: stringValue(input.color) || "bg-slate-500",
    prompt: optionalString(input.prompt),
    events: objectValue(input.events) as WorkflowStep["events"],
    allow_manual_move: booleanValue(input.allow_manual_move, true),
    is_start_step: booleanValue(input.is_start_step, false),
    show_in_command_panel: booleanValue(input.show_in_command_panel, false),
    auto_advance_requires_signal: booleanValue(input.auto_advance_requires_signal, false),
    wip_limit: numberValue(input.wip_limit, 0),
    pull_from_step_id: optionalString(input.pull_from_step_id),
    created_at: now,
    updated_at: now,
  };
  if (step.is_start_step) demoteStartSteps(state, step);
  state.steps.push(step);
  normalizeStepPositions(state, workflowId);
  changed("workflow.step.created", { step });
  return ok(step, 201);
}

function updateStepResponse(
  state: DemoWorkflowRuntimeSnapshot,
  step: WorkflowStep,
  input: Record<string, unknown>,
  changed: (action?: string, payload?: unknown) => void,
) {
  const fields = [
    "name",
    "position",
    "color",
    "prompt",
    "events",
    "allow_manual_move",
    "is_start_step",
    "show_in_command_panel",
    "auto_archive_after_hours",
    "agent_profile_id",
    "auto_advance_requires_signal",
    "wip_limit",
    "pull_from_step_id",
  ] as const;
  for (const field of fields) {
    if (input[field] !== undefined) Object.assign(step, { [field]: input[field] });
  }
  if (step.is_start_step) demoteStartSteps(state, step);
  step.updated_at = new Date().toISOString();
  changed("workflow.step.updated", { step });
  return ok(step);
}

function reorderStepsResponse(
  state: DemoWorkflowRuntimeSnapshot,
  workflowId: string,
  input: Record<string, unknown>,
  changed: (action?: string, payload?: unknown) => void,
) {
  const workflow = findWorkflow(state, workflowId);
  if (!workflow) return error(WORKFLOW_NOT_FOUND, 404);
  if (workflow.source === "github") return error(WORKFLOW_READ_ONLY, 409);
  const ids = stringArray(input.step_ids);
  const workflowSteps = stepsFor(state, workflowId);
  if (
    ids.length !== workflowSteps.length ||
    ids.some((id) => !workflowSteps.some((s) => s.id === id))
  ) {
    return error("step_ids must contain every workflow step", 400);
  }
  ids.forEach((id, position) => {
    const step = state.steps.find((item) => item.id === id);
    if (step) step.position = position;
  });
  const steps = stepsFor(state, workflowId);
  changed();
  return ok({ success: true, steps });
}

function bulkMoveTasks(
  input: Record<string, unknown>,
  tasks: Task[],
  changed: (action?: string, payload?: unknown) => void,
) {
  const sourceWorkflowId = stringValue(input.source_workflow_id);
  const sourceStepId = optionalString(input.source_step_id);
  const targetWorkflowId = stringValue(input.target_workflow_id);
  const targetStepId = stringValue(input.target_step_id);
  if (!targetWorkflowId || !targetStepId) {
    return error("target_workflow_id and target_step_id are required", 400);
  }
  let moved = 0;
  for (const task of tasks) {
    if (task.workflow_id !== sourceWorkflowId) continue;
    if (sourceStepId && task.workflow_step_id !== sourceStepId) continue;
    task.workflow_id = targetWorkflowId as Task["workflow_id"];
    task.workflow_step_id = targetStepId;
    task.updated_at = new Date().toISOString();
    moved++;
  }
  changed();
  return ok({ moved_count: moved });
}

function stepsFromTemplate(
  state: DemoWorkflowRuntimeSnapshot,
  workflowId: Workflow["id"],
  template: WorkflowTemplate,
) {
  const idMap = new Map<string, string>();
  for (const definition of template.default_steps ?? []) {
    if (definition.id) idMap.set(definition.id, `demo-step-created-${state.nextStep++}`);
  }
  return (template.default_steps ?? []).map((definition) => ({
    id: (definition.id && idMap.get(definition.id)) || `demo-step-created-${state.nextStep++}`,
    workflow_id: workflowId,
    name: definition.name,
    position: definition.position,
    color: definition.color ?? "bg-slate-500",
    prompt: definition.prompt,
    events: remapStepReferences(definition.events, idMap),
    allow_manual_move: true,
    is_start_step: definition.is_start_step ?? false,
    show_in_command_panel: definition.show_in_command_panel ?? false,
    auto_advance_requires_signal: false,
    wip_limit: definition.wip_limit ?? 0,
    pull_from_step_id: definition.pull_from_step_id
      ? (idMap.get(definition.pull_from_step_id) ?? null)
      : null,
    created_at: NOW,
    updated_at: NOW,
  })) as WorkflowStep[];
}

function remapStepReferences<T>(value: T, ids: Map<string, string>): T {
  if (Array.isArray(value)) return value.map((item) => remapStepReferences(item, ids)) as T;
  if (!value || typeof value !== "object") return value;
  return Object.fromEntries(
    Object.entries(value).map(([key, item]) => [
      key,
      key === "step_id" && typeof item === "string"
        ? (ids.get(item) ?? item)
        : remapStepReferences(item, ids),
    ]),
  ) as T;
}

function workflowSnapshot(state: DemoWorkflowRuntimeSnapshot, id: string, tasks: Task[]) {
  const workflow = findWorkflow(state, id);
  if (!workflow) return error(WORKFLOW_NOT_FOUND, 404);
  return ok({
    workflow,
    steps: stepsFor(state, id),
    tasks: tasks.filter((task) => task.workflow_id === id),
  });
}

function stepList(state: DemoWorkflowRuntimeSnapshot, workflowId: string) {
  if (!findWorkflow(state, workflowId)) return error("Workflow not found", 404);
  const steps = stepsFor(state, workflowId);
  return ok({ steps, total: steps.length });
}

function importPortableWorkflow(state: DemoWorkflowRuntimeSnapshot, portable: PortableWorkflow) {
  const workflow = createWorkflow(state, {
    workspace_id: DEMO_IDS.workspace,
    name: portable.name,
    description: portable.description ?? "",
  });
  const stepIds = portable.steps.map(() => `demo-step-created-${state.nextStep++}`);
  const now = new Date().toISOString();
  const steps = portable.steps.map((portableStep, index) => ({
    id: stepIds[index],
    workflow_id: workflow.id,
    name: portableStep.name,
    position: portableStep.position ?? index,
    color: portableStep.color || "bg-slate-500",
    prompt: portableStep.prompt,
    events: restoreEventStepIds(portableStep.events ?? {}, stepIds),
    is_start_step: portableStep.is_start_step ?? false,
    show_in_command_panel: portableStep.show_in_command_panel ?? false,
    allow_manual_move: portableStep.allow_manual_move ?? true,
    auto_archive_after_hours: portableStep.auto_archive_after_hours,
    auto_advance_requires_signal: portableStep.auto_advance_requires_signal ?? false,
    wip_limit: portableStep.wip_limit ?? 0,
    pull_from_step_id:
      portableStep.pull_from_step_position === undefined
        ? null
        : (stepIds[portableStep.pull_from_step_position] ?? null),
    created_at: now,
    updated_at: now,
  })) as WorkflowStep[];
  state.steps.push(...steps);
  return workflow;
}

function removeTasks(tasks: Task[], shouldRemove: (task: Task) => boolean) {
  for (let index = tasks.length - 1; index >= 0; index--) {
    if (shouldRemove(tasks[index])) tasks.splice(index, 1);
  }
}

function restoreEventStepIds<T>(value: T, stepIds: string[]): T {
  if (Array.isArray(value)) return value.map((item) => restoreEventStepIds(item, stepIds)) as T;
  if (!value || typeof value !== "object") return value;
  return Object.fromEntries(
    Object.entries(value).map(([key, item]) => {
      if (key === "step_position" && typeof item === "number") {
        return ["step_id", stepIds[item] ?? ""];
      }
      return [key, restoreEventStepIds(item, stepIds)];
    }),
  ) as T;
}

function reorderWorkflows(state: DemoWorkflowRuntimeSnapshot, ids: string[]) {
  const order = new Map(ids.map((id, index) => [id, index]));
  for (const workflow of state.workflows) {
    if (order.has(workflow.id)) workflow.sort_order = order.get(workflow.id);
  }
}

function normalizeStepPositions(state: DemoWorkflowRuntimeSnapshot, workflowId: string) {
  stepsFor(state, workflowId).forEach((step, position) => {
    step.position = position;
  });
}

function demoteStartSteps(state: DemoWorkflowRuntimeSnapshot, selected: WorkflowStep) {
  for (const step of state.steps) {
    if (step.workflow_id === selected.workflow_id && step.id !== selected.id)
      step.is_start_step = false;
  }
}

function workflowIsReadOnly(state: DemoWorkflowRuntimeSnapshot, workflowId: string) {
  return findWorkflow(state, workflowId)?.source === "github";
}

function findWorkflow(state: DemoWorkflowRuntimeSnapshot, id: string) {
  return state.workflows.find((workflow) => workflow.id === id);
}

function orderedWorkflows(state: DemoWorkflowRuntimeSnapshot) {
  return [...state.workflows].sort((a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0));
}

function stepsFor(state: DemoWorkflowRuntimeSnapshot, workflowId: string) {
  return orderedSteps(state.steps.filter((step) => step.workflow_id === workflowId));
}

function orderedSteps(steps: WorkflowStep[]) {
  return [...steps].sort((a, b) => a.position - b.position);
}

function optionalString(value: unknown): string | undefined {
  return typeof value === "string" && value ? value : undefined;
}

function stringValue(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function stringArray(value: unknown): string[] {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === "string")
    : [];
}

function numberValue(value: unknown, fallback: number): number {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

function booleanValue(value: unknown, fallback: boolean): boolean {
  return typeof value === "boolean" ? value : fallback;
}

function objectValue(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
}

function ok(body: unknown, status = 200): DemoHttpResponse {
  return { status, headers: { "Content-Type": "application/json" }, body };
}

function error(message: string, status: number): DemoHttpResponse {
  return ok({ error: message }, status);
}

function yamlResponse(body: string): DemoHttpResponse {
  return {
    status: 200,
    headers: { "Content-Type": "application/x-yaml" },
    body,
    bodyFormat: "text",
  };
}
