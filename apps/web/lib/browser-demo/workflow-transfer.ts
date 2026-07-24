import type { Workflow, WorkflowStep } from "@/lib/types/http";

export type PortableStep = {
  name: string;
  position: number;
  color: string;
  prompt?: string;
  events?: Record<string, unknown>;
  is_start_step?: boolean;
  show_in_command_panel?: boolean;
  allow_manual_move?: boolean;
  auto_archive_after_hours?: number;
  auto_advance_requires_signal?: boolean;
  wip_limit?: number;
  pull_from_step_position?: number;
};

export type PortableWorkflow = {
  name: string;
  description?: string;
  steps: PortableStep[];
};

export function exportWorkflowYaml(
  workflows: Workflow[],
  getSteps: (workflowId: string) => WorkflowStep[],
) {
  const lines = ["version: 1", "type: kandev_workflow", "workflows:"];
  for (const workflow of workflows) appendPortableWorkflow(lines, workflow, getSteps(workflow.id));
  return `${lines.join("\n")}\n`;
}

function appendPortableWorkflow(lines: string[], workflow: Workflow, steps: WorkflowStep[]) {
  lines.push(`  - name: ${yamlScalar(workflow.name)}`);
  if (workflow.description) lines.push(`    description: ${yamlScalar(workflow.description)}`);
  lines.push("    steps:");
  const positionById = new Map(steps.map((step) => [step.id, step.position]));
  for (const step of steps) appendPortableStep(lines, step, positionById);
}

function appendPortableStep(
  lines: string[],
  step: WorkflowStep,
  positionById: Map<string, number>,
) {
  lines.push(`      - name: ${yamlScalar(step.name)}`);
  lines.push(`        position: ${step.position}`);
  lines.push(`        color: ${yamlScalar(step.color)}`);
  if (step.prompt) lines.push(`        prompt: ${yamlScalar(step.prompt)}`);
  lines.push(`        events: ${JSON.stringify(portableEvents(step.events ?? {}, positionById))}`);
  lines.push(`        is_start_step: ${Boolean(step.is_start_step)}`);
  lines.push(`        show_in_command_panel: ${Boolean(step.show_in_command_panel)}`);
  lines.push(`        allow_manual_move: ${step.allow_manual_move !== false}`);
  lines.push(`        auto_advance_requires_signal: ${Boolean(step.auto_advance_requires_signal)}`);
  if (step.auto_archive_after_hours) {
    lines.push(`        auto_archive_after_hours: ${step.auto_archive_after_hours}`);
  }
  if (step.wip_limit) lines.push(`        wip_limit: ${step.wip_limit}`);
  const pullPosition = step.pull_from_step_id
    ? positionById.get(step.pull_from_step_id)
    : undefined;
  if (pullPosition !== undefined) lines.push(`        pull_from_step_position: ${pullPosition}`);
}

export function parseWorkflowImport(raw: string): PortableWorkflow[] | null {
  try {
    const parsed = JSON.parse(raw) as { version?: unknown; type?: unknown; workflows?: unknown };
    return validImportEnvelope(parsed) ? (parsed.workflows as PortableWorkflow[]) : null;
  } catch {
    return parseSimpleYaml(raw);
  }
}

function parseSimpleYaml(raw: string): PortableWorkflow[] | null {
  if (!/^version:\s*1\s*$/m.test(raw) || !/^type:\s*kandev_workflow\s*$/m.test(raw)) return null;
  const workflows: PortableWorkflow[] = [];
  let workflow: PortableWorkflow | null = null;
  let step: PortableStep | null = null;
  for (const line of raw.split(/\r?\n/)) {
    const workflowName = line.match(/^ {2}- name:\s*(.+)$/);
    if (workflowName) {
      workflow = { name: parseScalar(workflowName[1]) as string, steps: [] };
      workflows.push(workflow);
      step = null;
      continue;
    }
    const stepName = line.match(/^ {6}- name:\s*(.+)$/);
    if (stepName && workflow) {
      step = {
        name: parseScalar(stepName[1]) as string,
        position: workflow.steps.length,
        color: "bg-slate-500",
      };
      workflow.steps.push(step);
      continue;
    }
    applyWorkflowField(line, workflow);
    applyStepField(line, step);
  }
  return workflows.length && workflows.every((item) => item.name && item.steps.length)
    ? workflows
    : null;
}

function applyWorkflowField(line: string, workflow: PortableWorkflow | null) {
  const description = line.match(/^ {4}description:\s*(.+)$/);
  if (description && workflow) workflow.description = String(parseScalar(description[1]));
}

function applyStepField(line: string, step: PortableStep | null) {
  const field = line.match(/^ {8}([a-z_]+):\s*(.*)$/);
  if (field && step) Object.assign(step, { [field[1]]: parseScalar(field[2]) });
}

function validImportEnvelope(value: { version?: unknown; type?: unknown; workflows?: unknown }) {
  return value.version === 1 && value.type === "kandev_workflow" && Array.isArray(value.workflows);
}

function parseScalar(raw: string): unknown {
  const value = raw.trim();
  if (!value) return "";
  try {
    return JSON.parse(value);
  } catch {
    if (value === "true") return true;
    if (value === "false") return false;
    if (/^-?\d+$/.test(value)) return Number(value);
    return value.replace(/^['"]|['"]$/g, "");
  }
}

function yamlScalar(value: string) {
  return JSON.stringify(value);
}

function portableEvents<T>(value: T, positions: Map<string, number>): T {
  if (Array.isArray(value)) return value.map((item) => portableEvents(item, positions)) as T;
  if (!value || typeof value !== "object") return value;
  return Object.fromEntries(
    Object.entries(value).map(([key, item]) => {
      if (key === "step_id" && typeof item === "string" && positions.has(item)) {
        return ["step_position", positions.get(item)];
      }
      return [key, portableEvents(item, positions)];
    }),
  ) as T;
}
