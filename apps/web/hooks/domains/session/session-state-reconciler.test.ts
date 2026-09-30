import { afterEach, expect, it, vi } from "vitest";
import * as api from "@/lib/api";
import { createAppStore } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";
import { acquireSessionStateReconciliation } from "./session-state-reconciler";

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((next) => {
    resolve = next;
  });
  return { promise, resolve };
}

const SAVED_MODEL = "saved-model";
const SESSION_ETAG_V1 = '"session-v1"';

function sessionWithSavedModel(): TaskSession {
  return {
    id: "session",
    task_id: "task",
    state: "WAITING_FOR_INPUT",
    updated_at: "2026-09-29T00:00:00Z",
    agent_profile_snapshot: { model: SAVED_MODEL },
    metadata: {
      runtime_config_overrides: { model: SAVED_MODEL, config_options: { effort: "high" } },
      acp_model_state: {
        current_model_id: "default-model",
        models: [{ model_id: SAVED_MODEL, name: "Saved model" }],
        config_options: [{ type: "select", id: "effort", name: "Effort", current_value: "medium" }],
        config_options_settled: true,
      },
      acp_config_baseline: { effort: "medium" },
    },
  } as unknown as TaskSession;
}

it("backfills persisted model configuration while reconciling a session summary", async () => {
  const session = sessionWithSavedModel();
  const store = createAppStore();
  store.getState().setTaskSession({ ...session, metadata: {}, agent_profile_snapshot: undefined });
  vi.spyOn(api, "fetchTaskSessionConditional").mockResolvedValue({
    status: "ok",
    data: { session },
    etag: SESSION_ETAG_V1,
  });
  const release = acquireSessionStateReconciliation(store, session.id);
  try {
    await vi.waitFor(() =>
      expect(store.getState().sessionModels.bySessionId[session.id]).toMatchObject({
        currentModelId: SAVED_MODEL,
        models: [{ modelId: SAVED_MODEL }],
        configOptions: [{ id: "effort", currentValue: "high" }],
        configOptionsSettled: true,
        configBaseline: { effort: "medium" },
      }),
    );
    expect(store.getState().taskSessions.items[session.id].agent_profile_snapshot?.model).toBe(
      SAVED_MODEL,
    );
  } finally {
    release();
  }
});

it("keeps a live model event that arrives before background reconciliation", async () => {
  const session = sessionWithSavedModel();
  const store = createAppStore();
  store.getState().setTaskSession({ ...session, metadata: {} });
  let finish!: (value: Awaited<ReturnType<typeof api.fetchTaskSessionConditional>>) => void;
  vi.spyOn(api, "fetchTaskSessionConditional").mockReturnValue(
    new Promise((resolve) => {
      finish = resolve;
    }),
  );
  const release = acquireSessionStateReconciliation(store, session.id);
  try {
    store.getState().setSessionModels(session.id, {
      currentModelId: "live-model",
      models: [],
      configOptions: [],
    });
    finish({ status: "ok", data: { session }, etag: SESSION_ETAG_V1 });
    await vi.waitFor(() =>
      expect(store.getState().taskSessions.items[session.id].metadata).toEqual(session.metadata),
    );
    expect(store.getState().sessionModels.bySessionId[session.id].currentModelId).toBe(
      "live-model",
    );
  } finally {
    release();
  }
});

it("reconciles provider-restored selectors without reviving saved settings", async () => {
  const session = {
    ...sessionWithSavedModel(),
    metadata: {
      runtime_config: { model: "saved-runtime-model", mode: "saved-runtime-mode" },
      runtime_config_overrides: {
        model: "saved-override-model",
        mode: "saved-override-mode",
      },
      acp_model_state: {
        settings_policy: "provider_restored",
        current_model_id: "",
        current_mode_id: "",
        models: [],
        config_options: [],
      },
    },
  } as unknown as TaskSession;
  const store = createAppStore();
  store.getState().setTaskSession({ ...session, metadata: {} });
  vi.spyOn(api, "fetchTaskSessionConditional").mockResolvedValue({
    status: "ok",
    data: { session },
    etag: SESSION_ETAG_V1,
  });
  const release = acquireSessionStateReconciliation(store, session.id);
  try {
    await vi.waitFor(() => {
      expect(store.getState().sessionModels.bySessionId[session.id]).toMatchObject({
        currentModelId: "",
        settingsPolicy: "provider_restored",
      });
      expect(store.getState().sessionMode.bySessionId[session.id]).toMatchObject({
        currentModeId: "",
        settingsPolicy: "provider_restored",
      });
    });
  } finally {
    release();
  }
});

it("restarts with an unconditional read when consumers reacquire during a released read", async () => {
  vi.useFakeTimers();
  const session = { ...sessionWithSavedModel(), state: "RUNNING" as const };
  const store = createAppStore();
  store.getState().setTaskSession({ ...session, metadata: {} });
  const releasedRead = deferred<Awaited<ReturnType<typeof api.fetchTaskSessionConditional>>>();
  const freshRead = deferred<Awaited<ReturnType<typeof api.fetchTaskSessionConditional>>>();
  const fetch = vi
    .spyOn(api, "fetchTaskSessionConditional")
    .mockResolvedValueOnce({ status: "ok", data: { session }, etag: SESSION_ETAG_V1 })
    .mockReturnValueOnce(releasedRead.promise)
    .mockReturnValueOnce(freshRead.promise);

  const releaseFirst = acquireSessionStateReconciliation(store, session.id);
  await vi.waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
  await vi.waitFor(() =>
    expect(store.getState().taskSessions.items[session.id].metadata).toEqual(session.metadata),
  );
  vi.setSystemTime(Date.now() + 29_500);
  await vi.advanceTimersByTimeAsync(750);
  expect(fetch).toHaveBeenCalledTimes(2);
  expect(fetch).toHaveBeenLastCalledWith(session.id, SESSION_ETAG_V1);

  releaseFirst();
  const releaseSecond = acquireSessionStateReconciliation(store, session.id);
  releasedRead.resolve({ status: "not-modified", etag: SESSION_ETAG_V1 });
  for (let i = 0; i < 5; i += 1) await Promise.resolve();
  expect(fetch).toHaveBeenCalledTimes(3);
  expect(fetch).toHaveBeenLastCalledWith(session.id);

  const freshSession = {
    ...session,
    state: "WAITING_FOR_INPUT" as const,
    updated_at: "2026-09-29T00:00:01Z",
  };
  freshRead.resolve({ status: "ok", data: { session: freshSession }, etag: '"session-v2"' });
  await vi.waitFor(() =>
    expect(store.getState().taskSessions.items[session.id].state).toBe("WAITING_FOR_INPUT"),
  );
  releaseSecond();
});
