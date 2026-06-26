import { describe, expect, it } from "vitest";
import { resolveCenterPanelSessionId } from "./task-center-panel-session";

const ACTIVE_SESSION_ID = "active-session";
const PANEL_SESSION_ID = "panel-session";
const ACTIVE_TASK_ID = "active-task";
const OTHER_TASK_ID = "other-task";

describe("resolveCenterPanelSessionId", () => {
  it("uses the active session when no explicit panel session is provided", () => {
    expect(
      resolveCenterPanelSessionId(null, ACTIVE_SESSION_ID, ACTIVE_TASK_ID, ACTIVE_TASK_ID),
    ).toBe(ACTIVE_SESSION_ID);
    expect(
      resolveCenterPanelSessionId(undefined, ACTIVE_SESSION_ID, ACTIVE_TASK_ID, ACTIVE_TASK_ID),
    ).toBe(ACTIVE_SESSION_ID);
  });

  it("preserves an explicit panel session", () => {
    expect(
      resolveCenterPanelSessionId(
        PANEL_SESSION_ID,
        ACTIVE_SESSION_ID,
        OTHER_TASK_ID,
        ACTIVE_TASK_ID,
      ),
    ).toBe(PANEL_SESSION_ID);
  });

  it("returns null when neither source is available", () => {
    expect(resolveCenterPanelSessionId(null, null, null, ACTIVE_TASK_ID)).toBeNull();
  });

  it("does not fall back to an active session from another task", () => {
    expect(
      resolveCenterPanelSessionId(null, ACTIVE_SESSION_ID, OTHER_TASK_ID, ACTIVE_TASK_ID),
    ).toBeNull();
  });

  it("does not fall back when the active task is unavailable", () => {
    expect(resolveCenterPanelSessionId(null, ACTIVE_SESSION_ID, ACTIVE_TASK_ID, null)).toBeNull();
  });
});
