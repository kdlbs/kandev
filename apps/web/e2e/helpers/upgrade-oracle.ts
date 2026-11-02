import { isDeepStrictEqual } from "node:util";
import fs from "node:fs";
import path from "node:path";
import type { UpgradeBackend } from "../fixtures/upgrade-backend";
import type { UpgradeSeed } from "./upgrade-seed";

export function assertPreserved(expected: unknown, actual: unknown, at = "seed") {
  if (isDeepStrictEqual(expected, actual)) return;
  if (expected && actual && typeof expected === "object" && typeof actual === "object") {
    const left = expected as Record<string, unknown>,
      right = actual as Record<string, unknown>;
    for (const key of new Set([...Object.keys(left), ...Object.keys(right)]))
      assertPreserved(left[key], right[key], `${at}.${key}`);
  }
  throw new Error(
    `preservation mismatch at ${at}: expected ${JSON.stringify(expected)}, actual ${JSON.stringify(actual)}`,
  );
}
type HistoryTask = { id: string; sessions: { id: string; messages: { id: string }[] }[] };

export function assertHistoricalMessagesPreserved(
  before: { tasks: unknown[] },
  after: { tasks: unknown[] },
) {
  for (const task of before.tasks as HistoryTask[]) {
    const current = (after.tasks as HistoryTask[]).find((candidate) => candidate.id === task.id);
    for (const session of task.sessions) {
      const actual = current?.sessions.find((candidate) => candidate.id === session.id);
      const originalIds = new Set(session.messages.map((message) => message.id));
      assertPreserved(
        session.messages,
        actual?.messages.filter((message) => originalIds.has(message.id)),
        `history.${task.id}.${session.id}`,
      );
    }
  }
}

function select(value: unknown, keys: string[]) {
  const row = value as Record<string, unknown>;
  return Object.fromEntries(keys.map((key) => [key, row[key] ?? null]));
}
function sessionState(state: string, interrupted: boolean) {
  // Cold restart can detach a quiescent execution; neither state implies a
  // prompt ran. Interrupted sessions can settle to a recoverable state.
  if (
    interrupted &&
    ["RUNNING", "STARTING", "FAILED", "CANCELLED", "WAITING_FOR_INPUT", "IDLE"].includes(state)
  )
    return "interrupted-recoverable";
  if (["WAITING_FOR_INPUT", "IDLE"].includes(state)) return "quiescent";
  return state;
}
export async function captureUpgradeState(backend: UpgradeBackend, seed: UpgradeSeed) {
  const api = backend.api;
  const taskRecords: unknown[] = [];
  for (const [kind, task] of Object.entries(seed.tasks)) {
    const detail = await api.getTask(task.id);
    const { sessions } = await api.listTaskSessions(task.id);
    const records = [];
    for (const session of sessions.sort((a, b) => a.id.localeCompare(b.id))) {
      const { messages } = await api.listSessionMessages(session.id);
      const { turns } = await api.listSessionTurns(session.id);
      const unique = new Set(messages.map((m) => m.id));
      if (unique.size !== messages.length)
        throw new Error(`duplicate message identities: ${session.id}`);
      records.push({
        ...select(session, [
          "id",
          "task_id",
          "agent_profile_id",
          "executor_profile_id",
          "is_primary",
          "started_at",
        ]),
        state: sessionState(session.state, kind === "interrupted"),
        turnIds: turns.map((turn) => turn.id),
        messages: messages
          .filter(
            (m) => m.author_type === "user" || (m.author_type === "agent" && m.type === "message"),
          )
          .map((m) => select(m, ["id", "content", "author_type", "type", "created_at"])),
      });
    }
    taskRecords.push({
      kind,
      ...select(detail, [
        "id",
        "workspace_id",
        "workflow_id",
        "workflow_step_id",
        "title",
        "description",
        "repositories",
        "archived_at",
      ]),
      state: kind === "interrupted" ? "interrupted-recoverable" : detail.state,
      sessions: records,
      plan: select(await api.getTaskPlan(task.id), ["task_id", "title", "content", "created_by"]),
    });
  }
  const settings = (await api.getUserSettings()).settings;
  const { steps } = await api.listWorkflowSteps(seed.workflowId);
  return {
    workspace: select(
      (await api.listWorkspaces()).workspaces.find((w) => w.id === seed.workspaceId),
      ["id", "name"],
    ),
    repository: select(await api.getRepository(seed.repositoryId), [
      "id",
      "local_path",
      "default_branch",
    ]),
    workflow: select(
      (await api.listWorkflows(seed.workspaceId)).workflows.find((w) => w.id === seed.workflowId),
      ["id", "name"],
    ),
    steps: steps
      .sort((a, b) => a.id.localeCompare(b.id))
      .map((s) =>
        select(s, [
          "id",
          "name",
          "position",
          "agent_profile_id",
          "complete_task_on_enter",
          "events",
        ]),
      ),
    profile: select(await api.getAgentProfile(seed.profileId), ["id", "name", "model"]),
    reviewProfile: select(await api.getAgentProfile(seed.reviewProfileId), ["id", "name", "model"]),
    executorProfile: select(await api.getExecutorProfile(seed.executorId, seed.executorProfileId), [
      "id",
      "name",
    ]),
    settings: select(settings, [
      "workspace_id",
      "workflow_filter_id",
      "enable_preview_on_click",
      "confirm_task_archive",
    ]),
    tasks: taskRecords,
  };
}
export function saveUpgradeSnapshot(backend: UpgradeBackend, name: string, snapshot: unknown) {
  fs.writeFileSync(path.join(backend.root, name), JSON.stringify(snapshot, null, 2));
}

export async function assertPassiveUpgradeReads(backend: UpgradeBackend, seed: UpgradeSeed) {
  const before = await captureUpgradeState(backend, seed);
  for (const [kind, task] of Object.entries(seed.tasks)) {
    for (const session of (await backend.api.listTaskSessions(task.id)).sessions) {
      const status = await backend.api.wsRequest<{ is_agent_running: boolean }>(
        "task.session.status",
        { task_id: task.id, session_id: session.id },
      );
      // Startup may restore the interrupted native conversation without a prompt.
      // Other seeded sessions must stay detached until an explicit action.
      if (status.is_agent_running && kind !== "interrupted")
        throw new Error(`passive history read launched execution: ${session.id}`);
    }
  }
  assertPreserved(before, await captureUpgradeState(backend, seed));
}
