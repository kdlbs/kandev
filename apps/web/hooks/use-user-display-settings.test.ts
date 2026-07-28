import { describe, expect, it } from "vitest";
import type { UserSettingsState } from "@/lib/state/slices/settings/types";
import { isSettingsUnchanged } from "./use-user-display-settings";

function settings(tasksListShowDetails: boolean): UserSettingsState {
  return {
    loaded: true,
    workspaceId: "workspace-1",
    workflowId: null,
    repositoryIds: [],
    tasksListShowDetails,
  } as unknown as UserSettingsState;
}

describe("isSettingsUnchanged", () => {
  it("detects a details-only preference change", () => {
    expect(isSettingsUnchanged(settings(true), settings(false))).toBe(false);
  });
});
