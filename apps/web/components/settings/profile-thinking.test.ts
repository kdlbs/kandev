import { expect, it } from "vitest";
import { thinkingProfilePatch, thinkingProfileValue } from "./profile-thinking";

it("clears both Pi aliases without changing unrelated settings", () => {
  const profile = { mode: "medium", config_options: { thought_level: "high", custom: "keep" } };
  expect(thinkingProfilePatch(profile, "thought_level", "")).toEqual({
    mode: "",
    config_options: { custom: "keep" },
  });
  expect(profile.config_options.thought_level).toBe("high");
  expect(thinkingProfilePatch(profile, "thought_level", "off")).toEqual({
    mode: "",
    config_options: { thought_level: "off", custom: "keep" },
  });
});
it("shows canonical or legacy saved intent without substituting provider state", () => {
  expect(
    thinkingProfileValue(
      { mode: "medium", config_options: { thought_level: "off" } },
      "thought_level",
    ),
  ).toBe("off");
  expect(thinkingProfileValue({ mode: "medium" }, "thought_level")).toBe("medium");
  expect(thinkingProfileValue({ mode: "" }, "thought_level")).toBe("");
});

it("uses advertised legacy Pi choices only when no modern catalog exists", async () => {
  const { thinkingChoices } = await import("./profile-thinking");
  const legacy = [
    { id: "off", name: "Off" },
    { id: "medium", name: "Medium" },
  ];
  expect(thinkingChoices([], legacy)).toEqual(legacy);
  expect(
    thinkingChoices(
      [{ type: "select", id: "model", name: "Model", currentValue: "plain", options: [] }],
      legacy,
    ),
  ).toEqual([]);
  expect(
    thinkingChoices(
      [
        {
          type: "select",
          id: "thought_level",
          name: "Thinking",
          category: "thought_level",
          currentValue: "off",
          options: [{ value: "off", name: "Off" }],
        },
      ],
      legacy,
    ),
  ).toEqual([{ id: "off", name: "Off" }]);
});

it("reads and clears saved thinking intent when the advertised Pi option ID differs", () => {
  const saved = { mode: "off", config_options: { thought_level: "medium", custom: "keep" } };
  expect(thinkingProfileValue(saved, "pi_reasoning")).toBe("medium");
  expect(thinkingProfilePatch(saved, "pi_reasoning", "off")).toEqual({
    mode: "",
    config_options: { thought_level: "off", custom: "keep" },
  });
  expect(
    thinkingProfilePatch(
      { ...saved, config_options: { ...saved.config_options, pi_reasoning: "high" } },
      "pi_reasoning",
      "",
    ),
  ).toEqual({ mode: "", config_options: { custom: "keep" } });
});
