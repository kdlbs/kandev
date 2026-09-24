import { describe, expect, it } from "vitest";
import type { TaskSession } from "@/lib/types/http";
import { sessionModelsEntryFromTaskSession } from "./session-model-hydration";

function session(metadata: Record<string, unknown>): TaskSession {
  return { id: "session-1", metadata } as TaskSession;
}

describe("sessionModelsEntryFromTaskSession", () => {
  it("hydrates persisted model and config state for a resumed session", () => {
    expect(
      sessionModelsEntryFromTaskSession(
        session({
          runtime_config: { model: "mock-fast", config_options: { effort: "medium" } },
          runtime_config_overrides: { config_options: { effort: "high" } },
          acp_config_baseline: { model: "mock-fast", effort: "medium" },
          acp_model_state: {
            current_model_id: "mock-fast",
            config_options_settled: true,
            models: [{ model_id: "mock-fast", name: "Mock Fast" }],
            config_options: [
              {
                type: "select",
                id: "effort",
                name: "Effort",
                current_value: "medium",
              },
            ],
          },
        }),
      ),
    ).toEqual({
      currentModelId: "mock-fast",
      models: [
        {
          modelId: "mock-fast",
          name: "Mock Fast",
          description: undefined,
          usageMultiplier: undefined,
        },
      ],
      configOptions: [
        {
          type: "select",
          id: "effort",
          name: "Effort",
          description: undefined,
          currentValue: "high",
          category: undefined,
          options: undefined,
        },
      ],
      configOptionsSettled: true,
      configBaseline: { model: "mock-fast", effort: "medium" },
    });
  });

  it("returns no entry when a session has no usable model snapshot", () => {
    expect(sessionModelsEntryFromTaskSession(session({}))).toBeUndefined();
    expect(sessionModelsEntryFromTaskSession(null)).toBeUndefined();
  });
});
