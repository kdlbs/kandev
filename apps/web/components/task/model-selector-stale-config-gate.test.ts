import { describe, expect, it } from "vitest";
import { hasCompleteDynamicConfig, requiredConfigKeys } from "@/components/task/model-selector";
import type { Agent, TaskSession } from "@/lib/types/http";
import type { ConfigOptionEntry } from "@/lib/state/slices/session-runtime/types";

const noAgents: Agent[] = [];
const composerModelId = "composer-2.5";
const composerModelName = "Composer 2.5";
const composerNextModelId = "composer-3";

function makeSession(overrides: Partial<TaskSession> = {}): TaskSession {
  return {
    id: "session-1",
    task_id: "task-1",
    state: "RUNNING",
    started_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  } as TaskSession;
}

function selectOption(id: string, currentValue: string): ConfigOptionEntry {
  return {
    type: "select",
    id,
    name: id,
    currentValue,
    options: [{ value: currentValue, name: currentValue }],
  };
}

function composerModelOption(optionId = "model"): ConfigOptionEntry {
  return {
    type: "select",
    id: optionId,
    name: "Model",
    currentValue: composerModelId,
    category: "model",
    options: [
      { value: composerModelId, name: composerModelName },
      { value: composerNextModelId, name: "Composer 3" },
    ],
  };
}

function composerCatalog(
  configOptions: ConfigOptionEntry[] = [
    selectOption("mode", "agent"),
    composerModelOption(),
    selectOption("fast", "on"),
  ],
  settled?: boolean,
) {
  return {
    currentModelId: composerModelId,
    models: [],
    configOptions,
    ...(settled === undefined ? {} : { configOptionsSettled: settled }),
  } as Parameters<typeof hasCompleteDynamicConfig>[1];
}

function staleRuntimeSession(): TaskSession {
  return makeSession({
    metadata: {
      runtime_config: {
        model: composerModelId,
        config_options: {
          model: composerModelId,
          context: "200k",
          effort: "high",
          reasoning_effort: "high",
        },
      },
    },
  });
}

describe("stale runtime config availability", () => {
  it("hydrates persisted-only stale keys from an advertised catalog without settlement", () => {
    const session = staleRuntimeSession();

    expect(requiredConfigKeys(session, noAgents)).toEqual([
      "model",
      "context",
      "effort",
      "reasoning_effort",
    ]);
    expect(hasCompleteDynamicConfig(session, composerCatalog(), noAgents)).toBe(true);
  });

  it.each([
    ["missing", undefined],
    ["false", false],
    ["true", true],
  ])("accepts a usable advertised catalog with a %s settlement marker", (_label, settled) => {
    expect(
      hasCompleteDynamicConfig(
        staleRuntimeSession(),
        composerCatalog(undefined, settled),
        noAgents,
      ),
    ).toBe(true);
  });

  it("accepts unadvertised historical keys from runtime and override sources", () => {
    const session = makeSession({
      metadata: {
        runtime_config: { config_options: { model: composerModelId, context: "200k" } },
        runtime_config_overrides: { config_options: { reasoning_effort: "high" } },
      },
    });

    expect(requiredConfigKeys(session, noAgents)).toEqual(
      expect.arrayContaining(["model", "context", "reasoning_effort"]),
    );
    expect(hasCompleteDynamicConfig(session, composerCatalog(), noAgents)).toBe(true);
  });

  it("releases the model key through a category-aliased model select option", () => {
    const session = makeSession({
      metadata: {
        runtime_config: { config_options: { model: composerModelId, context: "200k" } },
      },
    });
    const catalog = composerCatalog([
      selectOption("mode", "agent"),
      composerModelOption("model_choice"),
      selectOption("fast", "on"),
    ]);

    expect(hasCompleteDynamicConfig(session, catalog, noAgents)).toBe(true);
  });
});

describe("stale runtime config availability boundaries", () => {
  it("keeps an unresolved snapshot key required beside ignorable stale runtime keys", () => {
    const session = makeSession({
      metadata: {
        runtime_config: { config_options: { model: composerModelId, context: "200k" } },
      },
      agent_profile_snapshot: { config_options: { verbosity: "terse" } },
    });

    expect(hasCompleteDynamicConfig(session, composerCatalog(), noAgents)).toBe(false);
  });

  it("keeps an unresolved current-profile key required beside ignorable stale runtime keys", () => {
    const agents = [
      {
        id: "agent-1",
        name: "mock",
        profiles: [{ id: "profile-1", configOptions: { verbosity: "terse" } }],
      },
    ] as unknown as Agent[];
    const session = makeSession({
      agent_profile_id: "profile-1" as TaskSession["agent_profile_id"],
      metadata: {
        runtime_config: { config_options: { model: composerModelId, context: "200k" } },
      },
    });

    expect(hasCompleteDynamicConfig(session, composerCatalog(), agents)).toBe(false);
  });

  it("keeps the existing gate when the advertised catalog is absent or empty", () => {
    const session = staleRuntimeSession();
    const emptyCatalog = {
      currentModelId: composerModelId,
      models: [],
      configOptions: [],
    };

    expect(hasCompleteDynamicConfig(session, undefined, noAgents)).toBe(false);
    expect(hasCompleteDynamicConfig(session, emptyCatalog, noAgents)).toBe(false);
  });

  it("keeps the existing gate for a flat-list-only catalog", () => {
    const session = staleRuntimeSession();
    const flatCatalog = {
      currentModelId: composerModelId,
      models: [{ modelId: composerModelId, name: composerModelName }],
      configOptions: [],
    };

    expect(hasCompleteDynamicConfig(session, flatCatalog, noAgents)).toBe(false);
  });

  it("keeps a provider-defined agent key required without settlement", () => {
    const session = makeSession({
      metadata: {
        runtime_config: {
          config_options: { model: composerModelId, agent: "build", context: "200k" },
        },
      },
    });

    expect(hasCompleteDynamicConfig(session, composerCatalog(), noAgents)).toBe(false);
  });
});
