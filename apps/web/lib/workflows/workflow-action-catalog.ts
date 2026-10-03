import type {
  GenericAction,
  OnEnterAction,
  OnExitAction,
  OnTurnCompleteAction,
  OnTurnStartAction,
  WorkflowScriptAction,
  WorkflowScriptActionConfig,
  WorkflowScriptFailurePolicy,
} from "@/lib/types/workflow-actions";

export type WorkflowLifecycleTrigger =
  | "on_enter"
  | "on_turn_start"
  | "on_turn_complete"
  | "on_exit"
  | "on_children_completed";

export type WorkflowActionRecord = {
  type: string;
  config?: Record<string, unknown>;
  [key: string]: unknown;
};

export type WorkflowActionDescriptor = {
  type: string;
  labelKey: string;
  compatibleTriggers: readonly WorkflowLifecycleTrigger[];
};

const ENTRY_TRIGGERS = ["on_enter"] as const;
const TURN_START_TRIGGERS = ["on_turn_start"] as const;
const TURN_COMPLETE_TRIGGERS = ["on_turn_complete"] as const;
const EXIT_TRIGGERS = ["on_exit"] as const;
const CHILDREN_COMPLETED_TRIGGERS = ["on_children_completed"] as const;

export const WORKFLOW_ACTION_CATALOG: readonly WorkflowActionDescriptor[] = [
  { type: "enable_plan_mode", labelKey: "workflows:planMode", compatibleTriggers: ENTRY_TRIGGERS },
  {
    type: "auto_start_agent",
    labelKey: "workflows:autoStartAgent",
    compatibleTriggers: ENTRY_TRIGGERS,
  },
  {
    type: "reset_agent_context",
    labelKey: "workflows:resetAgentContext",
    compatibleTriggers: ENTRY_TRIGGERS,
  },
  {
    type: "configure_session",
    labelKey: "workflows:configureOriginalSessionOptions",
    compatibleTriggers: ENTRY_TRIGGERS,
  },
  {
    type: "set_session_mode",
    labelKey: "workflows:setSessionMode",
    compatibleTriggers: ENTRY_TRIGGERS,
  },
  {
    type: "clear_decisions",
    labelKey: "workflows:clearDecisions",
    compatibleTriggers: ENTRY_TRIGGERS,
  },
  {
    type: "queue_run_for_each_participant",
    labelKey: "workflows:queueRunForEachParticipant",
    compatibleTriggers: ENTRY_TRIGGERS,
  },
  {
    type: "queue_run",
    labelKey: "workflows:queueRun",
    compatibleTriggers: ENTRY_TRIGGERS,
  },
  {
    type: "ensure_participant_seat",
    labelKey: "workflows:ensureParticipantSeat",
    compatibleTriggers: ENTRY_TRIGGERS,
  },
  {
    type: "run_code_review",
    labelKey: "workflows:runCodeReview",
    compatibleTriggers: ENTRY_TRIGGERS,
  },
  {
    type: "move_to_next",
    labelKey: "workflows:moveToNextStep",
    compatibleTriggers: [
      ...TURN_START_TRIGGERS,
      ...TURN_COMPLETE_TRIGGERS,
      ...CHILDREN_COMPLETED_TRIGGERS,
    ],
  },
  {
    type: "move_to_previous",
    labelKey: "workflows:moveToPreviousStep",
    compatibleTriggers: [
      ...TURN_START_TRIGGERS,
      ...TURN_COMPLETE_TRIGGERS,
      ...CHILDREN_COMPLETED_TRIGGERS,
    ],
  },
  {
    type: "move_to_step",
    labelKey: "workflows:moveToSpecificStep",
    compatibleTriggers: [
      ...TURN_START_TRIGGERS,
      ...TURN_COMPLETE_TRIGGERS,
      ...CHILDREN_COMPLETED_TRIGGERS,
    ],
  },
  {
    type: "disable_plan_mode",
    labelKey: "workflows:disablePlanMode",
    compatibleTriggers: [...TURN_COMPLETE_TRIGGERS, ...EXIT_TRIGGERS],
  },
  {
    type: "run_script",
    labelKey: "workflows:runScript",
    compatibleTriggers: [...ENTRY_TRIGGERS, ...TURN_COMPLETE_TRIGGERS, ...EXIT_TRIGGERS],
  },
];

const SCRIPT_DEFAULT_TIMEOUT_SECONDS = 600;
const SCRIPT_MIN_TIMEOUT_SECONDS = 1;
const SCRIPT_MAX_TIMEOUT_SECONDS = 86400;

export function getWorkflowActionCatalog(
  trigger: WorkflowLifecycleTrigger,
): readonly WorkflowActionDescriptor[] {
  return WORKFLOW_ACTION_CATALOG.filter((item) => item.compatibleTriggers.includes(trigger));
}

export function createWorkflowAction(
  trigger: WorkflowLifecycleTrigger,
  type: string,
): WorkflowActionRecord {
  if (!getWorkflowActionCatalog(trigger).some((item) => item.type === type)) {
    throw new Error(`Action ${type} is not compatible with ${trigger}`);
  }
  if (type === "run_script") {
    return {
      type,
      config: {
        command: "",
        timeout_seconds: SCRIPT_DEFAULT_TIMEOUT_SECONDS,
        failure_policy: "block",
      },
    };
  }
  if (type === "configure_session") return { type, config: { rules: [] } };
  if (type === "set_session_mode") return { type, config: { mode: "default" } };
  if (type === "queue_run") return { type, config: { target: "primary", task_id: "this" } };
  if (type === "move_to_step") return { type, config: { step_id: "" } };
  return { type };
}

export type WorkflowScriptValidation =
  | { valid: true }
  | {
      valid: false;
      errorKey:
        | "workflows:scriptCommandRequired"
        | "workflows:scriptTimeoutInvalid"
        | "workflows:scriptFailurePolicyInvalid";
    };

export function validateWorkflowScriptAction(action: unknown): WorkflowScriptValidation {
  const config = scriptConfig(action);
  if (!config || typeof config.command !== "string" || config.command.trim() === "") {
    return { valid: false, errorKey: "workflows:scriptCommandRequired" };
  }
  if (config.timeout_seconds !== undefined && !isValidTimeout(config.timeout_seconds)) {
    return { valid: false, errorKey: "workflows:scriptTimeoutInvalid" };
  }
  if (
    config.failure_policy !== undefined &&
    config.failure_policy !== "block" &&
    config.failure_policy !== "continue"
  ) {
    return { valid: false, errorKey: "workflows:scriptFailurePolicyInvalid" };
  }
  return { valid: true };
}

export function normalizeWorkflowScriptAction(action: unknown): WorkflowActionRecord | null {
  const config = scriptConfig(action);
  if (!config) return null;
  const source = action as WorkflowActionRecord;
  return {
    ...source,
    type: "run_script",
    config: {
      ...config,
      timeout_seconds: config.timeout_seconds ?? SCRIPT_DEFAULT_TIMEOUT_SECONDS,
      failure_policy: config.failure_policy ?? "block",
    },
  };
}

export function normalizeWorkflowAction(action: unknown): WorkflowActionRecord | null {
  if (!isRecord(action) || typeof action.type !== "string") return null;
  return action.type === "run_script"
    ? normalizeWorkflowScriptAction(action)
    : { ...action, type: action.type };
}

export function scriptActionConfig(action: unknown): WorkflowScriptActionConfig | null {
  const config = scriptConfig(action);
  return config ? ({ ...config } as WorkflowScriptActionConfig) : null;
}

export type WorkflowTypedAction =
  | OnEnterAction
  | OnTurnStartAction
  | OnTurnCompleteAction
  | OnExitAction
  | GenericAction
  | WorkflowScriptAction;

function scriptConfig(action: unknown): Record<string, unknown> | null {
  if (!isRecord(action) || action.type !== "run_script" || !isRecord(action.config)) return null;
  return action.config;
}

function isValidTimeout(value: unknown): value is number {
  return (
    typeof value === "number" &&
    Number.isInteger(value) &&
    value >= SCRIPT_MIN_TIMEOUT_SECONDS &&
    value <= SCRIPT_MAX_TIMEOUT_SECONDS
  );
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

export type { WorkflowScriptFailurePolicy };
