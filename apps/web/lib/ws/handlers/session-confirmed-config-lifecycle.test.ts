import { expect, it } from "vitest";
import { mapAvailableCommandToSlashCommand } from "@/components/task/chat/slash-command-types";
import { createAppStore } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";
import { registerTaskSessionHandlers } from "./agent-session";
import { registerSessionModelsHandlers } from "./session-models";

const taskId = "task-1";
const sessionId = "session-1";
const previousExecutionId = "execution-old";
const nextExecutionId = "execution-new";
const modelId = "gpt-5.6-sol";

const planCommand = {
  name: "plan",
  description: "Set plan mode",
  action: {
    kind: "set_config_option" as const,
    config_id: "collaboration_mode",
    value: "plan",
    reset_value: "default",
  },
};

function makeStore() {
  const store = createAppStore();
  store.getState().upsertTaskSessionFromEvent(taskId, {
    id: sessionId,
    task_id: taskId,
    state: "WAITING_FOR_INPUT",
    started_at: "2026-10-05T00:00:00.000Z",
    updated_at: "2026-10-05T00:00:00.000Z",
  } as TaskSession);
  store.getState().setSessionModels(sessionId, {
    currentModelId: modelId,
    models: [],
    configOptions: [],
    confirmedConfigOptions: { collaboration_mode: "plan" },
    confirmedConfigOptionsExecutionId: previousExecutionId,
  });
  return store;
}

function sessionStarting(store: ReturnType<typeof createAppStore>) {
  registerTaskSessionHandlers(store)["session.state_changed"]!({
    id: "starting-1",
    type: "notification",
    action: "session.state_changed",
    payload: {
      task_id: taskId,
      session_id: sessionId,
      old_state: "WAITING_FOR_INPUT",
      new_state: "STARTING",
      updated_at: "2026-10-05T00:01:00.000Z",
    },
  } as never);
}

function agentctlStarting(store: ReturnType<typeof createAppStore>, executionId: string) {
  registerTaskSessionHandlers(store)["session.agentctl_starting"]!({
    id: "agentctl-starting-1",
    type: "notification",
    action: "session.agentctl_starting",
    payload: {
      task_id: taskId,
      session_id: sessionId,
      agent_execution_id: executionId,
    },
  } as never);
}

function confirmedMode(store: ReturnType<typeof createAppStore>) {
  return store.getState().sessionModels.bySessionId[sessionId]?.confirmedConfigOptions;
}

function menuMode(store: ReturnType<typeof createAppStore>) {
  return mapAvailableCommandToSlashCommand(planCommand, confirmedMode(store)).modeState;
}

it("hides the old mode at STARTING and restores it only after the new execution settles", () => {
  const store = makeStore();
  expect(menuMode(store)).toBe("active");

  sessionStarting(store);
  expect(confirmedMode(store)).toBeUndefined();
  expect(menuMode(store)).not.toBe("active");
  expect(store.getState().sessionModels.bySessionId[sessionId].currentModelId).toBe(modelId);

  agentctlStarting(store, nextExecutionId);
  expect(confirmedMode(store)).toBeUndefined();
  expect(menuMode(store)).not.toBe("active");

  registerSessionModelsHandlers(store)["session.models_updated"]!({
    id: "models-settled-1",
    type: "notification",
    action: "session.models_updated",
    payload: {
      task_id: taskId,
      session_id: sessionId,
      agent_id: "agent-1",
      agent_execution_id: nextExecutionId,
      current_model_id: modelId,
      models: [],
      config_options: [
        {
          type: "select",
          id: "collaboration_mode",
          name: "Collaboration Mode",
          current_value: "plan",
          options: [],
        },
      ],
      config_options_settled: true,
      timestamp: "2026-10-05T00:02:00.000Z",
    },
  } as never);

  expect(confirmedMode(store)).toEqual({ collaboration_mode: "plan" });
  expect(
    store.getState().sessionModels.bySessionId[sessionId].confirmedConfigOptionsExecutionId,
  ).toBe(nextExecutionId);
  expect(menuMode(store)).toBe("active");
});

it("preserves a same-execution confirmation across startup event orderings", () => {
  const store = makeStore();

  agentctlStarting(store, previousExecutionId);
  sessionStarting(store);

  expect(confirmedMode(store)).toEqual({ collaboration_mode: "plan" });
  expect(menuMode(store)).toBe("active");
});

it("restores a pending confirmation when STARTING arrives before its same-execution identity", () => {
  const store = makeStore();

  sessionStarting(store);
  expect(confirmedMode(store)).toBeUndefined();

  agentctlStarting(store, previousExecutionId);

  expect(confirmedMode(store)).toEqual({ collaboration_mode: "plan" });
  expect(menuMode(store)).toBe("active");
});

it("ignores a delayed settled snapshot while startup identity is unresolved", () => {
  const store = makeStore();
  const handler = registerSessionModelsHandlers(store)["session.models_updated"]!;

  sessionStarting(store);
  expect(confirmedMode(store)).toBeUndefined();

  handler({
    id: "models-delayed-old-execution",
    type: "notification",
    action: "session.models_updated",
    payload: {
      task_id: taskId,
      session_id: sessionId,
      agent_id: "agent-1",
      agent_execution_id: previousExecutionId,
      current_model_id: modelId,
      models: [],
      config_options: [
        {
          type: "select",
          id: "collaboration_mode",
          name: "Collaboration Mode",
          current_value: "plan",
          options: [],
        },
      ],
      config_options_settled: true,
      timestamp: "2026-10-05T00:01:30.000Z",
    },
  } as never);

  expect(confirmedMode(store)).toBeUndefined();
  expect(menuMode(store)).not.toBe("active");

  agentctlStarting(store, nextExecutionId);
  expect(confirmedMode(store)).toBeUndefined();

  handler({
    id: "models-settled-new-execution",
    type: "notification",
    action: "session.models_updated",
    payload: {
      task_id: taskId,
      session_id: sessionId,
      agent_id: "agent-1",
      agent_execution_id: nextExecutionId,
      current_model_id: modelId,
      models: [],
      config_options: [
        {
          type: "select",
          id: "collaboration_mode",
          name: "Collaboration Mode",
          current_value: "plan",
          options: [],
        },
      ],
      config_options_settled: true,
      timestamp: "2026-10-05T00:02:00.000Z",
    },
  } as never);

  expect(confirmedMode(store)).toEqual({ collaboration_mode: "plan" });
  expect(menuMode(store)).toBe("active");
});

it.each([true, false])(
  "does not reuse an inferred startup identity for a later startup (STARTING observed: %s)",
  (observeStarting) => {
    const store = makeStore();
    agentctlStarting(store, previousExecutionId);
    if (observeStarting) sessionStarting(store);
    expect(confirmedMode(store)).toEqual({ collaboration_mode: "plan" });
    registerTaskSessionHandlers(store)["session.state_changed"]!({
      id: "running-1",
      type: "notification",
      action: "session.state_changed",
      payload: {
        task_id: taskId,
        session_id: sessionId,
        new_state: "RUNNING",
        updated_at: "2026-10-05T00:02:00.000Z",
      },
    } as never);
    registerTaskSessionHandlers(store)["session.state_changed"]!({
      id: "starting-2",
      type: "notification",
      action: "session.state_changed",
      payload: {
        task_id: taskId,
        session_id: sessionId,
        new_state: "STARTING",
        updated_at: "2026-10-05T00:03:00.000Z",
      },
    } as never);
    expect(confirmedMode(store)).toBeUndefined();
  },
);
