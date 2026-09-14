import { describe, expect, it } from "vitest";
import { defaultState } from "@/lib/state/default-state";
import {
  appearanceRevision,
  buildAppearanceUserSettingsPatch,
  createAppearanceSavedState,
  rebaseAppearanceDraft,
} from "./appearance-settings-state";

// Existing contributor contract, extended to the new enum value without new save logic.
// @covers AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.2
describe("Threads appearance draft", () => {
  const saved = createAppearanceSavedState("dark", "flat", true, defaultState.userSettings);
  it("only patches the explicit choice when selected", () => {
    const draft = { ...saved, startupPage: "threads" as const };
    expect(buildAppearanceUserSettingsPatch(draft, saved)).toEqual({ startup_page: "threads" });
    expect(appearanceRevision(draft)).not.toBe(appearanceRevision(saved));
    expect(saved.startupPage).toBe("task_overview");
  });
  it("preserves saved Threads through unrelated changes and an unchanged save", () => {
    const threads = { ...saved, startupPage: "threads" as const };
    expect(buildAppearanceUserSettingsPatch(threads, threads)).toEqual({});
    expect(
      buildAppearanceUserSettingsPatch({ ...threads, appStatusBarEnabled: true }, threads),
    ).toEqual({ app_status_bar_enabled: true });
  });
  it("rebases clean fields while retaining a pending Threads choice", () => {
    const next = { ...saved, appStatusBarEnabled: true };
    expect(rebaseAppearanceDraft({ ...saved, startupPage: "threads" }, saved, next)).toEqual({
      ...next,
      startupPage: "threads",
    });
    expect(rebaseAppearanceDraft(saved, saved, { ...next, startupPage: "threads" })).toEqual({
      ...next,
      startupPage: "threads",
    });
  });
});
