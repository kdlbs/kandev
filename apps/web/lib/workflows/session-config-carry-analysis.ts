import type { WorkflowStep } from "@/lib/types/http";
import type {
  ConfigureSessionOperation,
  ConfigureSessionRule,
  OnTurnCompleteAction,
  OnTurnStartAction,
} from "@/lib/types/workflow-actions";

export type SessionConfigCarryWarning = {
  agentName: string;
  sourceStepId: string;
  sourceStepName: string;
  model?: string;
  configOptions: Record<string, string>;
  message: string;
};

type CarryState = "original" | "changed";

type ChangedSource = {
  step: WorkflowStep;
  rule: ConfigureSessionRule;
};

type CarryVisit = {
  step: WorkflowStep;
  incoming: CarryState;
  changedSource?: ChangedSource;
};

type CarryEdge = {
  destination: WorkflowStep;
};

type TransitionAction = OnTurnStartAction | OnTurnCompleteAction;

/**
 * Finds session settings that can flow into a step because runtime overrides
 * intentionally persist on the original conversation tab.
 *
 * The analysis follows the configured workflow transition graph (including
 * relative moves, explicit moves, joins, and cycles). It tracks an original /
 * changed lattice per agent family and warns whenever any reachable path enters
 * the target with changed values and the target has no explicit decision.
 * Manual card moves are intentionally outside this graph.
 */
export function analyzeSessionConfigCarryForward(
  steps: WorkflowStep[],
  targetStepId: string,
): SessionConfigCarryWarning[] {
  const ordered = [...steps].sort((left, right) => left.position - right.position);
  const target = ordered.find((step) => step.id === targetStepId);
  if (!target || ordered.length === 0) return [];

  const targetFamilies = new Set(configureRules(target).map((rule) => rule.agent_name));
  const families = new Set(
    ordered.flatMap((step) => configureRules(step).map((rule) => rule.agent_name)),
  );
  const graph = buildTransitionGraph(ordered);
  const starts = ordered.filter((step) => step.is_start_step);
  const startSteps = starts.length > 0 ? starts : [ordered[0]];
  const warnings = new Map<string, SessionConfigCarryWarning>();

  for (const agentName of families) {
    if (targetFamilies.has(agentName)) continue;
    analyzeFamily({ ordered, graph, target, agentName, startSteps, warnings });
  }

  return [...warnings.values()];
}

function analyzeFamily({
  graph,
  target,
  agentName,
  startSteps,
  warnings,
}: {
  ordered: WorkflowStep[];
  graph: Map<string, CarryEdge[]>;
  target: WorkflowStep;
  agentName: string;
  startSteps: WorkflowStep[];
  warnings: Map<string, SessionConfigCarryWarning>;
}) {
  const queue: CarryVisit[] = startSteps.map((step) => ({ step, incoming: "original" }));
  const visited = new Set<string>();

  for (let index = 0; index < queue.length; index += 1) {
    const visit = queue[index];
    const key = [visit.step.id, visit.incoming, visit.changedSource?.step.id ?? ""].join("|");
    if (visited.has(key)) continue;
    visited.add(key);

    if (visit.step.id === target.id && visit.incoming === "changed") {
      const source = visit.changedSource;
      if (source) {
        warnings.set(`${agentName}|${source.step.id}`, carryWarning(agentName, source));
      }
    }

    const outgoing = applyRule(visit.step, agentName, visit.incoming, visit.changedSource);
    for (const edge of graph.get(visit.step.id) ?? []) {
      queue.push({
        step: edge.destination,
        incoming: outgoing.state,
        changedSource: outgoing.changedSource,
      });
    }
  }
}

function applyRule(
  step: WorkflowStep,
  agentName: string,
  incoming: CarryState,
  changedSource: ChangedSource | undefined,
): { state: CarryState; changedSource?: ChangedSource } {
  const rule = configureRules(step).find((candidate) => candidate.agent_name === agentName);
  if (!rule || rule.operation === "keep") return { state: incoming, changedSource };
  if (rule.operation === "restore_original") return { state: "original" };
  return { state: "changed", changedSource: { step, rule } };
}

function carryWarning(agentName: string, source: ChangedSource): SessionConfigCarryWarning {
  return {
    agentName,
    sourceStepId: source.step.id,
    sourceStepName: source.step.name,
    model: source.rule.operation === "set" ? source.rule.model : undefined,
    configOptions: source.rule.operation === "set" ? { ...(source.rule.config_options ?? {}) } : {},
    message: `Settings changed in ${source.step.name} may carry into this step for ${agentName}. Choose keep, restore original, or set new values.`,
  };
}

function buildTransitionGraph(steps: WorkflowStep[]): Map<string, CarryEdge[]> {
  const stepsById = new Map(steps.map((step) => [step.id, step]));
  const graph = new Map<string, CarryEdge[]>();

  steps.forEach((step, index) => {
    const destinations: WorkflowStep[] = [];
    const actions: TransitionAction[] = [
      ...(step.events?.on_turn_start ?? []),
      ...(step.events?.on_turn_complete ?? []),
    ];
    for (const action of actions) {
      const destination = resolveDestination(action, index, steps, stepsById);
      if (destination && !destinations.some((candidate) => candidate.id === destination.id)) {
        destinations.push(destination);
      }
    }
    graph.set(
      step.id,
      destinations.map((destination) => ({ destination })),
    );
  });
  return graph;
}

function resolveDestination(
  action: TransitionAction,
  sourceIndex: number,
  steps: WorkflowStep[],
  stepsById: Map<string, WorkflowStep>,
): WorkflowStep | undefined {
  if (action.type === "move_to_next") return steps[sourceIndex + 1];
  if (action.type === "move_to_previous") return steps[sourceIndex - 1];
  if (action.type === "move_to_step") return stepsById.get(action.config?.step_id ?? "");
  return undefined;
}

function configureRules(step: WorkflowStep): ConfigureSessionRule[] {
  const action = (step.events?.on_enter ?? []).find(
    (candidate) => candidate.type === "configure_session",
  );
  const rules = action?.type === "configure_session" ? action.config?.rules : undefined;
  if (!Array.isArray(rules)) return [];
  return rules.filter((rule): rule is ConfigureSessionRule => {
    if (!rule || typeof rule.agent_name !== "string" || rule.agent_name === "") return false;
    return isOperation(rule.operation);
  });
}

function isOperation(value: unknown): value is ConfigureSessionOperation {
  return value === "set" || value === "keep" || value === "restore_original";
}
