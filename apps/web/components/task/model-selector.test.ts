import { describe, expect, it } from "vitest";
import {
  triggerLabel,
  type ModelSelectorOption,
  usableConfigOptions,
} from "@/components/model-config-selector";
import {
  annotateFallbackOptions,
  buildModelOptions,
  hasCompleteDynamicConfig,
  requiredConfigKeys,
} from "@/components/task/model-selector";
import { t } from "@/lib/i18n";
import type { Agent, TaskSession } from "@/lib/types/http";
import type {
  ConfigOptionEntry,
  SessionModelEntry,
} from "@/lib/state/slices/session-runtime/types";

function compactTriggerLabel(
  modelOptions: ModelSelectorOption[],
  currentModel: string,
  configOptions: ConfigOptionEntry[],
  configBaseline?: Record<string, string>,
): string {
  return triggerLabel(modelOptions, currentModel, configOptions, {
    summary: "changed",
    configBaseline,
  });
}

const providerModelId = "gpt-5.6-sol";
const providerModelName = "GPT-5.6-Sol";
const modelOptions = [{ id: providerModelId, name: providerModelName }];

function sessionConfigOptions(reasoningEffort = "high", fastMode = "off"): ConfigOptionEntry[] {
  return [
    {
      type: "select",
      id: "model",
      name: "Model",
      currentValue: providerModelId,
      category: "model",
      options: [{ value: providerModelId, name: providerModelName }],
    },
    {
      type: "select",
      id: "reasoning_effort",
      name: "Reasoning Effort",
      currentValue: reasoningEffort,
      options: [
        { value: "high", name: "High" },
        { value: "low", name: "Low" },
      ],
    },
    {
      type: "select",
      id: "fast_mode",
      name: "Fast Mode",
      currentValue: fastMode,
      options: [
        { value: "off", name: "Off" },
        { value: "on", name: "On" },
      ],
    },
  ];
}

describe("model selector config options", () => {
  it("keeps model-adjacent config options and excludes mode", () => {
    const options: ConfigOptionEntry[] = [
      {
        type: "select",
        id: "mode",
        name: "Mode",
        currentValue: "agent",
        category: "mode",
        options: [{ value: "agent", name: "Agent" }],
      },
      {
        type: "select",
        id: "model",
        name: "Model",
        currentValue: "gpt-5.5",
        category: "model",
        options: [{ value: "gpt-5.5", name: "GPT-5.5" }],
      },
      {
        type: "select",
        id: "reasoning_effort",
        name: "Reasoning Effort",
        currentValue: "high",
        category: "thought_level",
        options: [{ value: "high", name: "High" }],
      },
    ];

    expect(usableConfigOptions(options).map((option) => option.id)).toEqual([
      "model",
      "reasoning_effort",
    ]);
  });

  it("summarizes model and extra option values in one toolbar label", () => {
    const label = triggerLabel([{ id: "gpt-5.5", name: "GPT-5.5" }], "gpt-5.5", [
      {
        type: "select",
        id: "model",
        name: "Model",
        currentValue: "gpt-5.5",
        category: "model",
        options: [{ value: "gpt-5.5", name: "GPT-5.5" }],
      },
      {
        type: "select",
        id: "reasoning_effort",
        name: "Reasoning Effort",
        currentValue: "medium",
        category: "thought_level",
        options: [{ value: "medium", name: "Medium" }],
      },
    ]);

    expect(label).toBe("GPT-5.5 / Medium");
  });
});

describe("task model selector compact label", () => {
  it("shows only the model when task configuration matches its baseline", () => {
    const label = compactTriggerLabel(modelOptions, providerModelId, sessionConfigOptions(), {
      model: providerModelId,
      reasoning_effort: "high",
      fast_mode: "off",
    });

    expect(label).toBe(providerModelName);
  });

  it("shows all current values while the task baseline is unavailable", () => {
    const label = compactTriggerLabel(
      modelOptions,
      providerModelId,
      sessionConfigOptions("low", "on"),
    );

    expect(label).toBe(`${providerModelName} / Low / On`);
  });

  it("shows every changed value in ACP option order", () => {
    const label = compactTriggerLabel(
      modelOptions,
      providerModelId,
      sessionConfigOptions("low", "on"),
      {
        model: providerModelId,
        reasoning_effort: "high",
        fast_mode: "off",
      },
    );

    expect(label).toBe(`${providerModelName} / Low / On`);
  });

  it("shows a profile-selected value that differs from the provider default", () => {
    const label = compactTriggerLabel(modelOptions, providerModelId, sessionConfigOptions("high"), {
      model: providerModelId,
      reasoning_effort: "medium",
      fast_mode: "off",
    });

    expect(label).toBe(`${providerModelName} / High`);
  });

  it("hides a value after it returns to baseline", () => {
    const label = compactTriggerLabel(modelOptions, providerModelId, sessionConfigOptions("high"), {
      reasoning_effort: "high",
      fast_mode: "off",
    });

    expect(label).toBe(providerModelName);
  });

  it("treats a currently advertised option with no baseline entry as changed", () => {
    const label = compactTriggerLabel(modelOptions, providerModelId, sessionConfigOptions(), {
      reasoning_effort: "high",
      removed_option: "legacy",
    });

    expect(label).toBe(`${providerModelName} / Off`);
  });
});

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

const flatModelId = "claude-opus-4-8";
const noAgents: Agent[] = [];

function flatModelEntries(): SessionModelEntry[] {
  return [
    { modelId: flatModelId, name: "Claude Opus 4.8" },
    { modelId: "claude-sonnet-4-8", name: "Claude Sonnet 4.8" },
  ];
}

describe("hasCompleteDynamicConfig", () => {
  it("treats a required model key as satisfied by a flat ACP model list", () => {
    const session = makeSession({
      metadata: { runtime_config: { config_options: { model: flatModelId } } },
    });
    expect(requiredConfigKeys(session, noAgents)).toEqual(["model"]);

    const hydrated = hasCompleteDynamicConfig(
      session,
      { currentModelId: flatModelId, models: flatModelEntries(), configOptions: [] },
      noAgents,
    );

    expect(hydrated).toBe(true);
  });

  it("still requires the model config option when there is no flat model list", () => {
    const session = makeSession({
      metadata: { runtime_config: { config_options: { model: providerModelId } } },
    });

    const hydrated = hasCompleteDynamicConfig(
      session,
      { currentModelId: providerModelId, models: [], configOptions: [] },
      noAgents,
    );

    expect(hydrated).toBe(false);
  });

  it("hydrates a config-option agent once the model option is present", () => {
    const session = makeSession({
      metadata: { runtime_config: { config_options: { model: providerModelId } } },
    });

    const modelOption: ConfigOptionEntry = {
      type: "select",
      id: "model",
      name: "Model",
      currentValue: providerModelId,
      category: "model",
      options: [{ value: providerModelId, name: providerModelName }],
    };

    const hydrated = hasCompleteDynamicConfig(
      session,
      { currentModelId: providerModelId, models: [], configOptions: [modelOption] },
      noAgents,
    );

    expect(hydrated).toBe(true);
  });

  it("does not let a flat model list satisfy a non-model required key", () => {
    const session = makeSession({
      metadata: {
        runtime_config: { config_options: { model: flatModelId, reasoning_effort: "high" } },
      },
    });

    const hydrated = hasCompleteDynamicConfig(
      session,
      { currentModelId: flatModelId, models: flatModelEntries(), configOptions: [] },
      noAgents,
    );

    expect(hydrated).toBe(false);
  });
});

describe("legacy agent config hydration", () => {
  it("treats an unadvertised agent config value as satisfied once ACP options have settled", () => {
    const session = makeSession({
      metadata: {
        runtime_config: {
          // i18n-exempt: persisted legacy agent configuration value
          config_options: { agent: "default", model: flatModelId, verbosity: "terse" },
        },
      },
      agent_profile_snapshot: { agent_id: "claude" },
    });
    expect(requiredConfigKeys(session, noAgents)).toEqual(["agent", "model", "verbosity"]);

    const settledOption: ConfigOptionEntry = {
      type: "select",
      id: "verbosity",
      name: "Verbosity",
      currentValue: "terse",
      options: [{ value: "terse", name: "Terse" }],
    };
    const hydrated = hasCompleteDynamicConfig(
      session,
      {
        currentModelId: flatModelId,
        models: flatModelEntries(),
        configOptions: [settledOption],
        configOptionsSettled: true,
      } as Parameters<typeof hasCompleteDynamicConfig>[1],
      noAgents,
    );

    expect(hydrated).toBe(true);
  });

  it("treats a settled empty ACP option snapshot as hydrated for legacy agent data", () => {
    const session = makeSession({
      metadata: {
        runtime_config: { config_options: { agent: "default", model: flatModelId } },
      },
      agent_profile_snapshot: { agent_id: "claude" },
    });

    const hydrated = hasCompleteDynamicConfig(
      session,
      {
        currentModelId: flatModelId,
        models: flatModelEntries(),
        configOptions: [],
        configOptionsSettled: true,
      } as Parameters<typeof hasCompleteDynamicConfig>[1],
      noAgents,
    );

    expect(hydrated).toBe(true);
  });

  it("keeps an unadvertised provider-defined agent option required", () => {
    const session = makeSession({
      metadata: {
        runtime_config: { config_options: { agent: "build", model: flatModelId } },
      },
      agent_profile_snapshot: { agent_id: "claude" },
    });

    const hydrated = hasCompleteDynamicConfig(
      session,
      {
        currentModelId: flatModelId,
        models: flatModelEntries(),
        configOptions: [],
        configOptionsSettled: true,
      } as Parameters<typeof hasCompleteDynamicConfig>[1],
      noAgents,
    );

    expect(hydrated).toBe(false);
  });

  it("does not mark an agent key satisfied while ACP config options are still empty", () => {
    const session = makeSession({
      metadata: {
        runtime_config: { config_options: { agent: "default", model: flatModelId } },
      },
      agent_profile_snapshot: { agent_id: "claude" },
    });

    const hydrated = hasCompleteDynamicConfig(
      session,
      {
        currentModelId: flatModelId,
        models: flatModelEntries(),
        configOptions: [],
        configOptionsSettled: false,
      } as Parameters<typeof hasCompleteDynamicConfig>[1],
      noAgents,
    );

    expect(hydrated).toBe(false);
  });
});

const modelConfigOption: ConfigOptionEntry = {
  type: "select",
  id: "model",
  name: "Model",
  currentValue: providerModelId,
  category: "model",
  options: [{ value: providerModelId, name: providerModelName }],
};

function selectOption(id: string, currentValue: string): ConfigOptionEntry {
  return {
    type: "select",
    id,
    name: id,
    currentValue,
    options: [{ value: currentValue, name: currentValue }],
  };
}

function catalogEntry(configOptions: ConfigOptionEntry[], settled: boolean) {
  return {
    currentModelId: providerModelId,
    models: [],
    configOptions,
    configOptionsSettled: settled,
  } as Parameters<typeof hasCompleteDynamicConfig>[1];
}

describe("cross-agent persisted config reconciliation", () => {
  it("hydrates when persisted-only keys are unadvertised and the catalog is settled", () => {
    const session = makeSession({
      metadata: {
        runtime_config: {
          // effort/thinking were written by a prior agent that no longer runs.
          config_options: {
            model: providerModelId,
            reasoning: "high",
            fast: "on",
            effort: "x",
            thinking: "y",
          },
        },
      },
    });
    const advertised = [
      modelConfigOption,
      selectOption("reasoning", "high"),
      selectOption("fast", "on"),
    ];

    expect(hasCompleteDynamicConfig(session, catalogEntry(advertised, true), noAgents)).toBe(true);
  });

  it("keeps an unadvertised profile-snapshot key required while the catalog is unsettled", () => {
    const session = makeSession({
      metadata: { runtime_config: { config_options: { model: providerModelId } } },
      agent_profile_snapshot: { config_options: { verbosity: "terse" } },
    });

    expect(
      hasCompleteDynamicConfig(session, catalogEntry([modelConfigOption], false), noAgents),
    ).toBe(false);
  });

  it("hydrates when a settled model catalog omits profile options from a prior model", () => {
    const session = makeSession({
      metadata: {
        runtime_config: {
          config_options: {
            effort: "high",
            fast: "false",
            mode: "agent",
            model: providerModelId,
            optimize_for: "balanced",
          },
        },
        runtime_config_overrides: {
          model: providerModelId,
          config_options: { model: providerModelId },
        },
      },
      agent_profile_snapshot: { config_options: { fast: "false" } },
    });
    const advertised = [
      selectOption("mode", "agent"),
      modelConfigOption,
      selectOption("optimize_for", "balanced"),
    ];

    expect(hasCompleteDynamicConfig(session, catalogEntry(advertised, true), noAgents)).toBe(true);
  });

  it("hydrates when a settled model catalog omits a profile-only option", () => {
    const session = makeSession({
      metadata: { runtime_config: { config_options: { model: providerModelId } } },
      agent_profile_snapshot: { config_options: { fast: "false" } },
    });

    expect(
      hasCompleteDynamicConfig(
        session,
        catalogEntry([modelConfigOption, selectOption("mode", "agent")], true),
        noAgents,
      ),
    ).toBe(true);
  });

  it("still requires persisted-only unadvertised keys while the catalog is unsettled", () => {
    const session = makeSession({
      metadata: { runtime_config: { config_options: { model: providerModelId, effort: "x" } } },
    });

    expect(
      hasCompleteDynamicConfig(session, catalogEntry([modelConfigOption], false), noAgents),
    ).toBe(false);
  });
});

it("is hydrated when there are no required config keys", () => {
  expect(hasCompleteDynamicConfig(makeSession(), undefined, noAgents)).toBe(true);
});

describe("buildModelOptions (gone current model)", () => {
  it("keeps a configured-but-unadvertised model visible and disabled with a translated reason", () => {
    const options = buildModelOptions([{ id: "gpt-5", name: "GPT-5" }], "claude-gone");
    expect(options).toHaveLength(2);
    const gone = options[0];
    expect(gone.id).toBe("claude-gone");
    expect(gone.disabled).toBe(true);
    expect(gone.disabledReason).toBe(t("settings:startModelUnavailable"));
  });

  it("does not duplicate the current model when it is still advertised", () => {
    const options = buildModelOptions([{ id: "gpt-5", name: "GPT-5" }], "gpt-5");
    expect(options).toHaveLength(1);
    expect(options[0].disabled).toBeUndefined();
  });
});

describe("annotateFallbackOptions", () => {
  const fallbackSuffix = t("settings:modelFallbackSuffix");
  const modelId = "gpt-5";
  const modelName = "GPT-5";
  const fallbackId = "gpt-5-mini";
  const fallbackName = "GPT-5-Mini";
  const base = [
    { id: modelId, name: modelName },
    { id: fallbackId, name: fallbackName },
  ];

  it("returns the same array when no fallback is active (clears stale markers)", () => {
    expect(annotateFallbackOptions(base, undefined)).toBe(base);
    expect(annotateFallbackOptions(base, "")).toBe(base);
  });

  it("annotates only the fallback option with the localized suffix", () => {
    const annotated = annotateFallbackOptions(base, fallbackId);
    expect(annotated[0]).toBe(base[0]);
    expect(annotated[1]).not.toBe(base[1]);
    expect(annotated[1].name).toBe(`${fallbackName} ${fallbackSuffix}`);
    expect(annotated[1].id).toBe(fallbackId);
  });

  it("preserves immutability of source options", () => {
    const annotated = annotateFallbackOptions(base, modelId);
    expect(base[0].name).toBe(modelName);
    expect(annotated[0].name).toBe(`${modelName} ${fallbackSuffix}`);
    // The source array and its objects are untouched.
    expect(base).toEqual([
      { id: modelId, name: modelName },
      { id: fallbackId, name: fallbackName },
    ]);
  });

  it("does not double-append the suffix (idempotent)", () => {
    const once = annotateFallbackOptions(base, modelId);
    const twice = annotateFallbackOptions(once, modelId);
    expect(twice[0].name).toBe(`${modelName} ${fallbackSuffix}`);
  });
});
