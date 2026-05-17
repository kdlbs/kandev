/**
 * Workflow step action types — mirrors the Go `workflow/models` package
 * (`OnEnterActionType`, `OnTurnStartActionType`, `OnTurnCompleteActionType`,
 * `OnExitActionType`). Each variant of the discriminated unions below carries
 * its `config` shape so call sites stop reaching into `Record<string, unknown>`
 * to read e.g. `step_id`.
 *
 * Keep this file in sync with:
 *   - apps/backend/internal/workflow/models/models.go (action constants)
 *   - apps/backend/internal/workflow/engine/types.go  (compiled action config readers)
 */

// On Enter action types
export type OnEnterActionType = "enable_plan_mode" | "auto_start_agent" | "reset_agent_context";

// On Turn Start action types
export type OnTurnStartActionType = "move_to_next" | "move_to_previous" | "move_to_step";

// On Turn Complete action types
export type OnTurnCompleteActionType =
  | "move_to_next"
  | "move_to_previous"
  | "move_to_step"
  | "disable_plan_mode";

// On Exit action types
export type OnExitActionType = "disable_plan_mode";

/**
 * Optional config flags shared by transition actions (move_to_next /
 * move_to_previous / move_to_step). The Go engine reads these via
 * `ConfigRequiresApproval` and `ConfigTransitionGuard` on every transition,
 * so they are valid on any move_to_* action across the on_turn_start and
 * on_turn_complete triggers.
 *
 * See `apps/backend/internal/workflow/engine/types.go`.
 */
export type TransitionConfig = {
  /** Gate the transition on manual approval before it fires. */
  requires_approval?: boolean;
  /**
   * Optional guard clause that gates the transition on quorum / decision
   * state. Today only `wait_for_quorum` is supported by the engine.
   */
  if?: {
    wait_for_quorum?: {
      /** Participant role to count decisions for (e.g. "reviewer", "approver"). */
      role: string;
      /**
       * Threshold expression: "all_approve" | "all_decide" | "any_reject" |
       * "majority_approve" | "n_approve:<N>".
       */
      threshold: string;
    };
  };
};

/** Config for `move_to_step` actions. Carries the target step id. */
export type MoveToStepConfig = TransitionConfig & {
  step_id: string;
};

// Discriminated union of on_enter actions. Today only `auto_start_agent`
// carries optional config (`queue_if_busy` + `prompt_override`), but kanban
// templates never set those so config is optional everywhere.
export type OnEnterAction =
  | { type: "enable_plan_mode" }
  | {
      type: "auto_start_agent";
      config?: {
        prompt_override?: string;
        queue_if_busy?: boolean;
      };
    }
  | { type: "reset_agent_context" };

export type OnTurnStartAction =
  | { type: "move_to_next"; config?: TransitionConfig }
  | { type: "move_to_previous"; config?: TransitionConfig }
  | { type: "move_to_step"; config: MoveToStepConfig };

export type OnTurnCompleteAction =
  | { type: "move_to_next"; config?: TransitionConfig }
  | { type: "move_to_previous"; config?: TransitionConfig }
  | { type: "move_to_step"; config: MoveToStepConfig }
  | { type: "disable_plan_mode" };

export type OnExitAction = { type: "disable_plan_mode" };

export type StepEvents = {
  on_enter?: OnEnterAction[];
  on_turn_start?: OnTurnStartAction[];
  on_turn_complete?: OnTurnCompleteAction[];
  on_exit?: OnExitAction[];
};
