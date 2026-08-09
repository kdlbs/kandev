import { describe, expect, it } from "vitest";
import type { UserSettingsState } from "@/lib/state/slices/settings/types";
import { isSettingsUnchanged, normalizeHiddenStepIds } from "./use-user-display-settings";

function settings(tasksListShowDetails: boolean): UserSettingsState {
  return {
    loaded: true,
    workspaceId: "workspace-1",
    workflowId: null,
    repositoryIds: [],
    tasksListShowDetails,
  } as unknown as UserSettingsState;
}

function settingsWithHidden(hiddenWorkflowStepIds: Record<string, string[]>): UserSettingsState {
  return {
    loaded: true,
    workspaceId: "workspace-1",
    workflowId: null,
    repositoryIds: [],
    tasksListShowDetails: false,
    hiddenWorkflowStepIds,
  } as unknown as UserSettingsState;
}

describe("isSettingsUnchanged", () => {
  it("detects a details-only preference change", () => {
    expect(isSettingsUnchanged(settings(true), settings(false))).toBe(false);
  });

  it("treats reordered hidden step ids for the same workflow as unchanged", () => {
    const normalized = settingsWithHidden({ "wf-1": ["step-a", "step-b"] });
    const current = settingsWithHidden({ "wf-1": ["step-b", "step-a"] });
    expect(isSettingsUnchanged(normalized, current)).toBe(true);
  });

  it("detects an actual hidden step id change for a workflow", () => {
    const normalized = settingsWithHidden({ "wf-1": ["step-a"] });
    const current = settingsWithHidden({ "wf-1": ["step-b"] });
    expect(isSettingsUnchanged(normalized, current)).toBe(false);
  });

  it("detects a hidden workflow being added or removed", () => {
    const normalized = settingsWithHidden({ "wf-1": ["step-a"], "wf-2": ["step-c"] });
    const current = settingsWithHidden({ "wf-1": ["step-a"] });
    expect(isSettingsUnchanged(normalized, current)).toBe(false);
  });

  it("treats both empty hidden-step maps as unchanged", () => {
    expect(isSettingsUnchanged(settingsWithHidden({}), settingsWithHidden({}))).toBe(true);
  });
});

describe("normalizeHiddenStepIds", () => {
  it("dedupes and sorts ids within each workflow", () => {
    expect(normalizeHiddenStepIds({ "wf-1": ["step-b", "step-a", "step-b"] })).toEqual({
      "wf-1": ["step-a", "step-b"],
    });
  });

  it("drops a workflow entry whose id array becomes empty", () => {
    expect(normalizeHiddenStepIds({ "wf-1": [], "wf-2": ["step-a"] })).toEqual({
      "wf-2": ["step-a"],
    });
  });

  it("returns an empty object for an empty input", () => {
    expect(normalizeHiddenStepIds({})).toEqual({});
  });
});
