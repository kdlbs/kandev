import { describe, expect, it } from "vitest";
import type { TaskSession } from "@/lib/types/http";
import { sessionModelsEntryFromTaskSession } from "./session-model-hydration";

function session(metadata: Record<string, unknown>): TaskSession {
  return { id: "session-1", metadata } as TaskSession;
}

describe("sessionModelsEntryFromTaskSession catalog hydration", () => {
  it("hydrates an optional catalog and preserves it across reload", () => {
    const entry = sessionModelsEntryFromTaskSession(
      session({
        acp_model_state: {
          current_model_id: "pair-a",
          config_options_settled: true,
          config_options: [
            {
              type: "select",
              id: "model",
              name: "Model",
              current_value: "pair-a",
              options: [{ value: "pair-a", name: "Balanced" }],
              catalog: {
                version: 1,
                nodes: [
                  {
                    kind: "group",
                    id: "fusion",
                    label: "Fusion",
                    children: [{ kind: "choice", value: "pair-a", label: "Balanced" }],
                  },
                ],
                compositions: [
                  {
                    id: "fusion",
                    group_id: "fusion",
                    dimensions: [
                      { id: "lead", label: "Lead", choices: [{ value: "a", label: "A" }] },
                    ],
                    bindings: [{ values: { lead: "a" }, model: "pair-a" }],
                  },
                ],
              },
            },
          ],
        },
      }),
    );
    expect(entry?.configOptions[0].catalog?.nodes[0].id).toBe("fusion");
    expect(entry?.configOptions[0].catalog?.compositions?.[0].bindings[0].model).toBe("pair-a");
    expect(entry?.configOptions[0].options).toEqual([{ value: "pair-a", name: "Balanced" }]);
  });

  it("drops invalid catalog data without losing the flat snapshot", () => {
    const entry = sessionModelsEntryFromTaskSession(
      session({
        acp_model_state: {
          current_model_id: "model-a",
          config_options_settled: true,
          config_options: [
            {
              type: "select",
              id: "model",
              name: "Model",
              current_value: "model-a",
              options: [{ value: "model-a", name: "Model A" }],
              catalog: { version: 99, nodes: [] },
            },
          ],
        },
      }),
    );
    expect(entry?.configOptions[0].catalog).toBeUndefined();
    expect(entry?.configOptions[0].options).toEqual([{ value: "model-a", name: "Model A" }]);
    expect(entry?.currentModelId).toBe("model-a");
  });

  it("keeps older flat snapshots unchanged", () => {
    const entry = sessionModelsEntryFromTaskSession(
      session({
        acp_model_state: {
          current_model_id: "model-a",
          config_options_settled: true,
          config_options: [
            {
              type: "select",
              id: "model",
              name: "Model",
              current_value: "model-a",
              options: [{ value: "model-a", name: "Model A" }],
            },
          ],
        },
      }),
    );
    expect(entry?.configOptions[0].catalog).toBeUndefined();
    expect(entry?.configOptions[0].currentValue).toBe("model-a");
  });
});
