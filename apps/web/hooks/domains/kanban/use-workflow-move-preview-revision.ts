"use client";

import { useCallback } from "react";
import { useAppStore } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import { parseTurnTimestamp } from "@/lib/state/slices/session/turn-actions";

type PreviewRevisionTask = AppState["kanban"]["tasks"][number];
type PreviewRevisionStep = AppState["kanban"]["steps"][number];
type PreviewRevisionSession = AppState["taskSessions"]["items"][string];

const TASK_METADATA_KEYS = [
  "agent_profile_id",
  "workflow_initial_session",
  "workflow_session_route",
  "initial_session_runtime_config",
  "initial_session_runtime_config_profile_id",
] as const;

const SESSION_METADATA_KEYS = [
  "created_by",
  "completion_follow_up",
  "origin",
  "runtime_config",
  "runtime_config_overrides",
  "original_effective_config",
  "acp_config_baseline",
  "session_mode",
] as const;

const PROFILE_SNAPSHOT_KEYS = [
  "name",
  "agent_name",
  "agent_id",
  "model",
  "mode",
  "config_options",
  "configOptions",
] as const;

type SessionReference = { source: string; id: string };
type SessionEntry = { source: string; session: PreviewRevisionSession };
type SessionSourceGroup = { source: string; entries: SessionEntry[] };

function stableValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(stableValue);
  if (value === null || typeof value !== "object") return value;
  return Object.fromEntries(
    Object.entries(value as Record<string, unknown>)
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([key, child]) => [key, stableValue(child)]),
  );
}

function serializeRevision(value: unknown): string {
  return JSON.stringify(stableValue(value));
}

function projectRecord(
  record: Record<string, unknown> | null | undefined,
  keys: readonly string[],
): Record<string, unknown> | undefined {
  if (!record) return undefined;
  const projection: Record<string, unknown> = {};
  for (const key of keys) {
    if (Object.prototype.hasOwnProperty.call(record, key)) projection[key] = record[key];
  }
  return Object.keys(projection).length > 0 ? projection : undefined;
}

function projectWorkflowAgentOverrides(task: PreviewRevisionTask) {
  const overrides = task.workflowAgentOverrides;
  if (!overrides) return undefined;
  const bindings = Array.isArray(overrides.steps) ? overrides.steps : [];
  return {
    workflow_id: overrides.workflow_id,
    steps: [...bindings]
      .sort(
        (left, right) =>
          left.step_id.localeCompare(right.step_id) ||
          left.replacement_profile_id.localeCompare(right.replacement_profile_id) ||
          left.source_profile_id.localeCompare(right.source_profile_id),
      )
      .map((binding) => ({
        step_id: binding.step_id,
        source_profile_id: binding.source_profile_id,
        replacement_profile_id: binding.replacement_profile_id,
      })),
  };
}

function projectTask(task: PreviewRevisionTask) {
  return {
    id: task.id,
    workflow_id: task.workflowId,
    workflow_step_id: task.workflowStepId,
    state: task.state,
    primary_session_id: task.primarySessionId,
    primary_session_state: task.primarySessionState,
    archived: task.isArchived === true,
    metadata: projectRecord(task.metadata, TASK_METADATA_KEYS),
    workflow_agent_overrides: projectWorkflowAgentOverrides(task),
  };
}

function projectStep(step: PreviewRevisionStep) {
  return {
    id: step.id,
    position: step.position,
    agent_profile_id: step.agent_profile_id,
    profile_session_start_policy: step.profile_session_start_policy,
    profile_session_end_policy: step.profile_session_end_policy,
    session_target: step.session_target,
    on_enter: step.events?.on_enter?.map((action) => ({
      type: action.type,
      config: action.config,
    })),
  };
}

function projectSession(session: PreviewRevisionSession, source: string) {
  return {
    source,
    session: {
      id: session.id,
      task_id: session.task_id,
      name: session.name,
      agent_profile_id: session.agent_profile_id,
      state: session.state,
      is_primary: session.is_primary,
      is_passthrough: session.is_passthrough,
      metadata: projectRecord(session.metadata, SESSION_METADATA_KEYS),
      agent_profile_snapshot: projectRecord(session.agent_profile_snapshot, PROFILE_SNAPSHOT_KEYS),
    },
  };
}

function sessionReference(entry: SessionEntry): SessionReference {
  return { source: entry.source, id: entry.session.id };
}

function compareTimestamps(left: string | undefined, right: string | undefined): number {
  const leftValue = left ? parseTurnTimestamp(left) : null;
  const rightValue = right ? parseTurnTimestamp(right) : null;
  if (leftValue === null) return rightValue === null ? 0 : -1;
  if (rightValue === null) return 1;
  if (leftValue === rightValue) return 0;
  return leftValue > rightValue ? 1 : -1;
}

function isTerminalSession(session: PreviewRevisionSession): boolean {
  return (
    session.state === "COMPLETED" || session.state === "FAILED" || session.state === "CANCELLED"
  );
}

function isPreviewActiveSession(session: PreviewRevisionSession): boolean {
  return (
    session.state === "CREATED" ||
    session.state === "STARTING" ||
    session.state === "RUNNING" ||
    session.state === "WAITING_FOR_INPUT"
  );
}

function hasCompletionFollowUp(session: PreviewRevisionSession): boolean {
  return session.metadata?.completion_follow_up === true;
}

function currentSessionReference(entries: SessionEntry[]): SessionReference | null {
  const active = entries.filter(({ session }) => isPreviewActiveSession(session));
  const primary = active.find(({ session }) => session.is_primary);
  if (primary) return sessionReference(primary);
  const latest = active.reduce<SessionEntry | undefined>((best, candidate) => {
    if (!best) return candidate;
    const started = compareTimestamps(candidate.session.started_at, best.session.started_at);
    if (started !== 0) return started > 0 ? candidate : best;
    return candidate.session.id > best.session.id ? candidate : best;
  }, undefined);
  return latest ? sessionReference(latest) : null;
}

function originalSessionReference(
  task: PreviewRevisionTask | undefined,
  entries: SessionEntry[],
): { status: "selected" | "missing" | "ambiguous"; session: SessionReference | null } {
  const initial = task?.metadata?.workflow_initial_session;
  if (initial && typeof initial === "object" && "session_id" in initial) {
    const sessionId = (initial as { session_id?: unknown }).session_id;
    if (typeof sessionId === "string" && sessionId !== "") {
      const match = entries.find(({ session }) => session.id === sessionId);
      return {
        status: match ? "selected" : "missing",
        session: match ? sessionReference(match) : null,
      };
    }
  }

  const marked = entries.filter(({ session }) => session.metadata?.origin === "task_initial");
  if (marked.length > 1) return { status: "ambiguous", session: null };
  if (marked.length === 1) return { status: "selected", session: sessionReference(marked[0]!) };

  const legacy = entries.filter(
    ({ session }) => session.metadata?.created_by !== "workflow_switch",
  );
  legacy.sort((left, right) => {
    const started = compareTimestamps(left.session.started_at, right.session.started_at);
    return started !== 0 ? started : left.session.id.localeCompare(right.session.id);
  });
  if (legacy.length === 0) return { status: "missing", session: null };
  if (
    legacy.length > 1 &&
    compareTimestamps(legacy[0]!.session.started_at, legacy[1]!.session.started_at) === 0
  ) {
    return { status: "ambiguous", session: null };
  }
  return { status: "selected", session: sessionReference(legacy[0]!) };
}

function reusableCandidateOrder(entries: SessionEntry[], excludedSessionId: string | null) {
  const candidatesByProfile = new Map<string, SessionEntry[]>();
  for (const entry of entries) {
    const { session } = entry;
    if (
      session.id === excludedSessionId ||
      !session.agent_profile_id ||
      isTerminalSession(session) ||
      hasCompletionFollowUp(session)
    ) {
      continue;
    }
    const candidates = candidatesByProfile.get(session.agent_profile_id) ?? [];
    candidates.push(entry);
    candidatesByProfile.set(session.agent_profile_id, candidates);
  }
  return [...candidatesByProfile.entries()]
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([profileId, candidates]) => ({
      profile_id: profileId,
      sessions: candidates
        .map((entry, index) => ({ entry, index }))
        .sort((left, right) => {
          const updated = compareTimestamps(
            right.entry.session.updated_at,
            left.entry.session.updated_at,
          );
          if (updated !== 0) return updated;
          const started = compareTimestamps(
            right.entry.session.started_at,
            left.entry.session.started_at,
          );
          return started !== 0 ? started : left.index - right.index;
        })
        .map(({ entry }) => sessionReference(entry)),
    }));
}

function profileIdsForRevision(
  tasks: Array<{ task: PreviewRevisionTask }>,
  steps: Array<{ step: PreviewRevisionStep }>,
  workflows: Array<{ id: string; agent_profile_id?: string }>,
  sessions: SessionEntry[],
): Set<string> {
  const profileIds = new Set<string>();
  tasks.forEach(({ task }) => addTaskProfileIds(profileIds, task));
  steps.forEach(({ step }) => addProfileId(profileIds, step.agent_profile_id));
  workflows.forEach((workflow) => addProfileId(profileIds, workflow.agent_profile_id));
  sessions.forEach(({ session }) => addProfileId(profileIds, session.agent_profile_id));
  return profileIds;
}

function addProfileId(profileIds: Set<string>, profileId: unknown) {
  if (typeof profileId === "string" && profileId) profileIds.add(profileId);
}

function addTaskProfileIds(profileIds: Set<string>, task: PreviewRevisionTask) {
  addProfileId(profileIds, task.metadata?.agent_profile_id);
  addProfileId(profileIds, profileIdFromMetadata(task.metadata?.workflow_initial_session));
  const overrides = task.workflowAgentOverrides;
  if (
    !overrides ||
    typeof overrides.workflow_id !== "string" ||
    overrides.workflow_id !== task.workflowId
  ) {
    return;
  }
  const bindings = Array.isArray(overrides.steps) ? overrides.steps : [];
  bindings.forEach((binding) => {
    if (binding && typeof binding === "object") {
      addProfileId(
        profileIds,
        (binding as { replacement_profile_id?: unknown }).replacement_profile_id,
      );
    }
  });
}

function profileIdFromMetadata(value: unknown): string | undefined {
  if (!value || typeof value !== "object") return undefined;
  const profileId = (value as { agent_profile_id?: unknown }).agent_profile_id;
  return typeof profileId === "string" && profileId !== "" ? profileId : undefined;
}

function projectProfile(state: AppState, profile: AppState["agentProfiles"]["items"][number]) {
  const settingsProfile = profileFromSettings(state, profile.id);
  const hasSettingsConfiguration =
    settingsProfile?.mode !== undefined || settingsProfile?.configOptions !== undefined;
  return {
    id: profile.id,
    label: profile.label,
    agent_id: profile.agent_id,
    agent_name: profile.agent_name,
    kind: profile.kind,
    cli_passthrough: profile.cli_passthrough,
    inference_capable: profile.inference_capable,
    model: profile.model,
    fallback_model: profile.fallback_model,
    auto_fallback: profile.auto_fallback,
    require_exact_model: profile.require_exact_model,
    enabled: profile.enabled,
    capability_status: profile.capability_status,
    updated_at: hasSettingsConfiguration ? undefined : profile.updatedAt,
    mode: settingsProfile?.mode,
    config_options: settingsProfile?.configOptions,
  };
}

function profileFromSettings(
  state: AppState,
  profileId: string,
): AppState["settingsAgents"]["items"][number]["profiles"][number] | undefined {
  for (const agent of state.settingsAgents.items) {
    const profile = agent.profiles.find(({ id }) => id === profileId);
    if (profile) return profile;
  }
  return undefined;
}

function projectSessionModels(state: AppState, sessionId: string) {
  const models = state.sessionModels?.bySessionId?.[sessionId];
  if (!models) return null;
  return {
    current_model_id: models.currentModelId,
    models: models.models
      .map(({ modelId, name, usageMultiplier }) => ({
        model_id: modelId,
        name,
        usage_multiplier: usageMultiplier,
      }))
      .sort((left, right) => left.model_id.localeCompare(right.model_id)),
    config_options: models.configOptions
      .map(({ type, id, currentValue }) => ({ type, id, current_value: currentValue }))
      .sort((left, right) => left.id.localeCompare(right.id)),
  };
}

function matchingTasks(state: AppState, taskId: string | null | undefined) {
  if (!taskId) return [];
  const tasks: Array<{ source: string; task: PreviewRevisionTask }> = [];
  for (const task of state.kanban.tasks) {
    if (task.id === taskId) tasks.push({ source: "active", task });
  }
  for (const [workflowId, snapshot] of Object.entries(state.kanbanMulti.snapshots).sort(
    ([a], [b]) => a.localeCompare(b),
  )) {
    for (const task of snapshot.tasks) {
      if (task.id === taskId) tasks.push({ source: `snapshot:${workflowId}`, task });
    }
  }
  return tasks.map(({ source, task }) => ({ source, task }));
}

function matchingSteps(state: AppState, workflowIds: Set<string>, workflowStepIds: Set<string>) {
  if (workflowIds.size === 0 || workflowStepIds.size === 0) return [];
  const steps: Array<{ source: string; step: PreviewRevisionStep }> = [];
  if (state.kanban.workflowId && workflowIds.has(state.kanban.workflowId)) {
    for (const step of state.kanban.steps) {
      if (workflowStepIds.has(step.id)) steps.push({ source: "active", step });
    }
  }
  for (const [workflowId, snapshot] of Object.entries(state.kanbanMulti.snapshots).sort(
    ([a], [b]) => a.localeCompare(b),
  )) {
    if (!workflowIds.has(workflowId)) continue;
    for (const step of snapshot.steps) {
      if (workflowStepIds.has(step.id)) steps.push({ source: `snapshot:${workflowId}`, step });
    }
  }
  return steps.map(({ source, step }) => ({ source, step }));
}

function matchingSessions(state: AppState, taskId: string | null | undefined) {
  if (!taskId) return [];
  // Keep both store projections observable. Semantic selectors group them by
  // source so one logical session cannot become two candidates.
  const sessions: SessionEntry[] = [];
  for (const session of state.taskSessionsByTask.itemsByTaskId[taskId] ?? []) {
    sessions.push({ source: "task", session });
  }
  for (const session of Object.values(state.taskSessions.items)) {
    if (session.task_id === taskId) sessions.push({ source: "session", session });
  }
  return sessions;
}

function sessionSourceGroups(entries: SessionEntry[]): SessionSourceGroup[] {
  const bySource = new Map<string, SessionEntry[]>();
  for (const entry of entries) {
    const sourceEntries = bySource.get(entry.source) ?? [];
    sourceEntries.push(entry);
    bySource.set(entry.source, sourceEntries);
  }
  return [...bySource.entries()]
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([source, sourceEntries]) => ({ source, entries: sourceEntries }));
}

function currentSessionBySource(entries: SessionEntry[]) {
  return sessionSourceGroups(entries).map(({ source, entries: sourceEntries }) => ({
    source,
    session_id: currentSessionReference(sourceEntries)?.id ?? null,
  }));
}

function originalSessionsBySource(task: PreviewRevisionTask | undefined, entries: SessionEntry[]) {
  return sessionSourceGroups(entries).map(({ source, entries: sourceEntries }) => {
    const original = originalSessionReference(task, sourceEntries);
    return {
      source,
      status: original.status,
      session_id: original.session?.id ?? null,
    };
  });
}

function reusableCandidateOrderBySource(entries: SessionEntry[]) {
  return sessionSourceGroups(entries).map(({ source, entries: sourceEntries }) => ({
    source,
    profiles: reusableCandidateOrder(
      sourceEntries,
      currentSessionReference(sourceEntries)?.id ?? null,
    ),
  }));
}

/**
 * Returns a serializable revision of the state that can change a move
 * prediction. It deliberately includes both active and background workflow
 * projections because a disclosure can remain open while either one receives
 * a WebSocket update.
 */
export function getWorkflowMovePreviewRevision(
  state: AppState,
  taskId: string | null | undefined,
  workflowId: string | null | undefined,
  workflowStepId: string | null | undefined,
): string {
  const tasks = matchingTasks(state, taskId);
  const workflowIds = new Set<string>();
  const workflowStepIds = new Set<string>();
  if (workflowId) workflowIds.add(workflowId);
  if (workflowStepId) workflowStepIds.add(workflowStepId);
  for (const { task } of tasks) {
    if (task.workflowId) workflowIds.add(task.workflowId);
    if (task.workflowStepId) workflowStepIds.add(task.workflowStepId);
  }

  const initialSteps = matchingSteps(state, workflowIds, workflowStepIds);
  for (const { step } of initialSteps) {
    if (step.session_target?.kind === "step" && step.session_target.step_id) {
      workflowStepIds.add(step.session_target.step_id);
    }
  }

  const steps = matchingSteps(state, workflowIds, workflowStepIds).sort(
    (left, right) =>
      left.source.localeCompare(right.source) || left.step.id.localeCompare(right.step.id),
  );
  const workflows = state.workflows.items
    .filter((workflow) => workflowIds.has(workflow.id))
    .sort((left, right) => left.id.localeCompare(right.id))
    .map((workflow) => ({
      id: workflow.id,
      agent_profile_id: workflow.agent_profile_id,
    }));
  const sessions = matchingSessions(state, taskId);
  const currentSessions = currentSessionBySource(sessions);
  const projectedSessions = [...sessions].sort(
    (left, right) =>
      left.source.localeCompare(right.source) || left.session.id.localeCompare(right.session.id),
  );
  const profileIds = profileIdsForRevision(tasks, steps, workflows, sessions);
  const profiles = state.agentProfiles.items
    .filter((profile) => profileIds.has(profile.id))
    .sort((left, right) => left.id.localeCompare(right.id))
    .map((profile) => projectProfile(state, profile));
  const sessionModels = Array.from(new Set(sessions.map(({ session }) => session.id)))
    .sort()
    .map((sessionId) => ({
      session_id: sessionId,
      models: projectSessionModels(state, sessionId),
    }));

  return serializeRevision({
    connection: state.connection.status,
    workspace_context_generation: state.workspaceContextGeneration,
    workflow_id: workflowId ?? "",
    workflow_step_id: workflowStepId ?? "",
    workflows,
    steps: steps.map(({ source, step }) => ({ source, step: projectStep(step) })),
    tasks: tasks.map(({ source, task }) => ({ source, task: projectTask(task) })),
    sessions_loaded: taskId ? state.taskSessionsByTask.loadedByTaskId[taskId] === true : false,
    sessions: projectedSessions.map(({ source, session }) => projectSession(session, source)),
    current_sessions: currentSessions,
    original_sessions: tasks.map(({ source, task }) => ({
      source,
      stores: originalSessionsBySource(task, sessions),
    })),
    reusable_candidate_order: reusableCandidateOrderBySource(sessions),
    session_models: sessionModels,
    profiles,
  });
}

export function useWorkflowMovePreviewRevision(
  taskId: string | null | undefined,
  workflowId: string | null | undefined,
  workflowStepId: string | null | undefined,
): string {
  const selector = useCallback(
    (state: AppState) => getWorkflowMovePreviewRevision(state, taskId, workflowId, workflowStepId),
    [taskId, workflowId, workflowStepId],
  );
  return useAppStore(selector);
}
