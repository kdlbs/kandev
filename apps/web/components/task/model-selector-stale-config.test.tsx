import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";

import { ModelSelector, hasCompleteDynamicConfig } from "@/components/task/model-selector";
import { buildSessionModelsState as buildBootSessionModelsState } from "@/lib/state/slices/session-runtime/model-hydration";
import {
  buildSessionModelsState as buildFetchedSessionModelsState,
  sessionModelsEntryFromTaskSession,
} from "@/lib/state/slices/session-runtime/session-model-hydration";
import type { SessionModelsState } from "@/lib/state/slices/session-runtime/types";
import { setSessionConfigOption, setSessionModel } from "@/lib/api/domains/session-api";
import { registerSessionModelsHandlers } from "@/lib/ws/handlers/session-models";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap } from "@/lib/types/backend";
import type { SessionModelsPayload } from "@/lib/types/session-runtime-payloads";
import type { Agent, TaskSession } from "@/lib/types/http";

type SessionModelsEntry = SessionModelsState["bySessionId"][string];

const MODEL_ID = "composer-2.5";
const NEXT_MODEL_ID = "composer-3";

const mocks = vi.hoisted(() => {
  const SESSION_ID = "session-stale-config";
  const listeners = new Set<() => void>();
  function notify() {
    for (const listener of listeners) listener();
  }
  const appState = {
    activeModel: { bySessionId: {} as Record<string, string> },
    sessionModels: { bySessionId: {} as Record<string, SessionModelsEntry | undefined> },
    settingsAgents: { items: [] as Agent[] },
    taskSessions: { items: {} as Record<string, TaskSession> },
    sessionAgentctl: {
      itemsBySessionId: {} as Record<string, { status?: string; agentExecutionId?: string }>,
    },
    setActiveModel: vi.fn((sessionId: string, modelId: string) => {
      appState.activeModel.bySessionId[sessionId] = modelId;
      notify();
    }),
    clearActiveModel: vi.fn((sessionId: string) => {
      delete appState.activeModel.bySessionId[sessionId];
      notify();
    }),
    setSessionModels: vi.fn((sessionId: string, data: SessionModelsEntry) => {
      appState.sessionModels.bySessionId[sessionId] = data;
      notify();
    }),
    clearContextWindow: vi.fn(),
  };
  return { appState, listeners, notify, SESSION_ID };
});

vi.mock("@/components/state-provider", async () => {
  const { useEffect, useReducer } = await import("react");
  return {
    useAppStoreApi: () => ({ getState: () => mocks.appState }),
    useAppStore: (selector: (state: typeof mocks.appState) => unknown) => {
      const [, force] = useReducer((count: number) => count + 1, 0);
      useEffect(() => {
        mocks.listeners.add(force);
        return () => {
          mocks.listeners.delete(force);
        };
      }, [force]);
      return selector(mocks.appState);
    },
  };
});

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("@/hooks/domains/settings/use-available-agents", () => ({
  useAvailableAgents: () => ({ items: [] }),
}));

vi.mock("@/hooks/domains/settings/use-settings-data", () => ({
  useSettingsData: vi.fn(),
}));

vi.mock("@/lib/api/domains/session-api", () => ({
  setSessionConfigOption: vi.fn(() => Promise.resolve({ ok: true })),
  setSessionMode: vi.fn(() => Promise.resolve({ ok: true })),
  setSessionModel: vi.fn(() => Promise.resolve({ ok: true })),
}));

const SESSION_ID = mocks.SESSION_ID;

function configOption(
  id: string,
  currentValue: string,
  options: { value: string; name: string }[],
  category?: string,
): ConfigOptionEntryShape {
  return {
    type: "select",
    id,
    name: id,
    currentValue,
    category,
    options,
  };
}

type ConfigOptionEntryShape = SessionModelsEntry["configOptions"][number];

function advertisedConfigOptions(): ConfigOptionEntryShape[] {
  return [
    configOption("mode", "agent", [{ value: "agent", name: "Agent" }]),
    configOption(
      "model",
      MODEL_ID,
      [
        { value: MODEL_ID, name: "Composer 2.5" },
        { value: NEXT_MODEL_ID, name: "Composer 3" },
      ],
      "model",
    ),
    configOption("fast", "on", [
      { value: "off", name: "Off" },
      { value: "on", name: "On" },
    ]),
  ];
}

function snapshotConfigOptions(options: ConfigOptionEntryShape[]) {
  return options.map((option) => ({
    type: option.type,
    id: option.id,
    name: option.name,
    current_value: option.currentValue,
    category: option.category,
    options: option.options,
  }));
}

function composerSession(): TaskSession {
  return {
    id: SESSION_ID,
    task_id: "task-1",
    state: "WAITING_FOR_INPUT",
    agent_profile_id: "profile-1",
    agent_profile_snapshot: { config_options: {} },
    metadata: {
      runtime_config: {
        model: MODEL_ID,
        config_options: {
          model: MODEL_ID,
          context: "200k",
          effort: "high",
          reasoning_effort: "high",
        },
      },
      acp_model_state: {
        current_model_id: MODEL_ID,
        models: [],
        config_options: snapshotConfigOptions(advertisedConfigOptions()),
      },
    },
    started_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  } as unknown as TaskSession;
}

function livePayload(
  currentModelId: string,
  configOptions: ConfigOptionEntryShape[],
): SessionModelsPayload {
  return {
    task_id: "task-1",
    session_id: SESSION_ID,
    agent_id: "agent-1",
    current_model_id: currentModelId,
    models: [],
    config_options: snapshotConfigOptions(configOptions),
    timestamp: "2026-01-01T00:00:00Z",
  };
}

function liveMessage(payload: SessionModelsPayload): BackendMessageMap["session.models_updated"] {
  return {
    id: "message-live",
    type: "notification",
    action: "session.models_updated",
    payload,
  };
}

function applyBootHydration(session: TaskSession) {
  mocks.appState.taskSessions.items[session.id] = session;
  const hydrated = buildBootSessionModelsState(session);
  mocks.appState.sessionModels.bySessionId = hydrated.sessionModels?.bySessionId ?? {};
}

function applyFetchHydration(session: TaskSession) {
  mocks.appState.taskSessions.items[session.id] = session;
  const entry = sessionModelsEntryFromTaskSession(session);
  mocks.appState.sessionModels.bySessionId = entry ? { [session.id]: entry } : {};
}

function renderSelector() {
  render(
    <TooltipProvider>
      <ModelSelector sessionId={SESSION_ID} />
    </TooltipProvider>,
  );
  return screen.getByRole("button", { name: "Session model settings" });
}

function openPicker(trigger: HTMLElement) {
  if (trigger.getAttribute("aria-expanded") !== "true") {
    fireEvent.pointerDown(trigger);
    fireEvent.click(trigger);
  }
}

function wsStore(): StoreApi<AppState> {
  return { getState: () => mocks.appState } as unknown as StoreApi<AppState>;
}

beforeEach(() => {
  mocks.appState.activeModel.bySessionId = {};
  mocks.appState.sessionModels.bySessionId = {};
  mocks.appState.taskSessions.items = {};
  mocks.appState.settingsAgents.items = [];
  vi.clearAllMocks();
});

afterEach(() => {
  cleanup();
});

describe("model selector with stale runtime config", () => {
  it.each([
    ["boot hydration", applyBootHydration],
    ["client fetch hydration", applyFetchHydration],
  ])("keeps advertised controls usable after %s without settlement", (_label, hydrate) => {
    const session = composerSession();
    hydrate(session);
    const entry = mocks.appState.sessionModels.bySessionId[SESSION_ID];

    expect(entry?.configOptionsSettled).toBe(false);
    expect(hasCompleteDynamicConfig(session, entry, mocks.appState.settingsAgents.items)).toBe(
      true,
    );

    const trigger = renderSelector();
    expect(trigger).toBeTruthy();
    openPicker(trigger);
    expect(screen.getByRole("option", { name: /Composer 3/ })).toBeTruthy();
    expect(setSessionConfigOption).not.toHaveBeenCalled();
    expect(setSessionModel).not.toHaveBeenCalled();

    const runtime = session.metadata?.runtime_config as {
      config_options?: Record<string, string>;
    };
    expect(runtime.config_options?.context).toBe("200k");
    expect(runtime.config_options?.reasoning_effort).toBe("high");
  });

  it("keeps advertised controls usable after a live update without settlement", () => {
    const session = composerSession();
    applyBootHydration(session);
    const handler = registerSessionModelsHandlers(wsStore())["session.models_updated"]!;

    act(() => {
      handler(liveMessage(livePayload(MODEL_ID, advertisedConfigOptions())));
    });

    const liveEntry = mocks.appState.sessionModels.bySessionId[SESSION_ID];
    expect(liveEntry?.configOptionsSettled).toBe(false);
    expect(hasCompleteDynamicConfig(session, liveEntry, mocks.appState.settingsAgents.items)).toBe(
      true,
    );

    const trigger = renderSelector();
    expect(trigger).toBeTruthy();
    openPicker(trigger);
    expect(screen.getByTestId("config-option-trigger-fast")).toBeTruthy();
    expect(setSessionConfigOption).not.toHaveBeenCalled();
    expect(setSessionModel).not.toHaveBeenCalled();
  });

  it("sends only the explicit advertised model choice", () => {
    const session = composerSession();
    applyBootHydration(session);

    const trigger = renderSelector();
    openPicker(trigger);
    expect(setSessionConfigOption).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("option", { name: /Composer 3/ }));

    expect(setSessionConfigOption).toHaveBeenCalledTimes(1);
    expect(setSessionConfigOption).toHaveBeenCalledWith(SESSION_ID, "model", NEXT_MODEL_ID);
    expect(setSessionModel).not.toHaveBeenCalled();
  });

  it("keeps the selector visible when a later update removes an advertised option", () => {
    const session = composerSession();
    applyBootHydration(session);
    const handler = registerSessionModelsHandlers(wsStore())["session.models_updated"]!;

    act(() => {
      handler(liveMessage(livePayload(MODEL_ID, advertisedConfigOptions())));
    });
    expect(mocks.appState.sessionModels.bySessionId[SESSION_ID]?.configOptionsSettled).toBe(false);

    const remainingOptions = advertisedConfigOptions().filter((option) => option.id !== "fast");
    act(() => {
      handler(liveMessage(livePayload(MODEL_ID, remainingOptions)));
    });

    const updated = mocks.appState.sessionModels.bySessionId[SESSION_ID];
    expect(updated?.configOptions.map((option) => option.id)).toEqual(["mode", "model"]);
    expect(updated?.configOptionsSettled).toBe(false);
    expect(hasCompleteDynamicConfig(session, updated, mocks.appState.settingsAgents.items)).toBe(
      true,
    );

    const trigger = renderSelector();
    expect(trigger).toBeTruthy();
    openPicker(trigger);
    expect(screen.queryByTestId("config-option-trigger-fast")).toBeNull();
    expect(screen.getByRole("option", { name: /Composer 3/ })).toBeTruthy();
  });

  it("keeps both hydration helpers aligned on the absent settlement marker", () => {
    const session = composerSession();
    const boot = buildBootSessionModelsState(session).sessionModels?.bySessionId[SESSION_ID];
    const fetched = buildFetchedSessionModelsState(session).sessionModels?.bySessionId[SESSION_ID];

    expect(boot?.configOptionsSettled).toBe(false);
    expect(fetched?.configOptionsSettled).toBe(false);
    expect(sessionModelsEntryFromTaskSession(session)?.configOptionsSettled).toBe(false);
  });
});

describe("model selector with provider-restored stale runtime config", () => {
  it("keeps provider-restored values while releasing stale runtime keys", () => {
    const session = composerSession();
    const metadata = session.metadata as {
      acp_model_state?: Record<string, unknown>;
      runtime_config?: { model?: string };
    };
    metadata.acp_model_state = {
      ...metadata.acp_model_state,
      settings_policy: "provider_restored",
      current_model_id: NEXT_MODEL_ID,
      config_options: snapshotConfigOptions(
        advertisedConfigOptions().map((option) =>
          option.id === "model" ? { ...option, currentValue: NEXT_MODEL_ID } : option,
        ),
      ),
    };
    if (metadata.runtime_config) metadata.runtime_config.model = MODEL_ID;

    applyBootHydration(session);
    const entry = mocks.appState.sessionModels.bySessionId[SESSION_ID];
    expect(entry?.settingsPolicy).toBe("provider_restored");
    expect(entry?.currentModelId).toBe(NEXT_MODEL_ID);
    expect(hasCompleteDynamicConfig(session, entry, mocks.appState.settingsAgents.items)).toBe(
      true,
    );

    const trigger = renderSelector();
    expect(trigger.textContent).toContain("Composer 3");
    openPicker(trigger);
    fireEvent.click(screen.getByRole("option", { name: /Composer 2.5/ }));
    expect(setSessionConfigOption).toHaveBeenCalledWith(SESSION_ID, "model", MODEL_ID);
  });
});
