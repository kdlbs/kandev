import { stepHasOnEnterAction, type WorkflowStep } from "@/lib/types/http";
import type {
  OnTurnCompleteAction,
  OnTurnCompleteActionType,
  OnTurnStartAction,
  OnTurnStartActionType,
} from "@/lib/types/workflow-actions";

export type WorkflowReplayCycleSeverity = "blocking" | "warning";
export type WorkflowReplayCycleTrigger = "on_turn_start" | "on_turn_complete";
export type WorkflowReplayCycleActionKind = Extract<
  OnTurnStartActionType | OnTurnCompleteActionType,
  "move_to_next" | "move_to_previous" | "move_to_step"
>;
export type WorkflowReplayPromptSource =
  | "task_description"
  | "step_prompt_with_task_description"
  | "step_prompt";

export type WorkflowReplayCycleHop = {
  sourceStepId: string;
  sourceStepName: string;
  trigger: WorkflowReplayCycleTrigger;
  actionKind: WorkflowReplayCycleActionKind;
  destinationStepId: string;
  destinationStepName: string;
  requiresUserInvolvement: boolean;
};

export type WorkflowReplayCycleDiagnostic = {
  identity: string;
  severity: WorkflowReplayCycleSeverity;
  autoStartStepId: string;
  autoStartStepName: string;
  affectedStepIds: string[];
  trace: WorkflowReplayCycleHop[];
  promptSource: WorkflowReplayPromptSource;
};

type TransitionAction = OnTurnStartAction | OnTurnCompleteAction;

type ReplayGraphEdge = {
  source: WorkflowStep;
  destination: WorkflowStep;
  trigger: WorkflowReplayCycleTrigger;
  actionKind: WorkflowReplayCycleActionKind;
  requiresUserInvolvement: boolean;
};

type StepResolutionContext = {
  orderedSteps: WorkflowStep[];
  stepsById: Map<string, WorkflowStep>;
};

type CycleCandidate = {
  identity: string;
  severity: WorkflowReplayCycleSeverity;
  trace: WorkflowReplayCycleHop[];
};

export function analyzeWorkflowReplayCycles(
  steps: WorkflowStep[],
): WorkflowReplayCycleDiagnostic[] {
  const orderedSteps = steps
    .map((step, inputIndex) => ({ step, inputIndex }))
    .sort(
      (left, right) =>
        left.step.position - right.step.position || left.inputIndex - right.inputIndex,
    )
    .map(({ step }) => step);
  const graph = buildReplayGraph(orderedSteps);

  return orderedSteps.flatMap((autoStartStep) => {
    if (!stepHasOnEnterAction(autoStartStep, "auto_start_agent")) return [];
    const selected = findPreferredCycleCandidate(autoStartStep, graph);
    if (!selected) return [];

    return [
      {
        identity: selected.identity,
        severity: selected.severity,
        autoStartStepId: autoStartStep.id,
        autoStartStepName: autoStartStep.name,
        affectedStepIds: affectedStepIds(autoStartStep.id, selected.trace),
        trace: selected.trace,
        promptSource: classifyPromptSource(autoStartStep.prompt),
      },
    ];
  });
}

function buildReplayGraph(steps: WorkflowStep[]): Map<string, ReplayGraphEdge[]> {
  const graph = new Map<string, ReplayGraphEdge[]>();
  const stepsById = new Map(steps.map((step) => [step.id, step]));
  const resolution = { orderedSteps: steps, stepsById };

  steps.forEach((source, sourceIndex) => {
    const edges = [
      ...resolveTriggerEdges(
        source,
        sourceIndex,
        "on_turn_start",
        source.events?.on_turn_start ?? [],
        resolution,
      ),
      ...resolveTriggerEdges(
        source,
        sourceIndex,
        "on_turn_complete",
        source.events?.on_turn_complete ?? [],
        resolution,
      ),
    ].sort(compareGraphEdges);
    graph.set(source.id, edges);
  });

  return graph;
}

function resolveTriggerEdges(
  source: WorkflowStep,
  sourceIndex: number,
  trigger: WorkflowReplayCycleTrigger,
  actions: TransitionAction[],
  resolution: StepResolutionContext,
): ReplayGraphEdge[] {
  const edges: ReplayGraphEdge[] = [];
  for (const action of actions) {
    if (!isMoveAction(action)) continue;
    const destination = resolveDestination(action, sourceIndex, resolution);
    if (destination) {
      edges.push({
        source,
        destination,
        trigger,
        actionKind: action.type,
        requiresUserInvolvement:
          !stepHasOnEnterAction(source, "auto_start_agent") ||
          action.config?.requires_approval === true,
      });
    }
    if (isAlwaysEligibleTransition(action)) break;
  }
  return edges;
}

function isMoveAction(
  action: TransitionAction,
): action is Extract<TransitionAction, { type: WorkflowReplayCycleActionKind }> {
  return ["move_to_next", "move_to_previous", "move_to_step"].includes(action.type);
}

function isAlwaysEligibleTransition(
  action: Extract<TransitionAction, { type: WorkflowReplayCycleActionKind }>,
): boolean {
  return action.config?.requires_approval !== true && !hasValidTransitionGuard(action.config);
}

function hasValidTransitionGuard(
  config: Extract<TransitionAction, { type: WorkflowReplayCycleActionKind }>["config"],
): boolean {
  const legacyConfig = config as
    | (typeof config & {
        wait_for_quorum?: { role?: unknown; threshold?: unknown };
      })
    | undefined;
  const quorum = legacyConfig?.wait_for_quorum ?? config?.if?.wait_for_quorum;
  return (
    typeof quorum?.role === "string" &&
    quorum.role !== "" &&
    typeof quorum.threshold === "string" &&
    quorum.threshold !== ""
  );
}

function resolveDestination(
  action: Extract<TransitionAction, { type: WorkflowReplayCycleActionKind }>,
  sourceIndex: number,
  resolution: StepResolutionContext,
): WorkflowStep | undefined {
  if (action.type === "move_to_next") return resolution.orderedSteps[sourceIndex + 1];
  if (action.type === "move_to_previous") return resolution.orderedSteps[sourceIndex - 1];
  return resolution.stepsById.get(action.config?.step_id ?? "");
}

type CycleSearchState = {
  stepId: string;
  hasUserInvolvement: boolean;
  trace: WorkflowReplayCycleHop[];
};

type CycleSearchExpansion =
  | { kind: "closing"; candidate: CycleCandidate }
  | { kind: "next"; state: CycleSearchState };

function findPreferredCycleCandidate(
  autoStartStep: WorkflowStep,
  graph: Map<string, ReplayGraphEdge[]>,
): CycleCandidate | undefined {
  return (
    findShortestCycleCandidate(autoStartStep, graph, "blocking") ??
    findShortestCycleCandidate(autoStartStep, graph, "warning")
  );
}

function findShortestCycleCandidate(
  autoStartStep: WorkflowStep,
  graph: Map<string, ReplayGraphEdge[]>,
  severity: WorkflowReplayCycleSeverity,
): CycleCandidate | undefined {
  let frontier: CycleSearchState[] = [
    { stepId: autoStartStep.id, hasUserInvolvement: false, trace: [] },
  ];
  const visitedStates = new Set([cycleSearchStateKey(autoStartStep.id, false)]);

  while (frontier.length > 0) {
    const { closingCandidates, nextFrontier } = expandCycleSearchLevel(
      autoStartStep.id,
      graph,
      severity,
      frontier,
      visitedStates,
    );

    if (closingCandidates.length > 0) {
      return closingCandidates.sort(compareCandidates)[0];
    }

    frontier = [...nextFrontier.values()].sort((left, right) =>
      compareTraceIdentity(autoStartStep.id, left.trace, right.trace),
    );
    for (const state of frontier) {
      visitedStates.add(cycleSearchStateKey(state.stepId, state.hasUserInvolvement));
    }
  }

  return undefined;
}

function expandCycleSearchLevel(
  autoStartStepId: string,
  graph: Map<string, ReplayGraphEdge[]>,
  severity: WorkflowReplayCycleSeverity,
  frontier: CycleSearchState[],
  visitedStates: Set<string>,
): {
  closingCandidates: CycleCandidate[];
  nextFrontier: Map<string, CycleSearchState>;
} {
  const closingCandidates: CycleCandidate[] = [];
  const nextFrontier = new Map<string, CycleSearchState>();

  for (const state of frontier) {
    for (const edge of graph.get(state.stepId) ?? []) {
      const expansion = expandCycleSearchEdge(autoStartStepId, severity, state, edge);
      if (!expansion) continue;
      if (expansion.kind === "closing") {
        closingCandidates.push(expansion.candidate);
        continue;
      }
      retainPreferredSearchState(autoStartStepId, expansion.state, visitedStates, nextFrontier);
    }
  }

  return { closingCandidates, nextFrontier };
}

function expandCycleSearchEdge(
  autoStartStepId: string,
  severity: WorkflowReplayCycleSeverity,
  state: CycleSearchState,
  edge: ReplayGraphEdge,
): CycleSearchExpansion | undefined {
  const hasUserInvolvement = state.hasUserInvolvement || edge.requiresUserInvolvement;
  if (severity === "blocking" && hasUserInvolvement) return undefined;

  const trace = [...state.trace, toTraceHop(edge)];
  if (edge.destination.id !== autoStartStepId) {
    return {
      kind: "next",
      state: { stepId: edge.destination.id, hasUserInvolvement, trace },
    };
  }
  if (edge.trigger !== "on_turn_complete") return undefined;
  if ((severity === "warning") !== hasUserInvolvement) return undefined;
  return { kind: "closing", candidate: toCandidate(autoStartStepId, trace) };
}

function retainPreferredSearchState(
  autoStartStepId: string,
  nextState: CycleSearchState,
  visitedStates: Set<string>,
  nextFrontier: Map<string, CycleSearchState>,
): void {
  const stateKey = cycleSearchStateKey(nextState.stepId, nextState.hasUserInvolvement);
  if (visitedStates.has(stateKey)) return;

  const currentState = nextFrontier.get(stateKey);
  if (
    !currentState ||
    compareTraceIdentity(autoStartStepId, nextState.trace, currentState.trace) < 0
  ) {
    nextFrontier.set(stateKey, nextState);
  }
}

function toTraceHop(edge: ReplayGraphEdge): WorkflowReplayCycleHop {
  return {
    sourceStepId: edge.source.id,
    sourceStepName: edge.source.name,
    trigger: edge.trigger,
    actionKind: edge.actionKind,
    destinationStepId: edge.destination.id,
    destinationStepName: edge.destination.name,
    requiresUserInvolvement: edge.requiresUserInvolvement,
  };
}

function toCandidate(autoStartStepId: string, trace: WorkflowReplayCycleHop[]): CycleCandidate {
  const severity = trace.some((hop) => hop.requiresUserInvolvement) ? "warning" : "blocking";
  return {
    identity: traceIdentity(autoStartStepId, trace),
    severity,
    trace,
  };
}

function traceIdentity(autoStartStepId: string, trace: WorkflowReplayCycleHop[]): string {
  const identityParts = trace.map((hop) => [
    hop.sourceStepId,
    hop.trigger,
    hop.actionKind,
    hop.destinationStepId,
    hop.requiresUserInvolvement,
  ]);
  return `workflow-replay-cycle:${JSON.stringify([autoStartStepId, identityParts])}`;
}

function compareTraceIdentity(
  autoStartStepId: string,
  left: WorkflowReplayCycleHop[],
  right: WorkflowReplayCycleHop[],
): number {
  return compareLexical(
    traceIdentity(autoStartStepId, left),
    traceIdentity(autoStartStepId, right),
  );
}

function cycleSearchStateKey(stepId: string, hasUserInvolvement: boolean): string {
  return JSON.stringify([stepId, hasUserInvolvement]);
}

function compareGraphEdges(left: ReplayGraphEdge, right: ReplayGraphEdge): number {
  return compareLexical(graphEdgeIdentity(left), graphEdgeIdentity(right));
}

function graphEdgeIdentity(edge: ReplayGraphEdge): string {
  return JSON.stringify([
    edge.source.id,
    edge.trigger,
    edge.actionKind,
    edge.destination.id,
    edge.requiresUserInvolvement,
  ]);
}

function compareLexical(left: string, right: string): number {
  if (left === right) return 0;
  return left < right ? -1 : 1;
}

function compareCandidates(left: CycleCandidate, right: CycleCandidate): number {
  if (left.severity !== right.severity) return left.severity === "blocking" ? -1 : 1;
  const lengthDifference = left.trace.length - right.trace.length;
  if (lengthDifference !== 0) return lengthDifference;
  if (left.identity === right.identity) return 0;
  return left.identity < right.identity ? -1 : 1;
}

function affectedStepIds(autoStartStepId: string, trace: WorkflowReplayCycleHop[]): string[] {
  const affected = new Set([autoStartStepId]);
  trace.forEach((hop) => affected.add(hop.destinationStepId));
  return [...affected];
}

function classifyPromptSource(prompt: string | undefined): WorkflowReplayPromptSource {
  if (!prompt) return "task_description";
  return prompt.includes("{{task_prompt}}") ? "step_prompt_with_task_description" : "step_prompt";
}
