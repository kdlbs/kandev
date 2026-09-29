import { afterEach, expect, it, vi } from "vitest";
import * as api from "@/lib/api";
import { createAppStore } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";
import { acquireSessionStateReconciliation } from "./session-state-reconciler";

afterEach(() => vi.restoreAllMocks());

const SAVED_MODEL = "saved-model";

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
  vi.spyOn(api, "fetchTaskSession").mockResolvedValue({ session });
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
  let finish!: (value: { session: TaskSession }) => void;
  vi.spyOn(api, "fetchTaskSession").mockReturnValue(
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
    finish({ session });
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
  vi.spyOn(api, "fetchTaskSession").mockResolvedValue({ session });
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
