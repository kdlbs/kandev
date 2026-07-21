import { describe, expect, it } from "vitest";
import { shouldActivateSessionPanel } from "./dockview-session-tab-activation";

describe("shouldActivateSessionPanel", () => {
  const sessionId = "s-current";
  const baseArgs = {
    prevTaskId: null,
    prevSessionId: null,
    currentTaskId: "task-A",
    currentSessionId: sessionId,
    currentActivePanelId: null,
  };

  it("activates when the session panel did not exist before", () => {
    expect(shouldActivateSessionPanel({ ...baseArgs, sessionPanelExistedBefore: false })).toBe(
      true,
    );
  });

  it("preserves a panel focused before a fresh session hydrates", () => {
    expect(
      shouldActivateSessionPanel({
        ...baseArgs,
        sessionPanelExistedBefore: false,
        currentActivePanelId: "mr-detail|https://gitlab.example.test|platform/kandev|81",
      }),
    ).toBe(false);
  });

  it("does not treat first-session hydration as an intra-task switch", () => {
    expect(
      shouldActivateSessionPanel({
        ...baseArgs,
        sessionPanelExistedBefore: true,
        prevTaskId: "task-A",
        currentActivePanelId: "mr-detail|https://gitlab.example.test|platform/kandev|81",
      }),
    ).toBe(false);
  });

  it("activates on first mount when no panel is active", () => {
    expect(
      shouldActivateSessionPanel({
        ...baseArgs,
        sessionPanelExistedBefore: true,
      }),
    ).toBe(true);
  });

  it("activates on first mount when the restored panel is the session", () => {
    expect(
      shouldActivateSessionPanel({
        ...baseArgs,
        sessionPanelExistedBefore: true,
        currentActivePanelId: `session:${sessionId}`,
      }),
    ).toBe(true);
  });

  it("preserves restored non-session panels on first mount", () => {
    for (const panelId of ["preview:commit-detail", "preview:file-editor", "plan"]) {
      expect(
        shouldActivateSessionPanel({
          ...baseArgs,
          sessionPanelExistedBefore: true,
          currentActivePanelId: panelId,
        }),
      ).toBe(false);
    }
  });

  it("activates on an intra-task session switch", () => {
    expect(
      shouldActivateSessionPanel({
        sessionPanelExistedBefore: true,
        prevTaskId: "task-A",
        prevSessionId: "s-old",
        currentTaskId: "task-A",
        currentSessionId: sessionId,
        currentActivePanelId: "preview:commit-detail",
      }),
    ).toBe(true);
  });

  it("preserves the active panel on a task switch", () => {
    expect(
      shouldActivateSessionPanel({
        sessionPanelExistedBefore: true,
        prevTaskId: "task-A",
        prevSessionId: "s-old",
        currentTaskId: "task-B",
        currentSessionId: sessionId,
        currentActivePanelId: "preview:commit-detail",
      }),
    ).toBe(false);
  });
});
