import type { StepDefinition, WorkflowTemplate } from "@/lib/types/http";

const NOW = "2026-07-18T12:00:00.000Z";

type TemplateStepInput = {
  id: string;
  name: string;
  position: number;
  color: string;
  events: StepDefinition["events"];
  isStartStep?: boolean;
};

function templateStep(input: TemplateStepInput): StepDefinition {
  return {
    id: input.id,
    name: input.name,
    position: input.position,
    color: input.color,
    events: input.events,
    is_start_step: input.isStartStep ?? false,
    show_in_command_panel: input.isStartStep ?? false,
    wip_limit: 0,
    pull_from_step_id: null,
  };
}

export const DEMO_WORKFLOW_TEMPLATES: WorkflowTemplate[] = [
  {
    id: "simple",
    name: "Kanban",
    description: "Move work from backlog through implementation and review.",
    is_system: true,
    default_steps: [
      templateStep({
        id: "backlog",
        name: "Backlog",
        position: 0,
        color: "bg-neutral-400",
        events: { on_turn_start: [{ type: "move_to_next" }] },
      }),
      templateStep({
        id: "in-progress",
        name: "In Progress",
        position: 1,
        color: "bg-blue-500",
        events: {
          on_enter: [{ type: "auto_start_agent" }],
          on_turn_complete: [{ type: "move_to_step", config: { step_id: "review" } }],
        },
        isStartStep: true,
      }),
      templateStep({
        id: "review",
        name: "Review",
        position: 2,
        color: "bg-yellow-500",
        events: { on_turn_start: [{ type: "move_to_previous" }] },
      }),
      templateStep({ id: "done", name: "Done", position: 3, color: "bg-green-500", events: {} }),
    ],
    created_at: NOW,
    updated_at: NOW,
  },
  {
    id: "plan-execute",
    name: "Plan and execute",
    description: "Require an approved plan before implementation begins.",
    is_system: true,
    default_steps: [
      templateStep({
        id: "planning",
        name: "Planning",
        position: 0,
        color: "bg-cyan-500",
        events: {
          on_enter: [{ type: "enable_plan_mode" }, { type: "auto_start_agent" }],
          on_turn_complete: [{ type: "move_to_step", config: { step_id: "implementation" } }],
        },
        isStartStep: true,
      }),
      templateStep({
        id: "implementation",
        name: "Implementation",
        position: 1,
        color: "bg-blue-500",
        events: {
          on_enter: [{ type: "auto_start_agent" }],
          on_turn_complete: [{ type: "move_to_step", config: { step_id: "review" } }],
        },
      }),
      templateStep({
        id: "review",
        name: "Review",
        position: 2,
        color: "bg-amber-500",
        events: {},
      }),
      templateStep({ id: "done", name: "Done", position: 3, color: "bg-emerald-500", events: {} }),
    ],
    created_at: NOW,
    updated_at: NOW,
  },
];

export function findDemoWorkflowTemplate(id: string) {
  return DEMO_WORKFLOW_TEMPLATES.find((template) => template.id === id);
}
