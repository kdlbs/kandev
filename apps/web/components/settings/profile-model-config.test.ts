import { describe, expect, it } from "vitest";
import { reconcileConfigOptionValues } from "./profile-model-config";

describe("reconcileConfigOptionValues", () => {
  it("keeps supported values and removes values not advertised by the snapshot", () => {
    expect(
      reconcileConfigOptionValues({ effort: "medium", safe: "on", unknown: "value" }, [
        {
          type: "select",
          id: "effort",
          name: "Effort",
          current_value: "high",
          options: [
            { value: "low", name: "Low" },
            { value: "high", name: "High" },
          ],
        },
        {
          type: "select",
          id: "safe",
          name: "Safe mode",
          current_value: "on",
        },
      ]),
    ).toEqual({ safe: "on" });
  });
});
